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
	ConfigJSON      string   `json:"configJson,omitempty"`
}

type StatusPayload struct {
	Active         bool   `json:"active"`
	Infinite       bool   `json:"infinite"`
	Mode           string `json:"mode"` // "off", "infinite", "timer", or "schedule"
	RemainingSec   int    `json:"remainingSec"`
	RemainingLabel string `json:"remainingLabel"`
	KeepDisplayOn  bool   `json:"keepDisplayOn"`
}

type ConfigPayload struct {
	AutoStart       bool   `json:"autoStart"`
	KeepDisplayOn   bool   `json:"keepDisplayOn"`
	AutoUpdateCheck bool   `json:"autoUpdateCheck"`
	AutoUpdateApply bool   `json:"autoUpdateApply"`
	TimerList       []int  `json:"timerList"`
	TrayClickAction string `json:"trayClickAction"`
}

type Response struct {
	Success bool           `json:"success"`
	Message string         `json:"message"`
	Status  *StatusPayload `json:"status,omitempty"`
	Config  *ConfigPayload `json:"config,omitempty"`
}

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
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 1*time.Second)
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

// sendToRunningInstance sends req to a running mugcup.exe. ok=false means no
// running instance was found.
func sendToRunningInstance(req Request) (resp Response, ok bool) {
	conn, found := dialRunningInstance()
	if !found {
		return Response{}, false
	}
	defer conn.Close()
	// Bound the wait so a non-responsive instance can't block forever.
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{Success: false, Message: "failed to send the command to the running mugcup: " + describeIOError(err)}, true
	}

	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{Success: false, Message: "did not get a response from the running mugcup: " + describeIOError(err)}, true
	}
	return resp, true
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
