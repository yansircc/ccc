package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// replaceSettings atomically replaces a Claude settings file without ever
// creating a predictable or group-readable token-bearing temporary file.
func replaceSettings(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings.json.*")
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

// clearManagedSettings removes the given provider env keys plus every
// ANTHROPIC_DEFAULT_* model lock from Claude Code's settings.json, without
// touching connection keys (ANTHROPIC_BASE_URL / ANTHROPIC_AUTH_TOKEN), which
// belong to whichever provider is currently active. Used when a provider is
// removed so direct `claude` runs cannot keep using its model overrides.
func clearManagedSettings(removedEnvKeys []string) error {
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
	for _, k := range removedEnvKeys {
		if k == "ANTHROPIC_BASE_URL" || k == "ANTHROPIC_AUTH_TOKEN" {
			continue
		}
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
	if err := replaceSettings(path, out); err != nil {
		return err
	}
	return nil
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

	if err := replaceSettings(path, data); err != nil {
		return fmt.Errorf("replace settings %s: %w", path, err)
	}
	return nil
}
