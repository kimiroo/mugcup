package tray

import (
	"sync"
	"time"

	"mugcup/assets"
	"mugcup/settings"

	"fyne.io/systray"
)

// Callbacks are the tray menu's entry points into the rest of the app: the
// three popup views (all timer control lives in the tray itself; the popups
// are config/one-off-start/info only) and quitting.
type Callbacks struct {
	OnSettings func()
	OnCustom   func()
	OnAbout    func()
	OnQuit     func()
}

// Start registers the tray icon in external-loop mode.
func Start(ctrl *settings.Controller, cb Callbacks) (start, end func()) {
	return systray.RunWithExternalLoop(
		func() { onReady(ctrl, cb) },
		func() {
			if cb.OnQuit != nil {
				cb.OnQuit()
			}
		},
	)
}

func onReady(ctrl *settings.Controller, cb Callbacks) {
	cfg := ctrl.Config()
	state := ctrl.State()
	updateTrayIcon(state)
	updateTooltip(state)

	mIndefinite := systray.AddMenuItemCheckbox("Indefinite", "Keep the system awake indefinitely", state.Active && state.Indefinite)
	mPresets := systray.AddMenuItemCheckbox("Preset", "Choose a preset duration", state.PresetActive)

	type presetItem struct {
		item    *systray.MenuItem
		seconds int
		label   string
	}
	var subItems []presetItem
	var subItemsMu sync.Mutex

	updatePresetChecks := func(s settings.State) {
		subItemsMu.Lock()
		defer subItemsMu.Unlock()
		anyChecked := false
		for _, p := range subItems {
			if s.Active && s.PresetActive && p.seconds == s.PresetSeconds {
				p.item.Check()
				p.item.SetTooltip(p.label + " - " + s.RemainingLabel())
				anyChecked = true
			} else {
				p.item.Uncheck()
				p.item.SetTooltip("Start " + p.label)
			}
		}
		if anyChecked {
			mPresets.Check()
		} else {
			mPresets.Uncheck()
		}
	}

	rebuildPresets := func(timerList []int) {
		subItemsMu.Lock()
		for _, p := range subItems {
			p.item.Remove()
		}
		subItems = nil
		for _, sec := range timerList {
			label := settings.FormatDurationSec(sec)
			sub := mPresets.AddSubMenuItemCheckbox(label, "Start "+label, false)
			subItems = append(subItems, presetItem{item: sub, seconds: sec, label: label})
			go func(s *systray.MenuItem, durationSec int) {
				for range s.ClickedCh {
					ctrl.SetPreset(durationSec)
				}
			}(sub, sec)
		}
		subItemsMu.Unlock()
		updatePresetChecks(ctrl.State())
	}

	rebuildPresets(cfg.TimerList)

	mCustom := systray.AddMenuItem("Custom...", "Start for a custom duration or until a specific time")
	mOff := systray.AddMenuItemCheckbox("Turn off", "Turn off the timer", !state.Active)

	systray.AddSeparator()
	mKeepDisplay := systray.AddMenuItemCheckbox("Keep display on", "Also keep the display from turning off", cfg.KeepDisplayOn)
	mSettings := systray.AddMenuItem("Settings", "Open settings window")

	systray.AddSeparator()
	mAbout := systray.AddMenuItem("About", "About mugcup")
	mQuit := systray.AddMenuItem("Quit", "Quit mugcup")

	updateTapHandler := func(action settings.TrayClickAction) {
		if action == settings.ActionOpenMenu {
			systray.SetOnTapped(nil) // nil tappedLeft makes left-click open the context menu on Windows
		} else {
			systray.SetOnTapped(func() {
				c := ctrl.Config()
				if c.TrayClickAction == settings.ActionIndefinite {
					ctrl.ToggleIndefinite()
				} else {
					ctrl.Cycle()
				}
			})
		}
	}
	updateTapHandler(cfg.TrayClickAction)

	ctrl.OnConfigChange(func(c settings.Config) {
		if c.KeepDisplayOn {
			mKeepDisplay.Check()
		} else {
			mKeepDisplay.Uncheck()
		}
		updateTapHandler(c.TrayClickAction)
		rebuildPresets(c.TimerList)
	})

	ctrl.OnStateChange(func(s settings.State) {
		updateTrayIcon(s)
		updateTooltip(s)
		if s.Active {
			mOff.Uncheck()
		} else {
			mOff.Check()
		}
		if s.Active && s.Indefinite {
			mIndefinite.Check()
		} else {
			mIndefinite.Uncheck()
		}
		updatePresetChecks(s)
	})

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			s := ctrl.State()
			updateTooltip(s)
			updatePresetChecks(s)
		}
	}()

	go func() {
		for {
			select {
			case <-mKeepDisplay.ClickedCh:
				_ = ctrl.ToggleKeepDisplayOn()
			case <-mIndefinite.ClickedCh:
				ctrl.ToggleIndefinite()
			case <-mOff.ClickedCh:
				ctrl.TurnOff()
			case <-mCustom.ClickedCh:
				if cb.OnCustom != nil {
					cb.OnCustom()
				}
			case <-mSettings.ClickedCh:
				if cb.OnSettings != nil {
					cb.OnSettings()
				}
			case <-mAbout.ClickedCh:
				if cb.OnAbout != nil {
					cb.OnAbout()
				}
			case <-mQuit.ClickedCh:
				systray.Quit()
				if cb.OnQuit != nil {
					cb.OnQuit()
				}
				return
			}
		}
	}()
}

func updateTooltip(s settings.State) {
	systray.SetTooltip("mugcup - " + s.RemainingLabel())
}

func updateTrayIcon(s settings.State) {
	switch s.Mode {
	case settings.ModeIndefinite:
		systray.SetIcon(assets.IconIndefiniteICO)
	case settings.ModeTimer:
		systray.SetIcon(assets.IconClockICO)
	case settings.ModeSchedule:
		systray.SetIcon(assets.IconScheduleICO)
	default:
		systray.SetIcon(assets.IconDisabledICO)
	}
}
