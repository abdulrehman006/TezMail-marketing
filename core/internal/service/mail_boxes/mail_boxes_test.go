package mail_boxes

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Path traversal guard
// ---------------------------------------------------------------------------

func TestResolveMailPath_RejectsTraversal(t *testing.T) {
	base := filepath.Clean("/var/vmail")

	// Every one of these tries to escape the mail root and must be refused.
	bad := []struct {
		domain, local string
	}{
		{"example.com", "../../etc"},
		{"example.com", "../../../tmp/evil"},
		{"..", "x"},
		{"example.com", ".."},
		{"../other", "user"},
		{"example.com", "../../.."},
	}
	for _, tc := range bad {
		t.Run(tc.domain+"/"+tc.local, func(t *testing.T) {
			got, err := resolveMailPath(base, tc.domain, tc.local)
			if err == nil {
				t.Errorf("expected traversal to be rejected, but got path %q", got)
			}
		})
	}
}

func TestResolveMailPath_AcceptsValidAndStaysUnderRoot(t *testing.T) {
	base := filepath.Clean("/var/vmail")

	good := []struct {
		domain, local string
	}{
		{"example.com", "user"},
		{"example.com", "first.last"},   // dotted local parts are legitimate
		{"sub.example.co.uk", "a_b-c"},
		{"example.com", "sales+promo"},  // plus addressing
	}
	for _, tc := range good {
		t.Run(tc.domain+"/"+tc.local, func(t *testing.T) {
			got, err := resolveMailPath(base, tc.domain, tc.local)
			if err != nil {
				t.Fatalf("valid mailbox rejected: %v", err)
			}
			// The result must be inside the root.
			if got != base && !strings.HasPrefix(got, base+string(filepath.Separator)) {
				t.Errorf("resolved path %q is not under base %q", got, base)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Password generation
// ---------------------------------------------------------------------------

func TestGenerateRandomPassword_LengthAndCharset(t *testing.T) {
	const n = 16
	pw := generateRandomPassword("", n)
	if len(pw) != n {
		t.Fatalf("length = %d, want %d", len(pw), n)
	}
	for _, r := range pw {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", r) {
			t.Errorf("password contains out-of-charset rune %q", r)
		}
	}
}

func TestGenerateRandomPassword_IsUnique(t *testing.T) {
	// crypto/rand should never repeat across a small sample. A collision here
	// would signal a regression back to a seeded/predictable generator.
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		pw := generateRandomPassword("", 16)
		if pw == "" {
			t.Fatal("generator returned empty (crypto/rand unavailable?)")
		}
		if seen[pw] {
			t.Fatalf("duplicate password generated: %q", pw)
		}
		seen[pw] = true
	}
}

func TestGenerateMailboxPassword_MinimumLength(t *testing.T) {
	// The exported helper must never return a short password even if asked.
	if got := len(GenerateMailboxPassword(4)); got < 12 {
		t.Errorf("length = %d, want >= 12", got)
	}
	if got := len(GenerateMailboxPassword(20)); got != 20 {
		t.Errorf("length = %d, want 20", got)
	}
}

// ---------------------------------------------------------------------------
// local_part validation regex (the anchored pattern used in the API contract)
// ---------------------------------------------------------------------------

func TestLocalPartRegex_AnchoredRejectsTraversal(t *testing.T) {
	// This mirrors the pattern in api/mail_boxes/v1/mailboxes.go. Anchoring is
	// what stops path-traversal input from passing validation; GoFrame matches
	// with the same anchored semantics.
	re := regexp.MustCompile(`^[a-zA-Z0-9._+-]{1,}$`)

	accept := []string{"user", "first.last", "a_b-c", "sales+promo", "John.Doe99"}
	for _, s := range accept {
		if !re.MatchString(s) {
			t.Errorf("valid local part %q was rejected", s)
		}
	}

	// The regex blocks path separators, spaces, shell/at chars. Note ".." is NOT
	// in this list: the anchored charset allows dots (for "first.last"), so a
	// bare ".." passes the regex -- and the resolveMailPath guard is the second
	// layer that stops it from escaping the mail root (tested above).
	reject := []string{"../../etc", "a/b", "foo bar", "x;rm", "user@evil", "a\\b", ""}
	for _, s := range reject {
		if re.MatchString(s) {
			t.Errorf("dangerous local part %q was accepted", s)
		}
	}
}
