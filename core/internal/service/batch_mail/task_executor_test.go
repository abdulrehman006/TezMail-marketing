package batch_mail

import (
	"strings"
	"testing"
)

// mailstat_message_ids, mailstat_senders and mailstat_send_mails all declare
// postfix_message_id as TEXT PRIMARY KEY, and the SES stats writers use
// InsertIgnore. A duplicate key is therefore discarded in silence rather than
// raising an error, so key uniqueness is what keeps the statistics honest.

func TestSESStatsKey_UsesSESMessageIDWhenPresent(t *testing.T) {
	got := sesStatsKey("ses-", "0100018f-aaaa-bbbb", "<123.abc@example.com>")
	if got != "ses-0100018f-aaaa-bbbb" {
		t.Errorf("got %q, want the SES message id to be used", got)
	}
}

func TestSESStatsKey_FallsBackWhenSESMessageIDEmpty(t *testing.T) {
	// An empty SES MessageId would otherwise make every send in the campaign
	// collapse onto the single key "ses-", so all but the first row vanish.
	const local = "123.abc@example.com"
	got := sesStatsKey("ses-", "", local)
	if got != "ses-"+local {
		t.Errorf("got %q, want fallback to the local message id", got)
	}
	if got == "ses-" {
		t.Fatal("key collapsed to the bare prefix")
	}
}

func TestSESStatsKey_FallsBackWhenSESMessageIDIsWhitespace(t *testing.T) {
	const local = "123.abc@example.com"
	if got := sesStatsKey("ses-", "   ", local); got != "ses-"+local {
		t.Errorf("got %q, want whitespace treated as absent", got)
	}
}

func TestSESStatsKey_FailureKeysAreUniquePerMessage(t *testing.T) {
	// The old failure key was "ses-fail-" + unix milliseconds, so concurrent
	// failures inside one millisecond produced identical keys and all but one
	// bounce was silently dropped. Keying on the per-message id fixes that.
	seen := make(map[string]bool)
	for i := 0; i < 500; i++ {
		local := generateMessageID("sender@example.com")
		key := sesStatsKey("ses-fail-", "", strings.Trim(local, "<>"))
		if seen[key] {
			t.Fatalf("duplicate failure key generated: %q", key)
		}
		seen[key] = true
	}
}

func TestGenerateMessageID_IsUniqueAndWellFormed(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := generateMessageID("sender@example.com")
		if !strings.HasPrefix(id, "<") || !strings.HasSuffix(id, ">") {
			t.Fatalf("Message-ID is not angle-bracketed: %q", id)
		}
		if !strings.HasSuffix(id, "@example.com>") {
			t.Fatalf("Message-ID does not carry the sender domain: %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate Message-ID generated: %q", id)
		}
		seen[id] = true
	}
}

func TestGenerateMessageID_FallsBackWithoutDomain(t *testing.T) {
	id := generateMessageID("not-an-email")
	if !strings.Contains(id, "@tezmail>") {
		t.Errorf("expected the tezmail fallback domain, got %q", id)
	}
}
