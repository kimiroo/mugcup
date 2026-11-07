package power

import (
	"testing"
	"unsafe"
)

func systemState(t *testing.T) uint32 {
	t.Helper()
	var state uint32
	r, _, _ := procCallNtPowerInformation.Call(
		systemExecutionState, 0, 0,
		uintptr(unsafe.Pointer(&state)), unsafe.Sizeof(state),
	)
	if r != 0 {
		t.Skipf("CallNtPowerInformation failed: 0x%x", r)
	}
	return state
}

// Apply must take effect system-wide even though it's called from a test
// goroutine, not the owner thread. (Clearing isn't asserted: another process
// may hold the same flags.)
func TestApplyReachesOS(t *testing.T) {
	if err := Apply(true, true); err != nil {
		t.Fatalf("Apply(true, true): %v", err)
	}
	defer Apply(false, false)
	want := uint32(esSystemRequired | esDisplayRequired)
	if got := systemState(t); got&want != want {
		t.Errorf("system execution state 0x%x missing 0x%x after Apply", got, want)
	}
}

// A replaced owner must hand the desired state to its successor.
func TestReplaceOwnerKeepsState(t *testing.T) {
	if err := Apply(true, false); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	defer Apply(false, false)
	old := current.Load()
	replaceOwner(old)
	if current.Load() == old {
		t.Fatal("owner was not replaced")
	}
	// Round-trip through the new owner so it has started and applied state.
	if err := Apply(true, false); err != nil {
		t.Fatalf("Apply after replace: %v", err)
	}
	if got := systemState(t); got&esSystemRequired == 0 {
		t.Errorf("system execution state 0x%x lost ES_SYSTEM_REQUIRED after owner replacement", got)
	}
}
