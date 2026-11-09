package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// replaceSettings atomically replaces a Claude settings file without ever
// creating a predictable or group-readable token-bearing temporary file.
func replaceSettings(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".settings.json.*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
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

// claudeSettingsPath returns the path to Claude Code's user settings.json.
// Like Claude Code itself, it honors CLAUDE_CONFIG_DIR when set.
func claudeSettingsPath() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json")
	}
	return filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
}

// readSettings parses Claude Code's settings.json, treating a missing file as
// an empty document. A file that is not JSON is an error: ccc refuses to guess.
func readSettings(path string) (map[string]any, error) {
	doc := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return doc, nil
		}
		return nil, fmt.Errorf("read settings %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("settings %s is not valid JSON: %w", path, err)
	}
	return doc, nil
}

// hardEnvKeys are the keys ccc owns in every mode: they are what decides where
// a request goes and which model name it names.
func hardEnvKey(key string) bool {
	switch key {
	case "ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_MODEL":
		return true
	default:
		return false
	}
}

// cccEnvKeys returns the env keys ccc owns inside settings.json: the transport
// keys, and every key a provider declares.
//
// The top-level env is subtracted: it is the user's standing preference, the
// same in every mode, so it may stay in settings.json — scrubbing it would only
// take it away from a plain `claude` run. A provider's env is this run's
// transport, so it may not linger there.
func cccEnvKeys(cfg *config) map[string]bool {
	keys := map[string]bool{}
	for _, provider := range cfg.Providers {
		for key := range provider.Env {
			keys[key] = true
		}
	}
	for key := range cfg.Env {
		if !hardEnvKey(key) {
			delete(keys, key)
		}
	}
	for _, key := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_MODEL"} {
		keys[key] = true
	}
	return keys
}

// cccOwnsSettingsKey reports whether ccc may remove this key from settings.json.
// Provider model locks live in the ANTHROPIC_DEFAULT_* namespace and are swept
// wholesale, so a removed provider's overrides never linger.
func cccOwnsSettingsKey(key string, owned map[string]bool) bool {
	return owned[key] || strings.HasPrefix(key, "ANTHROPIC_DEFAULT_")
}

// scrubSettings removes every env key ccc owns from Claude Code's
// settings.json.
//
// Why: Claude Code merges settings.env OVER the process environment, so a base
// URL parked in settings.json beats anything ccc injects — and it would also
// silently keep a plain `claude` run on the relay. ccc therefore keeps no
// transport in settings.json at all: a relay lives in the environment of the
// one process it launches, and an official run inherits nothing. This is what
// makes `ccc` (and plain `claude`) official by default.
//
// It is fail-closed: a settings file that cannot be read or parsed aborts the
// run before claude starts, and the file is replaced atomically, private.
// Set CCC_NO_SYNC_SETTINGS=1 to keep ccc's hands off the file entirely.
func scrubSettings(cfg *config, extraKeys ...string) error {
	if os.Getenv("CCC_NO_SYNC_SETTINGS") != "" {
		return nil
	}

	path := claudeSettingsPath()
	doc, err := readSettings(path)
	if err != nil {
		return err
	}
	env, _ := doc["env"].(map[string]any)
	if env == nil {
		return nil
	}

	owned := cccEnvKeys(cfg)
	for _, key := range extraKeys {
		if !hardEnvKey(key) {
			if _, standing := cfg.Env[key]; standing {
				continue
			}
		}
		owned[key] = true
	}
	removed := 0
	for key := range env {
		if cccOwnsSettingsKey(key, owned) {
			delete(env, key)
			removed++
		}
	}
	if removed == 0 {
		return nil
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if err := replaceSettings(path, out); err != nil {
		return fmt.Errorf("replace settings %s: %w", path, err)
	}
	return nil
}

// scrubPreview lists the keys scrubSettings would remove, sorted, without
// touching the file. Used by --print-plan.
func scrubPreview(cfg *config) ([]string, error) {
	doc, err := readSettings(claudeSettingsPath())
	if err != nil {
		return nil, err
	}
	env, _ := doc["env"].(map[string]any)
	owned := cccEnvKeys(cfg)
	var stale []string
	for key := range env {
		if cccOwnsSettingsKey(key, owned) {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	return stale, nil
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
