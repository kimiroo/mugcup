package ipc

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mugcup/applog"
)

var logger = applog.New("ipc")

type Request struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	// DisplayOn, AutoStart, AutoUpdateCheck, and AutoUpdateApply are
	// tri-state: nil=unset (keep config), non-nil=apply and persist
	// immediately. They're accepted alongside any command (so e.g.
	// "start 30m -d true" both starts a timer and updates the display
	// setting), and are also what the standalone "set" command uses to
	// change settings that no other command touches.
	DisplayOn       *bool `json:"displayOn,omitempty"`
	AutoStart       *bool `json:"autoStart,omitempty"`
	AutoUpdateCheck *bool `json:"autoUpdateCheck,omitempty"`
	AutoUpdateApply *bool `json:"autoUpdateApply,omitempty"`
	// ConfigJSON carries the full config to apply for the "import" command.
	ConfigJSON string `json:"configJson,omitempty"`
}

type StatusPayload struct {
	Active         bool   `json:"active"`
	Indefinite     bool   `json:"indefinite"`
	Mode           string `json:"mode"` // "off", "indefinite", "timer", or "schedule" — see settings.Mode
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

// UpdatePayload reports the outcome of an "update" check — Version is only
// set when Available is true.
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

func portFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	appDir := filepath.Join(dir, "mugcup")
	_ = os.MkdirAll(appDir, 0755)
	return filepath.Join(appDir, "ipc.port"), nil
}

// TrySendToExisting sends req to an already-running instance, if any.
func TrySendToExisting(req Request) (Response, bool) {
	pPath, err := portFilePath()
	if err != nil {
		return Response{}, false
	}
	data, err := os.ReadFile(pPath)
	if err != nil {
		return Response{}, false
	}
	portStr := strings.TrimSpace(string(data))
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		return Response{}, false
	}

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		// Not running; the port file is stale (e.g. left behind by a crash).
		_ = os.Remove(pPath)
		return Response{}, false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{Success: false, Message: "failed to send the command to the running mugcup: " + err.Error()}, true
	}

	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{Success: false, Message: "timed out waiting for a response from the running mugcup."}, true
	}

	return resp, true
}

// StartServer starts the single-instance IPC server.
func StartServer(handler func(Request) Response) (func(), error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		logger.Printf("failed to listen on a loopback port: %v", err)
		return nil, err
	}

	tcpAddr := ln.Addr().(*net.TCPAddr)
	pPath, err := portFilePath()
	if err != nil {
		logger.Printf("failed to resolve the port file path: %v", err)
		ln.Close()
		return nil, err
	}

	if err := os.WriteFile(pPath, []byte(strconv.Itoa(tcpAddr.Port)), 0644); err != nil {
		logger.Printf("failed to write the port file: %v", err)
		ln.Close()
		return nil, err
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				// Expected on normal shutdown (cleanup calls ln.Close(),
				// which unblocks Accept with this same error) — only note it
				// when it happens for any other reason.
				if !strings.Contains(err.Error(), "use of closed network connection") {
					logger.Printf("IPC listener stopped unexpectedly: %v", err)
				}
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				var req Request
				if err := json.NewDecoder(c).Decode(&req); err != nil {
					return
				}
				resp := handler(req)
				_ = json.NewEncoder(c).Encode(resp)
			}(conn)
		}
	}()

	cleanup := func() {
		ln.Close()
		_ = os.Remove(pPath)
	}

	return cleanup, nil
}
