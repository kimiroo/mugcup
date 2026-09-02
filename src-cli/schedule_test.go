package main

import (
	"testing"
	"time"
)

func TestParseUntilTarget(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		in   string
		want time.Time
	}{
		{"bare time HH:MM today", now.Add(time.Hour).Format("15:04"), time.Date(now.Year(), now.Month(), now.Day(), now.Add(time.Hour).Hour(), now.Add(time.Hour).Minute(), 0, 0, now.Location())},
		{"date and time, space-separated", "2099-06-15 18:00", time.Date(2099, 6, 15, 18, 0, 0, 0, now.Location())},
		{"date and time, T-separated", "2099-06-15T18:00", time.Date(2099, 6, 15, 18, 0, 0, 0, now.Location())},
		{"date and time with seconds", "2099-06-15 18:00:30", time.Date(2099, 6, 15, 18, 0, 30, 0, now.Location())},
		{"RFC3339 with offset", "2099-06-15T18:00:00+09:00", time.Date(2099, 6, 15, 18, 0, 0, 0, time.FixedZone("", 9*60*60))},
		{"RFC3339 with Z", "2099-06-15T18:00:00Z", time.Date(2099, 6, 15, 18, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUntilTarget(tt.in)
			if err != nil {
				t.Fatalf("parseUntilTarget(%q) error: %v", tt.in, err)
			}
			gotTarget := time.Now().Add(got)
			// Comparing durations directly would be flaky (time passes between
			// computing want and calling parseUntilTarget); compare against the
			// instant it should resolve to instead, with a generous tolerance.
			if diff := gotTarget.Sub(tt.want); diff < -5*time.Second || diff > 5*time.Second {
				t.Errorf("parseUntilTarget(%q) resolved to %v, want ~%v (diff %v)", tt.in, gotTarget, tt.want, diff)
			}
		})
	}
}

func TestParseUntilTargetInvalid(t *testing.T) {
	for _, in := range []string{"", "not-a-date", "25:99", "2099-13-01 18:00"} {
		if _, err := parseUntilTarget(in); err == nil {
			t.Errorf("parseUntilTarget(%q): expected an error, got none", in)
		}
	}
}
