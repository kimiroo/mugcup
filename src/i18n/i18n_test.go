package i18n

import "testing"

// TestKeyParity catches a key added to one catalog and forgotten in the
// other — T's fallback-to-English would hide a missing Korean key silently,
// so this is the only thing that actually enforces both stay in sync.
func TestKeyParity(t *testing.T) {
	for k := range messagesEN {
		if _, ok := messagesKO[k]; !ok {
			t.Errorf("messagesKO is missing key %q (present in messagesEN)", k)
		}
	}
	for k := range messagesKO {
		if _, ok := messagesEN[k]; !ok {
			t.Errorf("messagesEN is missing key %q (present in messagesKO)", k)
		}
	}
}

func TestSetLangAndT(t *testing.T) {
	SetLang(string(EN))
	if got := T("tray.settings.label"); got != "Settings" {
		t.Errorf("T(tray.settings.label) with EN = %q, want %q", got, "Settings")
	}

	SetLang(string(KO))
	if got := T("tray.settings.label"); got != "설정" {
		t.Errorf("T(tray.settings.label) with KO = %q, want %q", got, "설정")
	}

	if got := T("no.such.key"); got != "no.such.key" {
		t.Errorf("T on a missing key = %q, want the key itself back", got)
	}
}
