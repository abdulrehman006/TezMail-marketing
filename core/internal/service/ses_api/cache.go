package ses_api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// Caching for the SES send path.
//
// Both of these ran once per recipient before:
//
//   - The domain -> account lookup, which also executed several diagnostic
//     queries including a full scan of bm_ses_domain_mapping, and was called
//     twice per send (once by sendEmail, once by GetSenderForEmail).
//   - Construction of a fresh sesv2.Client, which brings its own HTTP
//     transport and connection pool, so a campaign got zero TLS reuse and
//     every in-flight send carried an independent retry budget.
//
// On a 50k-recipient campaign that is hundreds of thousands of queries and
// 50k TLS handshakes. Neither input changes during a campaign.
//
// Entries expire on a short TTL so a change made directly in the database is
// still picked up, and the write paths invalidate explicitly so a change made
// through the UI takes effect at once.

const (
	// accountCacheTTL bounds how long a change that bypasses the write paths
	// (a manual database edit) can go unnoticed.
	accountCacheTTL = 60 * time.Second

	// accountCacheMaxEntries caps memory. Keys are sender domains, which come
	// from configured mailboxes and are few, so this is a backstop rather than
	// an expected limit.
	accountCacheMaxEntries = 1024
)

type accountCacheEntry struct {
	account *AccountConfig // nil is a valid, cached "no SES for this domain"
	expires time.Time
}

var (
	accountCacheMu sync.RWMutex
	accountCache   = make(map[string]accountCacheEntry)

	senderCacheMu sync.RWMutex
	senderCache   = make(map[string]*SESSender)
)

// lookupAccountCache returns a cached resolution for domain. found is false
// when there is no usable entry; a found entry may still carry a nil account,
// which means "this domain has no SES account" and is worth caching because it
// is the common case for a Postfix-only domain.
func lookupAccountCache(domain string) (account *AccountConfig, found bool) {
	accountCacheMu.RLock()
	entry, ok := accountCache[domain]
	accountCacheMu.RUnlock()

	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	return entry.account, true
}

func storeAccountCache(domain string, account *AccountConfig) {
	accountCacheMu.Lock()
	defer accountCacheMu.Unlock()

	// Cheapest safe eviction: once the cap is hit, drop everything and let the
	// map refill. The working set is tiny, so this should never fire.
	if len(accountCache) >= accountCacheMaxEntries {
		accountCache = make(map[string]accountCacheEntry)
	}

	accountCache[domain] = accountCacheEntry{
		account: account,
		expires: time.Now().Add(accountCacheTTL),
	}
}

// InvalidateAccountCache clears both caches. Call it from every path that
// changes account credentials, status, enabled flag or domain mapping, so an
// admin's change takes effect immediately rather than after the TTL.
func InvalidateAccountCache() {
	accountCacheMu.Lock()
	accountCache = make(map[string]accountCacheEntry)
	accountCacheMu.Unlock()

	senderCacheMu.Lock()
	senderCache = make(map[string]*SESSender)
	senderCacheMu.Unlock()

	sesTracef(context.Background(), "account and sender caches invalidated")
}

// senderCacheKey fingerprints everything that would require a different AWS
// client. Hashing keeps credentials out of the map key while still changing
// the key whenever a credential is rotated.
func senderCacheKey(account *AccountConfig, accountName string) string {
	h := sha256.New()
	h.Write([]byte(accountName))
	h.Write([]byte{0})
	h.Write([]byte(account.Region))
	h.Write([]byte{0})
	h.Write([]byte(account.AccessKey))
	h.Write([]byte{0})
	h.Write([]byte(account.SecretKey))
	return hex.EncodeToString(h.Sum(nil))
}

// cachedSender returns a shared SESSender for the account, building one on
// first use.
//
// sesv2.Client is safe for concurrent use and designed to be long-lived, so
// sharing one across a campaign is both correct and the point: it restores
// connection reuse and gives the SDK a single retry token bucket to rate-limit
// against, instead of every send retrying independently under throttling.
func cachedSender(account *AccountConfig, accountName string) (*SESSender, error) {
	key := senderCacheKey(account, accountName)

	senderCacheMu.RLock()
	sender, ok := senderCache[key]
	senderCacheMu.RUnlock()
	if ok {
		return sender, nil
	}

	sender, err := NewSESSender(account, accountName)
	if err != nil {
		return nil, err
	}

	senderCacheMu.Lock()
	// Another goroutine may have won the race; either instance is equivalent,
	// so keep whichever landed first and let this one be collected.
	if existing, raced := senderCache[key]; raced {
		senderCacheMu.Unlock()
		return existing, nil
	}
	senderCache[key] = sender
	senderCacheMu.Unlock()

	return sender, nil
}
