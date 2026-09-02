package settings

import (
	"syscall"
	"time"
	"unsafe"
)

// GetDateFormatEx/GetTimeFormatEx render a date/time using the OS's current
// user locale (Windows Settings > Region) — e.g. "1/2/2026" for en-US,
// "2026-01-02" for a locale set to ISO order, whatever the user actually
// configured — rather than a format this app has to guess at. FormatUntil
// falls back to a fixed ISO-like layout if either call fails.
var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	procGetDateFormatEx = kernel32.NewProc("GetDateFormatEx")
	procGetTimeFormatEx = kernel32.NewProc("GetTimeFormatEx")
)

const (
	dateShortDate = 0x00000001 // DATE_SHORTDATE
	timeNoSeconds = 0x00000002 // TIME_NOSECONDS
)

// win32SystemTime mirrors Win32's SYSTEMTIME, which GetDateFormatEx and
// GetTimeFormatEx both take a pointer to instead of accepting a time value
// directly.
type win32SystemTime struct {
	Year, Month, DayOfWeek, Day, Hour, Minute, Second, Milliseconds uint16
}

func toSystemTime(t time.Time) win32SystemTime {
	t = t.Local()
	return win32SystemTime{
		Year:      uint16(t.Year()),
		Month:     uint16(t.Month()),
		DayOfWeek: uint16(t.Weekday()),
		Day:       uint16(t.Day()),
		Hour:      uint16(t.Hour()),
		Minute:    uint16(t.Minute()),
		Second:    uint16(t.Second()),
	}
}

// localDate renders t's date portion per the OS locale's short-date format
// (DATE_SHORTDATE). ok is false if the Windows API call fails.
func localDate(t time.Time) (string, bool) {
	st := toSystemTime(t)
	buf := make([]uint16, 64)
	r, _, _ := procGetDateFormatEx.Call(
		0, // lpLocaleName: NULL = LOCALE_NAME_USER_DEFAULT
		uintptr(dateShortDate),
		uintptr(unsafe.Pointer(&st)),
		0, // lpFormat: NULL, use dwFlags' predefined format
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		0, // lpCalendar: NULL
	)
	if r == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

// localTime renders t's time portion per the OS locale's time format, minus
// seconds (TIME_NOSECONDS). ok is false if the Windows API call fails.
func localTime(t time.Time) (string, bool) {
	st := toSystemTime(t)
	buf := make([]uint16, 64)
	r, _, _ := procGetTimeFormatEx.Call(
		0, // lpLocaleName: NULL = LOCALE_NAME_USER_DEFAULT
		uintptr(timeNoSeconds),
		uintptr(unsafe.Pointer(&st)),
		0, // lpFormat: NULL, use dwFlags' predefined format
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if r == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}
