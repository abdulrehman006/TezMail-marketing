package ses_api

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"mime/quotedprintable"
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
		rawMessage := s.buildRawMessage(input)
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

	// Send email
	result, err := s.client.SendEmail(ctx, sendInput)
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

// buildRawMessage builds a raw MIME message with custom headers
func (s *SESSender) buildRawMessage(input *SendEmailInput) string {
	var builder strings.Builder

	// Add Message-ID header
	if input.MessageID != "" {
		builder.WriteString(fmt.Sprintf("Message-ID: %s\r\n", input.MessageID))
	}

	// Add From header
	builder.WriteString(fmt.Sprintf("From: %s\r\n", input.From))

	// Add To header
	builder.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(input.To, ", ")))

	// Add Subject header
	builder.WriteString(fmt.Sprintf("Subject: %s\r\n", encodeSubject(input.Subject)))

	// Add Date header
	builder.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))

	// Add MIME version
	builder.WriteString("MIME-Version: 1.0\r\n")

	// Add custom headers
	for key, value := range input.Headers {
		builder.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
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
	errStr := err.Error()

	// Check for credential errors
	if strings.Contains(errStr, "InvalidClientTokenId") ||
		strings.Contains(errStr, "SignatureDoesNotMatch") ||
		strings.Contains(errStr, "AccessDenied") {
		// Update account status to failed
		_ = UpdateAccountStatus(s.accountName, StatusFailed, errStr, nil, nil)
	}
}

// encodeSubject encodes subject for MIME (RFC 2047)
func encodeSubject(subject string) string {
	// Check if subject contains non-ASCII characters
	needsEncoding := false
	for _, r := range subject {
		if r > 127 {
			needsEncoding = true
			break
		}
	}

	if !needsEncoding {
		return subject
	}

	// Use UTF-8 encoding
	return fmt.Sprintf("=?UTF-8?B?%s?=", base64Encode(subject))
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
	g.Log().Warningf(ctx, "[SES-DEBUG] TrySendEmail called: sender=%s, recipients=%v, subject=%s", senderEmail, recipients, subject)
	account := GetAccountForDomain(senderEmail)
	if account == nil {
		g.Log().Warning(ctx, "[SES-DEBUG] TrySendEmail: GetAccountForDomain returned nil, no SES for", senderEmail)
		return false, nil // No SES configured, caller should use SMTP
	}
	g.Log().Warningf(ctx, "[SES-DEBUG] TrySendEmail: got account name=%s, region=%s, status=%s", account.Name, account.Region, account.Status)

	sesSender, _, sesErr := GetSenderForEmail(ctx, senderEmail)
	if sesErr != nil {
		g.Log().Warning(ctx, "[SES-DEBUG] TrySendEmail: Failed to create SES sender for", senderEmail, "error:", sesErr)
		return false, fmt.Errorf("SES account %q is mapped to this domain but its sender could not be created: %w", account.Name, sesErr)
	}
	g.Log().Warning(ctx, "[SES-DEBUG] TrySendEmail: SES sender created successfully")

	// Build From address with display name
	fromAddress := senderEmail
	if displayName != "" {
		fromAddress = fmt.Sprintf("%s <%s>", displayName, senderEmail)
	}

	input := &SendEmailInput{
		From:      fromAddress,
		To:        recipients,
		Subject:   subject,
		HtmlBody:  htmlBody,
		TextBody:  textBody,
		Headers:   extraHeaders,
		MessageID: messageID,
	}

	g.Log().Warning(ctx, "[SES-DEBUG] TrySendEmail: calling SendEmail now...")
	result := sesSender.SendEmail(ctx, input)
	if result.Success {
		g.Log().Warningf(ctx, "[SES-DEBUG] TrySendEmail: SUCCESS! Email sent via SES API to %v, MessageID=%s", recipients, result.MessageID)
		return true, nil
	}

	g.Log().Warningf(ctx, "[SES-DEBUG] TrySendEmail: SES send FAILED, error: %v", result.Error)
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

	sender, err := NewSESSender(account, accountName)
	if err != nil {
		return nil, "", err
	}

	return sender, accountName, nil
}
