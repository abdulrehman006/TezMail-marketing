package ses_api

import (
	"context"
	"strings"
	"testing"
)

// splitHeadersAndBody splits a raw MIME message at the first blank line, the
// same way a receiving MTA does. Anything the tests find in the header half
// that was not deliberately put there is an injection.
func splitHeadersAndBody(t *testing.T, raw string) (headers []string, body string) {
	t.Helper()
	idx := strings.Index(raw, "\r\n\r\n")
	if idx < 0 {
		t.Fatalf("raw message has no header/body separator:\n%q", raw)
	}
	return strings.Split(raw[:idx], "\r\n"), raw[idx+4:]
}

func newTestSender() *SESSender {
	// buildRawMessage does not touch the AWS client, so a bare struct is enough.
	return &SESSender{accountName: "test", region: "us-east-1"}
}

// ---------------------------------------------------------------------------
// Subject encoding
// ---------------------------------------------------------------------------

func TestEncodeSubject_NeutralisesCRLF(t *testing.T) {
	// A contact whose name contains CRLF reaches the subject through template
	// rendering. RFC 5322 has no escaping for CR/LF inside a field value, so the
	// only safe outcome is that they do not survive as raw control characters.
	cases := []struct {
		name    string
		subject string
	}{
		{"crlf then header", "Hello\r\nBcc: attacker@example.com"},
		{"bare lf", "Hello\nBcc: attacker@example.com"},
		{"bare cr", "Hello\rBcc: attacker@example.com"},
		{"trailing crlf", "Hello\r\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := encodeSubject(tc.subject)
			if strings.ContainsAny(got, "\r\n") {
				t.Errorf("encodeSubject leaked a raw control character.\n input: %q\noutput: %q", tc.subject, got)
			}
		})
	}
}

func TestEncodeSubject_PlainASCIIUnchanged(t *testing.T) {
	// Guard against over-encoding: ordinary subjects should stay readable so we
	// do not regress deliverability or make messages harder to debug.
	const s = "Your October newsletter"
	if got := encodeSubject(s); got != s {
		t.Errorf("plain ASCII subject was altered: got %q, want %q", got, s)
	}
}

func TestEncodeSubject_NonASCIIIsEncoded(t *testing.T) {
	got := encodeSubject("Café Newsletter")
	if strings.Contains(got, "Café") {
		t.Errorf("non-ASCII subject was not encoded: %q", got)
	}
	if !strings.Contains(got, "=?") {
		t.Errorf("expected an RFC 2047 encoded-word, got %q", got)
	}
}

func TestFoldHeaderLine_ShortLineUntouched(t *testing.T) {
	// The common case must be byte-for-byte unchanged, so ordinary mail keeps
	// exactly the wire format it has today.
	const line = "Subject: Your October newsletter"
	if got := foldHeaderLine(line); got != line {
		t.Errorf("short line was altered:\n got %q\nwant %q", got, line)
	}
}

func TestFoldHeaderLine_LongLineIsFolded(t *testing.T) {
	long := "Subject: " + strings.Repeat("word ", 60)
	got := foldHeaderLine(long)

	for _, l := range strings.Split(got, "\r\n") {
		if len(l) > maxHeaderLineLen {
			t.Errorf("folded line still exceeds %d octets: %d (%q)", maxHeaderLineLen, len(l), l)
		}
	}
	// Every continuation line must begin with whitespace, or it reads as a new
	// header field rather than a continuation.
	for _, l := range strings.Split(got, "\r\n")[1:] {
		if !strings.HasPrefix(l, " ") {
			t.Errorf("continuation line does not start with a space: %q", l)
		}
	}
}

func TestFoldHeaderLine_UnfoldableRunLeftIntact(t *testing.T) {
	// A long URL has no space to fold at. Corrupting it would break one-click
	// unsubscribe, so it must survive whole.
	url := "https://example.test/unsubscribe?jwt=" + strings.Repeat("x", 400)
	got := foldHeaderLine("List-Unsubscribe: <" + url + ">")
	if !strings.Contains(got, url) {
		t.Error("unfoldable URL was corrupted by folding")
	}
}

func TestBuildRawMessage_LongNonASCIISubjectStaysWithinLineLimit(t *testing.T) {
	// RFC 5322 2.1.1 caps a line at 998 octets. mime.QEncoding emits
	// space-separated encoded-words without folding, so the header writer has
	// to fold them or a long non-ASCII subject breaches the hard limit.
	s := newTestSender()
	raw := s.buildRawMessage(context.Background(), &SendEmailInput{
		From:     "sender@example.com",
		To:       []string{"rcpt@example.com"},
		Subject:  strings.Repeat("Görüşürüz ", 40),
		HtmlBody: "<p>body</p>",
	})

	headers, _ := splitHeadersAndBody(t, raw)
	for _, line := range headers {
		if len(line) > 998 {
			t.Errorf("header line exceeds RFC 5322 hard limit: %d octets", len(line))
		}
	}
}

