package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

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

// viewSpec's width is fixed (DisableResize means the user can't touch it
// anyway), but height ranges between minHeight and maxHeight: the frontend
// measures its own actual content height on every change (tab switches,
// toggled fields, a growing preset list) and asks ResizeToContent to match
// it, so the window's padding stays visually even instead of a static guess
// leaving leftover space pooling at the bottom.
type viewSpec struct {
	title                string
	width                int
	minHeight, maxHeight int
}

var viewSpecs = map[windowView]viewSpec{
	viewSettings: {"Settings", 440, 300, 850},
	viewCustom:   {"Custom", 400, 300, 460},
	viewAbout:    {"About", 360, 300, 400},
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

// applyView pushes the window chrome (title, size bounds) and tells the
// already-loaded frontend which view to render, for switching views in a
// window that's already open. The very first open sizes itself from
// viewSpecs directly in run(), before the window exists. The actual height
// starts at minHeight as a placeholder; the frontend corrects it via
// ResizeToContent within a frame of the view becoming visible.
func (a *App) applyView(ctx context.Context, view windowView) {
	spec := viewSpecs[view]
	wailsruntime.WindowSetTitle(ctx, spec.title)
	wailsruntime.WindowSetMinSize(ctx, spec.width, spec.minHeight)
	wailsruntime.WindowSetMaxSize(ctx, spec.width, spec.maxHeight)
	wailsruntime.WindowSetSize(ctx, spec.width, spec.minHeight)
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
			popupLogger.Println("settings window panic:", r)
		}
		a.mu.Lock()
		if a.ctx == nil {
			a.started = false
		}
		a.mu.Unlock()
	}()

	assets, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		popupLogger.Println("frontend assets:", err)
		return
	}

	spec := viewSpecs[view]
	err = wails.Run(&options.App{
		Title:            spec.title,
		Width:            spec.width,
		Height:           spec.minHeight,
		MinWidth:         spec.width,
		MinHeight:        spec.minHeight,
		MaxWidth:         spec.width,
		MaxHeight:        spec.maxHeight,
		DisableResize:    true,
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
			WindowClassName:      popupWindowClassName,
		},
	})
	if err != nil {
		popupLogger.Println("wails.Run failed:", err)
	}
}

func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	view := a.view
	a.mu.Unlock()

	// Push live config updates so the window reflects changes made elsewhere
	// (CLI, tray) while it's open, not just what was true when it was opened.
	// Timer state isn't shown in this window (that's the tray's job), so
	// there's no equivalent state subscription.
	a.ctrl.OnConfigChange(func(settings.Config) {
		wailsruntime.EventsEmit(ctx, "mugcup:config", configPayload(a.ctrl))
	})

	setPopupWindowIcon()

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

// ShowError shows message in a native, modal message box. The popup window
// is small and every other alert in the app (update prompts, warnings)
// already uses one — an inline banner at the bottom of the content was easy
// to miss entirely (e.g. a duplicate-preset or save error), so validation
// and settings errors from the frontend go through here too.
func (a *App) ShowError(message string) {
	title, _ := syscall.UTF16PtrFromString("mugcup")
	text, _ := syscall.UTF16PtrFromString(message)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), uintptr(mbOK|mbIconWarning|mbSetForeground))
}

// Version returns the build-time version string (e.g. "1.2.3" or "dev").
// Exposed as a bound method so the About view can display it without an
// additional IPC round-trip.
func (a *App) Version() string { return Version }

// CurrentView reports which view the window should render. The frontend
// calls this once on load, since it can't rely on catching the startup
// "mugcup:view" event if its listener attaches after that event fired.
func (a *App) CurrentView() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return string(a.view)
}

// windowChromeHeight is this window style's native title bar + bottom
// border thickness, measured empirically (GetWindowRect vs GetClientRect)
// at 100% display scale. WindowSetSize scales whatever height we pass it by
// the window's current per-monitor DPI, so folding this constant in keeps
// content-to-window sizing consistent at other scales too.
const windowChromeHeight = 39

