package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	StartHour  int    `json:"start_hour"`
	DailyLimit int    `json:"daily_limit"`
	DataRepo   string `json:"data_repo"`
	Device     string `json:"device"`
}

func defaults() Config {
	return Config{
		StartHour:  4,
		DailyLimit: 5,
		DataRepo:   "git@github-personal:sousa16/neetcode150-srs-data.git",
	}
}

func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "neetcode-srs", "config.json"), nil
}

// Load reads the config file, filling in defaults for any field the file
// doesn't set. On first run (no file yet), it writes the defaults to disk
// so the user has something to find and edit, then returns them.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return Config{}, err
		}
		cfg := defaults()
		return cfg, save(path, cfg)
	}

	cfg := defaults()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	return os.WriteFile(path, data, 0644)
}
