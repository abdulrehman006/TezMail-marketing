package ses_api

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/gogf/gf/v2/frame/g"
)

// SESSender handles sending emails via Amazon SES API
type SESSender struct {
	client      *sesv2.Client
	accountName string
	region      string
}

// NewSESSender creates a new SES sender for the given account
func NewSESSender(account *AccountConfig, accountName string) (*SESSender, error) {
	if account == nil {
		return nil, fmt.Errorf("account config is nil")
	}

	if !HasValidCredentials(account) {
		return nil, fmt.Errorf("invalid credentials for account %s", accountName)
	}

	// Create AWS config with static credentials
	cfg := aws.Config{
		Region: account.Region,
		Credentials: credentials.NewStaticCredentialsProvider(
			account.AccessKey,
			account.SecretKey,
			"",
		),
	}

	client := sesv2.NewFromConfig(cfg)

	return &SESSender{
		client:      client,
		accountName: accountName,
		region:      account.Region,
	}, nil
}

// SendEmailInput contains all parameters for sending an email
type SendEmailInput struct {
	From             string
	To               []string
	Subject          string
	HtmlBody         string
	TextBody         string
	ReplyTo          []string
	Headers          map[string]string
	MessageID        string
}

// SendEmailOutput contains the result of sending an email
type SendEmailOutput struct {
	MessageID string
	Success   bool
	Error     error
}

// SendEmail sends an email via SES API
func (s *SESSender) SendEmail(ctx context.Context, input *SendEmailInput) *SendEmailOutput {
	output := &SendEmailOutput{
		Success: false,
	}

	// Build destination
	destination := &types.Destination{
		ToAddresses: input.To,
	}

	// Build email content
	var emailContent *types.EmailContent

	// Check if we need to send raw email (for custom headers)
	if len(input.Headers) > 0 || input.MessageID != "" {
		// Use raw email format for custom headers
		rawMessage := s.buildRawMessage(ctx, input)
		emailContent = &types.EmailContent{
			Raw: &types.RawMessage{
				Data: []byte(rawMessage),
			},
		}
	} else {
		// Use simple email format
		body := &types.Body{}

		if input.HtmlBody != "" {
			body.Html = &types.Content{
				Data:    aws.String(input.HtmlBody),
				Charset: aws.String("UTF-8"),
			}
		}

		if input.TextBody != "" {
			body.Text = &types.Content{
				Data:    aws.String(input.TextBody),
				Charset: aws.String("UTF-8"),
			}
		}

		emailContent = &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{
					Data:    aws.String(input.Subject),
					Charset: aws.String("UTF-8"),
				},
				Body: body,
			},
		}
	}

	// Build send email input
	sendInput := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(input.From),
		Destination:      destination,
		Content:          emailContent,
	}

	// Add reply-to addresses if provided
	if len(input.ReplyTo) > 0 {
		sendInput.ReplyToAddresses = input.ReplyTo
	}

	// Bound each send with its own deadline. The caller passes the long-lived
	// campaign context, which has no per-request timeout, so a half-open socket
	// or a stall mid-body could otherwise block a worker for a very long time
	// instead of failing fast and being retried. 30s is comfortably longer than
	// a healthy SendEmail round-trip.
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Send email
	result, err := s.client.SendEmail(sendCtx, sendInput)
	if err != nil {
		output.Error = fmt.Errorf("SES API error: %w", err)
		g.Log().Error(ctx, "SES SendEmail failed:", err)

		// Update account status on error
		s.handleSendError(ctx, err)

		return output
	}

	output.MessageID = aws.ToString(result.MessageId)
	output.Success = true

	g.Log().Debug(ctx, "SES email sent successfully, MessageID:", output.MessageID)

	return output
}

// reservedRawHeaders are the headers buildRawMessage emits itself, so a caller
// must not also supply them via SendEmailInput.Headers.
var reservedRawHeaders = map[string]bool{
	"message-id":                true,
	"from":                      true,
	"to":                        true,
	"subject":                   true,
	"date":                      true,
	"mime-version":              true,
	"content-type":              true,
	"content-transfer-encoding": true,
}

