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
	// FailurePermanent means retrying cannot help. The configuration, the
	// credentials or the message itself has to change first.
	FailurePermanent FailureKind = iota

	// FailureTransient means the same request could succeed later --
	// throttling, a network problem, or a service-side error.
	FailureTransient
)

func (k FailureKind) String() string {
	if k == FailureTransient {
		return "transient"
	}
	return "permanent"
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

	// --- Definitely not worth retrying: configuration or content is wrong ---

	var accountSuspended *types.AccountSuspendedException
	var sendingPaused *types.SendingPausedException
	var mailFromNotVerified *types.MailFromDomainNotVerifiedException
	var badRequest *types.BadRequestException
	var notFound *types.NotFoundException

	switch {
	case errors.As(err, &accountSuspended),
		errors.As(err, &sendingPaused),
		errors.As(err, &mailFromNotVerified),
		errors.As(err, &badRequest),
		errors.As(err, &notFound):
		return FailurePermanent
	}

	// --- Worth retrying: throttling, contention, service-side failure ---

	var tooManyRequests *types.TooManyRequestsException
	var limitExceeded *types.LimitExceededException
	var internalError *types.InternalServiceErrorException
	var concurrentMod *types.ConcurrentModificationException

	switch {
	case errors.As(err, &tooManyRequests),
		errors.As(err, &limitExceeded),
		errors.As(err, &internalError),
		errors.As(err, &concurrentMod):
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

	// Credential and authorisation problems. These are permanent in the sense
	// that retrying this message will not help -- an operator has to fix the
	// account -- so there is no point burning quota on them.
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "InvalidClientTokenId", "SignatureDoesNotMatch", "UnrecognizedClientException",
			"InvalidSignatureException", "AccessDenied", "AccessDeniedException",
			"ExpiredTokenException", "MissingAuthenticationToken":
			return FailurePermanent
		case "ThrottlingException", "Throttling", "RequestThrottled",
			"RequestTimeout", "RequestTimeoutException", "ServiceUnavailable",
			"InternalFailure", "InternalError", "ServiceFailure":
			return FailureTransient
		}
	}

	// Last resort. Kept narrow and anchored on codes that only appear in
	// authentication failures, so a message merely mentioning one of these
	// words is unlikely to be misfiled.
	msg := err.Error()
	for _, permanent := range []string{
		"InvalidClientTokenId",
		"SignatureDoesNotMatch",
		"UnrecognizedClientException",
		"ExpiredToken",
		"Email address is not verified",
		"not authorized to perform",
	} {
		if strings.Contains(msg, permanent) {
			return FailurePermanent
		}
	}

	// Unknown: prefer retrying over silently losing the recipient.
	return FailureTransient
}

// IsRetryable is a convenience wrapper for callers that only need the boolean.
func IsRetryable(err error) bool {
	return ClassifyFailure(err) == FailureTransient
}
