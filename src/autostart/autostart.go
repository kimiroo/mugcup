// Package autostart syncs mugcup's "start with Windows" setting to the
// per-user Run registry key. It's deliberately stateless: Sync just makes
// the registry match what enabled says, so it's safe to call on every
// launch and every config change without tracking prior state.
package autostart

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	valueName  = "mugcup"
)

// Sync ensures the registry either does (enabled=true) or does not
// (enabled=false) launch mugcup.exe at sign-in, overwriting whatever is
// there if it doesn't already match.
func Sync(enabled bool) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("failed to open the Run registry key: %w", err)
	}
	defer key.Close()

	if !enabled {
		if err := key.DeleteValue(valueName); err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("failed to remove the auto-start registry value: %w", err)
		}
		return nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve mugcup's own executable path: %w", err)
	}
	want := `"` + exePath + `"`

	if current, _, err := key.GetStringValue(valueName); err == nil && current == want {
		return nil
	}

	if err := key.SetStringValue(valueName, want); err != nil {
		return fmt.Errorf("failed to set the auto-start registry value: %w", err)
	}
	return nil
}
