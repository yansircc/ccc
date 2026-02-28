package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type providerConfig struct {
	BaseURL string            `json:"base_url"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type config struct {
	DefaultProvider string                    `json:"default_provider"`
	Providers       map[string]providerConfig `json:"providers"`
}

func configPath() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "ccc", "config.json")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "ccc", "config.json")
}

// loadConfig reads config from disk. Returns empty config if file doesn't exist.
func loadConfig() (*config, error) {
	cfg := &config{Providers: make(map[string]providerConfig)}

	data, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]providerConfig)
	}
	return cfg, nil
}

// saveConfig writes config to disk, creating directories as needed.
func saveConfig(cfg *config) error {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(data, '\n'), 0644)
}
