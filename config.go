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

// config is ~/.config/ccc/config.json.
//
// DefaultProvider names the relay `ccc --relay` uses; it is never the default
// run. Args and Env are ccc's standing arguments and environment — they apply
// to an official run and a relay run alike.
type config struct {
	DefaultProvider string                    `json:"default_provider"`
	Args            []string                  `json:"args,omitempty"`
	Env             map[string]string         `json:"env,omitempty"`
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
	if cfg.Env == nil {
		cfg.Env = make(map[string]string)
	}
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validateConfig(cfg *config) error {
	for key := range cfg.Env {
		if key == "ANTHROPIC_BASE_URL" || key == "ANTHROPIC_AUTH_TOKEN" {
			return fmt.Errorf("%s belongs to a provider: use its base_url and `ccc token set <provider> <value>`", key)
		}
	}

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

		// A standing env key stays in settings.json, and settings.env beats the
		// process environment: a provider that disagreed with it would be
		// silently overruled, so the two must say the same thing.
		for key, value := range cfg.Providers[name].Env {
			if hardEnvKey(key) {
				continue
			}
			if standing, exists := cfg.Env[key]; exists && standing != value {
				return fmt.Errorf("provider %q sets %s=%q but the standing env says %q", name, key, value, standing)
			}
		}
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
