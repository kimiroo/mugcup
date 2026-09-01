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

	mPresets := systray.AddMenuItem("Timer presets", "Choose a preset duration")
	var subItems []*systray.MenuItem
	var subItemsMu sync.Mutex

	rebuildPresets := func(timerList []int) {
		subItemsMu.Lock()
		defer subItemsMu.Unlock()
		for _, item := range subItems {
			item.Remove()
		}
		subItems = nil
		for _, sec := range timerList {
			label := settings.FormatDurationSec(sec)
			sub := mPresets.AddSubMenuItem(label, "Start "+label)
			subItems = append(subItems, sub)
			go func(s *systray.MenuItem, durationSec int) {
				for range s.ClickedCh {
					ctrl.SetPreset(durationSec)
				}
			}(sub, sec)
		}
	}

	rebuildPresets(cfg.TimerList)

	mToggle := systray.AddMenuItem("Turn Off", "Turn off the timer")
	updateToggleItem := func(s settings.State) {
		if s.Active {
			mToggle.SetTitle("Turn Off")
			mToggle.SetTooltip("Turn off the active timer")
		} else {
			mToggle.SetTitle("Turn On")
			mToggle.SetTooltip("Turn on the timer")
		}
	}
	updateToggleItem(state)

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
		updateToggleItem(s)
		if s.Active && s.Infinite {
			mInfinite.Check()
		} else {
			mInfinite.Uncheck()
		}
	})

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			updateTooltip(ctrl.State())
		}
	}()

	go func() {
		for {
			select {
			case <-mKeepDisplay.ClickedCh:
				_ = ctrl.ToggleKeepDisplayOn()
			case <-mInfinite.ClickedCh:
				ctrl.ToggleInfinite()
			case <-mToggle.ClickedCh:
				if ctrl.State().Active {
					ctrl.TurnOff()
				} else {
					ctrl.Cycle()
				}
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
