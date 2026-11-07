package settings

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"mugcup/power"
)

// ---- Persisted config (the only data saved as JSON) ----

type TrayClickAction string

const (
	ActionCycle      TrayClickAction = "cycle"      // cycle through presets
	ActionIndefinite TrayClickAction = "indefinite" // keep on indefinitely
	ActionOpenMenu   TrayClickAction = "menu"       // open tray menu
)

type Config struct {
	AutoStart     bool `json:"autoStart"`
	KeepDisplayOn bool `json:"keepDisplayOn"`
	// AutoUpdateCheck periodically checks GitHub for a newer release.
	// AutoUpdateApply, only meaningful when AutoUpdateCheck is also on,
	// installs it without asking; otherwise a found update just prompts for
	// confirmation before installing.
	AutoUpdateCheck bool            `json:"autoUpdateCheck"`
	AutoUpdateApply bool            `json:"autoUpdateApply"`
	TimerList       []int           `json:"timerList"` // seconds, 0 = indefinite
	TrayClickAction TrayClickAction `json:"trayClickAction"`
	// Language is "auto" (follow the OS's UI language), "en", or "ko" — see
	// mugcup/i18n.Lang, which this deliberately doesn't import (settings
	// stays independent of presentation; main.go wires the two together).
	Language string `json:"language"`
}

func DefaultConfig() Config {
	return Config{
		AutoStart:       false,
		KeepDisplayOn:   false,
		AutoUpdateCheck: true,
		AutoUpdateApply: false,
		TimerList:       []int{15 * 60, 30 * 60, 60 * 60, 2 * 60 * 60, 0},
		TrayClickAction: ActionCycle,
		Language:        "auto",
	}
}

func ValidateConfig(cfg Config) error {
	if len(cfg.TimerList) == 0 {
		return errors.New("at least one timer is required")
	}
	for _, seconds := range cfg.TimerList {
		if seconds < 0 {
			return fmt.Errorf("timer value must be >= 0: %d", seconds)
		}
	}
	switch cfg.TrayClickAction {
	case "", ActionCycle, ActionIndefinite, ActionOpenMenu:
	default:
		return fmt.Errorf("unknown tray click action: %q", cfg.TrayClickAction)
	}
	switch cfg.Language {
	case "", "auto", "en", "ko":
	default:
		return fmt.Errorf("unknown language: %q", cfg.Language)
	}
	return nil
}

// Mode is how the current state was started, for callers that display it
// differently per-origin (the tray's icon, the CLI's status output) even
// though Timer and Schedule behave identically once running (both just
// count down to off).
type Mode string

const (
	ModeOff        Mode = "off"        // TurnOff, or a timer that ran out
	ModeIndefinite Mode = "indefinite" // SetIndefinite, or a preset/Cycle landing on an indefinite entry
	ModeTimer      Mode = "timer"      // SetPreset/Cycle on a timed entry, or SetCustomDuration ("for a duration")
	ModeSchedule   Mode = "schedule"   // SetSchedule ("until a date & time")
)

// Label renders a Mode for display (tray icon tooltip, CLI status output).
func (m Mode) Label() string {
	switch m {
	case ModeIndefinite:
		return "Indefinite"
	case ModeTimer:
		return "Timer"
	case ModeSchedule:
		return "Schedule"
	default:
		return "Off"
	}
}

// ---- Runtime state (mirrored to state.json on every change — see
// PersistedState below and Controller.apply, its only writer) ----

type State struct {
	Active     bool
	Indefinite bool
	ExpiresAt  time.Time // valid only when Active && !Indefinite
	Mode       Mode

	// PresetActive is true when the current state was started from an entry
	// in Config.TimerList (via SetPreset or Cycle landing on one), as opposed
	// to SetIndefinite, SetCustomDuration, or SetSchedule. PresetSeconds
	// (valid only when PresetActive) holds that entry's value, so callers
	// can tell which preset is currently selected.
	PresetActive  bool
	PresetSeconds int
}

// RemainingLabel renders State for display. A Schedule timer ("until a date
// & time") shows its target instead of a countdown, since that's what the
// user actually picked; every other active state (preset, custom duration)
// still counts down, since those were specified as a duration in the first
// place.
func (s State) RemainingLabel() string {
	if !s.Active {
		return "Off"
	}
	if s.Indefinite {
		return "Indefinite"
	}
	if s.Mode == ModeSchedule {
		return "Until " + FormatUntil(s.ExpiresAt)
	}
	remaining := time.Until(s.ExpiresAt)
	if remaining < 0 {
		remaining = 0
	}
	h := int(remaining.Hours())
	m := int(remaining.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm left", h, m)
	}
	return fmt.Sprintf("%dm left", m)
}

