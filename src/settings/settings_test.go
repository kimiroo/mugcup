package settings

import (
	"testing"
	"time"
)

func TestFormatDurationSec(t *testing.T) {
	tests := []struct {
		sec      int
		expected string
	}{
		{0, "Indefinite"},
		{-1, "Indefinite"},
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

	ctrl.SetIndefinite()
	st := ctrl.State()
	if !st.Active || !st.Indefinite {
		t.Errorf("Expected Active=true, Indefinite=true, got %+v", st)
	}

	ctrl.SetPreset(15 * 60)
	st = ctrl.State()
	if !st.Active || st.Indefinite {
		t.Errorf("Expected Active=true, Indefinite=false after preset, got %+v", st)
	}

	ctrl.ToggleIndefinite()
	st = ctrl.State()
	if !st.Active || !st.Indefinite {
		t.Errorf("Expected Active=true, Indefinite=true after ToggleIndefinite, got %+v", st)
	}

	ctrl.ToggleIndefinite()
	st = ctrl.State()
	if st.Active {
		t.Errorf("Expected Active=false after ToggleIndefinite on active indefinite, got %+v", st)
	}
}

func TestControllerMode(t *testing.T) {
	cfg := DefaultConfig() // TimerList: 15m, 30m, 1h, 2h, Indefinite
	ctrl := NewController(cfg)

	if got := ctrl.State().Mode; got != ModeOff {
		t.Errorf("Expected initial Mode=%q, got %q", ModeOff, got)
	}

	ctrl.SetIndefinite()
	if got := ctrl.State().Mode; got != ModeIndefinite {
		t.Errorf("SetIndefinite: expected Mode=%q, got %q", ModeIndefinite, got)
	}

	ctrl.SetPreset(15 * 60)
	if got := ctrl.State().Mode; got != ModeTimer {
		t.Errorf("SetPreset(timed): expected Mode=%q, got %q", ModeTimer, got)
	}

	ctrl.SetPreset(0)
	if got := ctrl.State().Mode; got != ModeIndefinite {
		t.Errorf("SetPreset(indefinite): expected Mode=%q, got %q", ModeIndefinite, got)
	}

	if err := ctrl.SetCustomDuration(10 * time.Minute); err != nil {
		t.Fatalf("SetCustomDuration: %v", err)
	}
	if got := ctrl.State().Mode; got != ModeTimer {
		t.Errorf("SetCustomDuration: expected Mode=%q, got %q", ModeTimer, got)
	}

	if err := ctrl.SetSchedule(10 * time.Minute); err != nil {
		t.Fatalf("SetSchedule: %v", err)
	}
	if got := ctrl.State().Mode; got != ModeSchedule {
		t.Errorf("SetSchedule: expected Mode=%q, got %q", ModeSchedule, got)
	}

	ctrl.TurnOff()
	if got := ctrl.State().Mode; got != ModeOff {
		t.Errorf("TurnOff: expected Mode=%q, got %q", ModeOff, got)
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
