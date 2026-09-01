package i18n

// messagesEN is the fallback catalog — every key mugcup ever looks up must
// exist here, since T falls back to this map for anything missing from the
// active locale.
var messagesEN = map[string]string{
	// ---- Tray menu (tray/tray.go) ----
	"tray.indefinite.label":     "Indefinite",
	"tray.indefinite.tooltip":   "Keep the system awake indefinitely",
	"tray.preset.label":         "Preset",
	"tray.preset.tooltip":       "Choose a preset duration",
	"tray.preset.start_tooltip": "Start %s",
	"tray.custom.label":         "Custom...",
	"tray.custom.tooltip":       "Start for a custom duration or until a specific time",
	"tray.turnoff.label":        "Turn off",
	"tray.turnoff.tooltip":      "Turn off the timer",
	"tray.keepdisplay.label":    "Keep display on",
	"tray.keepdisplay.tooltip":  "Also keep the display from turning off",
	"tray.settings.label":       "Settings",
	"tray.settings.tooltip":     "Open settings window",
	"tray.about.label":          "About",
	"tray.about.tooltip":        "About mugcup",
	"tray.quit.label":           "Quit",
	"tray.quit.tooltip":         "Quit mugcup",

	// ---- Popup window titles/dialogs (wailsapp.go) ----
	"window.title.settings":      "Settings",
	"window.title.custom":        "Custom",
	"window.title.about":         "About",
	"dialog.export_config.title": "Export mugcup config",
	"dialog.import_config.title": "Import mugcup config",
	"config.export.success":      "Config exported to:\n%s",
	"config.import.success":      "Config imported successfully.",

	// ---- Native message boxes (main.go, updateflow.go, webview2.go) ----
	"cli_warning.text": "mugcup.exe does not process command-line arguments directly.\n\n" +
		"Use mugcup-cli.exe to send commands to the running mugcup.\nExample: mugcup-cli start 1h30m",
	"update.available.title": "mugcup update available",
	"update.available.text":  "mugcup %s is available (you have %s).\n\nInstall it now? mugcup will restart.",
	"update.failed.title":    "mugcup update failed",
	"update.failed.prefix":   "Failed to update mugcup:\n\n",
	"update.up_to_date":      "mugcup %s is already up to date.",
	"update.dev_build":       "This is a development build, so it can't check for updates.",
	"update.restart_needed":  "mugcup was updated to %s. Please restart it to finish.",
	"webview2.missing": "This window needs the Microsoft Edge WebView2 Runtime, which isn't installed on this PC.\n\n" +
		"Install it from https://developer.microsoft.com/microsoft-edge/webview2/ and try again. " +
		"The keep-awake tray feature works fine without it.",

	// ---- Frontend UI (frontend/dist/index.html, app.js — see Translations
	// in wailsapp.go and applyTranslations in app.js) ----
	"ui.settings.presets_heading":       "Presets",
	"ui.settings.preset_placeholder":    "e.g. 1h30m, 45m, or 0 for indefinite",
	"ui.settings.add_button":            "+ Add",
	"ui.settings.general_heading":       "General",
	"ui.settings.keep_display_on":       "Keep display on",
	"ui.settings.start_with_windows":    "Start automatically with Windows",
	"ui.settings.auto_check_updates":    "Automatically check for updates",
	"ui.settings.auto_install_updates":  "Automatically install updates",
	"ui.settings.tray_click_action":     "Tray click action",
	"ui.settings.tray_click_cycle":      "Cycle through presets",
	"ui.settings.tray_click_indefinite": "Toggle indefinite mode",
	"ui.settings.tray_click_menu":       "Open tray menu",
	"ui.settings.config_heading":        "Config",
	"ui.settings.export_button":         "Export...",
	"ui.settings.import_button":         "Import...",
	"ui.settings.language":              "Language",

	"ui.custom.heading":              "Custom",
	"ui.custom.tab_duration":         "For a duration",
	"ui.custom.tab_until":            "Until a date & time",
	"ui.custom.duration_placeholder": "e.g. 1h30m, 45m",
	"ui.custom.start_button":         "Start",
	"ui.custom.choose_date":          "Choose a specific date",
	"ui.custom.cancel_button":        "Cancel",

	"ui.about.tagline":        "Keep your PC awake, on your terms.",
	"ui.about.version_prefix": "Version",
	"ui.about.view_github":    "View on GitHub",
	"ui.about.check_updates":  "Check for Updates",
	"ui.about.close":          "Close",

	"ui.common.indefinite": "Indefinite",

	"ui.preset.empty":        "No presets yet — add one below.",
	"ui.preset.drag_title":   "Drag to reorder",
	"ui.preset.remove_title": "Remove preset",

	"ui.error.unrecognized_with_indefinite": "Unrecognized format. Try e.g. 1h30m, 45m, or 0 for indefinite.",
	"ui.error.preset_exists":                "That preset already exists.",
	"ui.error.enter_duration":               "Enter a duration, e.g. 1h30m, 45m.",
	"ui.error.unrecognized_duration":        "Unrecognized format. Try e.g. 1h30m, 45m.",
	"ui.error.pick_datetime":                "Pick a date and time.",
	"ui.error.pick_time":                    "Pick a time.",
	"ui.error.pick_future_datetime":         "Pick a date and time in the future.",
	"ui.error.pick_future_time":             `Pick a time later today, or turn on "With date" to choose another day.`,
}