func isReservedRawHeader(key string) bool {
	return reservedRawHeaders[strings.ToLower(strings.TrimSpace(key))]
}

// writeHeader appends one header field, dropping it if the value cannot be
// safely represented.
//
// Dropping rather than passing through is the conservative choice: a header
// value carrying CR/LF would otherwise terminate its own field and inject
// whatever follows. Dropping a List-Unsubscribe for one malformed recipient is
// a far smaller problem than injecting a Bcc into everyone's mail. The drop is
// always logged so it is visible on a live server rather than silent.
//
// Values are not silently rewritten -- a stripped value would ship subtly
// wrong content to the whole list with nothing to indicate it happened.
func (s *SESSender) writeHeader(ctx context.Context, b *strings.Builder, key, value string) {
	if !headerValueIsSafe(key) || !headerValueIsSafe(value) {
		sesWarnf(ctx, "dropped header %q on a message from account %q: value contains CR, LF or NUL and cannot be represented (RFC 5322). This usually means contact data or a template needs cleaning.",
			key, s.accountName)
		return
	}
	b.WriteString(foldHeaderLine(key + ": " + value))
	b.WriteString("\r\n")
}

// buildRawMessage builds a raw MIME message with custom headers
func (s *SESSender) buildRawMessage(ctx context.Context, input *SendEmailInput) string {
	var builder strings.Builder

	// Add Message-ID header
	if input.MessageID != "" {
		s.writeHeader(ctx, &builder, "Message-ID", input.MessageID)
	}

	s.writeHeader(ctx, &builder, "From", input.From)
	s.writeHeader(ctx, &builder, "To", strings.Join(input.To, ", "))

	// encodeSubject cannot emit CR/LF, so this is safe by construction, but it
	// goes through the same path so nothing bypasses validation.
	s.writeHeader(ctx, &builder, "Subject", encodeSubject(input.Subject))
	s.writeHeader(ctx, &builder, "Date", time.Now().Format(time.RFC1123Z))

	builder.WriteString("MIME-Version: 1.0\r\n")

	// Add custom headers, skipping any that this function already emits.
	// buildRawMessage writes Message-ID, From, To, Subject, Date, MIME-Version
	// and Content-* itself; if a caller also passed one of those in Headers we
	// would emit it twice and produce a malformed message some receivers
	// reject. Today only List-Unsubscribe headers are passed, but the map is a
	// public field, so guard the surface rather than trust every caller.
	for key, value := range input.Headers {
		if isReservedRawHeader(key) {
			sesWarnf(ctx, "ignoring caller-supplied reserved header %q; it is set by the message builder", key)
			continue
		}
		s.writeHeader(ctx, &builder, key, value)
	}

	// Add X-Mailer
	builder.WriteString("X-Mailer: TezMail\r\n")

	// Add Content-Type and properly encoded body
	if input.HtmlBody != "" {
		builder.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		builder.WriteString("Content-Transfer-Encoding: base64\r\n")
		builder.WriteString("\r\n")
		builder.WriteString(encodeBase64Lines([]byte(input.HtmlBody)))
	} else {
		builder.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		builder.WriteString("Content-Transfer-Encoding: base64\r\n")
		builder.WriteString("\r\n")
		builder.WriteString(encodeBase64Lines([]byte(input.TextBody)))
	}

	return builder.String()
}

// handleSendError handles send errors and updates account status if needed
func (s *SESSender) handleSendError(ctx context.Context, err error) {
	// Only flag the account when the failure means the account itself is broken
	// -- revoked credentials, expired token, suspension. IsCredentialFailure
	// excludes daily-quota (temporary) and domain-not-verified (per-domain), so
	// those do not wrongly disable the whole account. The previous substring
	// check on "AccessDenied" alone was both too broad and DB-blind.
	if !IsCredentialFailure(err) {
		return
	}

	errStr := err.Error()

	// Legacy file-config path -- a no-op for DB-backed accounts, which is why
	// this used to silently do nothing for the standard setup.
	_ = UpdateAccountStatus(s.accountName, StatusFailed, errStr, nil, nil)

	// DB-backed path -- the standard case. Flips the stored status so the UI
	// stops showing the account green and routing stops using it; periodic
	// re-verification restores it once the credentials are fixed.
	_ = MarkDBAccountFailedByName(ctx, s.accountName, errStr)
}

