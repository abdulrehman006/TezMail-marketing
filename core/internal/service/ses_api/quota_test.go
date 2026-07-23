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

func TestDescribeQuotaShortfall_IsActionable(t *testing.T) {
	q := &QuotaStatus{Max24HourSend: 50000, SentLast24Hours: 47000}
	msg := DescribeQuotaShortfall(q, 12000)

	// The operator needs all four numbers to decide what to do, plus a route
	// out. A message saying only "limit reached" forces them to go digging.
	for _, want := range []string{"12000", "3000", "50000", "47000"} {
		if !strings.Contains(msg, want) {
			t.Errorf("shortfall message is missing %q: %s", want, msg)
		}
	}
	if !strings.Contains(strings.ToLower(msg), "reduce the recipient list") {
		t.Error("shortfall message should point at a remedy the operator can act on")
	}

	// Provider-neutral: people running campaigns should not see vendor names.
	for _, forbidden := range []string{"SES", "AWS", "Amazon"} {
		if strings.Contains(msg, forbidden) {
			t.Errorf("shortfall message leaks the provider name %q: %s", forbidden, msg)
		}
	}
}

func TestQuotaGate_DecisionBoundary(t *testing.T) {
	// The exact comparison used before a campaign starts.
	allowed := func(q *QuotaStatus, needed int) bool {
		return q.Remaining() >= float64(needed)
	}

	q := &QuotaStatus{Max24HourSend: 1000, SentLast24Hours: 900} // 100 left

	if !allowed(q, 100) {
		t.Error("a campaign that exactly fits the remaining quota should be allowed")
	}
	if allowed(q, 101) {
		t.Error("a campaign one over the remaining quota must be refused")
	}
	if !allowed(q, 0) {
		t.Error("an empty campaign should never be blocked on quota")
	}
}
