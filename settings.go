package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// claudeSettingsPath returns the path to Claude Code's user settings.json.
// Like Claude Code itself, it honors CLAUDE_CONFIG_DIR when set.
func claudeSettingsPath() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json")
	}
	return filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
}

// clearManagedSettings removes every ccc-managed env key from Claude Code's
// settings.json without writing new values. Used when a provider is removed
// so direct `claude` runs cannot keep using the deleted provider's config.
func clearManagedSettings(cfg *config) error {
	if os.Getenv("CCC_NO_SYNC_SETTINGS") != "" {
		return nil
	}

	path := claudeSettingsPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	doc := map[string]any{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("settings %s is not valid JSON: %w", path, err)
	}

	env, _ := doc["env"].(map[string]any)
	if env == nil {
		return nil
	}
	changed := false
	for k := range cccManagedEnvKeys(cfg) {
		if _, ok := env[k]; ok {
			delete(env, k)
			changed = true
		}
	}
	for k := range env {
		if strings.HasPrefix(k, "ANTHROPIC_DEFAULT_") {
			delete(env, k)
			changed = true
		}
	}
	if !changed {
		return nil
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// cccManagedEnvKeys returns the env keys ccc owns inside settings.json:
// the connection keys plus every key referenced by any provider env.
// On provider switch, all managed keys are removed before writing the
// active provider's values, so stale overrides (e.g. a previous provider's
// model locks) never leak into the next provider.
func cccManagedEnvKeys(cfg *config) map[string]bool {
	keys := map[string]bool{
		"ANTHROPIC_BASE_URL":   true,
		"ANTHROPIC_AUTH_TOKEN": true,
		"ANTHROPIC_MODEL":      true,
	}
	for _, p := range cfg.Providers {
		for k := range p.Env {
			keys[k] = true
		}
	}
	return keys
}

// syncSettings persists the active provider's connection environment into
// Claude Code's user settings.json.
//
// Why: recent Claude Code versions give settings.json env precedence over the
// process environment. ccc's env injection alone therefore cannot move the
// base URL or auth token — a base URL pinned in settings.json always wins.
// Writing settings.json makes both `ccc` runs and direct `claude` invocations
// (wechat bridges, pawl, scripts) follow the same provider, with the same
// semantics cc-switch users already know.
//
// It is fail-closed: any read/write error aborts the run before claude starts,
// and the file is replaced atomically. Set CCC_NO_SYNC_SETTINGS=1 to disable.
func syncSettings(providerName string, p providerConfig, token string, cfg *config) error {
	if os.Getenv("CCC_NO_SYNC_SETTINGS") != "" {
		return nil
	}

	path := claudeSettingsPath()
	doc := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("settings %s is not valid JSON: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read settings %s: %w", path, err)
	}

	env, _ := doc["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	for k := range cccManagedEnvKeys(cfg) {
		delete(env, k)
	}
	// Provider model locks live in the ANTHROPIC_DEFAULT_* namespace; sweep
	// them wholesale so a removed provider's overrides never linger.
	for k := range env {
		if strings.HasPrefix(k, "ANTHROPIC_DEFAULT_") {
			delete(env, k)
		}
	}
	env["ANTHROPIC_BASE_URL"] = p.BaseURL
	env["ANTHROPIC_AUTH_TOKEN"] = token
	for k, v := range p.Env {
		env[k] = v
	}
	doc["env"] = env

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write settings %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace settings %s: %w", path, err)
	}
	return nil
}
