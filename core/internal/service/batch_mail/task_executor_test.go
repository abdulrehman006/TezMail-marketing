package batch_mail

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// TestGetTaskIdFromContext_ConcurrentWithRegistration reproduces the crash
// scenario: several scheduled campaigns come due at once, so ProcessEmailTasks
// registers executors in a tight loop while already-spawned goroutines are
// inside ProcessTask -> getTaskIdFromContext iterating the same map.
//
// Before the fix this raised "concurrent map iteration and map write", which is
// a fatal runtime error -- recover() does not catch it, the process dies. Run
// with -race to make this meaningful.
func TestGetTaskIdFromContext_ConcurrentWithRegistration(t *testing.T) {
	ctx := context.Background()

	// Keep the global map clean for other tests.
	t.Cleanup(func() {
		taskExecutorsMutex.Lock()
		taskExecutors = make(map[int]*TaskExecutor)
		taskExecutorsMutex.Unlock()
	})

	self := NewTaskExecutor(ctx)
	RegisterTaskExecutor(1, self)

	var wg sync.WaitGroup

	// Writers: registration and removal, as ProcessEmailTasks and the idle
	// cleanup timer do.
	for i := 2; i < 40; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			RegisterTaskExecutor(id, NewTaskExecutor(ctx))
			RemoveTaskExecutor(id)
		}(i)
	}

	// Readers: the lookup that used to iterate unguarded.
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := self.getTaskIdFromContext(ctx); err != nil {
				t.Errorf("lookup failed for a registered executor: %v", err)
			}
		}()
	}

	wg.Wait()
}

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