// encodeSubject encodes a subject for use in a MIME header (RFC 2047).
//
// This delegates to the standard library rather than hand-rolling the check.
// The previous implementation only encoded when it saw a rune above 127, so CR
// and LF -- both well below that -- passed straight through into the header
// block and could inject arbitrary headers. Subjects are template-rendered
// with contact data, so that input is attacker-reachable.
//
// mime.QEncoding treats every octet below 0x20 as needing encoding, which
// neutralises CR/LF as a consequence of being correct rather than as a special
// case someone has to remember. It also folds long values into multiple
// encoded-words, which RFC 2047 requires (75 chars per word) and the old
// single-word version never did.
//
// This matches what the SMTP path has always done via Message.MailTitle.
func encodeSubject(subject string) string {
	return mime.QEncoding.Encode("UTF-8", subject)
}

// FormatFromAddress builds a From header value from a display name and an
// address, per RFC 5322 3.4.
//
// The previous fmt.Sprintf("%s <%s>", name, addr) broke on two ordinary
// inputs. A name containing a comma -- "Acme, Inc." -- parses as two separate
// addresses, and SESv2 validates FromEmailAddress strictly, so it returned
// InvalidParameterValue and failed *every* recipient of the campaign. A
// non-ASCII name emitted raw 8-bit bytes into the header.
//
// net/mail handles quoting and RFC 2047 encoding correctly, so this delegates
// rather than reimplementing the rules. An empty display name yields the bare
// address, which is what the SES API is given today.
func FormatFromAddress(displayName, email string) string {
	if displayName == "" {
		return email
	}
	addr := mail.Address{Name: displayName, Address: email}
	return addr.String()
}

// headerValueIsSafe reports whether v can appear in a header without
// terminating the field. RFC 5322 gives no way to escape CR or LF inside a
// field value, so a value containing them cannot be represented at all.
func headerValueIsSafe(v string) bool {
	return !strings.ContainsAny(v, "\r\n\x00")
}

// maxHeaderLineLen is the RFC 5322 2.1.1 "SHOULD" limit of 78 octets. The hard
// limit is 998; folding at 78 keeps us clear of both and matches what common
// MTAs emit.
const maxHeaderLineLen = 78

// foldHeaderLine folds a single "Key: Value" line so no line exceeds
// maxHeaderLineLen, per RFC 5322 2.2.3.
//
// mime.QEncoding.Encode emits a run of space-separated encoded-words but does
// not fold -- folding is the header writer's job. Without this, a long
// non-ASCII subject, or a long List-Unsubscribe URL alongside one, produces a
// single line that can breach the 998-octet hard limit and get the message
// rejected outright.
//
// Folding only ever happens at an existing space, which is a legal FWS point.
// A long run with no spaces (a bare URL) has no safe fold point, so it is left
// intact rather than corrupted -- receivers tolerate this in practice.
// A line already within the limit is returned untouched, so ordinary headers
// are byte-for-byte unchanged.
func foldHeaderLine(line string) string {
	if len(line) <= maxHeaderLineLen {
		return line
	}

	var out strings.Builder
	cur := 0 // length of the line being built

	for i, word := range strings.Split(line, " ") {
		switch {
		case i == 0:
			out.WriteString(word)
			cur = len(word)
		case cur+1+len(word) <= maxHeaderLineLen:
			out.WriteString(" ")
			out.WriteString(word)
			cur += 1 + len(word)
		default:
			// Fold: CRLF followed by a single space continues the field.
			out.WriteString("\r\n ")
			out.WriteString(word)
			cur = 1 + len(word)
		}
	}
	return out.String()
}