// FormatUntil renders t as a Schedule target: just the clock time when t
// falls on today, or date-and-time when it doesn't, so a schedule that
// crosses midnight isn't misread as today. Both use the OS locale's own
// format (localDate/localTime, localetime.go) where available, falling
// back to a fixed ISO-like layout ("15:04" / "2006-01-02 15:04") if that
// Windows API call fails.
func FormatUntil(t time.Time) string {
	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		if s, ok := localTime(t); ok {
			return s
		}
		return t.Format("15:04")
	}
	if dateStr, ok := localDate(t); ok {
		if timeStr, ok := localTime(t); ok {
			return dateStr + " " + timeStr
		}
	}
	return t.Format("2006-01-02 15:04")
}

// ---- Persisted state (state.json — resuming a timer across a restart) ----

// PersistedState is the on-disk snapshot of State written to state.json on
// every state change (Controller.apply) so a restart can resume an
// in-progress timer instead of always coming up Off. It's a separate type
// from State rather than State itself because it also carries TimerIndex,
// which State doesn't (that's Cycle's position in Config.TimerList, needed
// to resume cycling from the right spot, but meaningless outside the
// Controller); PresetActive/PresetSeconds aren't needed here since apply
// re-derives them from TimerIndex the same way every other caller does.
type PersistedState struct {
	Active     bool      `json:"active"`
	Indefinite bool      `json:"indefinite"`
	ExpiresAt  time.Time `json:"expiresAt"`
	Mode       Mode      `json:"mode"`
	TimerIndex int       `json:"timerIndex"`
}

// String renders a PersistedState for logging (state.json read/write).
func (s PersistedState) String() string {
	return fmt.Sprintf("active=%v indefinite=%v mode=%s expiresAt=%s timerIndex=%d",
		s.Active, s.Indefinite, s.Mode, s.ExpiresAt.Format(time.RFC3339), s.TimerIndex)
}

// ---- Controller: owns both config and runtime state ----

type Controller struct {
	mu         sync.Mutex
	cfg        Config
	state      State
	timerIndex int // position in TimerList cycle, -1 = outside the cycle (off/custom)
	timer      *time.Timer
	// gen increments on every state transition. A timer's expiry callback
	// captures the gen it was armed for and does nothing if the state has
	// moved on since — otherwise a timer that fired just as the user started
	// something new (already past Stop, waiting on mu) would turn the new
	// state off immediately.
	gen          uint64
	cfgListeners []func(Config)
	stListeners  []func(State)
}

func NewController(cfg Config) *Controller {
	if cfg.TrayClickAction == "" {
		cfg.TrayClickAction = ActionCycle
	}
	if cfg.Language == "" {
		cfg.Language = "auto"
	}
	return &Controller{cfg: cfg, timerIndex: -1, state: State{Mode: ModeOff}}
}

func (c *Controller) Config() Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

func (c *Controller) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *Controller) OnConfigChange(fn func(Config)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfgListeners = append(c.cfgListeners, fn)
}

func (c *Controller) OnStateChange(fn func(State)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stListeners = append(c.stListeners, fn)
}

// UpdateConfig is the single entry point every settings change goes through
// (tray toggles, the popup window's Save/Import, the CLI's "set"/"import"
// commands), so a failure here — however it was triggered — is logged right
// here rather than duplicated at each caller.
func (c *Controller) UpdateConfig(cfg Config) error {
	if cfg.TrayClickAction == "" {
		cfg.TrayClickAction = ActionCycle
	}
	if cfg.Language == "" {
		cfg.Language = "auto"
	}
	if err := ValidateConfig(cfg); err != nil {
		logger.Printf("rejected config update: %v", err)
		return err
	}
	if err := SaveConfig(cfg); err != nil { // store.go
		logger.Printf("failed to save config: %v", err)
		return err
	}
	c.mu.Lock()
	c.cfg = cfg
	listeners := append([]func(Config){}, c.cfgListeners...)
	// Re-applied under mu, from the current state rather than a snapshot
	// read earlier, so a concurrent TurnOff can't be overwritten by a stale
	// "active" here (which used to leave the machine stuck awake while Off).
	_ = power.Apply(c.state.Active, cfg.KeepDisplayOn)
	c.mu.Unlock()

	for _, l := range listeners {
		l(cfg)
	}
	return nil
}

func (c *Controller) ToggleKeepDisplayOn() error {
	c.mu.Lock()
	newCfg := c.cfg
	newCfg.KeepDisplayOn = !newCfg.KeepDisplayOn
	c.mu.Unlock()
	return c.UpdateConfig(newCfg)
}

