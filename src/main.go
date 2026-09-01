package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"mugcup/applog"
	"mugcup/autostart"
	"mugcup/ipc"
	"mugcup/settings"
	"mugcup/tray"
	"mugcup/update"

	"github.com/tailscale/walk"
)

func init() {
	// walk must init/run on the process's first OS thread, so pin the main
	// goroutine to it.
	runtime.LockOSThread()
}

func init() {
	// manifest.xml (embedded as rsrc.syso) predates WebView2 and doesn't
	// declare DPI awareness, so Windows would otherwise DPI-virtualize this
	// process's windows while WebView2 renders its content at the real DPI
	// internally — the two drift apart at any scale other than 100%, and
	// fixed window sizes stop matching their content. Declaring Per-Monitor-v2
	// awareness here (before any window exists) avoids needing to touch the
	// prebuilt rsrc.syso.
	if procSetProcessDpiAwarenessContext.Find() == nil {
		procSetProcessDpiAwarenessContext.Call(dpiAwarenessContextPerMonitorAwareV2)
	}
}

var (
	user32                            = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW                   = user32.NewProc("MessageBoxW")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
)

// Subsystem loggers, all writing to the one shared rotating log file (see
// mugcup/applog). New is safe to call before applog.Init runs — these are
// resolved eagerly at package-init time but don't actually write anywhere
// until something logs, which only happens after main() has called Init.
var (
	mainLogger     = applog.New("main")
	settingsLogger = applog.New("settings")
	popupLogger    = applog.New("popup")
	updateLogger   = applog.New("update")
)

// dpiAwarenessContextPerMonitorAwareV2 is DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2,
// a pseudo-handle Windows defines as -4 (see winuser.h) rather than a real
// value, hence the bit trick to get -4 into a uintptr.
const dpiAwarenessContextPerMonitorAwareV2 = ^uintptr(3)

// stopIPCServer shuts down the single-instance IPC listener. It's set once
// StartServer succeeds and called early by applyUpdateAndRestart (before
// spawning the replacement process), so the new instance's own
// TrySendToExisting check doesn't see this still-exiting one and mistake it
// for "already running".
var stopIPCServer func()

const (
	mbOK              = 0x00000000
	mbYesNo           = 0x00000004
	mbIconInformation = 0x00000040
	mbIconWarning     = 0x00000030
	mbIconQuestion    = 0x00000020

	mbSetForeground = 0x00010000

	idYes = 6
)

// Version is set at build time via -ldflags "-X main.Version=1.2.3"
// (build.ps1 does this automatically from the nearest git tag). The default,
// "dev", marks a local/unreleased build; self-update is disabled for those,
// since there's no meaningful version to compare against a release.
var Version = "dev"

// showUseCliWarning shows a native message box without needing walk
// initialized, for when mugcup.exe is launched with CLI-style arguments.
func showUseCliWarning() {
	title, _ := syscall.UTF16PtrFromString("mugcup")
	text, _ := syscall.UTF16PtrFromString("mugcup.exe does not process command-line arguments directly.\n\nUse mugcup-cli.exe to send commands to the running mugcup.\nExample: mugcup-cli start 1h30m")
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), uintptr(mbOK|mbIconWarning|mbSetForeground))
}

func main() {
	if err := applog.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "applog: failed to open log file, falling back to stderr:", err)
	}

	// mugcup.exe never parses commands itself; any CLI-style argument just
	// shows the warning above and exits.
	if len(os.Args[1:]) > 0 {
		showUseCliWarning()
		os.Exit(0)
	}

	// Clean up leftover .old executable files from previous updates
	update.CleanOldExecutables()

	// Don't start a second instance if one is already running.
	if _, ok := ipc.TrySendToExisting(ipc.Request{}); ok {
		return
	}

	// Failures are already logged inside settings.LoadConfig (settings
	// subsystem), which also already falls back to defaults on its own.
	cfg, _ := settings.LoadConfig()
	ctrl := settings.NewController(cfg)

	// Self-heal the auto-start registry entry on every launch (in case it
	// was removed or edited outside mugcup), and again whenever the setting
	// changes so toggling it takes effect immediately. Failures are already
	// logged inside autostart.Sync (registry subsystem), so they're just
	// swallowed here — a failed self-heal isn't fatal to starting up.
	_ = autostart.Sync(cfg.AutoStart)
	ctrl.OnConfigChange(func(c settings.Config) {
		_ = autostart.Sync(c.AutoStart)
	})

	app, err := walk.InitApp()
	if err != nil {
		mainLogger.Fatal("walk.InitApp failed:", err)
	}

	initWails(ctrl, app)

	// Each command arrives on a TCP accept goroutine, but Controller changes
	// can trigger tray menu rebuilds, which must run on the walk main thread
	// (calling from another thread hangs systray's menu operations).
	cleanupIPC, err := ipc.StartServer(func(r ipc.Request) ipc.Response {
		return runOnMainThread(app, func() ipc.Response {
			return handleIPCRequest(ctrl, app, r)
		})
	})
	if err == nil {
		stopIPCServer = cleanupIPC
		defer cleanupIPC()
	}

	quitHandler := func() {
		app.Exit(0)
	}

	start, end := tray.Start(ctrl, tray.Callbacks{
		OnSettings: openSettingsWindow,
		OnCustom:   openCustomWindow,
		OnAbout:    openAboutWindow,
		OnQuit:     quitHandler,
	})
	start()
	defer end()

	go maybeAutoCheckUpdate(ctrl, app)

	app.Run() // handles both tray and settings-window messages
	os.Exit(0)
}

