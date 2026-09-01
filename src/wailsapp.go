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

// windowView identifies which of the app's popup screens the single Wails
// window is currently showing. Wails v2 only supports one native window per
// process, so "separate popups" are implemented as views within that one
// window: opening a different view swaps its content, title, and size
// instead of creating a second window.
type windowView string

const (
	viewSettings windowView = "settings"
	viewCustom   windowView = "custom"
	viewAbout    windowView = "about"
)

type viewSpec struct {
	title               string
	width, height       int
	minWidth, minHeight int
}

var viewSpecs = map[windowView]viewSpec{
	viewSettings: {"mugcup — Settings", 440, 760, 380, 480},
	viewCustom:   {"mugcup — Custom", 400, 420, 400, 420},
	viewAbout:    {"mugcup — About", 360, 300, 360, 300},
}

// App is the Wails-bound backend for the popup window. Every method here is
// callable from the frontend as window.go.main.App.<Method>(...).
type App struct {
	ctrl    *settings.Controller
	walkApp *walk.Application

	mu      sync.Mutex
	ctx     context.Context
	started bool
	view    windowView
}

var wailsApp *App

// initWails wires up the popup window's backend. The Wails runtime itself
// isn't started here — it starts lazily on first openWindow() call, so a
// machine without WebView2 never pays for it unless a popup is opened.
func initWails(ctrl *settings.Controller, walkApp *walk.Application) {
	wailsApp = &App{ctrl: ctrl, walkApp: walkApp, view: viewSettings}
}

// openSettingsWindow, openCustomWindow, and openAboutWindow show the popup
// window on the requested view, starting the Wails runtime on first use.
// Safe to call from any goroutine (tray click handler, or the walk-main-
// thread IPC dispatch).
func openSettingsWindow() { openWindow(viewSettings) }
func openCustomWindow()   { openWindow(viewCustom) }
func openAboutWindow()    { openWindow(viewAbout) }

func openWindow(view windowView) {
	if wailsApp == nil {
		return
	}
	wailsApp.open(view)
}

func (a *App) open(view windowView) {
	a.mu.Lock()
	alreadyStarted := a.started
	a.started = true
	a.view = view
	ctx := a.ctx
	a.mu.Unlock()

	if alreadyStarted {
		if ctx != nil {
			a.applyView(ctx, view)
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
	go a.run(view)
}

// applyView pushes the window chrome (title, size) and tells the already-
// loaded frontend which view to render, for switching views in a window
// that's already open. The very first open sizes itself from viewSpecs
// directly in run(), before the window exists.
func (a *App) applyView(ctx context.Context, view windowView) {
	spec := viewSpecs[view]
	wailsruntime.WindowSetTitle(ctx, spec.title)
	wailsruntime.WindowSetMinSize(ctx, spec.minWidth, spec.minHeight)
	wailsruntime.WindowSetSize(ctx, spec.width, spec.height)
	wailsruntime.WindowCenter(ctx)
	wailsruntime.EventsEmit(ctx, "mugcup:view", string(view))
}

func (a *App) run(view windowView) {
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

	spec := viewSpecs[view]
	err = wails.Run(&options.App{
		Title:            spec.title,
		Width:            spec.width,
		Height:           spec.height,
		MinWidth:         spec.minWidth,
		MinHeight:        spec.minHeight,
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
	view := a.view
	a.mu.Unlock()

	// Push live updates so the window reflects changes made elsewhere (CLI,
	// tray) while it's open, not just what was true when it was opened.
	a.ctrl.OnStateChange(func(settings.State) {
		wailsruntime.EventsEmit(ctx, "mugcup:status", statusPayload(a.ctrl))
	})
	a.ctrl.OnConfigChange(func(settings.Config) {
		wailsruntime.EventsEmit(ctx, "mugcup:config", configPayload(a.ctrl))
	})

	// Also emit the view on startup for good measure; CurrentView() is what
	// the frontend actually relies on for its first render, since it can't
	// guarantee its event listener is attached before this fires.
	a.applyView(ctx, view)
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

// CurrentView reports which view the window should render. The frontend
// calls this once on load, since it can't rely on catching the startup
// "mugcup:view" event if its listener attaches after that event fired.
func (a *App) CurrentView() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return string(a.view)
}

// OpenRepo opens the project's GitHub page in the system's default browser.
func (a *App) OpenRepo() {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx != nil {
		wailsruntime.BrowserOpenURL(ctx, "https://github.com/kimiroo/mugcup")
	}
}

// StartDurationSeconds starts a one-off custom timer, used by the Custom
// view for both its "for a duration" and "until a date/time" modes (the
// latter converts to a duration client-side before calling this).
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
