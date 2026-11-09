package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPath_XDGSet(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	got := configPath()
	want := filepath.Join(tmp, "ccc", "config.json")
	if got != want {
		t.Errorf("configPath() = %q, want %q", got, want)
	}
}

func TestConfigPath_FallbackHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", tmp)

	got := configPath()
	want := filepath.Join(tmp, ".config", "ccc", "config.json")
	if got != want {
		t.Errorf("configPath() = %q, want %q", got, want)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v, want nil", err)
	}
	if cfg.DefaultProvider != "" {
		t.Errorf("DefaultProvider = %q, want empty", cfg.DefaultProvider)
	}
	if cfg.Providers == nil {
		t.Fatal("Providers map should be initialized, got nil")
	}
	if len(cfg.Providers) != 0 {
		t.Errorf("len(Providers) = %d, want 0", len(cfg.Providers))
	}
}

func TestLoadConfig_ValidJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "ccc")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data := `{
  "default_provider": "acme",
  "providers": {
    "acme": {
      "base_url": "https://api.acme.com",
      "env": {"FOO": "bar"}
    }
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if cfg.DefaultProvider != "acme" {
		t.Errorf("DefaultProvider = %q, want %q", cfg.DefaultProvider, "acme")
	}
	p, ok := cfg.Providers["acme"]
	if !ok {
		t.Fatal("provider 'acme' not found")
	}
	if p.BaseURL != "https://api.acme.com" {
		t.Errorf("BaseURL = %q, want %q", p.BaseURL, "https://api.acme.com")
	}
	if p.Env["FOO"] != "bar" {
		t.Errorf("Env[FOO] = %q, want %q", p.Env["FOO"], "bar")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "ccc")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{bad json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := loadConfig()
	if err == nil {
		t.Fatal("loadConfig() expected error for invalid JSON, got nil")
	}
	if _, ok := err.(*json.SyntaxError); !ok {
		t.Errorf("expected *json.SyntaxError, got %T", err)
	}
}

func TestLoadConfigRejectsTokenEnvironmentCollision(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dir := filepath.Join(tmp, "ccc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{"providers":{"a-b":{"base_url":"https://one.example"},"a_b":{"base_url":"https://two.example"}}}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(); err == nil {
		t.Fatal("token environment collision was accepted")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	original := &config{
		DefaultProvider: "corp",
		Providers: map[string]providerConfig{
			"corp": {
				BaseURL: "https://corp.example.com/v1",
				Args:    []string{"--model", "claude-3"},
				Env:     map[string]string{"X_API": "yes", "REGION": "us"},
			},
			"dev": {
				BaseURL: "https://dev.example.com",
			},
		},
	}

	if err := saveConfig(original); err != nil {
		t.Fatalf("saveConfig() error = %v", err)
	}

	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}

	if loaded.DefaultProvider != original.DefaultProvider {
		t.Errorf("DefaultProvider = %q, want %q", loaded.DefaultProvider, original.DefaultProvider)
	}
	if len(loaded.Providers) != len(original.Providers) {
		t.Fatalf("len(Providers) = %d, want %d", len(loaded.Providers), len(original.Providers))
	}

	for name, op := range original.Providers {
		lp, ok := loaded.Providers[name]
		if !ok {
			t.Errorf("provider %q missing after round-trip", name)
			continue
		}
		if lp.BaseURL != op.BaseURL {
			t.Errorf("provider %q BaseURL = %q, want %q", name, lp.BaseURL, op.BaseURL)
		}
		if len(lp.Args) != len(op.Args) {
			t.Errorf("provider %q len(Args) = %d, want %d", name, len(lp.Args), len(op.Args))
		} else {
			for i := range op.Args {
				if lp.Args[i] != op.Args[i] {
					t.Errorf("provider %q Args[%d] = %q, want %q", name, i, lp.Args[i], op.Args[i])
				}
			}
		}
		if len(lp.Env) != len(op.Env) {
			t.Errorf("provider %q len(Env) = %d, want %d", name, len(lp.Env), len(op.Env))
		}
		for k, v := range op.Env {
			if lp.Env[k] != v {
				t.Errorf("provider %q Env[%s] = %q, want %q", name, k, lp.Env[k], v)
			}
		}
	}
}

func TestLoadConfig_StandingArgsAndEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := filepath.Join(tmp, "ccc")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data := `{
  "default_provider": "relay",
  "args": ["--dangerously-skip-permissions"],
  "env": {"DISABLE_TELEMETRY": "1"},
  "providers": {"relay": {"base_url": "https://relay.example"}}
}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if len(cfg.Args) != 1 || cfg.Args[0] != "--dangerously-skip-permissions" {
		t.Errorf("Args = %v, want the standing arg", cfg.Args)
	}
	if cfg.Env["DISABLE_TELEMETRY"] != "1" {
		t.Errorf("Env = %v, want DISABLE_TELEMETRY=1", cfg.Env)
	}

	// Round-trip: standing args and env survive a save.
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig() error = %v", err)
	}
	reloaded, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() after save error = %v", err)
	}
	if len(reloaded.Args) != 1 || reloaded.Env["DISABLE_TELEMETRY"] != "1" {
		t.Errorf("standing args/env lost on save: %+v", reloaded)
	}
}

func TestValidateConfig_RejectsTransportInStandingEnv(t *testing.T) {
	cfg := &config{
		Env:       map[string]string{"ANTHROPIC_BASE_URL": "https://relay.example"},
		Providers: map[string]providerConfig{},
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("a standing base URL was accepted")
	}
}

func TestValidateConfig_RejectsProviderDisagreeingWithStandingEnv(t *testing.T) {
	cfg := &config{
		Env: map[string]string{"DISABLE_TELEMETRY": "1"},
		Providers: map[string]providerConfig{
			"relay": {BaseURL: "https://relay.example", Env: map[string]string{"DISABLE_TELEMETRY": "0"}},
		},
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("a provider contradicting the standing env was accepted")
	}

	// Agreement is fine, and model keys are the provider's own even when the
	// standing env names a model.
	cfg.Providers["relay"] = providerConfig{
		BaseURL: "https://relay.example",
		Env:     map[string]string{"DISABLE_TELEMETRY": "1", "ANTHROPIC_MODEL": "relay-model"},
	}
	cfg.Env["ANTHROPIC_MODEL"] = "official-model"
	if err := validateConfig(cfg); err != nil {
		t.Errorf("validateConfig() = %v, want nil", err)
	}
}
