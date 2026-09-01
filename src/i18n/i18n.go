// Package i18n is mugcup's GUI-only translation catalog (tray menu, popup
// window, native dialogs). mugcup-cli deliberately has no equivalent — its
// own interface (help text, command output) stays English-only regardless
// of the GUI's language.
package i18n

import (
	"sync"

	"golang.org/x/sys/windows"
	"golang.org/x/text/language"
)

// Lang mirrors the values settings.Config.Language can hold ("auto", "en",
// "ko"). Kept as plain strings there — settings doesn't import this package,
// so the presentation layer stays independent of the config layer; main.go
// is what wires SetLang to config changes.
type Lang string

const (
	Auto Lang = "auto"
	EN   Lang = "en"
	KO   Lang = "ko"
)

// supported is every locale mugcup ships a catalog for, in the priority
// order language.NewMatcher falls back through — English first, so an
// unmatched OS language lands there.
var supported = []language.Tag{
	language.English,
	language.Korean,
}

var matcher = language.NewMatcher(supported)

var catalogs = map[language.Tag]map[string]string{
	language.English: messagesEN,
	language.Korean:  messagesKO,
}

var (
	mu     sync.RWMutex
	active = messagesEN
)

// SetLang resolves pref ("auto", "en", "ko", or anything unrecognized —
// treated the same as "auto") to one of the locales mugcup ships, and makes
// it active for T and Map. "auto" matches the OS's preferred UI language
// against `supported` via x/text/language's own BCP-47 negotiation (the same
// mechanism the Go toolchain's own i18n tooling is built on) rather than a
// hand-rolled prefix comparison.
func SetLang(pref string) {
	var tag language.Tag
	switch Lang(pref) {
	case EN:
		tag = language.English
	case KO:
		tag = language.Korean
	default: // "auto" or unrecognized
		tag = preferredOSTag()
	}
	_, index, _ := matcher.Match(tag)

	mu.Lock()
	active = catalogs[supported[index]]
	mu.Unlock()
}

// preferredOSTag reads the current user's preferred Windows UI language
// (e.g. "ko-KR") and parses it as a BCP-47 tag. Falls back to English on any
// failure (API unavailable, unparseable name, ...) rather than blocking
// startup on it.
func preferredOSTag() language.Tag {
	names, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil || len(names) == 0 {
		return language.English
	}
	tag, err := language.Parse(names[0])
	if err != nil {
		return language.English
	}
	return tag
}

// T looks up key in the active locale, falling back to English (every key
// is guaranteed present there) and finally the key itself, so a missing
// translation shows up as a visibly wrong string instead of a blank one.
func T(key string) string {
	mu.RLock()
	defer mu.RUnlock()
	if s, ok := active[key]; ok {
		return s
	}
	if s, ok := messagesEN[key]; ok {
		return s
	}
	return key
}

// Map returns a copy of the active locale's full catalog, for the Wails
// frontend (wailsapp.go's Translations bound method) to apply by key.
func Map() map[string]string {
	mu.RLock()
	defer mu.RUnlock()
	out := make(map[string]string, len(active))
	for k, v := range active {
		out[k] = v
	}
	return out
}
