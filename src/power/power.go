package power

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"mugcup/applog"
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")

	powrprof                                   = syscall.NewLazyDLL("powrprof.dll")
	procPowerRegisterSuspendResumeNotification = powrprof.NewProc("PowerRegisterSuspendResumeNotification")
	procCallNtPowerInformation                 = powrprof.NewProc("CallNtPowerInformation")

	logger = applog.New("power")
)

const (
	esContinuous      = 0x80000000
	esSystemRequired  = 0x00000001
	esDisplayRequired = 0x00000002

	deviceNotifyCallback = 2 // DEVICE_NOTIFY_CALLBACK

	pbtAPMResumeSuspend   = 0x7  // PBT_APMRESUMESUSPEND (user-initiated resume)
	pbtAPMResumeAutomatic = 0x12 // PBT_APMRESUMEAUTOMATIC (any resume)

	systemExecutionState = 16 // POWER_INFORMATION_LEVEL SystemExecutionState

	// tickInterval drives everything periodic on the owner thread: resume
	// detection via clock drift, verifying the OS still sees our flags, and
	// re-asserting them. Each tick is a couple of cheap syscalls, so this can
	// be short enough to react almost immediately.
	tickInterval = 5 * time.Second

	// clockJumpThreshold: if the wall clock advances this much more than the
	// monotonic clock between two ticks, the machine was almost certainly
	// suspended in between — treated as a resume even if the OS notification
	// never arrived.
	clockJumpThreshold = 10 * time.Second

	// staleAfter: the watchdog replaces the owner thread if it hasn't
	// completed a tick for this long.
	staleAfter = 3 * tickInterval

	// applyTimeout bounds how long Apply waits on the owner thread, so a
	// wedged owner can't freeze callers (which hold the Controller's lock).
	applyTimeout = 3 * time.Second
)

// Why a dedicated thread: SetThreadExecutionState's ES_CONTINUOUS flags
// belong to the *OS thread* that called it, not the process. Go goroutines
// hop between OS threads freely, so calling it from whatever goroutine
// happened to change state (tray click, timer expiry, IPC, Wails binding)
// meant that:
//   - "off" could be issued on a different thread than "on", leaving the
//     original thread's flags in place (stuck awake), and toggling
//     Keep-display-on could leave stale display flags on another thread;
//   - the thread holding the flags could later be claimed by a goroutine that
//     calls runtime.LockOSThread and exits without unlocking (the Wails popup
//     goroutine does exactly that when its window closes), at which point Go
//     terminates that OS thread and Windows silently drops its flags (sleep
//     prevention lost).
// Owning the state from a single goroutine pinned to its own OS thread for
// the process lifetime removes all of that.
//
// Defense in depth on top of that, every tickInterval:
//   - the owner asks the OS (CallNtPowerInformation) whether the required
//     flags are actually in effect, logs if they aren't, and re-asserts;
//   - a separate watchdog goroutine checks the owner's heartbeat and, if the
//     owner stopped ticking for any reason, starts a fresh owner on a fresh
//     thread that re-applies the desired state.

type request struct {
	active, display bool
	done            chan error
}

// owner is one instance of the thread that holds the execution state. Only
// the one in current is live; a replaced owner exits when it next wakes up.
type owner struct {
	requests  chan request
	heartbeat atomic.Int64 // monotonic-ish nanoseconds (see monoNow) of the last completed loop iteration
}

var (
	startOnce sync.Once
	resumed   = make(chan struct{}, 1)
	epoch     = time.Now() // reference for monoNow

	// desired is the last state requested through Apply. It lives outside
	// the owner so a replacement owner can re-apply it.
	desiredMu      sync.Mutex
	desiredActive  bool
	desiredDisplay bool

	current atomic.Pointer[owner]

	hooksMu     sync.Mutex
	resumeHooks []func()

	// resumeCallback must stay referenced for the process lifetime: Windows
	// holds on to the pointer passed to PowerRegisterSuspendResumeNotification.
	resumeCallback  uintptr
	subscribeParams deviceNotifySubscribeParameters
	notifyHandle    uintptr
)

