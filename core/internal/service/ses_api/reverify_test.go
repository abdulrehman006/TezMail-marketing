package ses_api

import (
	"testing"
	"time"
)

func TestRecentlyVerified(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"empty means never verified", "", false},
		{"30s ago is recent", now.Add(-30 * time.Second).Format("2006-01-02 15:04:05"), true},
		{"90s ago is recent", now.Add(-90 * time.Second).Format("2006-01-02 15:04:05"), true},
		{"3 min ago is stale", now.Add(-3 * time.Minute).Format("2006-01-02 15:04:05"), false},
		{"an hour ago is stale", now.Add(-time.Hour).Format("2006-01-02 15:04:05"), false},
		{"unparseable is treated as stale", "not a timestamp", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := recentlyVerified(tc.value); got != tc.want {
				t.Errorf("recentlyVerified(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestRecentlyVerified_RFC3339(t *testing.T) {
	// The column may come back in RFC3339 depending on the driver, so both
	// layouts must parse.
	recent := time.Now().Add(-time.Minute).Format(time.RFC3339)
	if !recentlyVerified(recent) {
		t.Errorf("RFC3339 timestamp %q should be recent", recent)
	}
	old := time.Now().Add(-time.Hour).Format(time.RFC3339)
	if recentlyVerified(old) {
		t.Errorf("RFC3339 timestamp %q should be stale", old)
	}
}
