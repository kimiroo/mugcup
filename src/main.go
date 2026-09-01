package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"mugcup/ipc"
	"mugcup/settings"
	"mugcup/tray"
	"mugcup/ui"

	"github.com/tailscale/walk"
)

func init() {
	// walk must init/run on the process's first OS thread, so pin the main
	// goroutine to it.
	runtime.LockOSThread()
}

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
)

const (
	mbOK          = 0x00000000
	mbIconWarning = 0x00000030
)

// showUseCliWarning shows a native message box without needing walk
// initialized, for when mugcup.exe is launched with CLI-style arguments.
func showUseCliWarning() {
	title, _ := syscall.UTF16PtrFromString("mugcup")
	text, _ := syscall.UTF16PtrFromString("mugcup.exe does not process command-line arguments directly.\n\nUse mugcup-cli.exe to send commands to the running mugcup.\nExample: mugcup-cli start 1h30m")
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), uintptr(mbOK|mbIconWarning))
}

func main() {
	// mugcup.exe never parses commands itself; any CLI-style argument just
	// shows the warning above and exits.
	if len(os.Args[1:]) > 0 {
		showUseCliWarning()
		os.Exit(0)
	}

	// Don't start a second instance if one is already running.
	if _, ok := ipc.TrySendToExisting(ipc.Request{}); ok {
		return
	}

	cfg, err := settings.LoadConfig()
	if err != nil {
		log.Println("failed to load config, using defaults:", err)
		cfg = settings.DefaultConfig()
	}
	ctrl := settings.NewController(cfg)

	app, err := walk.InitApp()
	if err != nil {
		log.Fatal("walk.InitApp failed:", err)
	}

	ui.Init(ctrl)

	// Each command arrives on a TCP accept goroutine, but Controller changes
	// can trigger tray menu rebuilds, which must run on the walk main thread
	// (calling from another thread hangs systray's menu operations).
	cleanupIPC, err := ipc.StartServer(func(r ipc.Request) ipc.Response {
		return runOnMainThread(app, func() ipc.Response {
			return handleIPCRequest(ctrl, app, r)
		})
	})
	if err == nil {
		defer cleanupIPC()
	}

	quitHandler := func() {
		app.Exit(0)
	}

	start, end := tray.Start(ctrl, func() {
		ui.OpenSettingsWindow()
	}, quitHandler)
	start()
	defer end()

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

func handleIPCRequest(ctrl *settings.Controller, app *walk.Application, req ipc.Request) ipc.Response {
	// -d/--display-on is a global option applied before dispatching to the
	// actual command, regardless of which command it is.
	if req.DisplayOn != nil {
		cfg := ctrl.Config()
		if cfg.KeepDisplayOn != *req.DisplayOn {
			cfg.KeepDisplayOn = *req.DisplayOn
			if err := ctrl.UpdateConfig(cfg); err != nil {
				return ipc.Response{Success: false, Message: "failed to change the keep-display-on setting: " + err.Error()}
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

	case "status":
		return ipc.Response{
			Success: true,
			Message: fmt.Sprintf("mugcup status: %s (keep display on: %v)", ctrl.State().RemainingLabel(), ctrl.Config().KeepDisplayOn),
			Status:  statusPayload(ctrl),
		}

	case "config":
		return ipc.Response{Success: true, Message: "Retrieved the current config.", Config: configPayload(ctrl)}

	case "export":
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
		ctrl.Cycle()
		return ipc.Response{Success: true, Message: fmt.Sprintf("Timer started (%s)", ctrl.State().RemainingLabel()), Status: statusPayload(ctrl)}
	}

	switch strings.ToLower(args[0]) {
	case "infinite", "inf", "0":
		ctrl.SetInfinite()
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
		return ipc.Response{Success: false, Message: "the config JSON to import is empty."}
	}
	var cfg settings.Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return ipc.Response{Success: false, Message: "failed to parse config JSON: " + err.Error()}
	}
	if err := ctrl.UpdateConfig(cfg); err != nil {
		return ipc.Response{Success: false, Message: "failed to apply config: " + err.Error()}
	}
	return ipc.Response{Success: true, Message: "Imported config.", Config: configPayload(ctrl)}
}

func statusPayload(ctrl *settings.Controller) *ipc.StatusPayload {
	st := ctrl.State()
	remainingSec := 0
	if st.Active && !st.Infinite {
		if remaining := int(time.Until(st.ExpiresAt).Seconds()); remaining > 0 {
			remainingSec = remaining
		}
	}
	return &ipc.StatusPayload{
		Active:         st.Active,
		Infinite:       st.Infinite,
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
		AutoUpdate:      cfg.AutoUpdate,
		TimerList:       cfg.TimerList,
		TrayClickAction: string(cfg.TrayClickAction),
	}
}
