package main

import (
	"fmt"
	"os"
	"strings"
)

// resolveToken resolves a provider's token.
// Convention: env var CCC_<UPPER(name)>_TOKEN > Keychain.
func resolveToken(name string) string {
	envKey := "CCC_" + strings.ToUpper(name) + "_TOKEN"
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	if v, err := keychainGet(name); err == nil && v != "" {
		return v
	}
	return ""
}

// setupProvider configures environment variables for the chosen provider.
func setupProvider(name string, cfg *config) error {
	p, ok := cfg.Providers[name]
	if !ok {
		return fmt.Errorf("provider %q not found in config (%s)", name, configPath())
	}

	token := resolveToken(name)
	if token == "" {
		envKey := "CCC_" + strings.ToUpper(name) + "_TOKEN"
		return fmt.Errorf("%s is empty and no keychain entry for %q\n  Run: ccc token set %s <value>", envKey, name, name)
	}

	os.Setenv("ANTHROPIC_BASE_URL", p.BaseURL)
	os.Setenv("ANTHROPIC_AUTH_TOKEN", token)
	for k, v := range p.Env {
		os.Setenv(k, v)
	}
	return nil
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

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
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

	for name, p := range cfg.Providers {
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

	delete(cfg.Providers, name)
	if cfg.DefaultProvider == name {
		cfg.DefaultProvider = ""
	}

	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
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
