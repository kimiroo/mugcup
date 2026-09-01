package ui

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"mugcup/settings"

	"github.com/tailscale/walk"
	. "github.com/tailscale/walk/declarative"
	"github.com/tailscale/win"
)

var (
	ctrlRef       *settings.Controller
	mw            *walk.MainWindow
	displayBox    *walk.CheckBox
	autoStartBox  *walk.CheckBox
	autoUpdateBox *walk.CheckBox
	trayClickBox  *walk.ComboBox
	timerListEdit *walk.LineEdit
	hoursEdit     *walk.NumberEdit
	minutesEdit   *walk.NumberEdit
	freeTextEdit  *walk.LineEdit
	errLabel      *walk.Label
	isSyncing     bool
)

var trayActionLabels = []string{"Cycle through presets", "Keep on indefinitely", "Open tray menu"}

func actionToIndex(action settings.TrayClickAction) int {
	switch action {
	case settings.ActionInfinite:
		return 1
	case settings.ActionOpenMenu:
		return 2
	default:
		return 0
	}
}

func indexToAction(index int) settings.TrayClickAction {
	switch index {
	case 1:
		return settings.ActionInfinite
	case 2:
		return settings.ActionOpenMenu
	default:
		return settings.ActionCycle
	}
}

func Init(ctrl *settings.Controller) { ctrlRef = ctrl }

func OpenSettingsWindow() {
	walk.App().Synchronize(func() {
		if mw != nil {
			syncControls()
			mw.Show()
			mw.SetFocus()
			return
		}
		buildWindow()
	})
}

// saveCurrentConfig only saves once the settings validate cleanly.
func saveCurrentConfig() bool {
	if isSyncing || ctrlRef == nil || timerListEdit == nil {
		return true
	}
	list, err := parseTimerList(timerListEdit.Text())
	if err != nil {
		if errLabel != nil {
			errLabel.SetText(err.Error())
		}
		return false
	}
	if err := ctrlRef.UpdateConfig(settings.Config{
		AutoStart: autoStartBox.Checked(), KeepDisplayOn: displayBox.Checked(),
		AutoUpdate: autoUpdateBox.Checked(), TimerList: list,
		TrayClickAction: indexToAction(trayClickBox.CurrentIndex()),
	}); err != nil {
		if errLabel != nil {
			errLabel.SetText("Could not save settings: " + err.Error())
		}
		return false
	}
	if errLabel != nil {
		errLabel.SetText("")
	}
	return true
}

func syncControls() {
	if ctrlRef == nil {
		return
	}
	isSyncing = true
	defer func() { isSyncing = false }()
	cfg := ctrlRef.Config()
	if displayBox != nil {
		displayBox.SetChecked(cfg.KeepDisplayOn)
	}
	if autoStartBox != nil {
		autoStartBox.SetChecked(cfg.AutoStart)
	}
	if autoUpdateBox != nil {
		autoUpdateBox.SetChecked(cfg.AutoUpdate)
	}
	if trayClickBox != nil {
		trayClickBox.SetCurrentIndex(actionToIndex(cfg.TrayClickAction))
	}
	if timerListEdit != nil {
		timerListEdit.SetText(timerListToText(cfg.TimerList))
	}
	if errLabel != nil {
		errLabel.SetText("")
	}
}

