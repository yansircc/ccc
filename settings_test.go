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

func TestCCCEnvKeys(t *testing.T) {
	cfg := &config{
		Env: map[string]string{"STANDS_ALONG": "1", "ANTHROPIC_MODEL": "mine"},
		Providers: map[string]providerConfig{
			"a": {Env: map[string]string{
				"ANTHROPIC_DEFAULT_OPUS_MODEL": "x",
				"FEATURE_FLAG":                 "1",
				"STANDS_ALONG":                 "1",
			}},
			"b": {Env: map[string]string{"ANTHROPIC_DEFAULT_SONNET_MODEL": "y"}},
		},
	}
	keys := cccEnvKeys(cfg)
	for _, want := range []string{
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_MODEL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
		"FEATURE_FLAG",
	} {
		if !keys[want] {
			t.Errorf("cccEnvKeys() missing %q", want)
		}
	}
	// The standing env is the user's global preference: it is not ccc's to
	// scrub, so plain `claude` keeps it.
	if keys["STANDS_ALONG"] {
		t.Error("standing env key STANDS_ALONG was claimed for scrubbing")
	}
	if len(keys) != 6 {
		t.Errorf("cccEnvKeys() = %d keys, want 6: %v", len(keys), keys)
	}
}

// A relay must leave no transport in settings.json: ccc drops its own keys and
// preserves everything else (permissions, hooks, unrelated env vars).
func TestScrubSettings_RemovesOnlyCCCOwnedKeys(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmp)
	t.Setenv("CCC_NO_SYNC_SETTINGS", "")

	path := filepath.Join(tmp, "settings.json")
	os.WriteFile(path, []byte(`{
  "permissions": {"defaultMode": "dontAsk"},
  "env": {
    "ANTHROPIC_BASE_URL": "https://ccc.210k.cc",
    "ANTHROPIC_AUTH_TOKEN": "old-token",
    "ANTHROPIC_MODEL": "relay-model",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "stale-model",
    "ANTHROPIC_API_KEY": "keep-official-key",
    "MY_OWN_VAR": "keep-me"
  }
}`), 0644)

	cfg := &config{
		Providers: map[string]providerConfig{
			"newp": {BaseURL: "https://new.example.com", Env: map[string]string{"ANTHROPIC_DEFAULT_SONNET_MODEL": "fresh-model"}},
		},
	}
	if err := scrubSettings(cfg); err != nil {
		t.Fatalf("scrubSettings: %v", err)
	}

	data, _ := os.ReadFile(path)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	env := doc["env"].(map[string]any)
	for _, gone := range []string{
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_MODEL",
		"ANTHROPIC_DEFAULT_OPUS_MODEL",
		"ANTHROPIC_DEFAULT_SONNET_MODEL",
	} {
		if _, ok := env[gone]; ok {
			t.Errorf("%s survived the scrub", gone)
		}
	}
	if env["MY_OWN_VAR"] != "keep-me" {
		t.Errorf("unmanaged env key MY_OWN_VAR lost: %v", env["MY_OWN_VAR"])
	}
	if env["ANTHROPIC_API_KEY"] != "keep-official-key" {
		t.Errorf("official API key was scrubbed: %v", env["ANTHROPIC_API_KEY"])
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

// A clean settings.json must not be rewritten on every run.
func TestScrubSettings_NoWriteWhenAlreadyClean(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmp)
	t.Setenv("CCC_NO_SYNC_SETTINGS", "")

	path := filepath.Join(tmp, "settings.json")
	original := "{\n  \"env\": {\n    \"MY_OWN_VAR\": \"keep-me\"\n  }\n}\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := scrubSettings(&config{}); err != nil {
		t.Fatalf("scrubSettings: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("clean settings.json was rewritten:\n%s", got)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("clean settings.json was touched")
	}
}

