package tray

import (
	"sync"
	"time"

	"mugcup/assets"
	"mugcup/settings"

	"fyne.io/systray"
)

// Start registers the tray icon in external-loop mode.
func Start(ctrl *settings.Controller, onSettings func(), onQuit func()) (start, end func()) {
	return systray.RunWithExternalLoop(
		func() { onReady(ctrl, onSettings, onQuit) },
		func() {
			if onQuit != nil {
				onQuit()
			}
		},
	)
}

func onReady(ctrl *settings.Controller, onSettings func(), onQuit func()) {
	systray.SetIcon(assets.IconICO)
	cfg := ctrl.Config()
	state := ctrl.State()
	updateTooltip(state)

	mKeepDisplay := systray.AddMenuItemCheckbox("Keep display on", "Also keep the display from turning off", cfg.KeepDisplayOn)
	mInfinite := systray.AddMenuItemCheckbox("Keep on indefinitely", "Keep the system awake indefinitely", state.Active && state.Infinite)

	mPresets := systray.AddMenuItemCheckbox("Timer presets", "Choose a preset duration", state.PresetActive)

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

	mOff := systray.AddMenuItemCheckbox("Off", "Turn off the timer", !state.Active)

	systray.AddSeparator()
	mSettings := systray.AddMenuItem("Settings...", "Open settings window")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Exit", "Exit mugcup")

	updateTapHandler := func(action settings.TrayClickAction) {
		if action == settings.ActionOpenMenu {
			systray.SetOnTapped(nil) // nil tappedLeft makes left-click open the context menu on Windows
		} else {
			systray.SetOnTapped(func() {
				c := ctrl.Config()
				if c.TrayClickAction == settings.ActionInfinite {
					ctrl.ToggleInfinite()
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
		updateTooltip(s)
		if s.Active {
			mOff.Uncheck()
		} else {
			mOff.Check()
		}
		if s.Active && s.Infinite {
			mInfinite.Check()
		} else {
			mInfinite.Uncheck()
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
			case <-mInfinite.ClickedCh:
				ctrl.ToggleInfinite()
			case <-mOff.ClickedCh:
				ctrl.TurnOff()
			case <-mSettings.ClickedCh:
				onSettings()
			case <-mQuit.ClickedCh:
				systray.Quit()
				if onQuit != nil {
					onQuit()
				}
				return
			}
		}
	}()
}

func updateTooltip(s settings.State) {
	systray.SetTooltip("mugcup - " + s.RemainingLabel())
}
