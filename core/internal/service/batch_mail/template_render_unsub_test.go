package batch_mail

import (
	"billionmail-core/internal/model/entity"
	"context"
	"strings"
	"testing"
)

// TestRenderEmailTemplate_ResolvesUnsubscribeWithoutContact verifies the fix for
// the "Send test mail" raw-placeholder bug: rendering must resolve
// {{ .UnsubscribeURL }} even when there is NO contact — the common case for test
// sends to externally-hosted recipients that are not stored contacts.
func TestRenderEmailTemplate_ResolvesUnsubscribeWithoutContact(t *testing.T) {
	engine := GetTemplateEngine()
	url := "https://panel.example.com/unsubscribe.html?jwt=abc&email=x@y.z"

	body := `<p>Hello</p><a href="{{ .UnsubscribeURL }}">Unsubscribe</a>`
	out, err := engine.RenderEmailTemplate(context.Background(), body, nil, nil, url)
	if err != nil {
		t.Fatalf("render returned error with nil contact: %v", err)
	}
	if strings.Contains(out, "{{ .UnsubscribeURL }}") {
		t.Fatalf("unsubscribe placeholder NOT resolved with nil contact; got: %q", out)
	}
	if !strings.Contains(out, url) {
		t.Fatalf("expected unsubscribe URL %q in output; got: %q", url, out)
	}
}

// TestRenderEmailTemplate_ResolvesUnsubscribeWithContact is the regression guard:
// the pre-existing known-contact path must still resolve UnsubscribeURL.
func TestRenderEmailTemplate_ResolvesUnsubscribeWithContact(t *testing.T) {
	engine := GetTemplateEngine()
	url := "https://panel.example.com/unsubscribe.html?jwt=def"

	c := &entity.Contact{Id: 1, Email: "user@example.com"}
	out, err := engine.RenderEmailTemplate(context.Background(), `<a href="{{ .UnsubscribeURL }}">u</a>`, c, nil, url)
	if err != nil {
		t.Fatalf("render returned error with contact: %v", err)
	}
	if strings.Contains(out, "{{ .UnsubscribeURL }}") || !strings.Contains(out, url) {
		t.Fatalf("unsubscribe URL not resolved with contact; got: %q", out)
	}
}

// TestRenderEmailTemplate_NilContactDoesNotPanicOnMergeTags ensures a template
// that references subscriber merge tags does not panic when rendered without a
// contact (worst case it falls back to best-effort content, never a crash).
func TestRenderEmailTemplate_NilContactDoesNotPanicOnMergeTags(t *testing.T) {
	engine := GetTemplateEngine()
	url := "https://panel.example.com/unsubscribe.html"

	body := `Hi {{ .Subscriber.Email }} <a href="{{ .UnsubscribeURL }}">u</a>`
	out, err := engine.RenderEmailTemplate(context.Background(), body, nil, nil, url)
	if err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	if out == "" {
		t.Fatalf("render produced empty output")
	}
}