func TestScrubSettings_DisabledByEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmp)
	t.Setenv("CCC_NO_SYNC_SETTINGS", "1")

	path := filepath.Join(tmp, "settings.json")
	original := `{"env":{"ANTHROPIC_BASE_URL":"https://ccc.210k.cc"}}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := scrubSettings(&config{}); err != nil {
		t.Fatalf("scrubSettings: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != original {
		t.Error("CCC_NO_SYNC_SETTINGS=1 did not keep ccc's hands off settings.json")
	}
}

func TestScrubSettings_MissingFileIsFine(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("CCC_NO_SYNC_SETTINGS", "")
	if err := scrubSettings(&config{}); err != nil {
		t.Errorf("scrubSettings on missing file = %v, want nil", err)
	}
}

// A broken settings.json must abort (fail closed), not clobber the file.
func TestScrubSettings_FailClosedOnCorruptJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmp)
	t.Setenv("CCC_NO_SYNC_SETTINGS", "")

	path := filepath.Join(tmp, "settings.json")
	os.WriteFile(path, []byte(`{ not json`), 0644)

	if err := scrubSettings(&config{}); err == nil {
		t.Fatal("scrubSettings() = nil, want error on corrupt settings.json")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "{ not json" {
		t.Error("corrupt settings.json was modified")
	}
}

func TestScrubPreview_ListsStaleKeysWithoutWriting(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmp)

	path := filepath.Join(tmp, "settings.json")
	original := `{"env":{"ANTHROPIC_BASE_URL":"https://ccc.210k.cc","ANTHROPIC_DEFAULT_OPUS_MODEL":"x","KEEP":"1"}}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}

	stale, err := scrubPreview(&config{})
	if err != nil {
		t.Fatalf("scrubPreview: %v", err)
	}
	if len(stale) != 2 || stale[0] != "ANTHROPIC_BASE_URL" || stale[1] != "ANTHROPIC_DEFAULT_OPUS_MODEL" {
		t.Errorf("scrubPreview() = %v, want the two transport keys sorted", stale)
	}
	if got, _ := os.ReadFile(path); string(got) != original {
		t.Error("scrubPreview wrote to settings.json")
	}
}

// The standing env is shared by every mode, so it may stay in settings.json:
// scrubbing it would only take it away from a plain `claude` run.
func TestScrubSettings_KeepsStandingEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmp)
	t.Setenv("CCC_NO_SYNC_SETTINGS", "")

	path := filepath.Join(tmp, "settings.json")
	os.WriteFile(path, []byte(`{"env":{"DISABLE_TELEMETRY":"1","ANTHROPIC_BASE_URL":"https://ccc.210k.cc"}}`), 0600)

	cfg := &config{Env: map[string]string{"DISABLE_TELEMETRY": "1"}}
	if err := scrubSettings(cfg); err != nil {
		t.Fatalf("scrubSettings: %v", err)
	}

	data, _ := os.ReadFile(path)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	env := doc["env"].(map[string]any)
	if env["DISABLE_TELEMETRY"] != "1" {
		t.Error("standing env was scrubbed from settings.json")
	}
	if _, ok := env["ANTHROPIC_BASE_URL"]; ok {
		t.Error("transport survived the scrub")
	}
}

// provider remove sweeps the removed provider's own keys, but must not take the
// standing env with it.
func TestScrubSettings_ExtraKeysSkipStandingEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmp)
	t.Setenv("CCC_NO_SYNC_SETTINGS", "")

	path := filepath.Join(tmp, "settings.json")
	os.WriteFile(path, []byte(`{"env":{"DISABLE_TELEMETRY":"1","GONE_PROVIDER_FLAG":"1"}}`), 0600)

	cfg := &config{Env: map[string]string{"DISABLE_TELEMETRY": "1"}}
	if err := scrubSettings(cfg, "DISABLE_TELEMETRY", "GONE_PROVIDER_FLAG"); err != nil {
		t.Fatalf("scrubSettings: %v", err)
	}

	data, _ := os.ReadFile(path)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	env := doc["env"].(map[string]any)
	if env["DISABLE_TELEMETRY"] != "1" {
		t.Error("standing env was swept with a removed provider")
	}
	if _, ok := env["GONE_PROVIDER_FLAG"]; ok {
		t.Error("removed provider's flag survived")
	}
}
