package main

import (
	"fmt"
	"strings"
	"time"
)

// untilDateTimeLayouts are the full date+time forms "start until <target>"
// accepts, checked before untilTimeLayouts below since a full timestamp
// would otherwise misparse as a bare time-of-day. The RFC3339 forms carry
// their own timezone offset (e.g. "+09:00" or "Z"), so ParseInLocation's
// location argument only matters for the rest, which assume the CLI's own
// machine-local timezone; both "T" and a plain space are accepted as the
// separator there since a shell-unquoted "until 2026-01-02 18:00" arrives as
// two positional args that runStart rejoins with a single space, and typing
// a bare space is easier than remembering ISO 8601 wants a "T".
var untilDateTimeLayouts = []string{
	time.RFC3339,             // 2026-01-02T18:00:00+09:00 (or ...Z) — e.g. what `date -Iseconds` prints
	"2006-01-02T15:04Z07:00", // same, without seconds
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
}

// untilTimeLayouts are the bare time-of-day forms — not ISO 8601 (which has
// no "just a time, today" notion), but kept deliberately as the CLI
// equivalent of the GUI Custom view's "without date" toggle: the common
// case of "keep on until 18:00" shouldn't need a date typed out.
var untilTimeLayouts = []string{
	"15:04:05",
	"15:04",
}

// parseUntilTarget resolves "start until <target>" — target being a bare
// time-of-day (meaning today), a full date & time, or an RFC3339 timestamp —
// to how long from now that target is, mirroring the GUI Custom view's
// "until a date & time" tab (see startUntil/StartScheduleSeconds in
// app.js), just resolved here instead of in JS so the CLI needs no round
// trip to ask mugcup.exe what time it thinks it is. The result can be <= 0
// (target already passed); callers reject that themselves, same as the GUI
// does.
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