// Cycle steps through TimerList in order (tray left-click) and turns off at the end.
func (c *Controller) Cycle() {
	c.mu.Lock()
	list := c.cfg.TimerList
	next := c.timerIndex + 1
	if next >= len(list) {
		next = -1
	}
	c.timerIndex = next
	var n notification
	switch {
	case next == -1:
		n = c.applyLocked(0, false, ModeOff)
	case list[next] <= 0:
		n = c.applyLocked(0, true, ModeIndefinite)
	default:
		n = c.applyLocked(time.Duration(list[next])*time.Second, false, ModeTimer)
	}
	c.mu.Unlock()
	n.deliver()
}

func (c *Controller) SetIndefinite() {
	c.mu.Lock()
	c.timerIndex = -1
	n := c.applyLocked(0, true, ModeIndefinite)
	c.mu.Unlock()
	n.deliver()
}

func (c *Controller) ToggleIndefinite() {
	c.mu.Lock()
	c.timerIndex = -1
	var n notification
	if c.state.Active && c.state.Indefinite {
		n = c.applyLocked(0, false, ModeOff)
	} else {
		n = c.applyLocked(0, true, ModeIndefinite)
	}
	c.mu.Unlock()
	n.deliver()
}

// SetPreset: seconds<=0 means indefinite; >0 keeps it on for that duration (Indefinite=false).
func (c *Controller) SetPreset(seconds int) {
	c.mu.Lock()
	c.timerIndex = -1
	for i, s := range c.cfg.TimerList {
		if s == seconds {
			c.timerIndex = i
			break
		}
	}
	var n notification
	if seconds <= 0 {
		n = c.applyLocked(0, true, ModeIndefinite)
	} else {
		n = c.applyLocked(time.Duration(seconds)*time.Second, false, ModeTimer)
	}
	c.mu.Unlock()
	n.deliver()
}

// SetCustomDuration starts a one-off timer for exactly d ("for a duration" in
// the Custom popup). See SetSchedule for the "until a date & time" variant.
func (c *Controller) SetCustomDuration(d time.Duration) error {
	return c.startFor(d, ModeTimer)
}

// SetSchedule starts a one-off timer for d, where d was computed by the
// caller as time-until-target ("until a date & time" in the Custom popup).
// Behaves identically to SetCustomDuration once running; only Mode differs,
// so callers (tray icon, CLI status) can tell how it was started.
func (c *Controller) SetSchedule(d time.Duration) error {
	return c.startFor(d, ModeSchedule)
}

func (c *Controller) startFor(d time.Duration, mode Mode) error {
	if d <= 0 {
		return errors.New("duration must be greater than 0")
	}
	c.mu.Lock()
	c.timerIndex = -1
	n := c.applyLocked(d, false, mode)
	c.mu.Unlock()
	n.deliver()
	return nil
}

func (c *Controller) TurnOff() {
	c.mu.Lock()
	c.timerIndex = -1
	n := c.applyLocked(0, false, ModeOff)
	c.mu.Unlock()
	n.deliver()
}

// expire is the timer callback: it turns off only if the state it was armed
// for is still current (see Controller.gen).
func (c *Controller) expire(gen uint64) {
	c.mu.Lock()
	if gen != c.gen {
		c.mu.Unlock()
		return
	}
	c.timerIndex = -1
	n := c.applyLocked(0, false, ModeOff)
	c.mu.Unlock()
	n.deliver()
}

// Resync re-checks a running timer against the wall clock and re-arms (or
// fires) it accordingly. Go's timers run on the monotonic clock, which isn't
// guaranteed to advance while the machine is suspended, so after a
// sleep/hibernate a timer could otherwise fire long after its ExpiresAt (or,
// for a Schedule, miss its wall-clock target entirely). main.go calls this on
// every resume (power.OnResume). It also copes with the wall clock itself
// having been changed — ExpiresAt is the source of truth.
func (c *Controller) Resync() {
	c.mu.Lock()
	if !c.state.Active || c.state.Indefinite {
		c.mu.Unlock()
		return
	}
	remaining := time.Until(c.state.ExpiresAt) // no monotonic reading in ExpiresAt → wall clock
	if remaining <= 0 {
		logger.Println("timer expired while the system was suspended; turning off")
		c.timerIndex = -1
		n := c.applyLocked(0, false, ModeOff)
		c.mu.Unlock()
		n.deliver()
		return
	}
	// Still running: re-arm for the real remaining time. The state itself
	// is unchanged, so gen stays the same and no listeners fire.
	if c.timer != nil {
		c.timer.Stop()
	}
	gen := c.gen
	c.timer = time.AfterFunc(remaining, func() { c.expire(gen) })
	c.mu.Unlock()
}

