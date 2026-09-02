package main

import (
	"fmt"
	"strings"
	"time"
)

// untilDateTimeLayouts are the accepted full date+time forms, checked before
// untilTimeLayouts below so a full timestamp doesn't misparse as a bare
// time. RFC3339 carries its own timezone offset; the rest assume local
// time and take "T" or a plain space as the separator (an unquoted "until
// 2026-01-02 18:00" arrives as two args and gets rejoined with a space).
var untilDateTimeLayouts = []string{
	time.RFC3339,             // 2026-01-02T18:00:00+09:00 (or ...Z) — e.g. `date -Iseconds`
	"2006-01-02T15:04Z07:00", // same, without seconds
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
}

// untilTimeLayouts are bare time-of-day forms (today) — not ISO 8601, but
// the CLI equivalent of the GUI Custom view's "without date" toggle.
var untilTimeLayouts = []string{
	"15:04:05",
	"15:04",
}

// parseUntilTarget resolves "start until <target>" to how long from now that
// target is — target being a bare time-of-day (today), a date & time, or an
// RFC3339 timestamp. Mirrors the GUI Custom view's "until a date & time" tab
// (startUntil/StartScheduleSeconds in app.js), just resolved CLI-side so
// mugcup.exe doesn't need to be asked what time it is. May return <= 0 for a
// target already in the past; callers reject that themselves.
func parseUntilTarget(raw string) (time.Duration, error) {
	s := strings.TrimSpace(raw)
	now := time.Now()
	for _, layout := range untilDateTimeLayouts {
		if t, err := time.ParseInLocation(layout, s, now.Location()); err == nil {
			return t.Sub(now), nil
		}
	}
	for _, layout := range untilTimeLayouts {
		if t, err := time.ParseInLocation(layout, s, now.Location()); err == nil {
			target := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, now.Location())
			return target.Sub(now), nil
		}
	}
	return 0, fmt.Errorf("unrecognized date/time (try 18:00, 2026-01-02 18:00, or an RFC3339 timestamp): %s", s)
}
