package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"mugcup/applog"
)

var logger = applog.New("settings")

// appDir resolves (and creates) %APPDATA%\mugcup, the directory both
// config.json and state.json live in.
func appDir() (string, error) {
	dir, err := os.UserConfigDir() // Windows: %APPDATA%
	if err != nil {
		return "", err
	}
	appDir := filepath.Join(dir, "mugcup")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", err
	}
	return appDir, nil
}

func configPath() (string, error) {
	dir, err := appDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func statePath() (string, error) {
	dir, err := appDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

func SaveConfig(c Config) error {
	if err := ValidateConfig(c); err != nil {
		return err
	}

	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	// Re-validate the marshaled JSON and write via a temp file + rename, so an
	// invalid config can never overwrite the existing file on disk.
	var checked Config
	if err := json.Unmarshal(data, &checked); err != nil {
		return fmt.Errorf("config JSON validation failed: %w", err)
	}
	if err := ValidateConfig(checked); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "config-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func LoadConfig() (Config, error) {
	path, err := configPath()
	if err != nil {
		logger.Printf("failed to resolve the config path, using defaults: %v", err)
		return DefaultConfig(), err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	if err != nil {
		logger.Printf("failed to read config.json, using defaults: %v", err)
		return DefaultConfig(), err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		logger.Printf("config.json is not valid JSON, using defaults: %v", err)
		return DefaultConfig(), err
	}
	if err := ValidateConfig(c); err != nil {
		logger.Printf("config.json failed validation, using defaults: %v", err)
		return DefaultConfig(), err
	}
	return c, nil
}

// saveState writes s to state.json (temp file + rename, same as SaveConfig)
// so a running timer survives a restart — see Controller.apply, its only
// caller. Failures are logged and otherwise swallowed: a lost state save
// just means the next restart comes up Off instead of resuming, which is
// the same as today's behavior, so it isn't worth surfacing to the user.
func saveState(s PersistedState) {
	path, err := statePath()
	if err != nil {
		logger.Printf("failed to resolve the state path, not saving timer state: %v", err)
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		logger.Printf("failed to marshal timer state, not saving: %v", err)
		return
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "state-*.json")
	if err != nil {
		logger.Printf("failed to save state.json: %v", err)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		logger.Printf("failed to save state.json: %v", err)
		return
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		logger.Printf("failed to save state.json: %v", err)
		return
	}
	if err := tmp.Close(); err != nil {
		logger.Printf("failed to save state.json: %v", err)
		return
	}
	if err := os.Rename(tmpPath, path); err != nil {
		logger.Printf("failed to save state.json: %v", err)
		return
	}
}

// loadState reads state.json, returning ok=false if it doesn't exist or is
// unusable (same fall-through-to-defaults approach as LoadConfig, just with
// no persisted state instead of DefaultConfig as the fallback value).
func loadState() (PersistedState, bool) {
	path, err := statePath()
	if err != nil {
		logger.Printf("failed to resolve the state path, starting Off: %v", err)
		return PersistedState{}, false
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		logger.Println("No saved timer state found, starting Off.")
		return PersistedState{}, false
	}
	if err != nil {
		logger.Printf("failed to read state.json, starting Off: %v", err)
		return PersistedState{}, false
	}
	var s PersistedState
	if err := json.Unmarshal(data, &s); err != nil {
		logger.Printf("state.json is not valid JSON, starting Off: %v", err)
		return PersistedState{}, false
	}
	logger.Printf("Timer state loaded: %s", s)
	return s, true
}

// configJSONKeys are the exact JSON object keys a config import must have,
// derived from Config's own json tags so the schema has one source of
// truth instead of a manually kept-in-sync list.
func configJSONKeys() []string {
	t := reflect.TypeOf(Config{})
	keys := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	return keys
}

// ParseConfigJSON strictly decodes raw as a Config for import: the JSON
// object's key set must exactly match Config's fields — no missing, no
// extra — and each value's type must match its field (DisallowUnknownFields
// plus json.Decoder's own per-field type checking, e.g. a string where
// timerList expects numbers). Deliberately stricter than LoadConfig's own
// parsing of config.json, since import brings in JSON from outside the app.
func ParseConfigJSON(raw []byte) (Config, error) {
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		return Config{}, fmt.Errorf("invalid JSON: %w", err)
	}

	expected := configJSONKeys()
	present := make(map[string]bool, len(asMap))
	for k := range asMap {
		present[k] = true
	}
	var missing, extra []string
	for _, k := range expected {
		if !present[k] {
			missing = append(missing, k)
		}
		delete(present, k)
	}
	for k := range present {
		extra = append(extra, k)
	}
	if len(missing) > 0 || len(extra) > 0 {
		sort.Strings(missing)
		sort.Strings(extra)
		var parts []string
		if len(missing) > 0 {
			parts = append(parts, "missing "+strings.Join(missing, ", "))
		}
		if len(extra) > 0 {
			parts = append(parts, "unexpected "+strings.Join(extra, ", "))
		}
		return Config{}, fmt.Errorf("config JSON keys don't match the expected schema (%s)", strings.Join(parts, "; "))
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("config JSON doesn't match the expected schema: %w", err)
	}
	return cfg, nil
}
