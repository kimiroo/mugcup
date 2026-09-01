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
	ActionCycle    TrayClickAction = "cycle"    // cycle through presets
	ActionInfinite TrayClickAction = "infinite" // keep on indefinitely
	ActionOpenMenu TrayClickAction = "menu"     // open tray menu
)

type Config struct {
	AutoStart       bool            `json:"autoStart"`
	KeepDisplayOn   bool            `json:"keepDisplayOn"`
	AutoUpdate      bool            `json:"autoUpdate"`
	TimerList       []int           `json:"timerList"` // seconds, 0 = unlimited
	TrayClickAction TrayClickAction `json:"trayClickAction"`
}

func DefaultConfig() Config {
	return Config{
		AutoStart:       false,
		KeepDisplayOn:   false,
		AutoUpdate:      true,
		TimerList:       []int{15 * 60, 30 * 60, 60 * 60, 2 * 60 * 60, 0},
		TrayClickAction: ActionCycle,
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
	case "", ActionCycle, ActionInfinite, ActionOpenMenu:
		return nil
	default:
		return fmt.Errorf("unknown tray click action: %q", cfg.TrayClickAction)
	}
}

// Mode is how the current state was started, for callers that display it
// differently per-origin (the tray's icon, the CLI's status output) even
// though Timer and Schedule behave identically once running (both just
// count down to off).
type Mode string

const (
	ModeOff      Mode = "off"      // TurnOff, or a timer that ran out
	ModeInfinite Mode = "infinite" // SetInfinite, or a preset/Cycle landing on an unlimited entry
	ModeTimer    Mode = "timer"    // SetPreset/Cycle on a timed entry, or SetCustomDuration ("for a duration")
	ModeSchedule Mode = "schedule" // SetSchedule ("until a date & time")
)

// Label renders a Mode for display (tray icon tooltip, CLI status output).
func (m Mode) Label() string {
	switch m {
	case ModeInfinite:
		return "Infinite"
	case ModeTimer:
		return "Timer"
	case ModeSchedule:
		return "Schedule"
	default:
		return "Off"
	}
}

// ---- Runtime state (not persisted — always reset on process restart) ----

type State struct {
	Active    bool
	Infinite  bool
	ExpiresAt time.Time // valid only when Active && !Infinite
	Mode      Mode

	// PresetActive is true when the current state was started from an entry
	// in Config.TimerList (via SetPreset or Cycle landing on one), as opposed
	// to SetInfinite, SetCustomDuration, or SetSchedule. PresetSeconds (valid
	// only when PresetActive) holds that entry's value, so callers can tell
	// which preset is currently selected.
	PresetActive  bool
	PresetSeconds int
}

func (s State) RemainingLabel() string {
	if !s.Active {
		return "Off"
	}
	if s.Infinite {
		return "Indefinite"
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

// ---- Controller: owns both config and runtime state ----

type Controller struct {
	mu           sync.Mutex
	cfg          Config
	state        State
	timerIndex   int // position in TimerList cycle, -1 = outside the cycle (off/custom)
	timer        *time.Timer
	cfgListeners []func(Config)
	stListeners  []func(State)
}

func NewController(cfg Config) *Controller {
	if cfg.TrayClickAction == "" {
		cfg.TrayClickAction = ActionCycle
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

func (c *Controller) UpdateConfig(cfg Config) error {
	if cfg.TrayClickAction == "" {
		cfg.TrayClickAction = ActionCycle
	}
	if err := ValidateConfig(cfg); err != nil {
		return err
	}
	if err := SaveConfig(cfg); err != nil { // store.go
		return err
	}
	c.mu.Lock()
	c.cfg = cfg
	listeners := append([]func(Config){}, c.cfgListeners...)
	active := c.state.Active
	c.mu.Unlock()

	if active {
		_ = power.Apply(true, cfg.KeepDisplayOn)
	}

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
	c.mu.Unlock()

	if next == -1 {
		c.apply(0, false, ModeOff)
		return
	}
	seconds := list[next]
	if seconds <= 0 {
		c.apply(0, true, ModeInfinite) // unlimited
	} else {
		c.apply(time.Duration(seconds)*time.Second, false, ModeTimer)
	}
}

func (c *Controller) SetInfinite() {
	c.mu.Lock()
	c.timerIndex = -1
	c.mu.Unlock()
	c.apply(0, true, ModeInfinite)
}

func (c *Controller) ToggleInfinite() {
	c.mu.Lock()
	activeInfinite := c.state.Active && c.state.Infinite
	c.mu.Unlock()
	if activeInfinite {
		c.TurnOff()
	} else {
		c.SetInfinite()
	}
}

// SetPreset: seconds<=0 means unlimited; >0 keeps it on for that duration (Infinite=false).
func (c *Controller) SetPreset(seconds int) {
	c.mu.Lock()
	c.timerIndex = -1
	for i, s := range c.cfg.TimerList {
		if s == seconds {
			c.timerIndex = i
			break
		}
	}
	c.mu.Unlock()

	if seconds <= 0 {
		c.apply(0, true, ModeInfinite)
	} else {
		c.apply(time.Duration(seconds)*time.Second, false, ModeTimer)
	}
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
	c.mu.Unlock()
	c.apply(d, false, mode)
	return nil
}

func (c *Controller) TurnOff() {
	c.mu.Lock()
	c.timerIndex = -1
	c.mu.Unlock()
	c.apply(0, false, ModeOff)
}

func (c *Controller) apply(d time.Duration, infinite bool, mode Mode) {
	c.mu.Lock()
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	active := d > 0 || infinite
	var expiresAt time.Time
	if active && !infinite {
		expiresAt = time.Now().Add(d)
		c.timer = time.AfterFunc(d, func() { c.TurnOff() })
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
		Infinite:      infinite,
		ExpiresAt:     expiresAt,
		Mode:          mode,
		PresetActive:  presetActive,
		PresetSeconds: presetSeconds,
	}
	state := c.state
	listeners := append([]func(State){}, c.stListeners...)
	cfg := c.cfg
	c.mu.Unlock()

	_ = power.Apply(active, cfg.KeepDisplayOn)

	for _, l := range listeners {
		l(state)
	}
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

// FormatDurationSec converts seconds into a readable string like "15m", "1h", "Unlimited".
func FormatDurationSec(sec int) string {
	if sec <= 0 {
		return "Unlimited"
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
