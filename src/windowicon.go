package main

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"mugcup/assets"
)

// popupWindowClassName is set on the Wails window (see wailsapp.go) so
// setPopupWindowIcon can reliably find its HWND with FindWindowW below.
const popupWindowClassName = "mugcupPopup"

var (
	procFindWindowW  = user32.NewProc("FindWindowW")
	procSendMessageW = user32.NewProc("SendMessageW")
	procLoadImageW   = user32.NewProc("LoadImageW")
)

const (
	wmSetIcon = 0x0080
	iconSmall = 0
	iconBig   = 1

	imageIcon      = 1
	lrLoadFromFile = 0x00000010
	lrDefaultSize  = 0x00000040
)

var (
	iconFileOnce sync.Once
	iconFilePath string
)

// extractIconFile writes the embedded tray icon to a temp file once, since
// LoadImageW(LR_LOADFROMFILE) needs a path rather than in-memory bytes.
func extractIconFile() string {
	iconFileOnce.Do(func() {
		path := filepath.Join(os.TempDir(), "mugcup-window-icon.ico")
		if err := os.WriteFile(path, assets.IconICO, 0644); err == nil {
			iconFilePath = path
		} else {
			popupLogger.Printf("failed to extract the window icon to %s: %v", path, err)
		}
	})
	return iconFilePath
}

// setPopupWindowIcon gives the Wails popup window a taskbar/title-bar/Alt-Tab
// icon loaded directly from our own embedded icon.ico, instead of relying on
// Wails' built-in behavior of loading it from the exe's PE resources: Wails
// hardcodes resource ID 3 for that (winc.AppIconID in
// wailsapp/wails/v2/internal/frontend/desktop/windows/winc), which our own
// resource_windows_*.syso (icon + manifest + version info, embedded via go
// tool goversioninfo — see build.ps1) doesn't happen to produce at that
// exact ID, so Wails' own lookup silently finds nothing and leaves the
// window with no icon.
func setPopupWindowIcon() {
	path := extractIconFile()
	if path == "" {
		return
	}
	classPtr, err := syscall.UTF16PtrFromString(popupWindowClassName)
	if err != nil {
		return
	}
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(classPtr)), 0)
	if hwnd == 0 {
		popupLogger.Println("setPopupWindowIcon: window handle not found")
		return
	}

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	if big, _, _ := procLoadImageW.Call(0, uintptr(unsafe.Pointer(pathPtr)), imageIcon, 0, 0, lrLoadFromFile|lrDefaultSize); big != 0 {
		procSendMessageW.Call(hwnd, wmSetIcon, iconBig, big)
	}
	if small, _, _ := procLoadImageW.Call(0, uintptr(unsafe.Pointer(pathPtr)), imageIcon, 16, 16, lrLoadFromFile); small != 0 {
		procSendMessageW.Call(hwnd, wmSetIcon, iconSmall, small)
	}
}