// DEVICE_NOTIFY_SUBSCRIBE_PARAMETERS
type deviceNotifySubscribeParameters struct {
	callback uintptr
	context  uintptr
}

func monoNow() int64 { return int64(time.Since(epoch)) }

// Apply reflects the given state to the OS. active=false clears the
// sleep-prevention flags (a lone ES_CONTINUOUS call). Safe to call from any
// goroutine; it's forwarded to the owner thread and waits for the result.
func Apply(active, keepDisplayOn bool) error {
	startOnce.Do(start)
	desiredMu.Lock()
	desiredActive, desiredDisplay = active, keepDisplayOn
	desiredMu.Unlock()

	o := current.Load()
	done := make(chan error, 1)
	timeout := time.NewTimer(applyTimeout)
	defer timeout.Stop()
	select {
	case o.requests <- request{active: active, display: keepDisplayOn, done: done}:
	case <-timeout.C:
		logger.Println("owner thread not responding to Apply; replacing it")
		replaceOwner(o)
		return errors.New("power: owner thread not responding")
	}
	select {
	case err := <-done:
		return err
	case <-timeout.C:
		logger.Println("owner thread didn't finish Apply in time; replacing it")
		replaceOwner(o)
		return errors.New("power: owner thread not responding")
	}
}

// OnResume registers fn to run (on its own goroutine) after the system
// resumes from sleep/hibernate, once the execution state has already been
// re-asserted. Callers use it to re-check wall-clock deadlines that may have
// passed while the machine was suspended.
func OnResume(fn func()) {
	startOnce.Do(start)
	hooksMu.Lock()
	resumeHooks = append(resumeHooks, fn)
	hooksMu.Unlock()
}

func start() {
	o := newOwner()
	current.Store(o)
	go o.loop()
	go watchdog()
	registerResumeNotification()
}

func newOwner() *owner {
	o := &owner{requests: make(chan request)}
	o.heartbeat.Store(monoNow())
	return o
}

// replaceOwner swaps in a fresh owner (fresh goroutine, fresh OS thread) if
// stale is still the current one. The new owner re-applies the desired state
// as its first action. Safe to call concurrently; only one swap wins.
func replaceOwner(stale *owner) {
	o := newOwner()
	if !current.CompareAndSwap(stale, o) {
		return // someone else already replaced it
	}
	go o.loop()
}

func (o *owner) loop() {
	// Never unlocked: while this owner is live, this OS thread — and the
	// execution state attached to it — stays put. If this owner is ever
	// replaced it returns, and Go terminates the thread, which also drops
	// its now-redundant flags (the replacement holds its own).
	runtime.LockOSThread()

	desiredMu.Lock()
	active, display := desiredActive, desiredDisplay
	desiredMu.Unlock()
	if active {
		_ = set(active, display)
	}

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	lastWall, lastMono := time.Now().Round(0), time.Now()

	for {
		if current.Load() != o {
			logger.Println("replaced owner thread exiting")
			return
		}
		o.heartbeat.Store(monoNow())

		select {
		case r := <-o.requests:
			active, display = r.active, r.display
			r.done <- set(active, display)

		case <-ticker.C:
			// Detect a suspend the notification may have missed: wall clock
			// moved on while the monotonic clock (which may not count time
			// spent suspended) did not keep up.
			nowWall, nowMono := time.Now().Round(0), time.Now()
			drift := nowWall.Sub(lastWall) - nowMono.Sub(lastMono)
			lastWall, lastMono = nowWall, nowMono
			if drift > clockJumpThreshold {
				logger.Printf("wall clock jumped %v ahead of monotonic clock; treating as resume", drift.Round(time.Second))
				handleResume(active, display)
				continue
			}
			// Re-read the desired state rather than trusting only what this
			// owner was told: if an Apply raced an owner replacement and
			// landed on the old one, this is where the new one catches up.
			desiredMu.Lock()
			active, display = desiredActive, desiredDisplay
			desiredMu.Unlock()
			if active {
				verify(display)
				// Re-assert every tick regardless: cheap, and restores this
				// thread's flags if anything reset them.
				_ = set(active, display)
			}

		case <-resumed:
			lastWall, lastMono = time.Now().Round(0), time.Now()
			handleResume(active, display)
		}
	}
}

