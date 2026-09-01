package power

import "syscall"

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
)

const (
	esContinuous      = 0x80000000
	esSystemRequired  = 0x00000001
	esDisplayRequired = 0x00000002
)

// Apply는 현재 상태를 실제 OS에 반영한다.
// active=false면 절전 방지를 해제(ES_CONTINUOUS만 단독 호출)한다.
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
		return err
	}
	return nil
}