// notification is what a state transition still has to do after c.mu is
// released: persist the new state and notify listeners (which may do slow
// things like rebuild the tray menu, so they never run under mu).
type notification struct {
	persisted PersistedState
	state     State
	listeners []func(State)
}

func (n notification) deliver() {
	saveState(n.persisted) // store.go
	for _, l := range n.listeners {
		l(n.state)
	}
}

// applyLocked performs a state transition; c.mu must be held. The OS power
// state is applied before mu is released, so concurrent transitions reach
// the OS in the same order they took effect in c.state (power.Apply used to
// run after unlocking, so two racing transitions could leave the OS awake
// while state said Off, or vice versa).
func (c *Controller) applyLocked(d time.Duration, indefinite bool, mode Mode) notification {
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.gen++
	active := d > 0 || indefinite
	var expiresAt time.Time
	if active && !indefinite {
		// Round(0) strips the monotonic reading, so every later
		// time.Until(ExpiresAt) — RemainingLabel, Resync, the CLI status —
		// is judged against the wall clock, which keeps counting while the
		// machine sleeps.
		expiresAt = time.Now().Add(d).Round(0)
		gen := c.gen
		c.timer = time.AfterFunc(d, func() { c.expire(gen) })
	}
	if !active {
		mode = ModeOff
	}
	presetActive := false
	presetSeconds := 0
	if active && c.timerIndex >= 0 && c.timerIndex < len(c.cfg.TimerList) {
		presetActive = true
		presetSeconds = c.cfg.TimerList[c.timerIndex]
	}
	c.state = State{
		Active:        active,
		Indefinite:    indefinite,
		ExpiresAt:     expiresAt,
		Mode:          mode,
		PresetActive:  presetActive,
		PresetSeconds: presetSeconds,
	}

	_ = power.Apply(active, c.cfg.KeepDisplayOn)

	return notification{
		persisted: PersistedState{
			Active:     c.state.Active,
			Indefinite: c.state.Indefinite,
			ExpiresAt:  c.state.ExpiresAt,
			Mode:       c.state.Mode,
			TimerIndex: c.timerIndex,
		},
		state:     c.state,
		listeners: append([]func(State){}, c.stListeners...),
	}
}

// RestoreState loads the timer state a previous run persisted to
// state.json (see saveState in store.go, called via applyLocked above) and
// resumes it if it's still valid: an indefinite timer resumes as-is, and a
// running timer resumes with whatever time is left, judged against the
// wall clock — ExpiresAt is an absolute time, so this doesn't need to know
// how long the process was down for. A timer whose ExpiresAt already
// passed while mugcup was closed is left Off instead of firing retroactively.
//
// Call this once, right after NewController and before anything else
// touches the Controller (main.go does) — like any other state change, it
// fires OnStateChange listeners and applies power state, and callers such
// as the tray build their initial menu/icon from Controller.State().
func (c *Controller) RestoreState() {
	saved, ok := loadState() // store.go
	if !ok || !saved.Active {
		return
	}

	var n notification
	c.mu.Lock()
	if saved.Indefinite {
		c.timerIndex = saved.TimerIndex
		n = c.applyLocked(0, true, saved.Mode)
	} else {
		remaining := time.Until(saved.ExpiresAt)
		if remaining <= 0 {
			c.mu.Unlock()
			logger.Println("Saved timer had already expired while mugcup was closed; staying Off.")
			return
		}
		c.timerIndex = saved.TimerIndex
		n = c.applyLocked(remaining, false, saved.Mode)
	}
	c.mu.Unlock()
	n.deliver()
}

// ParseDuration parses input like "1h30m", "45m".
func ParseDuration(input string) (time.Duration, error) {
	compact := strings.Join(strings.Fields(input), "")
	if compact == "" {
		return 0, errors.New("empty input")
	}
	d, err := time.ParseDuration(compact)
	if err != nil {
		return 0, fmt.Errorf("unrecognized format (e.g. 1h30m, 45m): %w", err)
	}
	if d <= 0 {
		return 0, errors.New("duration must be greater than 0")
	}
	return d, nil
}

// FormatDurationSec converts seconds into a readable string like "15m", "1h", "Indefinite".
func FormatDurationSec(sec int) string {
	if sec <= 0 {
		return "Indefinite"
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	if h > 0 && m > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	} else if h > 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dm", m)
}
