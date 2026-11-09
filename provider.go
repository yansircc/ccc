package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// tokenEnvironmentName is the only projection from provider identity to its
// shell-compatible token environment variable.
func tokenEnvironmentName(name string) string {
	var normalized strings.Builder
	for _, character := range strings.ToUpper(name) {
		if character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
			normalized.WriteRune(character)
		} else {
			normalized.WriteByte('_')
		}
	}
	return "CCC_" + normalized.String() + "_TOKEN"
}

// resolveToken resolves a provider's token.
// Convention: normalized environment variable > Keychain.
func resolveToken(name string) string {
	envKey := tokenEnvironmentName(name)
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	if v, err := keychainGet(name); err == nil && v != "" {
		return v
	}
	return ""
}

// environmentFor returns the environment ccc adds to this run.
//
// The transport of a relay lives here — in the environment of the one process
// ccc launches — and nowhere else: settings.json is scrubbed first, because
// Claude Code merges settings.env over the process environment and a pinned
// base URL would win. applied is "" for an official run, which adds ccc's
// standing env and no transport at all.
func environmentFor(cfg *config, applied string) (map[string]string, error) {
	environment := make(map[string]string, len(cfg.Env)+4)
	for key, value := range cfg.Env {
		environment[key] = value
	}
	if applied == "" {
		return environment, nil
	}

	provider, ok := cfg.Providers[applied]
	if !ok {
		return nil, fmt.Errorf("provider %q not found in config (%s)", applied, configPath())
	}
	token := resolveToken(applied)
	if token == "" {
		envKey := tokenEnvironmentName(applied)
		return nil, fmt.Errorf("%s is empty and no keychain entry for %q\n  Run: ccc token set %s <value>", envKey, applied, applied)
	}

	environment["ANTHROPIC_BASE_URL"] = provider.BaseURL
	environment["ANTHROPIC_AUTH_TOKEN"] = token
	for key, value := range provider.Env {
		environment[key] = value
	}
	return environment, nil
}

// handleProviderCommand handles the "ccc provider" subcommand.
func handleProviderCommand(args []string) {
	if len(args) == 0 {
		providerUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "add":
		providerAdd(args[1:])
	case "list":
		providerList()
	case "remove":
		providerRemove(args[1:])
	case "set-default":
		providerSetDefault(args[1:])
	default:
		providerUsage()
		os.Exit(1)
	}
}

func providerAdd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: ccc provider add <name> --base-url <url> [--arg <arg>]... [--env KEY=VAL]...")
		os.Exit(1)
	}
	name := args[0]
	var baseURL string
	var pArgs []string
	envMap := make(map[string]string)

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--base-url":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--base-url requires a value")
				os.Exit(1)
			}
			i++
			baseURL = args[i]
		case "--arg":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--arg requires a value")
				os.Exit(1)
			}
			i++
			pArgs = append(pArgs, args[i])
		case "--env":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--env requires KEY=VAL")
				os.Exit(1)
			}
			i++
			parts := strings.SplitN(args[i], "=", 2)
			if len(parts) != 2 {
				fmt.Fprintf(os.Stderr, "invalid --env format: %q (expected KEY=VAL)\n", args[i])
				os.Exit(1)
			}
			envMap[parts[0]] = parts[1]
		default:
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
			os.Exit(1)
		}
	}

	if baseURL == "" {
		fmt.Fprintln(os.Stderr, "--base-url is required")
		os.Exit(1)
	}

	// Connection keys are owned by ccc itself (--base-url flag and the
	// keychain via `ccc token set`); storing them in provider env would
	// shadow the base_url field and confuse settings sync.
	for _, k := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN"} {
		if _, ok := envMap[k]; ok {
			fmt.Fprintf(os.Stderr, "--env %s is not allowed; use --base-url and `ccc token set %s <value>`\n", k, name)
			os.Exit(1)
		}
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if _, exists := cfg.Providers[name]; exists {
		fmt.Fprintf(os.Stderr, "Provider %q already exists; remove it first to replace it.\n", name)
		os.Exit(1)
	}

	cfg.Providers[name] = providerConfig{
		BaseURL: baseURL,
		Args:    pArgs,
		Env:     envMap,
	}

	// Set as default if it's the first provider
	if len(cfg.Providers) == 1 {
		cfg.DefaultProvider = name
	}

	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Provider %q added.\n", name)
}

func providerList() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if len(cfg.Providers) == 0 {
		fmt.Println("No providers configured.")
		fmt.Println("  Run: ccc provider add <name> --base-url <url>")
		return
	}

	names := make([]string, 0, len(cfg.Providers))
	for k := range cfg.Providers {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		p := cfg.Providers[name]
		marker := "  "
		if name == cfg.DefaultProvider {
			marker = "* "
		}
		fmt.Printf("%s%-12s %s\n", marker, name, p.BaseURL)
		if len(p.Args) > 0 {
			fmt.Printf("  %-12s args: %s\n", "", strings.Join(p.Args, " "))
		}
		for k, v := range p.Env {
			fmt.Printf("  %-12s env:  %s=%s\n", "", k, v)
		}
	}
}

func providerRemove(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: ccc provider remove <name>")
		os.Exit(1)
	}
	name := args[0]

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if _, ok := cfg.Providers[name]; !ok {
		fmt.Fprintf(os.Stderr, "Provider %q not found.\n", name)
		os.Exit(1)
	}

	// Env keys this provider owns, so removal can sweep its model locks from
	// settings.json without touching the active provider's connection keys.
	removedKeys := make([]string, 0, len(cfg.Providers[name].Env))
	for k := range cfg.Providers[name].Env {
		removedKeys = append(removedKeys, k)
	}

	delete(cfg.Providers, name)
	if cfg.DefaultProvider == name {
		cfg.DefaultProvider = ""
	}

	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	// Sweep the removed provider's keys from Claude Code settings.json so a
	// direct `claude` run cannot keep using a provider that no longer exists.
	if err := scrubSettings(cfg, removedKeys...); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not clean settings.json: %v\n", err)
	}
	fmt.Printf("Provider %q removed.\n", name)
}

func providerSetDefault(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: ccc provider set-default <name>")
		os.Exit(1)
	}
	name := args[0]

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if _, ok := cfg.Providers[name]; !ok {
		fmt.Fprintf(os.Stderr, "Provider %q not found. Add it first.\n", name)
		os.Exit(1)
	}

	cfg.DefaultProvider = name
	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Default provider set to %q.\n", name)
}

func providerUsage() {
	fmt.Fprintln(os.Stderr, `Usage: ccc provider <command>

Commands:
  add <name> --base-url <url> [--arg <arg>]... [--env KEY=VAL]...
  list                     List configured providers
  remove <name>            Remove a provider
  set-default <name>       Set the default provider`)
}