// runOnMainThread runs fn on the walk main thread and waits synchronously for
// the result, since Application.Synchronize itself is non-blocking (it only
// queues the func).
func runOnMainThread(app *walk.Application, fn func() ipc.Response) ipc.Response {
	done := make(chan ipc.Response, 1)
	app.Synchronize(func() {
		done <- fn()
	})
	select {
	case resp := <-done:
		return resp
	case <-time.After(6 * time.Second):
		return ipc.Response{Success: false, Message: "mugcup timed out processing the request internally."}
	}
}

// runOnMainThreadVoid is runOnMainThread without a return value, used by the
// Wails-bound methods (wailsapp.go) for the same reason: any Controller
// mutation that can trigger a tray menu rebuild must run on the walk main
// thread, regardless of which goroutine Wails dispatches the JS call on.
func runOnMainThreadVoid(app *walk.Application, fn func()) {
	done := make(chan struct{}, 1)
	app.Synchronize(func() {
		fn()
		done <- struct{}{}
	})
	select {
	case <-done:
	case <-time.After(6 * time.Second):
	}
}

func handleIPCRequest(ctrl *settings.Controller, app *walk.Application, req ipc.Request) ipc.Response {
	// -d/--display-on, --auto-start, --auto-update-check, and
	// --auto-update-apply are global options applied before dispatching to
	// the actual command, regardless of which command it is (and are the
	// only thing the standalone "set" command does). They persist
	// independently of an active timer, unlike -d's on-screen effect which
	// only actually applies while a timer is running.
	if req.DisplayOn != nil || req.AutoStart != nil || req.AutoUpdateCheck != nil || req.AutoUpdateApply != nil {
		cfg := ctrl.Config()
		changed := false
		if req.DisplayOn != nil && cfg.KeepDisplayOn != *req.DisplayOn {
			cfg.KeepDisplayOn = *req.DisplayOn
			changed = true
		}
		if req.AutoStart != nil && cfg.AutoStart != *req.AutoStart {
			cfg.AutoStart = *req.AutoStart
			changed = true
		}
		if req.AutoUpdateCheck != nil && cfg.AutoUpdateCheck != *req.AutoUpdateCheck {
			cfg.AutoUpdateCheck = *req.AutoUpdateCheck
			changed = true
		}
		if req.AutoUpdateApply != nil && cfg.AutoUpdateApply != *req.AutoUpdateApply {
			cfg.AutoUpdateApply = *req.AutoUpdateApply
			changed = true
		}
		if changed {
			if err := ctrl.UpdateConfig(cfg); err != nil {
				return ipc.Response{Success: false, Message: "failed to update settings: " + err.Error()}
			}
		}
	}

	cmd := strings.ToLower(strings.TrimSpace(req.Command))
	switch cmd {
	case "start":
		return handleStart(ctrl, req.Args)

	case "stop":
		ctrl.TurnOff()
		return ipc.Response{Success: true, Message: "Timer turned off.", Status: statusPayload(ctrl)}

	case "set":
		return ipc.Response{Success: true, Message: "Updated settings.", Config: configPayload(ctrl)}

	case "status":
		return ipc.Response{
			Success: true,
			Message: fmt.Sprintf("mugcup status: %s (keep display on: %v)", ctrl.State().RemainingLabel(), ctrl.Config().KeepDisplayOn),
			Status:  statusPayload(ctrl),
		}

	case "config":
		return ipc.Response{Success: true, Message: "Retrieved the current config.", Config: configPayload(ctrl)}

	case "settings", "show":
		openSettingsWindow()
		return ipc.Response{Success: true, Message: "Opened the settings window."}

	case "export":
		settingsLogger.Println("Config exported.")
		return ipc.Response{Success: true, Message: "Exporting the current config.", Config: configPayload(ctrl)}

	case "import":
		return handleImport(ctrl, req.ConfigJSON)

	case "quit", "exit":
		go func() {
			time.Sleep(100 * time.Millisecond)
			app.Exit(0)
		}()
		return ipc.Response{Success: true, Message: "Exiting mugcup."}

	default:
		return ipc.Response{Success: false, Message: "Unknown command: " + req.Command}
	}
}

