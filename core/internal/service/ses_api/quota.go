package ses_api

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"
)

// QuotaStatus is an account's live 24-hour sending position.
//
// SES enforces a rolling 24-hour cap. Exceeding it returns
// TooManyRequestsException with "Daily message quota exceeded" -- the same
// exception type as per-second throttling, but a condition that does not clear
// until the window rolls. Checking before a campaign starts turns thousands of
// individual failures into one message up front.
type QuotaStatus struct {
	Max24HourSend   float64
	SentLast24Hours float64

	// Unlimited is true for accounts with no 24-hour cap. AWS signals this by
	// returning -1 for Max24HourSend, which would otherwise read as a quota of
	// minus one message and block every campaign.
	Unlimited bool
}

// Remaining reports how many more messages the account may send in the current
// window. Returns a large sentinel for unlimited accounts so callers can
// compare numerically without special-casing.
func (q *QuotaStatus) Remaining() float64 {
	if q == nil || q.Unlimited {
		return float64(1<<62 - 1)
	}
	remaining := q.Max24HourSend - q.SentLast24Hours
	if remaining < 0 {
		return 0
	}
	return remaining
}

// GetQuota fetches the account's live sending quota from AWS.
//
// Deliberately live rather than reading the cached copy in bm_ses_accounts:
// that copy is only refreshed when someone clicks "test connection", so it can
// be days stale, and a stale quota would either block a campaign that could
// have run or wave through one that cannot. One API call per campaign start is
// negligible.
func (s *SESSender) GetQuota(ctx context.Context) (*QuotaStatus, error) {
	// Bound the call so a stalled connection cannot hold up a campaign start.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	out, err := s.client.GetAccount(ctx, &sesv2.GetAccountInput{})
	if err != nil {
		return nil, fmt.Errorf("could not read SES quota for account %q: %w", s.accountName, err)
	}
	if out.SendQuota == nil {
		return nil, fmt.Errorf("SES returned no quota for account %q", s.accountName)
	}

	return &QuotaStatus{
		Max24HourSend:   out.SendQuota.Max24HourSend,
		SentLast24Hours: out.SendQuota.SentLast24Hours,
		Unlimited:       out.SendQuota.Max24HourSend < 0,
	}, nil
}

// CheckSendQuota reports whether the account mapped to senderEmail has room for
// `needed` more messages in its current 24-hour window.
//
// Three distinct outcomes, and callers must tell them apart:
//
//	(nil, nil)      no SES account for this domain -- nothing to check, and the
//	                send will go via SMTP, so do not block it
//	(status, nil)   a real answer; compare needed against status.Remaining()
//	(nil, err)      the check itself failed
//
// On (nil, err) the caller should log and PROCEED. Refusing to send because we
// could not reach AWS to ask about the quota would turn a monitoring blip into
// an outage, and the send path already handles a genuine quota rejection.
func CheckSendQuota(ctx context.Context, senderEmail string) (*QuotaStatus, error) {
	account := GetAccountForDomain(senderEmail)
	if account == nil {
		return nil, nil
	}

	sender, _, err := GetSenderForEmail(ctx, senderEmail)
	if err != nil {
		return nil, fmt.Errorf("could not build a SES client to check quota for %s: %w", senderEmail, err)
	}

	return sender.GetQuota(ctx)
}

// DescribeQuotaExhausted explains that the daily sending allowance is fully
// used, so the campaign cannot start yet. Used by the pre-flight check, which
// blocks only at zero remaining -- a campaign that can send anything at all is
// allowed to run and the breaker handles the overflow.
//
// Deliberately does not promise auto-resume: a paused campaign is restarted by
// the operator, not automatically when the window rolls.
func DescribeQuotaExhausted(q *QuotaStatus) string {
	return fmt.Sprintf(
		"Daily sending limit reached: none of the %.0f-message daily allowance remains in the "+
			"current 24-hour window (already sent %.0f). The campaign has been paused with no "+
			"messages sent. Resume it after the limit resets, or request a higher sending limit.",
		q.Max24HourSend, q.SentLast24Hours)
}
