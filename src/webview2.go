package main

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// webview2ClientGUID is the Evergreen WebView2 Runtime's stable product ID.
// Its presence under either EdgeUpdate registration key means the runtime is installed.
const webview2ClientGUID = `Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

func isWebView2Installed() bool {
	roots := []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER}
	subpaths := []string{
		`SOFTWARE\Microsoft\EdgeUpdate\` + webview2ClientGUID,
		`SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\` + webview2ClientGUID,
	}
	for _, root := range roots {
		for _, path := range subpaths {
			k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
			if err == nil {
				k.Close()
				return true
			}
		}
	}
	return false
}

func showWebView2MissingWarning() {
	title, _ := syscall.UTF16PtrFromString("mugcup")
	text, _ := syscall.UTF16PtrFromString("The Settings window needs the Microsoft Edge WebView2 Runtime, which isn't installed on this PC.\n\nInstall it from https://developer.microsoft.com/microsoft-edge/webview2/ and try again. The keep-awake tray feature works fine without it.")
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), uintptr(mbOK|mbIconWarning))
}
