package mail_service

import (
	"strings"
	"testing"
)

// The API send endpoints used to construct a full authenticated EmailSender --
// which needs a mailbox row with a decodable password -- purely to call
// GenerateMessageID, then closed it without sending. For a domain configured
// only for SES there is no local mailbox, so that failed before the message was
// ever queued, even though the send itself would have gone out over SES fine.
//
// Replacing it with NewMessageID removes the connection requirement. These
// tests establish that the substitution is behaviourally identical, since that
// is the only part of the change that could silently corrupt data.

func TestNewMessageID_EquivalentToGenerateMessageID(t *testing.T) {
	const addr = "sender@example.com"

	// GenerateMessageID delegates to NewMessageID using the sender's own Email
	// field, and NewEmailSenderWithLocal sets that field to the addresser. So
	// NewMessageID(addresser) must produce the same shape.
	viaSender := (&EmailSender{Email: addr}).GenerateMessageID()
	viaDirect := NewMessageID(addr)

	for name, id := range map[string]string{"via sender": viaSender, "direct": viaDirect} {
		if !strings.HasPrefix(id, "<") || !strings.HasSuffix(id, ">") {
			t.Errorf("%s: not angle-bracketed: %q", name, id)
		}
		if !strings.HasSuffix(id, "@example.com>") {
			t.Errorf("%s: wrong domain part: %q", name, id)
		}
		if strings.Count(id, "@") != 1 {
			t.Errorf("%s: malformed, expected exactly one @: %q", name, id)
		}
	}

	// Same structure: <millis>.<hex>@<domain>
	shape := func(id string) (parts int, domain string) {
		body := strings.TrimSuffix(strings.TrimPrefix(id, "<"), ">")
		at := strings.LastIndex(body, "@")
		return strings.Count(body[:at], ".") + 1, body[at+1:]
	}
	sParts, sDomain := shape(viaSender)
	dParts, dDomain := shape(viaDirect)
	if sParts != dParts || sDomain != dDomain {
		t.Errorf("shapes differ: sender=(%d,%s) direct=(%d,%s)", sParts, sDomain, dParts, dDomain)
	}
}

func TestNewMessageID_IsUnique(t *testing.T) {
	// Message-IDs key the statistics tables; a collision loses a row.
	seen := make(map[string]bool, 2000)
	for i := 0; i < 2000; i++ {
		id := NewMessageID("sender@example.com")
		if seen[id] {
			t.Fatalf("duplicate Message-ID: %q", id)
		}
		seen[id] = true
	}
}

func TestNewMessageID_DomainHandling(t *testing.T) {
	cases := []struct {
		in         string
		wantSuffix string
	}{
		{"user@example.com", "@example.com>"},
		{"user@sub.example.co.uk", "@sub.example.co.uk>"},
		{"no-at-sign", "@billionmail>"},   // fallback
		{"", "@billionmail>"},             // fallback
		{"user@", "@>"},                   // degenerate but must not panic
	}

	for _, tc := range cases {
		got := NewMessageID(tc.in)
		if !strings.HasSuffix(got, tc.wantSuffix) {
			t.Errorf("NewMessageID(%q) = %q, want suffix %q", tc.in, got, tc.wantSuffix)
		}
	}
}

func TestNewMessageID_SurvivesTrimForStorage(t *testing.T) {
	// The API path stores the ID with the angle brackets stripped, and the send
	// path adds them back. Verify the round trip is lossless, since a mismatch
	// would break the join between recipient_info and the mailstat tables.
	original := NewMessageID("sender@example.com")

	stored := strings.Trim(original, "<>")
	if strings.ContainsAny(stored, "<>") {
		t.Errorf("stored form still contains brackets: %q", stored)
	}
	if stored == "" {
		t.Fatal("stored form is empty")
	}

	restored := "<" + stored + ">"
	if restored != original {
		t.Errorf("round trip changed the ID:\n original %q\n restored %q", original, restored)
	}
}

func TestNewMessageID_NoConnectionRequired(t *testing.T) {
	// The whole point: this must work without a mailbox, a password, or a
	// reachable Postfix. If it ever starts needing those, SES-only domains
	// break again.
	if id := NewMessageID("ses-only@example.com"); id == "" {
		t.Error("NewMessageID returned empty without a configured sender")
	}
}
