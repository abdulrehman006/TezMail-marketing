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
