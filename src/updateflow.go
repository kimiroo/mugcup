package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

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

	rel, found, err := update.CheckLatest(context.Background(), Version)
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

// CheckForUpdates is bound to the frontend as the About view's "Check for
// Updates" button. Unlike the automatic check, it always asks for
// confirmation before installing — it's an explicit, interactive action.
func (a *App) CheckForUpdates() error {
	if !update.ParseableVersion(Version) {
		showInfo("This is a development build, so it can't check for updates.")
		return nil
	}

	rel, found, err := update.CheckLatest(context.Background(), Version)
	if err != nil {
		return err
	}
	if !found {
		showInfo(fmt.Sprintf("mugcup %s is already up to date.", Version))
		return nil
	}

	if !confirmUpdate(rel.Version()) {
		return nil
	}
	if err := applyUpdateAndRestart(a.walkApp, rel); err != nil {
		showUpdateFailure(err)
	}
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

	showInfo(fmt.Sprintf("mugcup was updated to %s. Please restart it to finish.", rel.Version()))
	return nil
}

func confirmUpdate(newVersion string) bool {
	title, _ := syscall.UTF16PtrFromString("mugcup update available")
	text, _ := syscall.UTF16PtrFromString(fmt.Sprintf(
		"mugcup %s is available (you have %s).\n\nInstall it now? mugcup will restart.",
		newVersion, Version,
	))
	ret, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), uintptr(mbYesNo|mbIconQuestion|mbSetForeground))
	return ret == idYes
}

func showUpdateFailure(err error) {
	title, _ := syscall.UTF16PtrFromString("mugcup update failed")
	text, _ := syscall.UTF16PtrFromString("Failed to update mugcup:\n\n" + err.Error())
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), uintptr(mbOK|mbIconWarning|mbSetForeground))
}

func showInfo(message string) {
	title, _ := syscall.UTF16PtrFromString("mugcup")
	text, _ := syscall.UTF16PtrFromString(message)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), uintptr(mbOK|mbIconInformation|mbSetForeground))
}
