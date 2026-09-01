package settings

import (
	"testing"
)

func TestFormatDurationSec(t *testing.T) {
	tests := []struct {
		sec      int
		expected string
	}{
		{0, "Unlimited"},
		{-1, "Unlimited"},
		{900, "15m"},
		{1800, "30m"},
		{3600, "1h"},
		{5400, "1h 30m"},
		{7200, "2h"},
	}

	for _, tt := range tests {
		got := FormatDurationSec(tt.sec)
		if got != tt.expected {
			t.Errorf("FormatDurationSec(%d) = %q, want %q", tt.sec, got, tt.expected)
		}
	}
}

func TestControllerStateAndPresets(t *testing.T) {
	cfg := DefaultConfig()
	ctrl := NewController(cfg)

	if ctrl.State().Active {
		t.Errorf("Expected initial state inactive, got active")
	}

	ctrl.SetInfinite()
	st := ctrl.State()
	if !st.Active || !st.Infinite {
		t.Errorf("Expected Active=true, Infinite=true, got %+v", st)
	}

	ctrl.SetPreset(15 * 60)
	st = ctrl.State()
	if !st.Active || st.Infinite {
		t.Errorf("Expected Active=true, Infinite=false after preset, got %+v", st)
	}

	ctrl.ToggleInfinite()
	st = ctrl.State()
	if !st.Active || !st.Infinite {
		t.Errorf("Expected Active=true, Infinite=true after ToggleInfinite, got %+v", st)
	}

	ctrl.ToggleInfinite()
	st = ctrl.State()
	if st.Active {
		t.Errorf("Expected Active=false after ToggleInfinite on active infinite, got %+v", st)
	}
}

func TestValidateConfig(t *testing.T) {
	valid := DefaultConfig()
	if err := ValidateConfig(valid); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}

	invalidTimers := valid
	invalidTimers.TimerList = nil
	if err := ValidateConfig(invalidTimers); err == nil {
		t.Error("empty timer list should be rejected")
	}

	invalidAction := valid
	invalidAction.TrayClickAction = "unexpected"
	if err := ValidateConfig(invalidAction); err == nil {
		t.Error("unknown tray action should be rejected")
	}
}