// ResizeToContent resizes the popup window's height to fit contentHeight,
// the frontend's own measurement of its actual rendered content (see
// measureContentHeight in app.js), clamped to the current view's
// min/maxHeight via the WindowSetMinSize/WindowSetMaxSize calls in
// applyView. Width never changes — only height varies content-to-content.
func (a *App) ResizeToContent(contentHeight int) {
	a.mu.Lock()
	ctx := a.ctx
	view := a.view
	a.mu.Unlock()
	if ctx == nil {
		return
	}
	spec := viewSpecs[view]
	wailsruntime.WindowSetSize(ctx, spec.width, contentHeight+windowChromeHeight)
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

// StartDurationSeconds starts a one-off custom timer (Mode: Timer), used by
// the Custom view's "for a duration" tab.
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

// StartScheduleSeconds starts a one-off custom timer (Mode: Schedule), used
// by the Custom view's "until a date & time" tab, which computes seconds
// until the target client-side before calling this.
func (a *App) StartScheduleSeconds(sec int) (*ipc.StatusPayload, error) {
	if sec <= 0 {
		return nil, fmt.Errorf("duration must be greater than 0")
	}
	var setErr error
	runOnMainThreadVoid(a.walkApp, func() {
		setErr = a.ctrl.SetSchedule(time.Duration(sec) * time.Second)
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
			AutoUpdateCheck: cfg.AutoUpdateCheck,
			AutoUpdateApply: cfg.AutoUpdateApply,
			TimerList:       cfg.TimerList,
			TrayClickAction: settings.TrayClickAction(cfg.TrayClickAction),
		})
	})
	return saveErr
}

// ExportConfig lets the user save the current config to a JSON file of their
// choosing via a native Save dialog — this wrapper owns the dialog and the
// file write; the actual serialization is the same configPayload the CLI's
// "export" IPC case (main.go) already uses. On success it confirms via a
// native dialog itself, since nothing else is watching this bound method's
// return value for good news (errors still surface through the frontend's
// existing App().ExportConfig().catch(...) -> ShowError).
func (a *App) ExportConfig() error {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return errors.New("window not ready")
	}

	path, err := wailsruntime.SaveFileDialog(ctx, wailsruntime.SaveDialogOptions{
		Title:           "Export mugcup config",
		DefaultFilename: "mugcup-config.json",
		Filters:         []wailsruntime.FileFilter{{DisplayName: "Config Files (*.json)", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return err // err is nil when the user just cancelled
	}

	settingsLogger.Println("Config exported.")
	data, err := json.MarshalIndent(configPayload(a.ctrl), "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	showInfo("Config exported to:\n" + path)
	return nil
}

// ImportConfig lets the user pick a JSON config file via a native Open
// dialog and applies it — this wrapper owns the dialog and the file read;
// parsing and applying is handleImport (main.go), the same function the
// CLI's "import" IPC case already calls with CLI-supplied JSON instead of a
// dialog-picked file. See ExportConfig above for the success-dialog note.
func (a *App) ImportConfig() error {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return errors.New("window not ready")
	}

	path, err := wailsruntime.OpenFileDialog(ctx, wailsruntime.OpenDialogOptions{
		Title:   "Import mugcup config",
		Filters: []wailsruntime.FileFilter{{DisplayName: "Config Files (*.json)", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return err // err is nil when the user just cancelled
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	var resp ipc.Response
	runOnMainThreadVoid(a.walkApp, func() {
		resp = handleImport(a.ctrl, string(data))
	})
	if !resp.Success {
		return errors.New(resp.Message)
	}
	showInfo("Config imported successfully.")
	return nil
}

func (a *App) Hide() {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx != nil {
		wailsruntime.WindowHide(ctx)
	}
}
