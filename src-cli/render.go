package main

import (
	"fmt"
	"strings"
)

func formatYesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// formatRemaining renders seconds as "9m 54s"; unlike settings.RemainingLabel
// on the GUI side, this includes seconds.
func formatRemaining(active, infinite bool, sec int) string {
	if !active {
		return "Off"
	}
	if infinite {
		return "Indefinite"
	}
	if sec < 0 {
		sec = 0
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	return fmt.Sprintf("%dm %ds", m, s)
}

// formatPresetSec renders a config preset (seconds) as "15m", "1h 30m", "Unlimited".
func formatPresetSec(sec int) string {
	if sec <= 0 {
		return "Unlimited"
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

var modeLabels = map[string]string{
	"off":      "Off",
	"infinite": "Infinite",
	"timer":    "Timer",
	"schedule": "Schedule",
}

func formatMode(mode string) string {
	if label, ok := modeLabels[mode]; ok {
		return label
	}
	return mode
}

var trayClickActionLabels = map[string]string{
	"cycle":    "Cycle through presets",
	"infinite": "Keep on indefinitely",
	"menu":     "Open tray menu",
}

func formatTrayClickAction(action string) string {
	if label, ok := trayClickActionLabels[action]; ok {
		return label
	}
	return action
}

func renderStatusText(s *StatusPayload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Mode: %s\n", formatMode(s.Mode))
	fmt.Fprintf(&b, "Remaining: %s\n", formatRemaining(s.Active, s.Infinite, s.RemainingSec))
	fmt.Fprintf(&b, "Keep display on: %s", formatYesNo(s.KeepDisplayOn))
	return b.String()
}

func renderConfigText(c *ConfigPayload) string {
	presets := make([]string, len(c.TimerList))
	for i, sec := range c.TimerList {
		presets[i] = formatPresetSec(sec)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Auto start: %s\n", formatYesNo(c.AutoStart))
	fmt.Fprintf(&b, "Keep display on: %s\n", formatYesNo(c.KeepDisplayOn))
	fmt.Fprintf(&b, "Auto update: %s\n", formatYesNo(c.AutoUpdate))
	fmt.Fprintf(&b, "Presets: %s\n", strings.Join(presets, ", "))
	fmt.Fprintf(&b, "Tray click action: %s", formatTrayClickAction(c.TrayClickAction))
	return b.String()
}
