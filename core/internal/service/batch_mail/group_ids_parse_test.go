package batch_mail

import (
	"testing"
)

// TestGetTaskGroupIds verifies the multi-list group_ids parser is safe and
// correct for every input shape it can receive from the database column.
func TestGetTaskGroupIds(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []int
	}{
		{"empty string (legacy row)", "", []int{}},
		{"single list", "[7]", []int{7}},
		{"multiple lists", "[1,2,3]", []int{1, 2, 3}},
		{"empty json array", "[]", []int{}},
		{"malformed json falls back to empty", "not-json", []int{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := GetTaskGroupIds(c.in)
			if got == nil {
				t.Fatalf("%s: returned nil (must be a non-nil empty slice)", c.name)
			}
			if len(got) != len(c.want) {
				t.Fatalf("%s: expected %v, got %v", c.name, c.want, got)
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Fatalf("%s: position %d expected %d, got %d", c.name, i, c.want[i], got[i])
				}
			}
		})
	}
}
