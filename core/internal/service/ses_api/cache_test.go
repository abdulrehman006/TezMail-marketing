package ses_api

import (
	"sync"
	"testing"
	"time"
)

func resetCaches() {
	accountCacheMu.Lock()
	accountCache = make(map[string]accountCacheEntry)
	accountCacheMu.Unlock()

	senderCacheMu.Lock()
	senderCache = make(map[string]*SESSender)
	senderCacheMu.Unlock()
}

func TestAccountCache_MissOnEmptyCache(t *testing.T) {
	resetCaches()
	if _, found := lookupAccountCache("example.com"); found {
		t.Error("empty cache reported a hit")
	}
}

func TestAccountCache_StoreAndLookup(t *testing.T) {
	resetCaches()
	want := &AccountConfig{Name: "acct", Region: "us-east-1"}
	storeAccountCache("example.com", want)

	got, found := lookupAccountCache("example.com")
	if !found {
		t.Fatal("stored entry was not found")
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestAccountCache_CachesNegativeResult(t *testing.T) {
	// A Postfix-only domain is the common case. If nil were not cacheable it
	// would hit the database on every single message.
	resetCaches()
	storeAccountCache("smtp-only.test", nil)

	got, found := lookupAccountCache("smtp-only.test")
	if !found {
		t.Fatal("negative result was not cached")
	}
	if got != nil {
		t.Errorf("expected nil account, got %+v", got)
	}
}

func TestAccountCache_EntriesExpire(t *testing.T) {
	resetCaches()

	accountCacheMu.Lock()
	accountCache["stale.test"] = accountCacheEntry{
		account: &AccountConfig{Name: "old"},
		expires: time.Now().Add(-time.Second),
	}
	accountCacheMu.Unlock()

	if _, found := lookupAccountCache("stale.test"); found {
		t.Error("expired entry was served")
	}
}

func TestInvalidateAccountCache_ClearsEverything(t *testing.T) {
	resetCaches()
	storeAccountCache("a.test", &AccountConfig{Name: "a"})
	storeAccountCache("b.test", nil)

	InvalidateAccountCache()

	if _, found := lookupAccountCache("a.test"); found {
		t.Error("positive entry survived invalidation")
	}
	if _, found := lookupAccountCache("b.test"); found {
		t.Error("negative entry survived invalidation")
	}
}

func TestAccountCache_EvictsAtCap(t *testing.T) {
	resetCaches()
	for i := 0; i < accountCacheMaxEntries+10; i++ {
		storeAccountCache(string(rune('a'+i%26))+string(rune('a'+i/26))+".test", nil)
	}

	accountCacheMu.RLock()
	size := len(accountCache)
	accountCacheMu.RUnlock()

	if size > accountCacheMaxEntries {
		t.Errorf("cache grew past its cap: %d entries", size)
	}
}

func TestAccountCache_ConcurrentAccessIsSafe(t *testing.T) {
	// Sends run through a concurrent worker pool, so this is the real usage
	// pattern. Run with -race to make it meaningful.
	resetCaches()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			domain := "d" + string(rune('a'+n%26)) + ".test"
			storeAccountCache(domain, &AccountConfig{Name: "acct"})
			_, _ = lookupAccountCache(domain)
			if n%10 == 0 {
				InvalidateAccountCache()
			}
		}(i)
	}
	wg.Wait()
}

func TestSenderCacheKey_ChangesWithEveryCredentialField(t *testing.T) {
	// A rotated secret must produce a different key, or the cache would keep
	// handing back a client signing with the old credentials.
	base := &AccountConfig{Region: "us-east-1", AccessKey: "AKIA1", SecretKey: "s1"}
	baseKey := senderCacheKey(base, "acct")

	cases := []struct {
		name    string
		account *AccountConfig
		asName  string
	}{
		{"different region", &AccountConfig{Region: "eu-west-1", AccessKey: "AKIA1", SecretKey: "s1"}, "acct"},
		{"different access key", &AccountConfig{Region: "us-east-1", AccessKey: "AKIA2", SecretKey: "s1"}, "acct"},
		{"different secret key", &AccountConfig{Region: "us-east-1", AccessKey: "AKIA1", SecretKey: "s2"}, "acct"},
		{"different account name", base, "other"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := senderCacheKey(tc.account, tc.asName); got == baseKey {
				t.Error("key did not change, so a stale client would be reused")
			}
		})
	}
}

func TestSenderCacheKey_StableForIdenticalInput(t *testing.T) {
	a := &AccountConfig{Region: "us-east-1", AccessKey: "AKIA1", SecretKey: "s1"}
	b := &AccountConfig{Region: "us-east-1", AccessKey: "AKIA1", SecretKey: "s1"}
	if senderCacheKey(a, "acct") != senderCacheKey(b, "acct") {
		t.Error("identical credentials produced different keys, defeating the cache")
	}
}

func TestSenderCacheKey_DoesNotLeakCredentials(t *testing.T) {
	a := &AccountConfig{Region: "us-east-1", AccessKey: "AKIAEXPOSED", SecretKey: "supersecret"}
	key := senderCacheKey(a, "acct")
	for _, secret := range []string{"AKIAEXPOSED", "supersecret"} {
		if contains(key, secret) {
			t.Errorf("cache key contains raw credential material: %q", key)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}()
}

func TestCachedSender_ReturnsSameInstance(t *testing.T) {
	resetCaches()
	account := &AccountConfig{Region: "us-east-1", AccessKey: "AKIAVALIDLOOKING1234", SecretKey: "secretvalue"}

	first, err := cachedSender(account, "acct")
	if err != nil {
		t.Fatalf("building sender failed: %v", err)
	}
	second, err := cachedSender(account, "acct")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if first != second {
		t.Error("cachedSender built a new client instead of reusing the cached one")
	}
}

func TestCachedSender_NewInstanceAfterInvalidation(t *testing.T) {
	resetCaches()
	account := &AccountConfig{Region: "us-east-1", AccessKey: "AKIAVALIDLOOKING1234", SecretKey: "secretvalue"}

	first, err := cachedSender(account, "acct")
	if err != nil {
		t.Fatalf("building sender failed: %v", err)
	}

	InvalidateAccountCache()

	second, err := cachedSender(account, "acct")
	if err != nil {
		t.Fatalf("rebuilding sender failed: %v", err)
	}
	if first == second {
		t.Error("sender survived invalidation; a rotated credential would keep using the old client")
	}
}

func TestCachedSender_RejectsPlaceholderCredentials(t *testing.T) {
	resetCaches()
	account := &AccountConfig{Region: "us-east-1", AccessKey: "AKIAXXXXXXXXXXXXXXXX", SecretKey: "YOUR_SECRET"}
	if _, err := cachedSender(account, "acct"); err == nil {
		t.Error("placeholder credentials were accepted")
	}
}