func buildWindow() {
	cfg := ctrlRef.Config()
	isSyncing = true
	err := MainWindow{
		AssignTo: &mw, Title: "mugcup Settings", Visible: false,
		Size: Size{Width: 400, Height: 500}, MinSize: Size{Width: 380, Height: 460},
		Layout: VBox{Margins: Margins{Left: 10, Top: 10, Right: 10, Bottom: 10}, Spacing: 8},
		Children: []Widget{
			GroupBox{Title: "General", Layout: VBox{Margins: Margins{Left: 8, Top: 8, Right: 8, Bottom: 8}, Spacing: 6}, Children: []Widget{
				CheckBox{AssignTo: &displayBox, Text: "Also keep the display on", Checked: cfg.KeepDisplayOn, OnCheckedChanged: func() { saveCurrentConfig() }},
				CheckBox{AssignTo: &autoStartBox, Text: "Start automatically with Windows", Checked: cfg.AutoStart, OnCheckedChanged: func() { saveCurrentConfig() }},
				CheckBox{AssignTo: &autoUpdateBox, Text: "Auto update", Checked: cfg.AutoUpdate, OnCheckedChanged: func() { saveCurrentConfig() }},
				Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
					Label{Text: "Tray click action:"},
					ComboBox{AssignTo: &trayClickBox, Model: trayActionLabels, CurrentIndex: actionToIndex(cfg.TrayClickAction), OnCurrentIndexChanged: func() { saveCurrentConfig() }},
				}},
			}},
			GroupBox{Title: "Tray cycle timers (minutes, comma-separated, 0=unlimited)", Layout: VBox{Margins: Margins{Left: 8, Top: 8, Right: 8, Bottom: 8}, Spacing: 6}, Children: []Widget{
				LineEdit{AssignTo: &timerListEdit, Text: timerListToText(cfg.TimerList), OnTextChanged: func() { saveCurrentConfig() }},
			}},
			GroupBox{Title: "Start immediately with a custom duration", Layout: VBox{Margins: Margins{Left: 8, Top: 8, Right: 8, Bottom: 8}, Spacing: 6}, Children: []Widget{
				Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
					PushButton{Text: "Keep on indefinitely", OnClicked: func() { ctrlRef.SetInfinite() }},
					PushButton{Text: "Turn Off", OnClicked: func() { ctrlRef.TurnOff() }},
				}},
				Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
					Label{Text: "Hours:"}, NumberEdit{AssignTo: &hoursEdit, MinValue: 0, MaxValue: 23},
					Label{Text: "Minutes:"}, NumberEdit{AssignTo: &minutesEdit, MinValue: 0, MaxValue: 59},
					PushButton{Text: "Start", OnClicked: func() {
						d := time.Duration(hoursEdit.Value())*time.Hour + time.Duration(minutesEdit.Value())*time.Minute
						if err := ctrlRef.SetCustomDuration(d); err != nil {
							errLabel.SetText(err.Error())
						} else {
							errLabel.SetText("")
						}
					}},
				}},
				Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
					LineEdit{AssignTo: &freeTextEdit, ToolTipText: "e.g. 1h30m, 45m, 2h15m30s"},
					PushButton{Text: "Start with custom input", OnClicked: func() {
						d, err := settings.ParseDuration(freeTextEdit.Text())
						if err != nil {
							errLabel.SetText(err.Error())
							return
						}
						if err := ctrlRef.SetCustomDuration(d); err != nil {
							errLabel.SetText(err.Error())
							return
						}
						errLabel.SetText("")
					}},
				}},
			}},
			Label{AssignTo: &errLabel, TextColor: walk.RGB(200, 0, 0)}, VSpacer{},
		},
	}.Create()
	isSyncing = false
	if err != nil {
		log.Println("failed to create settings window:", err)
		return
	}

	// Remove walk's default empty menu bar before showing the window.
	win.SetMenu(mw.Handle(), 0)
	win.DrawMenuBar(mw.Handle())
	mw.SetExitOnClose(false)
	mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		if !saveCurrentConfig() {
			*canceled = true
			message := "Some settings couldn't be saved. Please fix the timer list before closing."
			if errLabel != nil && errLabel.Text() != "" {
				message += "\n\n" + errLabel.Text()
			}
			walk.MsgBox(mw, "Settings Error", message, walk.MsgBoxIconWarning)
			timerListEdit.SetFocus()
			return
		}
		*canceled = true
		mw.Hide()
	})
	mw.Show()
}

func timerListToText(seconds []int) string {
	parts := make([]string, len(seconds))
	for i, second := range seconds {
		parts[i] = strconv.Itoa(second / 60)
	}
	return strings.Join(parts, ", ")
}

func parseTimerList(text string) ([]int, error) {
	fields := strings.Split(text, ",")
	result := make([]int, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		minutes, err := strconv.Atoi(field)
		if err != nil || minutes < 0 {
			return nil, fmt.Errorf("invalid timer value: %q", field)
		}
		result = append(result, minutes*60)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("at least one timer is required")
	}
	return result, nil
}
