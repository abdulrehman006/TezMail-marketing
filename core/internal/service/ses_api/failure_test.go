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
	// Retrying these cannot help -- an operator has to change something. Doing
	// so would burn send quota during an incident for no benefit.
	cases := []struct {
		name string
		err  error
	}{
		{"account suspended", &types.AccountSuspendedException{}},
		{"sending paused", &types.SendingPausedException{}},
		{"mail-from domain not verified", &types.MailFromDomainNotVerifiedException{}},
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
		{"throttling", &types.TooManyRequestsException{}},
		{"limit exceeded", &types.LimitExceededException{}},
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
		"prod", fmt.Errorf("api error: %w", &types.TooManyRequestsException{}))

	if got := ClassifyFailure(wrapped); got != FailureTransient {
		t.Errorf("wrapped throttling classified as %s, want transient", got)
	}

	wrappedPermanent := fmt.Errorf("send failed: %w", &types.AccountSuspendedException{})
	if got := ClassifyFailure(wrappedPermanent); got != FailurePermanent {
		t.Errorf("wrapped suspension classified as %s, want permanent", got)
	}
}

func TestClassifyFailure_CredentialErrorsByString(t *testing.T) {
	// These arrive as opaque signing errors rather than typed SES exceptions.
	for _, msg := range []string{
		"operation error SESv2: SendEmail, InvalidClientTokenId: The security token included in the request is invalid",
		"SignatureDoesNotMatch: Signature expired",
		"ExpiredToken: The security token included in the request is expired",
		"MessageRejected: Email address is not verified in region US-EAST-1",
	} {
		if got := ClassifyFailure(errors.New(msg)); got != FailurePermanent {
			t.Errorf("classified %q as %s, want permanent", msg, got)
		}
	}
}

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
	if !IsRetryable(&types.TooManyRequestsException{}) {
		t.Error("throttling should be retryable")
	}
	if IsRetryable(&types.AccountSuspendedException{}) {
		t.Error("account suspension should not be retryable")
	}
}
