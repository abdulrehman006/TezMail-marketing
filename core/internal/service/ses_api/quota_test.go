package ses_api

import (
	"strings"
	"testing"
)

func TestQuotaStatus_Remaining(t *testing.T) {
	cases := []struct {
		name string
		q    *QuotaStatus
		want float64
	}{
		{"room left", &QuotaStatus{Max24HourSend: 50000, SentLast24Hours: 12000}, 38000},
		{"exactly exhausted", &QuotaStatus{Max24HourSend: 200, SentLast24Hours: 200}, 0},
		{"fresh account", &QuotaStatus{Max24HourSend: 200, SentLast24Hours: 0}, 200},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.q.Remaining(); got != tc.want {
				t.Errorf("Remaining() = %.0f, want %.0f", got, tc.want)
			}
		})
	}
}

func TestQuotaStatus_OverQuotaNeverGoesNegative(t *testing.T) {
	// SES can report more sent than the limit when the limit was lowered, or
	// across a window boundary. A negative "remaining" would compare oddly and
	// could read as room available.
	q := &QuotaStatus{Max24HourSend: 200, SentLast24Hours: 250}
	if got := q.Remaining(); got != 0 {
		t.Errorf("Remaining() = %.0f, want 0 when over quota", got)
	}
}

func TestQuotaStatus_UnlimitedAccount(t *testing.T) {
	// AWS returns -1 for accounts with no 24-hour cap. Treating that
	// arithmetically would give a quota of minus one message and block every
	// campaign on the account.
	q := &QuotaStatus{Max24HourSend: -1, SentLast24Hours: 900000, Unlimited: true}

	if got := q.Remaining(); got <= 0 {
		t.Fatalf("unlimited account reported %.0f remaining", got)
	}
	if q.Remaining() < 1_000_000 {
		t.Error("unlimited account should not be constrained by any realistic campaign size")
	}
}

func TestQuotaStatus_NilIsTreatedAsUnconstrained(t *testing.T) {
	// A nil status means "no SES account for this domain", which must not
	// block a send that is going out over SMTP anyway.
	var q *QuotaStatus
	if got := q.Remaining(); got <= 0 {
		t.Errorf("nil quota reported %.0f remaining, want unconstrained", got)
	}
}

func TestQuotaGate_HybridBoundary(t *testing.T) {
	// The hybrid policy: block ONLY when there is no quota left at all. Any
	// positive remaining lets the campaign start -- it sends what fits and the
	// circuit breaker pauses the overflow. This is the exact comparison the
	// pre-flight gate uses (quota.Remaining() > 0 -> allow).
	allowed := func(q *QuotaStatus) bool {
		return q.Remaining() > 0
	}

	cases := []struct {
		name string
		q    *QuotaStatus
		want bool
	}{
		{"room to spare", &QuotaStatus{Max24HourSend: 50000, SentLast24Hours: 10000}, true},
		{"one message left", &QuotaStatus{Max24HourSend: 1000, SentLast24Hours: 999}, true},
		{"a huge campaign with a little quota still starts", &QuotaStatus{Max24HourSend: 50000, SentLast24Hours: 49000}, true},
		{"exactly exhausted -> block", &QuotaStatus{Max24HourSend: 200, SentLast24Hours: 200}, false},
		{"over quota -> block", &QuotaStatus{Max24HourSend: 200, SentLast24Hours: 250}, false},
		{"unlimited account -> always allowed", &QuotaStatus{Max24HourSend: -1, SentLast24Hours: 900000, Unlimited: true}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := allowed(tc.q); got != tc.want {
				t.Errorf("allowed=%v, want %v (remaining=%.0f)", got, tc.want, tc.q.Remaining())
			}
		})
	}
}

func TestDescribeQuotaExhausted_IsActionableAndNeutral(t *testing.T) {
	q := &QuotaStatus{Max24HourSend: 50000, SentLast24Hours: 50000}
	msg := DescribeQuotaExhausted(q)

	if !strings.Contains(msg, "50000") {
		t.Errorf("exhausted message should state the limit: %s", msg)
	}
	// Must not promise auto-resume, which the system does not do.
	if strings.Contains(strings.ToLower(msg), "automatically") {
		t.Errorf("message must not imply auto-resume: %s", msg)
	}
	// Provider-neutral, like the shortfall message.
	for _, forbidden := range []string{"SES", "AWS", "Amazon"} {
		if strings.Contains(msg, forbidden) {
			t.Errorf("message leaks provider name %q: %s", forbidden, msg)
		}
	}
}
