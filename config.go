package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validateConfig(cfg *config) error {
	names := make([]string, 0, len(cfg.Providers))
	for name := range cfg.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	owners := make(map[string]string, len(names))
	for _, name := range names {
		environment := tokenEnvironmentName(name)
		if owner, exists := owners[environment]; exists {
			return fmt.Errorf("providers %q and %q resolve to the same token environment %s", owner, name, environment)
		}
		owners[environment] = name
	}
	return nil
}

// saveConfig writes config to disk, creating directories as needed.
func saveConfig(cfg *config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
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
