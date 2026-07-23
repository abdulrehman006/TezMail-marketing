package ses_api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

func TestClassifyFailure_PermanentErrors(t *testing.T) {
	// Message-level problems: retrying cannot help for THIS message, but the
	// account is fine and other messages may well succeed, so the campaign
	// must keep going. Account-level conditions moved to
	// TestClassifyFailure_AccountLevelConditions.
	cases := []struct {
		name string
		err  error
	}{
		{"bad request", &types.BadRequestException{}},
		{"not found", &types.NotFoundException{}},
		{"context cancelled", context.Canceled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyFailure(tc.err); got != FailurePermanent {
				t.Errorf("got %s, want permanent", got)
			}
		})
	}
}

func TestClassifyFailure_TransientErrors(t *testing.T) {
	// These are exactly the cases that used to lose recipients permanently.
	cases := []struct {
		name string
		err  error
	}{
		// Throttles must carry the per-second rate wording to be retryable; an
		// unlabelled throttle now fails safe to a pause (see the throttle-split
		// test), because a reworded daily-quota message must not be retried.
		{"per-second rate limit", &types.TooManyRequestsException{Message: strPtr("Maximum sending rate exceeded")}},
		{"limit exceeded, rate", &types.LimitExceededException{Message: strPtr("Sending rate exceeded")}},
		{"internal service error", &types.InternalServiceErrorException{}},
		{"concurrent modification", &types.ConcurrentModificationException{}},
		{"dns failure", &net.DNSError{Err: "no such host", Name: "email.us-east-1.amazonaws.com"}},
		{"connection refused", &net.OpError{Op: "dial", Err: errors.New("connection refused")}},
		{"deadline exceeded", context.DeadlineExceeded},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyFailure(tc.err); got != FailureTransient {
				t.Errorf("got %s, want transient", got)
			}
		})
	}
}

func TestClassifyFailure_WrappedErrorsStillClassify(t *testing.T) {
	// The send path wraps errors before they reach the classifier, so
	// unwrapping has to work or everything falls through to the default.
	wrapped := fmt.Errorf("SES account %q rejected the message: %w",
		"prod", fmt.Errorf("api error: %w", &types.TooManyRequestsException{
			Message: strPtr("Maximum sending rate exceeded"),
		}))

	if got := ClassifyFailure(wrapped); got != FailureTransient {
		t.Errorf("wrapped rate limit classified as %s, want transient", got)
	}

	wrappedBlocked := fmt.Errorf("send failed: %w", &types.AccountSuspendedException{})
	if got := ClassifyFailure(wrappedBlocked); got != FailureAccountBlocked {
		t.Errorf("wrapped suspension classified as %s, want account blocked", got)
	}

	// The production error arrives wrapped twice over.
	wrappedQuota := fmt.Errorf("SES account %q rejected the message: %w", "prod",
		fmt.Errorf("api error: %w", &types.TooManyRequestsException{
			Message: strPtr("Daily message quota exceeded"),
		}))
	if got := ClassifyFailure(wrappedQuota); got != FailureAccountBlocked {
		t.Errorf("wrapped daily quota classified as %s, want account blocked", got)
	}
}

func TestClassifyFailure_CredentialErrorsByString(t *testing.T) {
	// These arrive as opaque signing errors rather than typed SES exceptions.
	// All are account-level: if the key is rejected once it is rejected for
	// every message, so the campaign should stop rather than work the list.
	for _, msg := range []string{
		"operation error SESv2: SendEmail, InvalidClientTokenId: The security token included in the request is invalid",
		"SignatureDoesNotMatch: Signature expired",
		"ExpiredToken: The security token included in the request is expired",
	} {
		if got := ClassifyFailure(errors.New(msg)); got != FailureAccountBlocked {
			t.Errorf("classified %q as %s, want account blocked", msg, got)
		}
	}

	// An unverified recipient is message-level, not account-level: other
	// recipients on the same account are unaffected, so this must not pause.
	unverified := "MessageRejected: Email address is not verified in region US-EAST-1"
	if got := ClassifyFailure(errors.New(unverified)); got != FailurePermanent {
		t.Errorf("classified %q as %s, want permanent", unverified, got)
	}
}

// TestClassifyFailure_RealProductionQuotaError uses the exact error string
// observed in production. SES reports a daily quota exhaustion through
// TooManyRequestsException -- the same type as per-second throttling -- so
// classifying on the type alone made the campaign retry for six minutes and
// then permanently fail every remaining recipient, for a condition that cannot
// clear until the 24-hour window rolls.
func TestClassifyFailure_RealProductionQuotaError(t *testing.T) {
	const observed = "SES API error: operation error SESv2: SendEmail, " +
		"exceeded maximum number of attempts, 3, https response error StatusCode: 429, " +
		"RequestID: c0638f08-89cd-4541-ab9f-7ff56af35fc6, " +
		"TooManyRequestsException: Daily message quota exceeded."

	if got := ClassifyFailure(errors.New(observed)); got != FailureAccountBlocked {
		t.Errorf("production quota error classified as %s, want account blocked", got)
	}
	if IsRetryable(errors.New(observed)) {
		t.Error("daily quota must not be retryable -- it cannot clear for hours")
	}
	if !IsAccountBlocked(errors.New(observed)) {
		t.Error("daily quota must mark the account blocked so the campaign pauses")
	}
}

