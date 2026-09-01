package tray

import (
	"fmt"
	"sync"
	"time"

	"mugcup/assets"
	"mugcup/i18n"
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

	mIndefinite := systray.AddMenuItemCheckbox(i18n.T("tray.indefinite.label"), i18n.T("tray.indefinite.tooltip"), state.Active && state.Indefinite)
	mPresets := systray.AddMenuItemCheckbox(i18n.T("tray.preset.label"), i18n.T("tray.preset.tooltip"), state.PresetActive)

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
				p.item.SetTooltip(fmt.Sprintf(i18n.T("tray.preset.start_tooltip"), p.label))
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
			sub := mPresets.AddSubMenuItemCheckbox(label, fmt.Sprintf(i18n.T("tray.preset.start_tooltip"), label), false)
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

	mCustom := systray.AddMenuItem(i18n.T("tray.custom.label"), i18n.T("tray.custom.tooltip"))
	mOff := systray.AddMenuItemCheckbox(i18n.T("tray.turnoff.label"), i18n.T("tray.turnoff.tooltip"), !state.Active)

	systray.AddSeparator()
	mKeepDisplay := systray.AddMenuItemCheckbox(i18n.T("tray.keepdisplay.label"), i18n.T("tray.keepdisplay.tooltip"), cfg.KeepDisplayOn)
	mSettings := systray.AddMenuItem(i18n.T("tray.settings.label"), i18n.T("tray.settings.tooltip"))

	systray.AddSeparator()
	mAbout := systray.AddMenuItem(i18n.T("tray.about.label"), i18n.T("tray.about.tooltip"))
	mQuit := systray.AddMenuItem(i18n.T("tray.quit.label"), i18n.T("tray.quit.tooltip"))

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

	// relabel refreshes every static item's title/tooltip from the active
	// locale — a no-op string swap most of the time, but the only way the
	// tray menu picks up a live language change (i18n.SetLang has already
	// run by the time this listener fires; main.go's own OnConfigChange
	// listener for that is registered first). rebuildPresets below handles
	// the dynamic preset submenu the same way, just via full recreation.
	relabel := func() {
		mIndefinite.SetTitle(i18n.T("tray.indefinite.label"))
		mIndefinite.SetTooltip(i18n.T("tray.indefinite.tooltip"))
		mPresets.SetTitle(i18n.T("tray.preset.label"))
		mPresets.SetTooltip(i18n.T("tray.preset.tooltip"))
		mCustom.SetTitle(i18n.T("tray.custom.label"))
		mCustom.SetTooltip(i18n.T("tray.custom.tooltip"))
		mOff.SetTitle(i18n.T("tray.turnoff.label"))
		mOff.SetTooltip(i18n.T("tray.turnoff.tooltip"))
		mKeepDisplay.SetTitle(i18n.T("tray.keepdisplay.label"))
		mKeepDisplay.SetTooltip(i18n.T("tray.keepdisplay.tooltip"))
		mSettings.SetTitle(i18n.T("tray.settings.label"))
		mSettings.SetTooltip(i18n.T("tray.settings.tooltip"))
		mAbout.SetTitle(i18n.T("tray.about.label"))
		mAbout.SetTooltip(i18n.T("tray.about.tooltip"))
		mQuit.SetTitle(i18n.T("tray.quit.label"))
		mQuit.SetTooltip(i18n.T("tray.quit.tooltip"))
	}

	ctrl.OnConfigChange(func(c settings.Config) {
		if c.KeepDisplayOn {
			mKeepDisplay.Check()
		} else {
			mKeepDisplay.Uncheck()
		}
		updateTapHandler(c.TrayClickAction)
		relabel()
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