// ---------------------------------------------------------------------------
// Raw message construction
// ---------------------------------------------------------------------------

func TestBuildRawMessage_SubjectCannotInjectHeaders(t *testing.T) {
	s := newTestSender()
	raw := s.buildRawMessage(context.Background(), &SendEmailInput{
		From:     "sender@example.com",
		To:       []string{"rcpt@example.com"},
		Subject:  "Hi\r\nBcc: attacker@example.com",
		HtmlBody: "<p>body</p>",
	})

	headers, _ := splitHeadersAndBody(t, raw)
	for _, h := range headers {
		if strings.HasPrefix(strings.ToLower(h), "bcc:") {
			t.Fatalf("subject injected a Bcc header. headers:\n%s", strings.Join(headers, "\n"))
		}
	}
}

func TestBuildRawMessage_CustomHeaderValueCannotInject(t *testing.T) {
	s := newTestSender()
	raw := s.buildRawMessage(context.Background(), &SendEmailInput{
		From:     "sender@example.com",
		To:       []string{"rcpt@example.com"},
		Subject:  "Normal subject",
		HtmlBody: "<p>body</p>",
		// List-Unsubscribe embeds the recipient address verbatim in production.
		Headers: map[string]string{
			"List-Unsubscribe": "<https://x.test/u?e=a@b.test>\r\nBcc: attacker@example.com",
		},
	})

	headers, _ := splitHeadersAndBody(t, raw)
	for _, h := range headers {
		if strings.HasPrefix(strings.ToLower(h), "bcc:") {
			t.Fatalf("custom header injected a Bcc header. headers:\n%s", strings.Join(headers, "\n"))
		}
	}
}

func TestBuildRawMessage_HasRequiredHeaders(t *testing.T) {
	s := newTestSender()
	raw := s.buildRawMessage(context.Background(), &SendEmailInput{
		From:      "sender@example.com",
		To:        []string{"rcpt@example.com"},
		Subject:   "Subject here",
		HtmlBody:  "<p>body</p>",
		MessageID: "<abc@example.com>",
	})

	headers, body := splitHeadersAndBody(t, raw)
	joined := strings.Join(headers, "\r\n")

	for _, want := range []string{"From:", "To:", "Subject:", "Date:", "MIME-Version:", "Message-ID:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing required header %q in:\n%s", want, joined)
		}
	}
	if strings.TrimSpace(body) == "" {
		t.Error("message body is empty")
	}
}

func TestBuildRawMessage_MultipleRecipientsCommaSeparated(t *testing.T) {
	s := newTestSender()
	raw := s.buildRawMessage(context.Background(), &SendEmailInput{
		From:     "sender@example.com",
		To:       []string{"a@example.com", "b@example.com"},
		Subject:  "Subject",
		HtmlBody: "<p>body</p>",
	})

	headers, _ := splitHeadersAndBody(t, raw)
	var to string
	for _, h := range headers {
		if strings.HasPrefix(h, "To:") {
			to = h
		}
	}
	if !strings.Contains(to, "a@example.com") || !strings.Contains(to, "b@example.com") {
		t.Errorf("To header lost a recipient: %q", to)
	}
}

// ---------------------------------------------------------------------------
// Body encoding
// ---------------------------------------------------------------------------

func TestEncodeBase64Lines_RespectsRFC2045LineLength(t *testing.T) {
	// RFC 2045 requires base64 lines of at most 76 characters.
	out := encodeBase64Lines([]byte(strings.Repeat("A", 1000)))
	for _, line := range strings.Split(strings.TrimRight(out, "\r\n"), "\r\n") {
		if len(line) > 76 {
			t.Errorf("base64 line exceeds 76 chars: %d", len(line))
		}
	}
}

func TestEncodeBase64Lines_RoundTripsEmptyInput(t *testing.T) {
	if got := encodeBase64Lines(nil); strings.TrimSpace(got) != "" {
		t.Errorf("empty input produced %q", got)
	}
}

// ---------------------------------------------------------------------------
// Domain extraction — drives which SES account a send is routed to
// ---------------------------------------------------------------------------

func TestExtractDomain(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"user@example.com", "example.com"},
		{"User@Example.COM", "example.com"},
		{"", ""},
		{"no-at-sign", ""},
		{"a@b@c", ""},
		{"user@", ""},
	}
	for _, tc := range cases {
		if got := extractDomain(tc.in); got != tc.want {
			t.Errorf("extractDomain(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
