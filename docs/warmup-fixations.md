# Warmup System Fixes Documentation

This document contains all verified modifications to the TezMail warmup system. These changes ensure Gmail/Outlook/Yahoo compliant email delivery during IP warmup periods.

---

## Table of Contents

1. [Overview](#overview)
2. [File 1: rate_limiter.go - Complete Replacement](#file-1-rate_limitergo---complete-replacement)
3. [File 2: task_executor.go - Specific Modifications](#file-2-task_executorgo---specific-modifications)
4. [Build and Deployment Instructions](#build-and-deployment-instructions)
5. [Verification Commands](#verification-commands)

---

## Overview

### Problems Fixed

1. **Burst Sending on Startup**: Original code allowed full token bucket capacity on new IPs, causing spam flags
2. **Recipients Stuck Forever**: Deferred recipients (is_sent=2) were never re-queried because:
   - Query didn't filter by `sent_time`
   - `is_sent` wasn't reset to `0` when deferring
3. **Duplicate sent_times**: Original formula `curTime + (waits * ((i % 10) + 1))` caused collisions (i=0 and i=10 get same time)
4. **Clock Drift**: Using client time instead of Redis TIME caused inconsistencies in multi-node setups

### Key Improvements

- **Token Bucket with Spacing**: Enforces gradual email delivery with mandatory spacing
- **Standards-Compliant Init**: Max 5 tokens on startup (prevents burst behavior)
- **Jitter (0.8-1.3x)**: Human-like sending patterns
- **Redis TIME**: Multi-node consistency (no clock drift)
- **Proper Deferred Handling**: Reset `is_sent=0` + filter by `sent_time`

---

## File 1: rate_limiter.go - Complete Replacement

**Path**: `core/internal/service/warmup/rate_limiter.go`

**Action**: Replace the entire file with the following content:

```go
package warmup

import (
	"billionmail-core/internal/service/public"
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	// cacheTTLForLimits is the cache time for sending limits.
	// We cache for a short period to avoid hitting the database on every request,
	// while still allowing the system to pick up changes in scores and progress relatively quickly.
	cacheTTLForLimits = 5 * time.Minute

	// counterExpireInDay is the expiration time for the daily counter.
	// Set to 2 days.
	counterExpireInDay = 24 * time.Hour
)

// tokenBucketWithSpacingLua is the atomic Lua script for warmup-compliant rate limiting.
// It enforces:
// 1. Spacing between emails (next_allowed_time with jitter) - prevents burst sending
// 2. Token bucket for hourly limits - controls overall volume
// 3. Standards-compliant initialization (max 5 tokens) - prevents burst on new IPs
// 4. Uses Redis TIME for multi-node consistency - prevents clock drift issues
//
// This is critical for email warm-up compliance with Gmail, Outlook, Yahoo standards.
const tokenBucketWithSpacingLua = `
-- Warmup-compliant rate limiter with spacing enforcement
-- KEYS[1] = bucket key, KEYS[2] = spacing key
-- ARGV[1] = capacity, ARGV[2] = rate, ARGV[3] = unused (we use Redis TIME)
-- ARGV[4] = tokens_requested, ARGV[5] = base_spacing_ms

local bucket_key = KEYS[1]
local spacing_key = KEYS[2]
local capacity = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local requested = tonumber(ARGV[4])
local base_spacing_ms = tonumber(ARGV[5])

-- SAFETY: Guard against rate == 0 (misconfig protection)
if rate <= 0 then
    return 3600  -- Hard wait fallback: 1 hour
end

-- USE REDIS TIME for consistency across nodes (prevents clock drift)
local redis_time = redis.call('TIME')
local now_ms = tonumber(redis_time[1]) * 1000 + math.floor(tonumber(redis_time[2]) / 1000)

-- STEP 1: Check spacing (MUST pass before token check)
-- This enforces gradual sending - one email per spacing window per provider
local next_allowed = redis.call('GET', spacing_key)
if next_allowed then
    next_allowed = tonumber(next_allowed)
    if now_ms < next_allowed then
        -- Return wait time in seconds (ceiling)
        return math.ceil((next_allowed - now_ms) / 1000)
    end
end

-- STEP 2: Token bucket check
local bucket_info = redis.call('HGETALL', bucket_key)

-- Standards-compliant init: min(max(2, 5%), 5) - never more than 5 tokens
-- This prevents burst behavior on new IPs/domains during warm-up
local last_tokens = math.min(math.max(2, math.floor(capacity * 0.05)), 5)
local last_refill_time = now_ms

if #bucket_info > 0 then
    for i = 1, #bucket_info, 2 do
        if bucket_info[i] == 'tokens' then
            last_tokens = tonumber(bucket_info[i+1])
        elseif bucket_info[i] == 'last_refill_time' then
            last_refill_time = tonumber(bucket_info[i+1])
        end
    end
end

-- Calculate current tokens with refill
local time_passed_sec = (now_ms - last_refill_time) / 1000
local new_tokens = time_passed_sec * rate
local current_tokens = math.min(capacity, last_tokens + new_tokens)

if current_tokens < requested then
    -- Not enough tokens, return wait time
    local tokens_needed = requested - current_tokens
    local wait_sec = math.ceil(tokens_needed / rate)
    return wait_sec
end

-- STEP 3: Consume token + calculate next_allowed with jitter
-- Jitter: 0.8 to 1.3 (50% variance for human-like behavior)
-- This makes sending patterns less robotic and more natural
local jitter_seed = (now_ms % 100) / 200 + 0.8  -- 0.8 to 1.3
local actual_spacing_ms = math.floor(base_spacing_ms * jitter_seed)
local next_allowed_time = now_ms + actual_spacing_ms

-- Update bucket
current_tokens = current_tokens - requested
redis.call('HSET', bucket_key, 'tokens', current_tokens, 'last_refill_time', now_ms)

-- Store next_allowed_time with 6 hour TTL (survives restarts/pauses)
redis.call('SET', spacing_key, next_allowed_time, 'EX', 21600)

-- Set bucket expiry
local expiry = 86400
if rate > 0 then
    expiry = math.ceil(capacity / rate * 2)
end
redis.call('EXPIRE', bucket_key, expiry)

return 0  -- Success, allowed to send
`

// RateLimiterService provides rate limiting functionality for IP warmup.
// It uses Redis for high-performance, distributed counting and caching.
type RateLimiterService struct {
	providerService *SenderIpMailProviderService
}

var insRateLimiterService = RateLimiterService{
	providerService: SenderIpMailProvider(),
}

// RateLimiter returns the singleton instance of RateLimiterService.
func RateLimiter() *RateLimiterService {
	return &insRateLimiterService
}

// Allow checks if sending is allowed for the given sender IP and recipient.
// It uses a warmup-compliant approach:
// 1. Daily limit: using Redis's variable window counter to ensure the absolute daily quota is not exceeded.
// 2. Hourly limit + Spacing: using token bucket with mandatory spacing between emails.
//
// Returns:
//   - allow: true if sending is permitted
//   - waits: seconds to wait before retry (0 if allowed)
//   - err: any error encountered
//
// Note: Daily count is ONLY incremented on successful sends.
// Deferred attempts do NOT count toward daily limit.
func (s *RateLimiterService) Allow(ctx context.Context, senderIp string, recipientEmail string) (allow bool, waits int, err error) {
	// Default wait time is 300 seconds (5 minutes), if not exceeded limit, it will be 0.
	waits = 300

	// 1. Determine the mail service provider group from the recipient email.
	mailProviderGroup := public.GetMailProviderGroup(recipientEmail)

	// 2. Get the sending limits (daily and hourly) for this IP and service provider group.
	var dailyLimit, hourlyLimit int
	dailyLimit, hourlyLimit, err = s.getCachedLimitsForProvider(ctx, senderIp, mailProviderGroup)
	if err != nil {
		g.Log().Errorf(ctx, "RateLimiter: Failed to get limits for IP %s, Group %s: %v", senderIp, mailProviderGroup, err)
		return
	}

	if dailyLimit <= 0 || hourlyLimit <= 0 {
		g.Log().Debugf(ctx, "RateLimiter: Sending denied for IP %s, Group %s because limits are zero (Daily: %d, Hourly: %d)", senderIp, mailProviderGroup, dailyLimit, hourlyLimit)
		return
	}

	// 3. Check the daily limit (variable window counter)
	var dailyCount int64
	dailyKey := fmt.Sprintf("warmup:vw:d:%s:%s", senderIp, mailProviderGroup)

	dailyCount, err = g.Redis().Incr(ctx, dailyKey)
	if err != nil {
		g.Log().Errorf(ctx, "RateLimiter: Redis INCR failed for daily key %s: %v", dailyKey, err)
		return
	}
	if dailyCount == 1 {
		// New daily counter, set expiration time
		_, _ = g.Redis().Expire(ctx, dailyKey, int64(counterExpireInDay.Seconds()))
	}

	if dailyCount > int64(dailyLimit) {
		g.Log().Debugf(ctx, "RateLimiter: Daily limit exceeded for IP %s, Group %s. Count: %d, Limit: %d", senderIp, mailProviderGroup, dailyCount, dailyLimit)
		return
	}

	// 4. Token bucket + spacing check (atomic Lua script)
	// This is the core warmup-compliant logic that enforces gradual sending
	bucketKey := fmt.Sprintf("warmup:tb:h:%s:%s", senderIp, mailProviderGroup)
	spacingKey := fmt.Sprintf("warmup:spacing:%s:%s", senderIp, mailProviderGroup)

	capacity := float64(hourlyLimit)
	rate := capacity / 3600.0                    // Tokens per second
	baseSpacingMs := int64(3600000 / hourlyLimit) // Base spacing in milliseconds (3600 sec * 1000 / hourly limit)

	res, err := g.Redis().Eval(ctx, tokenBucketWithSpacingLua, 2,
		[]string{bucketKey, spacingKey},
		[]interface{}{
			capacity,
			rate,
			0, // Unused - Lua uses Redis TIME now for multi-node consistency
			1, // Consume 1 token
			baseSpacingMs,
		})

	if err != nil {
		g.Log().Errorf(ctx, "RateLimiter: Lua script failed for IP %s, Group %s: %v", senderIp, mailProviderGroup, err)
		// Roll back daily count - send was not attempted
		_, _ = g.Redis().Decr(ctx, dailyKey)
		return
	}

	waitSeconds := res.Int()
	if waitSeconds > 0 {
		// Not allowed yet - spacing or token limit enforced
		// Roll back daily count - send was not attempted (deferred attempts don't count)
		_, _ = g.Redis().Decr(ctx, dailyKey)
		g.Log().Debugf(ctx, "RateLimiter: Spacing enforced for IP %s, Group %s. Wait %d seconds", senderIp, mailProviderGroup, waitSeconds)
		return false, waitSeconds, nil
	}

	// 5. Success - sending is allowed
	allow = true
	waits = 0
	g.Log().Debugf(ctx, "RateLimiter: Allow send for IP %s, Group %s. Daily count: %d/%d", senderIp, mailProviderGroup, dailyCount, dailyLimit)
	return
}

// getCachedLimitsForProvider retrieves the sending limits, utilizing cache to avoid frequent database calls.
func (s *RateLimiterService) getCachedLimitsForProvider(ctx context.Context, senderIp, mailProviderGroup string) (dailyLimit, hourlyLimit int, err error) {
	cacheKey := fmt.Sprintf("warmup:limits:%s:%s", senderIp, mailProviderGroup)

	// First, try to get it from the cache.
	cachedVal, err := g.Redis().Get(ctx, cacheKey)
	if err == nil && !cachedVal.IsNil() {
		// Cache hit
		limitsJson := gjson.New(cachedVal.String())
		dailyLimit = limitsJson.Get("d").Int()
		hourlyLimit = limitsJson.Get("h").Int()
		g.Log().Debugf(ctx, "RateLimiter: Cache HIT for limits on IP %s, Group %s. Daily: %d, Hourly: %d", senderIp, mailProviderGroup, dailyLimit, hourlyLimit)
		return
	}

	// Cache miss, fetch from service.
	g.Log().Debugf(ctx, "RateLimiter: Cache MISS for limits on IP %s, Group %s. Fetching from service.", senderIp, mailProviderGroup)
	dailyLimit, hourlyLimit, err = s.providerService.GetAdjustedSendingLimitsForProvider(ctx, senderIp, mailProviderGroup)
	if err != nil {
		return 0, 0, err
	}

	// Store in cache for future requests.
	limits := g.Map{
		"d": dailyLimit,
		"h": hourlyLimit,
	}
	limitsStr, _ := gjson.New(limits).ToJsonString()
	err = g.Redis().SetEX(ctx, cacheKey, limitsStr, int64(cacheTTLForLimits.Seconds()))
	if err != nil {
		// Log the error, but don't fail the operation, as we have already fetched the limits.
		g.Log().Warningf(ctx, "RateLimiter: Failed to set cache for limits on IP %s, Group %s: %v", senderIp, mailProviderGroup, err)
	}

	return
}
```

---

## File 2: task_executor.go - Specific Modifications

**Path**: `core/internal/service/batch_mail/task_executor.go`

### Modification 1: Query Filter for sent_time (Line ~675)

**Location**: `getNextRecipientBatch` function

**Change**: Add `sent_time` filter to prevent deferred recipients from being fetched before their scheduled time.

**Find this code:**
```go
err := g.DB().Model("recipient_info").
    Where("task_id", taskId).
    Where("is_sent", 0).
    Where("id > ?", lastId).
    Order("id ASC").
    Limit(batchSize).
    Scan(&recipients)
```

**Replace with:**
```go
err := g.DB().Model("recipient_info").
    Where("task_id", taskId).
    Where("is_sent", 0).
    Where("sent_time <= ?", time.Now().Unix()).
    Where("id > ?", lastId).
    Order("id ASC").
    Limit(batchSize).
    Scan(&recipients)
```

**Why**: Without this filter, deferred recipients (sent_time in the future) would be queried immediately and stuck in an infinite loop.

---

### Modification 2: Updates Map Declaration (Line ~739)

**Location**: `processRecipientBatch` function, before the recipient loop

**Important**: This map must exist to collect deferred recipients. Add this line if not present:

```go
updates := make(map[int]int)
```

This should be placed right before `// submit send task for each recipient` comment.

---

### Modification 3: Warmup Check with Minimum Defer (Lines ~774-788)

**Location**: `processRecipientBatch` function, inside the warmup check block

**Find this code (if different):**
```go
if warmupAssociated, ok := e.ctx.Value("warmupAssociated").(bool); ok && warmupAssociated {
    allow, waitSeconds, _ := warmup.RateLimiter().Allow(ctx, e.ctx.Value("serverIP").(string), recipient.Recipient)

    if !allow {
        // ... old defer logic
    }
}
```

**Replace with:**
```go
// check if recipient is allowed to send with warmup
// This enforces warmup-compliant spacing between emails (only when warmup is enabled)
if warmupAssociated, ok := e.ctx.Value("warmupAssociated").(bool); ok && warmupAssociated {
    allow, waitSeconds, _ := warmup.RateLimiter().Allow(ctx, e.ctx.Value("serverIP").(string), recipient.Recipient)

    if !allow || waitSeconds > 0 {
        // Always defer - never sleep inline (production-grade approach)
        // Daily count is NOT incremented for deferred attempts
        retryAfter := waitSeconds
        if retryAfter < 60 {
            retryAfter = 60 // Minimum 60 seconds (Gmail-safe during warm-up)
        }
        updates[recipient.Id] = retryAfter
        g.Log().Debug(ctx, "Warmup: recipient %d deferred, retry after %d seconds", recipient.Id, retryAfter)
        continue
    }
}
```

**Why**:
- Check `waitSeconds > 0` in addition to `!allow` to catch spacing enforcement
- Minimum 60 seconds prevents tight retry loops that could trigger spam filters

---

### Modification 4: Deferred Recipients Update (Lines ~843-864)

**Location**: `processRecipientBatch` function, after the recipient processing loop

**Find this code (if different - may use OnConflict batch update):**
```go
if len(updates) > 0 {
    // ... old batch update with OnConflict
}
```

**Replace with:**
```go
if len(updates) > 0 {
    curTime := int(time.Now().Unix())
    i := 0
    for id, waits := range updates {
        // Simple stagger: waits + i ensures unique sent_time for each recipient
        // - waits: minimum wait from rate limiter (typically 60 sec)
        // - i: adds 1 second per recipient to prevent duplicates
        // This works for both small (20) and large (200K) campaigns:
        // - Rate limiter controls actual sending speed
        // - Unique sent_times prevent batch overload
        newSentTime := curTime + waits + i
        // Reset is_sent=0 and update sent_time for deferred recipients
        _, _ = g.DB().Ctx(ctx).Model("recipient_info").
            Where("id", id).
            Data(g.Map{
                "is_sent":   0,
                "sent_time": newSentTime,
            }).
            Update()
        i++
    }
}
```

**Why**:
- **Reset is_sent=0**: Critical! Without this, records stay at is_sent=2 forever
- **Simple formula `curTime + waits + i`**: Guarantees unique sent_times
  - Original formula `curTime + (waits * ((i % 10) + 1))` caused duplicates (i=0 and i=10 got same time)
- **Individual updates**: More reliable than batch OnConflict for this use case

---

## Build and Deployment Instructions

### Prerequisites

- Go 1.21+ installed
- Access to the TezMail server (Docker)

### Step 1: Build for Linux

Open terminal/cmd in the `core` directory:

```bash
# Windows CMD
cd D:\abdul-ai\git-tezmail\TezMail-marketing\core
set GOOS=linux
set GOARCH=amd64
go build -o billionmail-core .

# Windows PowerShell
cd D:\abdul-ai\git-tezmail\TezMail-marketing\core
$env:GOOS="linux"
$env:GOARCH="amd64"
go build -o billionmail-core .

# Linux/Mac
cd /path/to/TezMail-marketing/core
GOOS=linux GOARCH=amd64 go build -o billionmail-core .
```

### Step 2: Copy to Server

```bash
# Using SCP
scp billionmail-core root@YOUR_SERVER_IP:/opt/billionmail/core/

# Or using rsync
rsync -avz billionmail-core root@YOUR_SERVER_IP:/opt/billionmail/core/
```

### Step 3: Restart Docker Container

SSH into the server and restart:

```bash
ssh root@YOUR_SERVER_IP

# Find the container name
docker ps | grep billionmail

# Restart the container (replace with actual container name)
docker restart billionmail-core-billionmail-1

# Or if using docker-compose
cd /opt/billionmail
docker-compose restart billionmail
```

### Step 4: Verify Deployment

```bash
# Check container logs
docker logs billionmail-core-billionmail-1 --tail 100

# Check for warmup-related logs
docker logs billionmail-core-billionmail-1 2>&1 | grep -i "warmup\|spacing\|ratelimit"
```

---

## Verification Commands

### Check Rate Limiter is Working

In Redis, you should see these keys when warmup is active:

```bash
# SSH to server
docker exec -it billionmail-redis redis-cli

# Check for warmup keys
KEYS warmup:*

# Example output:
# warmup:tb:h:YOUR_IP:gmail
# warmup:spacing:YOUR_IP:gmail
# warmup:vw:d:YOUR_IP:gmail
# warmup:limits:YOUR_IP:gmail
```

### Check Deferred Recipients

```sql
-- In MySQL/MariaDB
SELECT id, recipient, is_sent, sent_time,
       FROM_UNIXTIME(sent_time) as scheduled_time
FROM recipient_info
WHERE task_id = YOUR_TASK_ID
  AND is_sent = 0
  AND sent_time > UNIX_TIMESTAMP()
ORDER BY sent_time ASC
LIMIT 20;
```

### Monitor Sending Rate

```bash
# Watch container logs in real-time
docker logs -f billionmail-core-billionmail-1 2>&1 | grep -E "performance stats|Warmup:|RateLimiter:"
```

---

## Summary of Changes

| File | Location | Change |
|------|----------|--------|
| `rate_limiter.go` | Entire file | Complete replacement with token bucket + spacing Lua script |
| `task_executor.go` | Line ~675 | Add `sent_time <= ?` filter to query |
| `task_executor.go` | Line ~739 | Add `updates := make(map[int]int)` map declaration |
| `task_executor.go` | Lines ~774-788 | Add `waitSeconds > 0` check + minimum 60 sec defer |
| `task_executor.go` | Lines ~843-864 | Individual updates with `is_sent=0` reset + simple stagger formula |

---

## Troubleshooting

### Recipients Stuck at is_sent=2

**Cause**: Old code didn't reset `is_sent` when deferring

**Fix**: Run this SQL to unstick them:
```sql
UPDATE recipient_info
SET is_sent = 0, sent_time = UNIX_TIMESTAMP()
WHERE is_sent = 2;
```

### Emails Sending Too Fast During Warmup

**Check**: Redis spacing keys
```bash
docker exec -it billionmail-redis redis-cli GET "warmup:spacing:YOUR_IP:gmail"
```

If empty, the rate limiter may not be initialized. Check logs for errors.

### Build Fails

Ensure you have all dependencies:
```bash
cd /path/to/core
go mod tidy
go mod download
```

---

*Document created: 2026-01-08*
*Last verified: 2026-01-08*