// watchdog replaces the owner if its heartbeat goes stale — the owner thread
// is gone or wedged for some reason not otherwise accounted for.
func watchdog() {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	last := monoNow()
	for range ticker.C {
		now := monoNow()
		// If the watchdog itself was delayed this long (system suspended, or
		// the whole process stalled), the owner's stale heartbeat says
		// nothing about the owner — give it a fresh interval to catch up.
		delayed := time.Duration(now-last) > 2*tickInterval
		last = now
		if delayed {
			continue
		}
		o := current.Load()
		if age := time.Duration(now - o.heartbeat.Load()); age > staleAfter {
			logger.Printf("owner thread heartbeat is %v old; replacing it", age.Round(time.Second))
			replaceOwner(o)
		}
	}
}

// verify asks the OS which execution-state flags are currently in effect
// system-wide and logs if ours are missing. The value is an aggregate across
// all processes, so it can only prove a flag is missing, never that ours is
// the one providing it — which is all a "was it lost?" check needs.
func verify(display bool) {
	if procCallNtPowerInformation.Find() != nil {
		return
	}
	var state uint32
	r, _, _ := procCallNtPowerInformation.Call(
		systemExecutionState, 0, 0,
		uintptr(unsafe.Pointer(&state)), unsafe.Sizeof(state),
	)
	if r != 0 { // NTSTATUS; STATUS_SUCCESS == 0
		return
	}
	want := uint32(esSystemRequired)
	if display {
		want |= esDisplayRequired
	}
	missing := state&want != want
	// Log once per loss, not every tick while it persists.
	if missing && !lossLogged.Swap(true) {
		logger.Printf("sleep prevention was lost (system execution state 0x%x, want 0x%x); re-asserting", state, want)
	} else if !missing && lossLogged.Swap(false) {
		logger.Println("sleep prevention restored")
	}
}

var lossLogged atomic.Bool

func handleResume(active, display bool) {
	if active {
		logger.Println("system resumed; re-asserting sleep prevention")
		_ = set(active, display)
	}
	hooksMu.Lock()
	hooks := append([]func(){}, resumeHooks...)
	hooksMu.Unlock()
	for _, h := range hooks {
		go h()
	}
}

func set(active, keepDisplayOn bool) error {
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

// registerResumeNotification subscribes to suspend/resume events via a
// callback (no window needed). If it fails, the clock-drift check in
// owner.loop still catches resumes, just up to tickInterval later.
func registerResumeNotification() {
	if err := procPowerRegisterSuspendResumeNotification.Find(); err != nil {
		logger.Printf("PowerRegisterSuspendResumeNotification unavailable: %v", err)
		return
	}
	resumeCallback = syscall.NewCallback(func(context, typ, setting uintptr) uintptr {
		if typ == pbtAPMResumeAutomatic || typ == pbtAPMResumeSuspend {
			select {
			case resumed <- struct{}{}:
			default: // one already pending
			}
		}
		return 0
	})
	subscribeParams = deviceNotifySubscribeParameters{callback: resumeCallback}
	r, _, _ := procPowerRegisterSuspendResumeNotification.Call(
		deviceNotifyCallback,
		uintptr(unsafe.Pointer(&subscribeParams)),
		uintptr(unsafe.Pointer(&notifyHandle)),
	)
	if r != 0 { // returns a Win32 error code; ERROR_SUCCESS == 0
		logger.Printf("PowerRegisterSuspendResumeNotification failed: error %d", r)
	}
}
