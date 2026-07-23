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

// A TooManyRequestsException from SES is always one of exactly two things: a
// per-second sending-rate limit, or a 24-hour volume cap. They need opposite
// responses -- a rate limit clears in seconds and is worth retrying; a daily
// cap does not clear until the window rolls, so retrying is pure waste and the
// campaign should pause. Only the message text tells them apart.
//
// Rather than enumerate the (open-ended, occasionally reworded) ways AWS phrases
// the daily cap, match the ONE thing that reliably marks the retryable case --
// a per-second rate limit -- and treat every other throttle as a volume block.
// That way a reworded daily-quota message ("you have reached your daily sending
// quota", localised text, etc.) fails safe toward pausing rather than burning
// the list.

// perSecondRateMarkers identify a throttle that is a per-second sending-rate
// limit, i.e. the retryable kind.
var perSecondRateMarkers = []string{
	"maximum sending rate",
	"sending rate exceeded",
	"maximum send rate",
	"rate exceeded",
	"per second",
}

// isPerSecondRateLimit reports whether a throttle message is a per-second rate
// limit (retryable) rather than a volume cap (account-blocked).
func isPerSecondRateLimit(msg string) bool {
	lower := strings.ToLower(msg)
	for _, marker := range perSecondRateMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// isDailyQuotaExhausted reports whether a message names a daily/volume quota
// directly. Used by the string-fallback path, where -- unlike inside a typed
// throttle exception -- there is no type context to lean on, so this stays
// specific to avoid misclassifying unrelated errors that merely say "quota".
func isDailyQuotaExhausted(msg string) bool {
	lower := strings.ToLower(msg)
	for _, marker := range []string{
		"daily message quota",
		"daily sending quota",
		"daily quota",
		"24-hour",
		"24 hour",
	} {
		if strings.Contains(lower, marker) {
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

	// --- Account-level: nothing on this campaign can send until it is fixed ---

	var accountSuspended *types.AccountSuspendedException
	var sendingPaused *types.SendingPausedException
	// An unverified MAIL FROM domain affects every message in the campaign --
	// they all share the same sender -- and it is operator-fixable in minutes
	// by verifying the domain. Writing off each recipient one by one would burn
	// the whole list for a condition a pause preserves it through.
	var mailFromNotVerified *types.MailFromDomainNotVerifiedException

	switch {
	case errors.As(err, &accountSuspended),
		errors.As(err, &sendingPaused),
		errors.As(err, &mailFromNotVerified):
		return FailureAccountBlocked
	}

	// Throttling splits two ways on the message text. A per-second rate limit
	// clears in seconds; a 24-hour quota does not clear until the window rolls,
	// and every remaining message in the campaign will hit it identically.
	var tooManyRequests *types.TooManyRequestsException
	var limitExceeded *types.LimitExceededException

	if errors.As(err, &tooManyRequests) || errors.As(err, &limitExceeded) {
		// Inside a typed throttle exception, anything that is NOT a per-second
		// rate limit is a volume cap -- fail safe toward pausing.
		if isPerSecondRateLimit(err.Error()) {
			return FailureTransient
		}
		return FailureAccountBlocked
	}

	// --- Message-level: this message is wrong, others may be fine ---

	var badRequest *types.BadRequestException
	var notFound *types.NotFoundException
	// MessageRejected means SES refused this specific message -- invalid
	// content, a virus, or bad personalization. Retrying the identical message
	// cannot help, so it must not sit in the retry loop burning three attempts.
	// It is message-level, not account-level: a different recipient's
	// personalised content may be fine, so the campaign keeps going.
	var messageRejected *types.MessageRejected

	switch {
	case errors.As(err, &badRequest),
		errors.As(err, &notFound),
		errors.As(err, &messageRejected):
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
			// A generic throttle code is usually a rate limit, so default to
			// transient -- but if it names a daily/volume quota, block.
			if !isPerSecondRateLimit(err.Error()) && isDailyQuotaExhausted(err.Error()) {
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

// IsCredentialFailure reports whether the error means the ACCOUNT itself is
// broken and will keep failing until an operator fixes it -- revoked or wrong
// credentials, an expired token, or a suspended/paused account.
//
// This is narrower than IsAccountBlocked on purpose. It deliberately EXCLUDES:
//   - daily-quota exhaustion, which is temporary and clears when the window
//     rolls -- marking the account "failed" for that would wrongly stop routing
//     to it for the rest of the day
//   - MailFromDomainNotVerified, which is a per-domain configuration problem,
//     not a broken account -- other domains on the account may send fine
//
// Used to decide whether to flip an account's stored status to "failed" so the
// UI stops showing it green and routing stops using it. Periodic
// re-verification restores it automatically once the credentials are fixed, so
// a transient AccessDenied that clears is self-healing within one verification
// interval.
func IsCredentialFailure(err error) bool {
	if err == nil {
		return false
	}

	var suspended *types.AccountSuspendedException
	var paused *types.SendingPausedException
	if errors.As(err, &suspended) || errors.As(err, &paused) {
		return true
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "InvalidClientTokenId", "SignatureDoesNotMatch", "UnrecognizedClientException",
			"InvalidSignatureException", "AccessDenied", "AccessDeniedException",
			"ExpiredTokenException", "MissingAuthenticationToken":
			return true
		}
	}

	msg := err.Error()
	for _, marker := range []string{
		"InvalidClientTokenId",
		"SignatureDoesNotMatch",
		"UnrecognizedClientException",
		"ExpiredToken",
		"not authorized to perform",
		"Account is paused",
		"account is suspended",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
