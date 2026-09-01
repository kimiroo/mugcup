package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"mugcup/i18n"
	"mugcup/ipc"
	"mugcup/settings"
	"mugcup/update"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/tailscale/walk"
)

// maybeAutoCheckUpdate runs once at startup (in its own goroutine, so it
// never delays the tray showing up). If AutoUpdateApply is off, a found
// update is confirmed with the user before installing — only the
// check-and-apply-silently path skips the prompt.
func maybeAutoCheckUpdate(ctrl *settings.Controller, app *walk.Application) {
	cfg := ctrl.Config()
	if !cfg.AutoUpdateCheck || !update.ParseableVersion(Version) {
		return
	}

	rel, found, err := update.CheckLatest(context.Background(), Version, BuildVariant)
	if err != nil {
		updateLogger.Println("update check failed:", err)
		return
	}
	if !found {
		return
	}

	if !cfg.AutoUpdateApply && !confirmUpdate(rel.Version()) {
		return
	}
	if err := applyUpdateAndRestart(app, rel); err != nil {
		showUpdateFailure(err)
	}
}

// handleUpdateCheck answers main.go's "update" IPC case. The CLI does its
// own [y/N] confirmation (or skips it with -y/--yes) rather than the native
// dialog checkForUpdatesInteractive shows, so this only reports what's
// available; "update-apply" (main.go) does the actual install once the CLI
// has confirmed.
func handleUpdateCheck() ipc.Response {
	if !update.ParseableVersion(Version) {
		return ipc.Response{Success: true, Message: "This is a development build, so it can't check for updates.", Update: &ipc.UpdatePayload{Available: false}}
	}

	rel, found, err := update.CheckLatest(context.Background(), Version, BuildVariant)
	if err != nil {
		return ipc.Response{Success: false, Message: "failed to check for updates: " + err.Error()}
	}
	if !found {
		return ipc.Response{Success: true, Message: fmt.Sprintf("mugcup %s is already up to date.", Version), Update: &ipc.UpdatePayload{Available: false}}
	}
	return ipc.Response{
		Success: true,
		Message: fmt.Sprintf("mugcup %s is available (you have %s).", rel.Version(), Version),
		Update:  &ipc.UpdatePayload{Available: true, Version: rel.Version()},
	}
}

// checkForUpdatesInteractive runs a manual, always-confirmed update check
// for the About view's "Check for Updates" button — an explicit, one-off
// request, so (unlike the silent startup auto-check in maybeAutoCheckUpdate)
// every outcome, including a failed check itself, is shown via a native
// dialog rather than left for a caller that isn't necessarily listening.
func checkForUpdatesInteractive(app *walk.Application) {
	if !update.ParseableVersion(Version) {
		showInfo(i18n.T("update.dev_build"))
		return
	}

	rel, found, err := update.CheckLatest(context.Background(), Version, BuildVariant)
	if err != nil {
		showUpdateFailure(err)
		return
	}
	if !found {
		showInfo(fmt.Sprintf(i18n.T("update.up_to_date"), Version))
		return
	}

	if !confirmUpdate(rel.Version()) {
		return
	}
	if err := applyUpdateAndRestart(app, rel); err != nil {
		showUpdateFailure(err)
	}
}

// CheckForUpdates is bound to the frontend as the About view's "Check for
// Updates" button.
func (a *App) CheckForUpdates() error {
	checkForUpdatesInteractive(a.walkApp)
	return nil
}

// applyUpdateAndRestart installs rel and relaunches mugcup.exe from the
// now-updated file, then exits this process. If the relaunch itself can't
// be started, the update is still left installed on disk (it'll take effect
// next time mugcup starts), and the user is told to restart manually rather
// than being left running with no explanation.
func applyUpdateAndRestart(app *walk.Application, rel *selfupdate.Release) error {
	if err := update.Apply(context.Background(), rel); err != nil {
		return err
	}

	guiPath, pathErr := os.Executable()
	if pathErr == nil {
		// Close our IPC listener before spawning the replacement process:
		// otherwise its single-instance check can still reach this
		// (about to exit) process, mistake it for the real running instance,
		// and quit immediately without ever showing the tray icon.
		if stopIPCServer != nil {
			stopIPCServer()
		}

		cmd := exec.Command(guiPath)
		if cmd.Start() == nil {
			_ = cmd.Process.Release()
			go func() {
				time.Sleep(100 * time.Millisecond)
				app.Exit(0)
			}()
			return nil
		}
	}

	showInfo(fmt.Sprintf(i18n.T("update.restart_needed"), rel.Version()))
	return nil
}

func confirmUpdate(newVersion string) bool {
	title, _ := syscall.UTF16PtrFromString(i18n.T("update.available.title"))
	text, _ := syscall.UTF16PtrFromString(fmt.Sprintf(i18n.T("update.available.text"), newVersion, Version))
	ret, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), uintptr(mbYesNo|mbIconQuestion|mbSetForeground))
	return ret == idYes
}

func showUpdateFailure(err error) {
	title, _ := syscall.UTF16PtrFromString(i18n.T("update.failed.title"))
	text, _ := syscall.UTF16PtrFromString(i18n.T("update.failed.prefix") + err.Error())
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), uintptr(mbOK|mbIconWarning|mbSetForeground))
}

func showInfo(message string) {
	title, _ := syscall.UTF16PtrFromString("mugcup")
	text, _ := syscall.UTF16PtrFromString(message)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), uintptr(mbOK|mbIconInformation|mbSetForeground))
}
