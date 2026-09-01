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

func configPath() (string, error) {
	dir, err := os.UserConfigDir() // Windows: %APPDATA%
	if err != nil {
		return "", err
	}
	appDir := filepath.Join(dir, "mugcup")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(appDir, "config.json"), nil
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
