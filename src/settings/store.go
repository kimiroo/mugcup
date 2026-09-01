package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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
		return DefaultConfig(), err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return DefaultConfig(), err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return DefaultConfig(), err
	}
	if err := ValidateConfig(c); err != nil {
		return DefaultConfig(), err
	}
	return c, nil
}