// base64Encode encodes string to base64 (single line, for headers)
func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// encodeBase64Lines encodes data to base64 with line wrapping at 76 chars (RFC 2045)
func encodeBase64Lines(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	var result strings.Builder
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		result.WriteString(encoded[i:end])
		result.WriteString("\r\n")
	}
	return result.String()
}

// qpEncode encodes content using quoted-printable encoding
func qpEncode(content string) string {
	var buf bytes.Buffer
	w := quotedprintable.NewWriter(&buf)
	w.Write([]byte(content))
	w.Close()
	return buf.String()
}

// TrySendEmail attempts to send email via SES API first (if configured for the sender domain),
// falling back to SMTP. Returns (sent via SES, error). If sent via SES is true, the caller
// should skip SMTP.
//
// The error return distinguishes two very different "not sent via SES" cases:
//   - (false, nil) — no SES account is mapped to this sender domain. Falling back to SMTP
//     is the expected, correct behaviour.
//   - (false, err) — SES *is* configured for this domain but the send failed (bad
//     credentials, unverified identity, sandbox restriction, malformed message...).
//     Callers that must not silently misreport delivery (e.g. the test-email endpoint)
//     should surface this instead of quietly falling back to Postfix.
//
// This is the single shared method all email sending paths should use.
func TrySendEmail(ctx context.Context, senderEmail string, recipients []string, subject, htmlBody, textBody, displayName, messageID string, extraHeaders map[string]string) (sentViaSES bool, err error) {
	sesTracef(ctx, "send requested from %s to %d recipient(s)", senderEmail, len(recipients))
	account := GetAccountForDomain(senderEmail)
	if account == nil {
		sesTracef(ctx, "no SES account for %s; caller will use SMTP", senderEmail)
		return false, nil // No SES configured, caller should use SMTP
	}
	sesTracef(ctx, "using account %q (region %s, status %s)", account.Name, account.Region, account.Status)

	sesSender, _, sesErr := GetSenderForEmail(ctx, senderEmail)
	if sesErr != nil {
		sesErrorf(ctx, "account %q is mapped to %s but its client could not be built: %v", account.Name, senderEmail, sesErr)
		return false, fmt.Errorf("SES account %q is mapped to this domain but its sender could not be created: %w", account.Name, sesErr)
	}
	

	// Build From address with display name
	fromAddress := FormatFromAddress(displayName, senderEmail)

	input := &SendEmailInput{
		From:      fromAddress,
		To:        recipients,
		Subject:   subject,
		HtmlBody:  htmlBody,
		TextBody:  textBody,
		Headers:   extraHeaders,
		MessageID: messageID,
	}

	
	result := sesSender.SendEmail(ctx, input)
	if result.Success {
		sesTracef(ctx, "sent via SES, MessageID=%s", result.MessageID)
		return true, nil
	}

	sesErrorf(ctx, "account %q (region %s) rejected a message to %d recipient(s): %v", account.Name, account.Region, len(recipients), result.Error)
	return false, fmt.Errorf("SES account %q (region %s) rejected the message: %w", account.Name, account.Region, result.Error)
}

// GetSenderForEmail returns a SES sender for the given email address
func GetSenderForEmail(ctx context.Context, senderEmail string) (*SESSender, string, error) {
	account := GetAccountForDomain(senderEmail)
	if account == nil {
		return nil, "", fmt.Errorf("no SES account configured for domain")
	}

	// Use account name from DB lookup (already populated by GetAccountForDomainFromDB)
	accountName := account.Name

	// Fall back to file config only if DB didn't provide a name
	if accountName == "" {
		config := GetConfig()
		if config != nil {
			domain := extractDomain(senderEmail)
			if name, ok := config.DomainMapping[domain]; ok {
				accountName = name
			} else if name, ok := config.DomainMapping["*"]; ok {
				accountName = name
			}
		}
	}

	// Shared, cached client. sesv2.Client is concurrency-safe and meant to be
	// long-lived; building one per recipient gave every send its own HTTP
	// transport (no TLS reuse) and its own retry budget.
	sender, err := cachedSender(account, accountName)
	if err != nil {
		return nil, "", err
	}

	return sender, accountName, nil
}