func handleStart(ctrl *settings.Controller, args []string) ipc.Response {
	if len(args) == 0 {
		return ipc.Response{Success: false, Message: "start requires an argument: a duration (e.g. 30m, 1h30m), 'indefinite', or 'preset <n>'."}
	}

	switch strings.ToLower(args[0]) {
	case "indefinite", "indef", "0":
		ctrl.SetIndefinite()
		return ipc.Response{Success: true, Message: "Indefinite keep-on activated.", Status: statusPayload(ctrl)}

	case "preset":
		if len(args) < 2 {
			return ipc.Response{Success: false, Message: "preset requires a number (e.g. start preset 0)"}
		}
		idx, err := strconv.Atoi(args[1])
		if err != nil {
			return ipc.Response{Success: false, Message: "invalid preset number: " + args[1]}
		}
		list := ctrl.Config().TimerList
		if idx < 0 || idx >= len(list) {
			return ipc.Response{Success: false, Message: fmt.Sprintf("preset number out of range (0-%d)", len(list)-1)}
		}
		ctrl.SetPreset(list[idx])
		return ipc.Response{Success: true, Message: fmt.Sprintf("Timer started (%s)", ctrl.State().RemainingLabel()), Status: statusPayload(ctrl)}

	default:
		d, err := settings.ParseDuration(args[0])
		if err != nil {
			return ipc.Response{Success: false, Message: fmt.Sprintf("invalid duration format: %v", err)}
		}
		if err := ctrl.SetCustomDuration(d); err != nil {
			return ipc.Response{Success: false, Message: err.Error()}
		}
		return ipc.Response{Success: true, Message: fmt.Sprintf("Timer started (%s)", ctrl.State().RemainingLabel()), Status: statusPayload(ctrl)}
	}
}

func handleImport(ctrl *settings.Controller, raw string) ipc.Response {
	if strings.TrimSpace(raw) == "" {
		settingsLogger.Println("Import rejected: empty config JSON.")
		return ipc.Response{Success: false, Message: "the config JSON to import is empty."}
	}
	var cfg settings.Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		settingsLogger.Printf("Import failed: invalid config JSON: %v", err)
		return ipc.Response{Success: false, Message: "failed to parse config JSON: " + err.Error()}
	}
	if err := ctrl.UpdateConfig(cfg); err != nil {
		// Already logged inside Controller.UpdateConfig (settings subsystem).
		return ipc.Response{Success: false, Message: "failed to apply config: " + err.Error()}
	}
	settingsLogger.Println("Config imported successfully.")
	return ipc.Response{Success: true, Message: "Imported config.", Config: configPayload(ctrl)}
}

func statusPayload(ctrl *settings.Controller) *ipc.StatusPayload {
	st := ctrl.State()
	remainingSec := 0
	if st.Active && !st.Indefinite {
		if remaining := int(time.Until(st.ExpiresAt).Seconds()); remaining > 0 {
			remainingSec = remaining
		}
	}
	return &ipc.StatusPayload{
		Active:         st.Active,
		Indefinite:     st.Indefinite,
		Mode:           string(st.Mode),
		RemainingSec:   remainingSec,
		RemainingLabel: st.RemainingLabel(),
		KeepDisplayOn:  ctrl.Config().KeepDisplayOn,
	}
}

func configPayload(ctrl *settings.Controller) *ipc.ConfigPayload {
	cfg := ctrl.Config()
	return &ipc.ConfigPayload{
		AutoStart:       cfg.AutoStart,
		KeepDisplayOn:   cfg.KeepDisplayOn,
		AutoUpdateCheck: cfg.AutoUpdateCheck,
		AutoUpdateApply: cfg.AutoUpdateApply,
		TimerList:       cfg.TimerList,
		TrayClickAction: string(cfg.TrayClickAction),
	}
}
