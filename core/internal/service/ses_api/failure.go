package ses_api

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/aws/smithy-go"
)

// FailureKind says whether retrying a failed send could plausibly succeed.
//
// Everything used to be treated identically: one attempt, then the recipient
// was marked done and dropped. That loses mail on a transient blip, and it
// also wastes quota re-attempting things that can never work.
type FailureKind int

const (
	// FailurePermanent means retrying cannot help for THIS message. Something
	// about the recipient or the message itself has to change. Other messages
	// on the same account may still succeed.
	FailurePermanent FailureKind = iota

	// FailureTransient means the same request could succeed shortly --
	// per-second throttling, a network problem, or a service-side error.
	FailureTransient

	// FailureAccountBlocked means the whole account cannot send right now, so
	// every remaining message will fail identically. Daily quota exhausted,
	// credentials rejected, account suspended, sending paused.
	//
	// This is the class that must NOT be retried and must NOT be discarded.
	// Retrying wastes attempts on something that cannot clear for hours;
	// discarding burns the entire remaining list for a condition an operator
	// can fix in minutes. The campaign should stop and wait instead.
	FailureAccountBlocked
)

func (k FailureKind) String() string {
	switch k {
	case FailureTransient:
		return "transient"
	case FailureAccountBlocked:
		return "account blocked"
	default:
		return "permanent"
	}
}

// dailyQuotaMarkers identify a TooManyRequestsException that is a 24-hour
// sending-limit exhaustion rather than per-second throttling.
//
// SES reports both through the same exception type, but they need opposite
// responses: a rate limit clears in seconds and is worth retrying, whereas a
// daily quota does not clear until the 24-hour window rolls and retrying is
// pure waste. Only the message text distinguishes them.
var dailyQuotaMarkers = []string{
	"Daily message quota exceeded",
	"Daily sending quota exceeded",
	"quota exceeded",
}

func isDailyQuotaExhausted(msg string) bool {
	lower := strings.ToLower(msg)
	for _, marker := range dailyQuotaMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

// ClassifyFailure decides whether a SES send failure is worth retrying.
//
// Typed API errors are checked first, since they are unambiguous. Only if none
// match does it fall back to string matching, which is a last resort -- the
// existing handleSendError does substring matching on the formatted error and
// that is exactly how a transient regional problem gets misread as a dead
// credential.
//
// Unknown errors are treated as TRANSIENT. Retrying something unrecoverable
// costs a few wasted attempts against a capped budget; discarding something
// recoverable loses a recipient's mail permanently with no record. The former
// is much the cheaper mistake.
func ClassifyFailure(err error) FailureKind {
	if err == nil {
		return FailurePermanent
	}

	// --- Account-level: nothing on this account can send until it is fixed ---

	var accountSuspended *types.AccountSuspendedException
	var sendingPaused *types.SendingPausedException

	switch {
	case errors.As(err, &accountSuspended), errors.As(err, &sendingPaused):
		return FailureAccountBlocked
	}

	// Throttling splits two ways on the message text. A per-second rate limit
	// clears in seconds; a 24-hour quota does not clear until the window rolls,
	// and every remaining message in the campaign will hit it identically.
	var tooManyRequests *types.TooManyRequestsException
	var limitExceeded *types.LimitExceededException

	if errors.As(err, &tooManyRequests) || errors.As(err, &limitExceeded) {
		if isDailyQuotaExhausted(err.Error()) {
			return FailureAccountBlocked
		}
		return FailureTransient
	}

	// --- Message-level: this message is wrong, others may be fine ---

	var mailFromNotVerified *types.MailFromDomainNotVerifiedException
	var badRequest *types.BadRequestException
	var notFound *types.NotFoundException

	switch {
	case errors.As(err, &mailFromNotVerified),
		errors.As(err, &badRequest),
		errors.As(err, &notFound):
		return FailurePermanent
	}

	// --- Worth retrying: contention, service-side failure ---

	var internalError *types.InternalServiceErrorException
	var concurrentMod *types.ConcurrentModificationException

	switch {
	case errors.As(err, &internalError), errors.As(err, &concurrentMod):
		return FailureTransient
	}

	// Network-level trouble: unreachable, refused, DNS, TLS, reset.
	var netErr net.Error
	if errors.As(err, &netErr) {
		return FailureTransient
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return FailureTransient
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return FailureTransient
	}

	// A cancelled campaign is not a send failure to retry.
	if errors.Is(err, context.Canceled) {
		return FailurePermanent
	}

	// Credential and authorisation problems block the whole account: if the
	// key is rejected for one message it is rejected for all of them, so
	// continuing through the list only produces thousands of identical errors.
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "InvalidClientTokenId", "SignatureDoesNotMatch", "UnrecognizedClientException",
			"InvalidSignatureException", "AccessDenied", "AccessDeniedException",
			"ExpiredTokenException", "MissingAuthenticationToken":
			return FailureAccountBlocked
		case "ThrottlingException", "Throttling", "RequestThrottled":
			if isDailyQuotaExhausted(err.Error()) {
				return FailureAccountBlocked
			}
			return FailureTransient
		case "RequestTimeout", "RequestTimeoutException", "ServiceUnavailable",
			"InternalFailure", "InternalError", "ServiceFailure":
			return FailureTransient
		}
	}

	// Last resort, for errors that arrive as opaque strings rather than typed
	// exceptions. Kept narrow and anchored on codes that appear only in the
	// condition being matched.
	msg := err.Error()

	if isDailyQuotaExhausted(msg) {
		return FailureAccountBlocked
	}

	for _, blocked := range []string{
		"InvalidClientTokenId",
		"SignatureDoesNotMatch",
		"UnrecognizedClientException",
		"ExpiredToken",
		"not authorized to perform",
		"Account is paused",
		"account is suspended",
	} {
		if strings.Contains(msg, blocked) {
			return FailureAccountBlocked
		}
	}

	// Message-level: wrong for this recipient, fine for others.
	if strings.Contains(msg, "Email address is not verified") {
		return FailurePermanent
	}

	// Unknown: prefer retrying over silently losing the recipient.
	return FailureTransient
}

// IsRetryable is a convenience wrapper for callers that only need the boolean.
//
// Account-blocked failures are deliberately NOT retryable: the campaign should
// stop rather than work through the list producing identical errors.
func IsRetryable(err error) bool {
	return ClassifyFailure(err) == FailureTransient
}

// IsAccountBlocked reports whether the failure means the whole SES account
// cannot send, so the campaign should pause rather than continue.
func IsAccountBlocked(err error) bool {
	return ClassifyFailure(err) == FailureAccountBlocked
}
