package power

import (
	"syscall"

	"mugcup/applog"
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")

	logger = applog.New("power")
)

const (
	esContinuous      = 0x80000000
	esSystemRequired  = 0x00000001
	esDisplayRequired = 0x00000002
)

// Apply reflects the given state to the OS. active=false clears the
// sleep-prevention flags (a lone ES_CONTINUOUS call).
func Apply(active, keepDisplayOn bool) error {
	var flags uintptr = esContinuous
	if active {
		flags |= esSystemRequired
		if keepDisplayOn {
			flags |= esDisplayRequired
		}
	}
	r, _, err := procSetThreadExecutionState.Call(flags)
	if r == 0 {
		logger.Printf("SetThreadExecutionState failed (active=%v, keepDisplayOn=%v): %v", active, keepDisplayOn, err)
		return err
	}
	return nil
}
