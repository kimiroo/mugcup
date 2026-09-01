package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	guiExecutableName  = "mugcup.exe"
	launchReadyTimeout = 5 * time.Second
	stopTimeout        = 5 * time.Second
	pollInterval       = 100 * time.Millisecond
)

// guiExecutablePath finds mugcup.exe next to mugcup-cli.exe — build output
// always places both in the same folder (build/<arch>/).
func guiExecutablePath() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("could not resolve this executable's path: %w", err)
	}
	guiPath := filepath.Join(filepath.Dir(self), guiExecutableName)
	if _, err := os.Stat(guiPath); err != nil {
		return "", fmt.Errorf("could not find %s (it must be in the same folder as mugcup-cli.exe): %w", guiExecutableName, err)
	}
	return guiPath, nil
}

// runLaunch starts mugcup.exe if it isn't already running and waits until its
// IPC server actually responds before returning.
func runLaunch(opts options) Response {
	if isRunning() {
		return applySettingsOnly(opts, Response{Success: true, Message: "mugcup is already running."})
	}

	guiPath, err := guiExecutablePath()
	if err != nil {
		return Response{Success: false, Message: err.Error()}
	}

	cmd := exec.Command(guiPath)
	if err := cmd.Start(); err != nil {
		return Response{Success: false, Message: "failed to start mugcup: " + err.Error()}
	}
	// The child is an independent tray app; don't wait for it to exit.
	_ = cmd.Process.Release()

	if !waitUntil(launchReadyTimeout, pollInterval, isRunning) {
		return Response{Success: false, Message: "could not confirm mugcup started (timed out)."}
	}

	return applySettingsOnly(opts, Response{Success: true, Message: "Started mugcup."})
}

// applySettingsOnly makes one more IPC call after launch when -d, --auto-start,
// or --auto-update was given, so "launch with these settings" works in one command.
func applySettingsOnly(opts options, fallback Response) Response {
	if !opts.hasSetting() {
		return fallback
	}
	resp, ok := sendToRunningInstance(Request{Command: "set", DisplayOn: opts.displayOn, AutoStart: opts.autoStart, AutoUpdate: opts.autoUpdate})
	if !ok {
		return fallback
	}
	return resp
}

// runExit asks the running mugcup.exe to quit and waits until the process is
// actually gone.
func runExit(opts options) Response {
	resp, ok := sendToRunningInstance(Request{Command: "exit", DisplayOn: opts.displayOn, AutoStart: opts.autoStart, AutoUpdate: opts.autoUpdate})
	if !ok {
		return Response{Success: false, Message: "mugcup is not currently running."}
	}
	if !resp.Success {
		return resp
	}
	if !waitUntil(stopTimeout, pollInterval, func() bool { return !isRunning() }) {
		return Response{Success: false, Message: "could not confirm mugcup exited (timed out)."}
	}
	return resp
}
