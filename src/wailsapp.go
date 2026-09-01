package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"runtime"
	"sync"
	"time"

	"mugcup/ipc"
	"mugcup/settings"

	"github.com/tailscale/walk"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailswindows "github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var frontendFS embed.FS

// App is the Wails-bound backend for the settings window. Every method here
// is callable from the frontend as window.go.main.App.<Method>(...).
type App struct {
	ctrl    *settings.Controller
	walkApp *walk.Application

	mu      sync.Mutex
	ctx     context.Context
	started bool
}

var wailsApp *App

// initWails wires up the settings-window backend. The Wails runtime itself
// isn't started here — it starts lazily on first openSettingsWindow() call,
// so a machine without WebView2 never pays for it unless Settings is opened.
func initWails(ctrl *settings.Controller, walkApp *walk.Application) {
	wailsApp = &App{ctrl: ctrl, walkApp: walkApp}
}

// openSettingsWindow shows the settings window, starting the Wails runtime on
// first use. Safe to call from any goroutine (tray click handler, or the
// walk-main-thread IPC dispatch).
func openSettingsWindow() {
	if wailsApp == nil {
		return
	}
	wailsApp.open()
}

func (a *App) open() {
	a.mu.Lock()
	alreadyStarted := a.started
	a.started = true
	ctx := a.ctx
	a.mu.Unlock()

	if alreadyStarted {
		if ctx != nil {
			wailsruntime.WindowShow(ctx)
			wailsruntime.WindowUnminimise(ctx)
		}
		return
	}

	if !isWebView2Installed() {
		showWebView2MissingWarning()
		a.mu.Lock()
		a.started = false
		a.mu.Unlock()
		return
	}

	// wails.Run blocks for the app's lifetime; the window shows itself once
	// ready, so the caller (tray click / IPC dispatch) never waits on it.
	go a.run()
}

func (a *App) run() {
	// wails.Run creates a window and a COM/WebView2 apartment that must stay
	// on one specific OS thread for the app's lifetime. A plain goroutine can
	// be migrated between OS threads by the Go scheduler at any point, which
	// silently breaks that requirement (the window sometimes just never
	// appears) — so this goroutine, which does nothing else, is pinned here.
	runtime.LockOSThread()

	// If the window never actually starts (error, panic, or wails.Run
	// returning early for any reason before OnStartup fires), un-stick
	// "already started" so the next Settings click retries instead of
	// trying to show a window that was never created.
	defer func() {
		if r := recover(); r != nil {
			log.Println("settings window panic:", r)
		}
		a.mu.Lock()
		if a.ctx == nil {
			a.started = false
		}
		a.mu.Unlock()
	}()

	assets, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		log.Println("frontend assets:", err)
		return
	}

	err = wails.Run(&options.App{
		Title:            "mugcup",
		Width:            440,
		Height:           760,
		MinWidth:         380,
		MinHeight:        480,
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        a.startup,
		OnBeforeClose:    a.beforeClose,
		Bind:             []interface{}{a},
		Windows: &wailswindows.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			BackdropType:         wailswindows.Mica,
			Theme:                wailswindows.SystemDefault,
			Messages:             wailswindows.DefaultMessages(),
		},
	})
	if err != nil {
		log.Println("wails.Run failed:", err)
	}
}

func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()

	// Push live updates so the window reflects changes made elsewhere (CLI,
	// tray) while it's open, not just what was true when it was opened.
	a.ctrl.OnStateChange(func(settings.State) {
		wailsruntime.EventsEmit(ctx, "mugcup:status", statusPayload(a.ctrl))
	})
	a.ctrl.OnConfigChange(func(settings.Config) {
		wailsruntime.EventsEmit(ctx, "mugcup:config", configPayload(a.ctrl))
	})
}

// beforeClose hides the window instead of destroying it, matching a normal
// settings dialog: the "X" button dismisses it, only tray "Exit" quits.
func (a *App) beforeClose(ctx context.Context) bool {
	wailsruntime.WindowHide(ctx)
	return true
}

// ---- Bound methods (called from frontend/dist/app.js) ----

func (a *App) GetConfig() *ipc.ConfigPayload { return configPayload(a.ctrl) }
func (a *App) GetStatus() *ipc.StatusPayload { return statusPayload(a.ctrl) }

func (a *App) SetInfinite() *ipc.StatusPayload {
	runOnMainThreadVoid(a.walkApp, func() { a.ctrl.SetInfinite() })
	return statusPayload(a.ctrl)
}

func (a *App) Stop() *ipc.StatusPayload {
	runOnMainThreadVoid(a.walkApp, func() { a.ctrl.TurnOff() })
	return statusPayload(a.ctrl)
}

func (a *App) StartPreset(index int) (*ipc.StatusPayload, error) {
	list := a.ctrl.Config().TimerList
	if index < 0 || index >= len(list) {
		return nil, fmt.Errorf("preset index out of range (0-%d)", len(list)-1)
	}
	runOnMainThreadVoid(a.walkApp, func() { a.ctrl.SetPreset(list[index]) })
	return statusPayload(a.ctrl), nil
}

func (a *App) StartDurationSeconds(sec int) (*ipc.StatusPayload, error) {
	if sec <= 0 {
		return nil, fmt.Errorf("duration must be greater than 0")
	}
	var setErr error
	runOnMainThreadVoid(a.walkApp, func() {
		setErr = a.ctrl.SetCustomDuration(time.Duration(sec) * time.Second)
	})
	if setErr != nil {
		return nil, setErr
	}
	return statusPayload(a.ctrl), nil
}

// SaveConfig persists the full config (general settings + edited preset
// list) in one call, mirroring the old settings window's autosave-on-change.
func (a *App) SaveConfig(cfg ipc.ConfigPayload) error {
	var saveErr error
	runOnMainThreadVoid(a.walkApp, func() {
		saveErr = a.ctrl.UpdateConfig(settings.Config{
			AutoStart:       cfg.AutoStart,
			KeepDisplayOn:   cfg.KeepDisplayOn,
			AutoUpdate:      cfg.AutoUpdate,
			TimerList:       cfg.TimerList,
			TrayClickAction: settings.TrayClickAction(cfg.TrayClickAction),
		})
	})
	return saveErr
}

func (a *App) Hide() {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx != nil {
		wailsruntime.WindowHide(ctx)
	}
}
