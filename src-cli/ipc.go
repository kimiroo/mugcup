package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Request/Response/StatusPayload/ConfigPayload mirror mugcup.exe's protocol
// (src/ipc). The two apps are separate Go modules (destined for separate
// repos later), so the types are duplicated here rather than shared —
// only the JSON tags need to match.
type Request struct {
	Command         string   `json:"command"`
	Args            []string `json:"args"`
	DisplayOn       *bool    `json:"displayOn,omitempty"`
	AutoStart       *bool    `json:"autoStart,omitempty"`
	AutoUpdateCheck *bool    `json:"autoUpdateCheck,omitempty"`
	AutoUpdateApply *bool    `json:"autoUpdateApply,omitempty"`
	Language        *string  `json:"language,omitempty"`
	TrayClickAction *string  `json:"trayClickAction,omitempty"`
	ConfigJSON      string   `json:"configJson,omitempty"`
}

type StatusPayload struct {
	Active         bool   `json:"active"`
	Indefinite     bool   `json:"indefinite"`
	Mode           string `json:"mode"` // "off", "indefinite", "timer", or "schedule"
	RemainingSec   int    `json:"remainingSec"`
	RemainingLabel string `json:"remainingLabel"`
	KeepDisplayOn  bool   `json:"keepDisplayOn"`
	// Until is the Schedule target as RFC3339, alongside RemainingSec rather
	// than instead of it — set only when Mode is "schedule" and Active.
	Until string `json:"until,omitempty"`
}

type ConfigPayload struct {
	AutoStart       bool   `json:"autoStart"`
	KeepDisplayOn   bool   `json:"keepDisplayOn"`
	AutoUpdateCheck bool   `json:"autoUpdateCheck"`
	AutoUpdateApply bool   `json:"autoUpdateApply"`
	TimerList       []int  `json:"timerList"`
	TrayClickAction string `json:"trayClickAction"`
	Language        string `json:"language"` // "auto", "en", or "ko" — see mugcup/i18n
}

type UpdatePayload struct {
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
}

type Response struct {
	Success bool           `json:"success"`
	Message string         `json:"message"`
	Status  *StatusPayload `json:"status,omitempty"`
	Config  *ConfigPayload `json:"config,omitempty"`
	Update  *UpdatePayload `json:"update,omitempty"`
}

const (
	// dialTimeout bounds just the TCP connect: on loopback it either
	// connects or gets refused near-instantly, so this only matters as a
	// ceiling on how long a wedged attempt can block — short lets a caller
	// retry fast instead of waiting on it.
	dialTimeout = 50 * time.Millisecond

	// commandTimeout is the default per-attempt round-trip budget for a
	// command mugcup.exe answers purely from local state (status, config,
	// set, exit, ...). updateTimeout is for "update" specifically, whose
	// server-side handling makes a real network call to GitHub's API and
	// so needs real patience — see runUpdate.
	commandTimeout = 2 * time.Second
	updateTimeout  = 15 * time.Second

	retryBackoff = 100 * time.Millisecond
)

func portFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mugcup", "ipc.port"), nil
}

// dialRunningInstance opens a TCP connection to a running mugcup.exe without
// sending anything. A running instance is one whose port file exists and
// accepts the connection.
func dialRunningInstance() (net.Conn, bool) {
	pPath, err := portFilePath()
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(pPath)
	if err != nil {
		return nil, false
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || port <= 0 {
		return nil, false
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), dialTimeout)
	if err != nil {
		return nil, false
	}
	return conn, true
}

func isRunning() bool {
	conn, ok := dialRunningInstance()
	if ok {
		conn.Close()
	}
	return ok
}

// sendOnce is one dial+send+receive attempt against timeout (the round-trip
// budget after connecting; dialTimeout governs the connect itself). ok=false
// means no running instance was found; ioErr is the raw error behind
// resp.Message (nil on success or when ok=false), for sendToRunningInstance
// to decide whether a retry is worth it.
func sendOnce(req Request, timeout time.Duration) (resp Response, ok bool, ioErr error) {
	conn, found := dialRunningInstance()
	if !found {
		return Response{}, false, nil
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{Success: false, Message: "failed to send the command to the running mugcup: " + describeIOError(err)}, true, err
	}

	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{Success: false, Message: "did not get a response from the running mugcup: " + describeIOError(err)}, true, err
	}
	return resp, true, nil
}

// sendToRunningInstance sends req with the default commandTimeout. ok=false
// means no running instance was found.
func sendToRunningInstance(req Request) (Response, bool) {
	return sendToRunningInstanceTimeout(req, commandTimeout)
}

// sendToRunningInstanceTimeout is sendToRunningInstance with a caller-chosen
// round-trip budget per attempt (see updateTimeout). A fast connection-level
// failure (a reset, a dropped connection — anything but a timeout) gets a
// couple of quick retries before giving up: the very first connection into
// mugcup.exe can transiently reset right around when it finishes starting
// up (see runLaunch) or shuts down, a Windows loopback quirk rather than
// anything wrong with the request itself.
func sendToRunningInstanceTimeout(req Request, timeout time.Duration) (resp Response, ok bool) {
	for attempt := 0; ; attempt++ {
		var ioErr error
		resp, ok, ioErr = sendOnce(req, timeout)
		if ioErr == nil || !ok {
			return resp, ok
		}
		var netErr net.Error
		if attempt >= 2 || (errors.As(ioErr, &netErr) && netErr.Timeout()) {
			return resp, ok
		}
		time.Sleep(retryBackoff)
	}
}

// describeIOError distinguishes why a send/receive failed (timeout, dropped
// connection, or something else) instead of always blaming a timeout.
func describeIOError(err error) string {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timed out waiting for a response"
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "the connection dropped before a response arrived (mugcup may have exited)"
	}
	return "could not read the response: " + err.Error()
}

// waitUntil retries cond every interval until it returns true or timeout elapses.
func waitUntil(timeout, interval time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(interval)
	}
}
