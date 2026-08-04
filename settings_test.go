package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeSettingsPath_ConfigDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmp)
	t.Setenv("HOME", "/home/user")

	got := claudeSettingsPath()
	want := filepath.Join(tmp, "settings.json")
	if got != want {
		t.Errorf("claudeSettingsPath() = %q, want %q", got, want)
	}
}

func TestClaudeSettingsPath_Fallback(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("HOME", "/home/user")

	got := claudeSettingsPath()
	want := filepath.Join("/home/user", ".claude", "settings.json")
	if got != want {
		t.Errorf("claudeSettingsPath() = %q, want %q", got, want)
	}
}

func TestReplaceSettings_CreatesPrivateParent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", ".claude", "settings.json")
	if err := replaceSettings(path, []byte("{}\n")); err != nil {
		t.Fatalf("replaceSettings: %v", err)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat settings parent: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Errorf("settings parent mode = %04o, want 0700", got)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat settings: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0600 {
		t.Errorf("settings mode = %04o, want 0600", got)
	}
}

func TestCCCManagedEnvKeys(t *testing.T) {
	cfg := &config{
		Providers: map[string]providerConfig{
			"a": {Env: map[string]string{"ANTHROPIC_DEFAULT_OPUS_MODEL": "x", "FEATURE_FLAG": "1"}},
			"b": {Env: map[string]string{"ANTHROPIC_DEFAULT_SONNET_MODEL": "y"}},
		},
	}
	keys := cccManagedEnvKeys(cfg)
	for _, want := range []string{
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_MODEL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"FEATURE_FLAG",
	} {
		if !keys[want] {
			t.Errorf("cccManagedEnvKeys() missing %q", want)
		}
	}
	if len(keys) != 6 {
		t.Errorf("cccManagedEnvKeys() = %d keys, want 6", len(keys))
	}
}

// syncSettings must replace only ccc-managed keys and preserve everything
// else in settings.json (permissions, hooks, unrelated env vars).
func TestSyncSettings_PreservesAndSwitches(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmp)
	t.Setenv("CCC_NO_SYNC_SETTINGS", "")

	path := filepath.Join(tmp, "settings.json")
	os.WriteFile(path, []byte(`{
  "permissions": {"defaultMode": "dontAsk"},
  "env": {
    "ANTHROPIC_BASE_URL": "https://old.example.com",
    "ANTHROPIC_AUTH_TOKEN": "old-token",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "stale-model",
    "MY_OWN_VAR": "keep-me"
  }
}`), 0644)

	cfg := &config{
		Providers: map[string]providerConfig{
			"newp": {BaseURL: "https://new.example.com", Env: map[string]string{"ANTHROPIC_DEFAULT_SONNET_MODEL": "fresh-model"}},
		},
	}
	if err := syncSettings("newp", cfg.Providers["newp"], "new-token", cfg); err != nil {
		t.Fatalf("syncSettings: %v", err)
	}

	data, _ := os.ReadFile(path)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	env := doc["env"].(map[string]any)
	if env["ANTHROPIC_BASE_URL"] != "https://new.example.com" {
		t.Errorf("base url = %v, want new provider's", env["ANTHROPIC_BASE_URL"])
	}
	if env["ANTHROPIC_AUTH_TOKEN"] != "new-token" {
		t.Errorf("token = %v, want new provider's", env["ANTHROPIC_AUTH_TOKEN"])
	}
	if env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "fresh-model" {
		t.Errorf("sonnet model = %v, want fresh-model", env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if _, ok := env["ANTHROPIC_DEFAULT_OPUS_MODEL"]; ok {
		t.Error("stale ANTHROPIC_DEFAULT_OPUS_MODEL not removed")
	}
	if env["MY_OWN_VAR"] != "keep-me" {
		t.Errorf("unmanaged env key MY_OWN_VAR lost: %v", env["MY_OWN_VAR"])
	}
	if doc["permissions"].(map[string]any)["defaultMode"] != "dontAsk" {
		t.Error("permissions section not preserved")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat settings: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("settings mode = %04o, want 0600", got)
	}
}

func TestClearManagedSettings_WritesPrivateFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmp)
	t.Setenv("CCC_NO_SYNC_SETTINGS", "")

	path := filepath.Join(tmp, "settings.json")
	if err := os.WriteFile(path, []byte(`{"env":{"SECRET":"value"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := clearManagedSettings([]string{"SECRET"}); err != nil {
		t.Fatalf("clearManagedSettings: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat settings: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("settings mode = %04o, want 0600", got)
	}
}

// A broken settings.json must abort (fail closed), not clobber the file.
func TestSyncSettings_FailClosedOnCorruptJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmp)

	path := filepath.Join(tmp, "settings.json")
	os.WriteFile(path, []byte(`{ not json`), 0644)

	cfg := &config{Providers: map[string]providerConfig{}}
	err := syncSettings("x", providerConfig{BaseURL: "https://x"}, "t", cfg)
	if err == nil {
		t.Fatal("syncSettings() = nil, want error on corrupt settings.json")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "{ not json" {
		t.Error("corrupt settings.json was modified")
	}
}
