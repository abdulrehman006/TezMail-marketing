package batch_mail

import (
	"billionmail-core/internal/model/entity"
	"testing"
)

// TestDedupeContactsByEmail is the core guarantee of the multi-list feature:
// a contact present in several selected lists is kept exactly once.
func TestDedupeContactsByEmail(t *testing.T) {
	in := []*entity.Contact{
		{Id: 1, Email: "a@example.com"},   // list A
		{Id: 2, Email: "b@example.com"},   // list A
		{Id: 3, Email: "A@Example.com"},   // list B — same as #1 (case-insensitive)
		{Id: 4, Email: " b@example.com "}, // list B — same as #2 (trimmed)
		{Id: 5, Email: "c@example.com"},   // list B — new
		nil,                               // defensive
		{Id: 6, Email: ""},                // blank — skipped
		{Id: 7, Email: "  "},              // whitespace — skipped
		{Id: 8, Email: "c@example.com"},   // dup of #5
	}

	out := dedupeContactsByEmail(in)

	if len(out) != 3 {
		t.Fatalf("expected 3 unique contacts, got %d: %+v", len(out), out)
	}

	// Order preserved, first-seen wins.
	wantIds := []int{1, 2, 5}
	for i, want := range wantIds {
		if out[i].Id != want {
			t.Fatalf("position %d: expected contact id %d, got %d", i, want, out[i].Id)
		}
	}

	// No blank/whitespace/nil leaked through.
	for _, c := range out {
		if c == nil || c.Email == "" {
			t.Fatalf("blank or nil contact leaked into result: %+v", c)
		}
	}
}

// TestDedupeContactsByEmail_Empty and single-list pass-through (no regression).
func TestDedupeContactsByEmail_PassThrough(t *testing.T) {
	if got := dedupeContactsByEmail(nil); len(got) != 0 {
		t.Fatalf("nil input should yield empty, got %d", len(got))
	}
	single := []*entity.Contact{{Id: 1, Email: "x@y.z"}, {Id: 2, Email: "p@q.r"}}
	got := dedupeContactsByEmail(single)
	if len(got) != 2 || got[0].Id != 1 || got[1].Id != 2 {
		t.Fatalf("single-list pass-through altered order/count: %+v", got)
	}
}