func TestClassifyFailure_ThrottleVsQuotaSplit(t *testing.T) {
	// Same exception type, opposite correct responses. This is the distinction
	// the production incident exposed.
	//
	// The classifier matches the per-second rate case explicitly and treats
	// every other throttle as a volume block, so a reworded daily-quota message
	// fails safe toward pausing rather than burning the list.
	transient := []string{
		"Maximum sending rate exceeded",
		"Maximum sending rate exceeded. Please slow down.",
		"Sending rate exceeded",
	}
	for _, msg := range transient {
		err := &types.TooManyRequestsException{Message: strPtr(msg)}
		if got := ClassifyFailure(err); got != FailureTransient {
			t.Errorf("rate limit %q classified as %s, want transient", msg, got)
		}
	}

	blocked := []string{
		"Daily message quota exceeded",
		"Daily sending quota exceeded.",
		"You have reached your daily sending quota", // reworded -- the T1 case
		"Account is over its 24-hour sending limit",
		"Sending quota exceeded",                    // unknown phrasing, fails safe to blocked
	}
	for _, msg := range blocked {
		err := &types.TooManyRequestsException{Message: strPtr(msg)}
		if got := ClassifyFailure(err); got != FailureAccountBlocked {
			t.Errorf("volume cap %q classified as %s, want account blocked", msg, got)
		}
	}
}

func TestClassifyFailure_AccountLevelConditions(t *testing.T) {
	// Every one of these means the NEXT message fails identically, so working
	// through the list is pure waste. The campaign should stop.
	cases := []struct {
		name string
		err  error
	}{
		{"account suspended", &types.AccountSuspendedException{}},
		{"sending paused", &types.SendingPausedException{}},
		{"mail-from domain not verified", &types.MailFromDomainNotVerifiedException{}},
		{"invalid credentials", errors.New("InvalidClientTokenId: The security token is invalid")},
		{"signature mismatch", errors.New("SignatureDoesNotMatch: Signature expired")},
		{"expired token", errors.New("ExpiredToken: token has expired")},
		{"not authorized", errors.New("User is not authorized to perform ses:SendEmail")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyFailure(tc.err); got != FailureAccountBlocked {
				t.Errorf("got %s, want account blocked", got)
			}
			if IsRetryable(tc.err) {
				t.Error("account-level failures must not be retried")
			}
		})
	}
}

func TestClassifyFailure_MessageLevelStaysPermanent(t *testing.T) {
	// These are wrong for one message but fine for others, so they must NOT
	// pause the whole campaign.
	cases := []error{
		&types.BadRequestException{},
		&types.NotFoundException{},
		errors.New("MessageRejected: Email address is not verified"),
	}

	for _, err := range cases {
		if got := ClassifyFailure(err); got != FailurePermanent {
			t.Errorf("%T classified as %s, want permanent (message-level)", err, got)
		}
		if IsAccountBlocked(err) {
			t.Errorf("%T must not pause the campaign", err)
		}
	}
}

// TestClassifyFailure_TypedExceptionThroughWrappers proves the classifier
// reaches a typed SES exception through multiple wrapping layers via
// errors.As. In production the SDK wraps the concrete exception in
// OperationError -> MaxAttemptsError -> ResponseError, all of which implement
// Unwrap(); errors.As walks that chain. A regression here would silently drop
// classification to the fragile string-fallback path.
func TestClassifyFailure_TypedExceptionThroughWrappers(t *testing.T) {
	// Daily quota, wrapped twice.
	quota := fmt.Errorf("operation error SESv2: SendEmail, %w",
		fmt.Errorf("exceeded maximum number of attempts, 3: %w",
			&types.TooManyRequestsException{Message: strPtr("Daily message quota exceeded")}))
	if got := ClassifyFailure(quota); got != FailureAccountBlocked {
		t.Errorf("wrapped typed daily-quota classified as %s, want account blocked", got)
	}

	// Per-second rate, wrapped -- must stay retryable through the wrappers.
	rate := fmt.Errorf("send failed: %w",
		&types.TooManyRequestsException{Message: strPtr("Maximum sending rate exceeded")})
	if got := ClassifyFailure(rate); got != FailureTransient {
		t.Errorf("wrapped typed rate limit classified as %s, want transient", got)
	}

	// Suspension, wrapped.
	susp := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", &types.AccountSuspendedException{}))
	if got := ClassifyFailure(susp); got != FailureAccountBlocked {
		t.Errorf("wrapped suspension classified as %s, want account blocked", got)
	}
}

func TestFailureKind_StringsAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range []FailureKind{FailurePermanent, FailureTransient, FailureAccountBlocked} {
		s := k.String()
		if s == "" || seen[s] {
			t.Errorf("FailureKind %d has a missing or duplicate label: %q", k, s)
		}
		seen[s] = true
	}
}

func strPtr(s string) *string { return &s }

func TestClassifyFailure_UnknownDefaultsToTransient(t *testing.T) {
	// Deliberate: retrying something unrecoverable wastes a few attempts
	// against a capped budget, whereas discarding something recoverable loses
	// the recipient's mail with no record. Bias toward the cheaper mistake.
	if got := ClassifyFailure(errors.New("something nobody anticipated")); got != FailureTransient {
		t.Errorf("unknown error classified as %s, want transient", got)
	}
}

func TestClassifyFailure_NilIsPermanent(t *testing.T) {
	// A nil error means there is nothing to retry.
	if got := ClassifyFailure(nil); got != FailurePermanent {
		t.Errorf("got %s, want permanent", got)
	}
}

func TestIsRetryable_MatchesClassification(t *testing.T) {
	if !IsRetryable(&types.TooManyRequestsException{Message: strPtr("Maximum sending rate exceeded")}) {
		t.Error("per-second rate limit should be retryable")
	}
	if IsRetryable(&types.AccountSuspendedException{}) {
		t.Error("account suspension should not be retryable")
	}
}
