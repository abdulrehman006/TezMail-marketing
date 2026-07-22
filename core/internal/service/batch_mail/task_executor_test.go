package batch_mail

import (
	"billionmail-core/internal/model/entity"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// Retry policy
// ---------------------------------------------------------------------------

func TestRetryBackoffSeconds_IsIncreasingAndBounded(t *testing.T) {
	// The scheduler re-picks a task every 5s, so a backoff below that would
	// retry almost immediately against whatever is still broken.
	const schedulerInterval = 5

	prev := int64(0)
	for attempt := 0; attempt < 5; attempt++ {
		got := retryBackoffSeconds(attempt)

		if got <= schedulerInterval {
			t.Errorf("attempt %d: backoff %ds is not longer than the %ds scheduler tick",
				attempt, got, schedulerInterval)
		}
		if got < prev {
			t.Errorf("attempt %d: backoff went backwards (%ds after %ds)", attempt, got, prev)
		}
		if got > 3600 {
			t.Errorf("attempt %d: backoff %ds is unreasonably long", attempt, got)
		}
		prev = got
	}
}

func TestRetryBackoffSeconds_HandlesNegativeAttempts(t *testing.T) {
	if got := retryBackoffSeconds(-1); got <= 0 {
		t.Errorf("negative attempt count produced backoff %d", got)
	}
}

func TestMaxSendAttempts_IsBoundedAndGreaterThanOne(t *testing.T) {
	// Greater than one, or nothing is ever actually retried. Small, or a
	// permanently failing provider gets hammered.
	if maxSendAttempts < 2 {
		t.Errorf("maxSendAttempts = %d, retries are effectively disabled", maxSendAttempts)
	}
	if maxSendAttempts > 5 {
		t.Errorf("maxSendAttempts = %d is high enough to amplify an outage", maxSendAttempts)
	}
}

// TestRetryDecision_MatchesPolicy exercises the exact branch used in
// processSendResults, so the routing of a result cannot drift from the policy
// without this failing.
func TestRetryDecision_MatchesPolicy(t *testing.T) {
	decide := func(r *SendResult) string {
		switch {
		case r.Success:
			return "success"
		case r.Retryable && r.AttemptCount+1 < maxSendAttempts:
			return "retry"
		default:
			return "terminal"
		}
	}

	cases := []struct {
		name string
		r    *SendResult
		want string
	}{
		{"delivered", &SendResult{Success: true}, "success"},
		{"transient, first failure", &SendResult{Retryable: true, AttemptCount: 0}, "retry"},
		{"transient, second failure", &SendResult{Retryable: true, AttemptCount: 1}, "retry"},
		{"transient, budget exhausted", &SendResult{Retryable: true, AttemptCount: maxSendAttempts - 1}, "terminal"},
		{"transient, past budget", &SendResult{Retryable: true, AttemptCount: maxSendAttempts + 5}, "terminal"},
		{"permanent, first failure", &SendResult{Retryable: false, AttemptCount: 0}, "terminal"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := decide(tc.r); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestGroupFailures_CollapsesIdenticalOutcomes(t *testing.T) {
	// The outage case: every recipient in the batch fails the same way. This
	// must produce ONE group, or the failure path issues an UPDATE per
	// recipient exactly when the system is already under strain.
	throttled := errors.New("ThrottlingException: Maximum sending rate exceeded")

	results := make([]*SendResult, 0, 50)
	for i := 1; i <= 50; i++ {
		results = append(results, &SendResult{RecipientID: i, Error: throttled, AttemptCount: 0})
	}

	groups := groupFailures(results)
	if len(groups) != 1 {
		t.Fatalf("identical failures produced %d groups, want 1", len(groups))
	}
	for _, ids := range groups {
		if len(ids) != 50 {
			t.Errorf("group holds %d ids, want 50", len(ids))
		}
	}
}

func TestGroupFailures_SeparatesDifferentOutcomes(t *testing.T) {
	// Different attempt counts need different backoffs, and different errors
	// need different last_error values, so they must not be merged.
	errA := errors.New("throttled")
	errB := errors.New("connection refused")

	results := []*SendResult{
		{RecipientID: 1, Error: errA, AttemptCount: 0},
		{RecipientID: 2, Error: errA, AttemptCount: 0},
		{RecipientID: 3, Error: errA, AttemptCount: 1}, // different attempt
		{RecipientID: 4, Error: errB, AttemptCount: 0}, // different error
	}

	groups := groupFailures(results)
	if len(groups) != 3 {
		t.Fatalf("got %d groups, want 3", len(groups))
	}

	// Every recipient must appear exactly once -- losing one here would lose
	// the recipient entirely.
	seen := make(map[interface{}]int)
	for _, ids := range groups {
		for _, id := range ids {
			seen[id]++
		}
	}
	if len(seen) != 4 {
		t.Errorf("grouping covered %d recipients, want 4", len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("recipient %v appeared %d times", id, n)
		}
	}
}

func TestGroupFailures_EmptyInput(t *testing.T) {
	if groups := groupFailures(nil); len(groups) != 0 {
		t.Errorf("nil input produced %d groups", len(groups))
	}
}

func TestTruncateUTF8_NeverProducesInvalidUTF8(t *testing.T) {
	// Byte slicing can cut through a multi-byte rune. Postgres rejects invalid
	// UTF-8 in a text column, so the UPDATE carrying it fails -- and that
	// update is what releases a recipient from the claimed state, so the row
	// would be stranded at is_sent=2 and the task could never finish.
	//
	// Sweep the cut across a rune boundary so at least one case lands mid-rune.
	for pad := 495; pad <= 502; pad++ {
		msg := strings.Repeat("x", pad) + "é" + strings.Repeat("y", 50)
		got := truncateUTF8(msg, 500)

		if !utf8.ValidString(got) {
			t.Errorf("pad=%d produced invalid UTF-8", pad)
		}
		if len(got) > 500 {
			t.Errorf("pad=%d exceeded the byte cap: %d", pad, len(got))
		}
	}
}

func TestTruncateUTF8_MultiByteScripts(t *testing.T) {
	// 3-byte and 4-byte runes, so the walk-back has to skip more than one byte.
	for _, s := range []string{
		strings.Repeat("あ", 400), // 3 bytes each
		strings.Repeat("😀", 300), // 4 bytes each
		strings.Repeat("ü", 400), // 2 bytes each
	} {
		for _, limit := range []int{100, 250, 499, 500, 501} {
			got := truncateUTF8(s, limit)
			if !utf8.ValidString(got) {
				t.Errorf("invalid UTF-8 at limit %d", limit)
			}
			if len(got) > limit {
				t.Errorf("exceeded limit %d: got %d bytes", limit, len(got))
			}
		}
	}
}

func TestTruncateUTF8_ShortInputUnchanged(t *testing.T) {
	const s = "ThrottlingException: Maximum sending rate exceeded"
	if got := truncateUTF8(s, 500); got != s {
		t.Errorf("short string was altered: %q", got)
	}
}

func TestTruncateError_BoundsLengthAndHandlesNil(t *testing.T) {
	if got := truncateError(nil); got != "" {
		t.Errorf("nil error produced %q", got)
	}

	long := errors.New(strings.Repeat("x", 5000))
	if got := truncateError(long); len(got) > 500 {
		t.Errorf("error not truncated: %d chars", len(got))
	}

	short := errors.New("throttled")
	if got := truncateError(short); got != "throttled" {
		t.Errorf("short error was altered: %q", got)
	}
}

// ---------------------------------------------------------------------------
// Panic safety
// ---------------------------------------------------------------------------

func TestSafeGo_PanicDoesNotKillTheProcess(t *testing.T) {
	// Go has no default recovery for goroutines: an unrecovered panic anywhere
	// terminates the whole process. If this test completes at all, the panic
	// was contained.
	done := make(chan struct{})

	safeGo(context.Background(), "test panic", func() {
		defer close(done)
		panic("simulated failure in a stats writer")
	})

	select {
	case <-done:
		// Reached the deferred close, so the panic was recovered rather than
		// propagating out of the goroutine.
	case <-time.After(5 * time.Second):
		t.Fatal("safeGo goroutine never ran")
	}
}

func TestSafeGo_NilPointerPanicIsContained(t *testing.T) {
	// The realistic shape: a nil dereference inside a stats writer.
	done := make(chan struct{})

	safeGo(context.Background(), "nil deref", func() {
		defer close(done)
		var m map[string]string
		m["boom"] = "this panics: assignment to entry in nil map"
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("goroutine did not complete")
	}
}

func TestSafeGo_NormalExecutionIsUnaffected(t *testing.T) {
	// The safety net must not change behaviour when nothing goes wrong.
	result := make(chan int, 1)
	safeGo(context.Background(), "normal work", func() {
		result <- 42
	})

	select {
	case v := <-result:
		if v != 42 {
			t.Errorf("got %d, want 42", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("goroutine did not run")
	}
}

// TestSendResultDelivery_SurvivesPanic models the pool-submitted function's
// contract: exactly one result must reach the collector, whether the send
// returns normally or unwinds through a panic.
//
// Before this, safeSend was the last statement, so a panic delivered nothing --
// leaving the recipient stuck at is_sent=2, invisible to both the work query
// and the completion count, so the task could never finish.
func TestSendResultDelivery_SurvivesPanic(t *testing.T) {
	deliver := func(shouldPanic bool) (delivered []*SendResult) {
		recipient := &entity.RecipientInfo{Id: 7, AttemptCount: 1}

		func() {
			var result *SendResult
			defer func() {
				if r := recover(); r != nil {
					result = nil
				}
				if result == nil {
					result = &SendResult{
						RecipientID:  recipient.Id,
						Success:      false,
						Error:        errors.New("send aborted unexpectedly"),
						Retryable:    true,
						AttemptCount: recipient.AttemptCount,
					}
				}
				delivered = append(delivered, result)
			}()

			if shouldPanic {
				panic("boom inside sendEmail")
			}
			result = &SendResult{RecipientID: recipient.Id, Success: true}
		}()
		return delivered
	}

	t.Run("normal send delivers the real result", func(t *testing.T) {
		got := deliver(false)
		if len(got) != 1 {
			t.Fatalf("delivered %d results, want exactly 1", len(got))
		}
		if !got[0].Success {
			t.Error("successful send was not reported as success")
		}
	})

	t.Run("panicking send still delivers a result", func(t *testing.T) {
		got := deliver(true)
		if len(got) != 1 {
			t.Fatalf("delivered %d results, want exactly 1", len(got))
		}
		if got[0].Success {
			t.Error("panic was reported as a successful send")
		}
		if got[0].RecipientID != 7 {
			t.Errorf("RecipientID = %d, want 7 -- a zero here strands the recipient", got[0].RecipientID)
		}
		if !got[0].Retryable {
			t.Error("panic result should be retryable, not written off")
		}
		if got[0].AttemptCount != 1 {
			t.Errorf("AttemptCount = %d, want the recipient's existing count", got[0].AttemptCount)
		}
	})
}

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
