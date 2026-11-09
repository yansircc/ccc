package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// wrapperFlags is ccc's own command line: what it consumes, and what it hands
// to claude untouched.
type wrapperFlags struct {
	provider    string // --provider <name>
	relay       bool   // --relay
	relayName   string // --relay=<name>
	safe        bool   // --safe
	printPlan   bool   // --print-plan
	passthrough []string
}

func main() {
	// Top-level help: describe ccc's own interface, not claude's.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			printUsage(os.Stdout)
			return
		}
	}

	// Token subcommand
	if len(os.Args) > 1 && os.Args[1] == "token" {
		handleTokenCommand(os.Args[2:])
		return
	}

	// Provider subcommand
	if len(os.Args) > 1 && os.Args[1] == "provider" {
		handleProviderCommand(os.Args[2:])
		return
	}

	// Recursion guard
	if os.Getenv("_CCC_WRAPPED") != "" {
		realBin := os.Getenv("_CCC_REAL_BIN")
		if err := syscall.Exec(realBin, append([]string{realBin}, os.Args[1:]...), os.Environ()); err != nil {
			fmt.Fprintf(os.Stderr, "exec %s: %v\n", realBin, err)
			os.Exit(1)
		}
	}
	os.Setenv("_CCC_WRAPPED", "1")
	defer os.Unsetenv("_CCC_WRAPPED")
	defer os.Unsetenv("_CCC_REAL_BIN")

	// Load config
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	home := os.Getenv("HOME")

	// Prevent nested session detection
	os.Unsetenv("CLAUDECODE")

	// Discover real binary
	realBin := discoverClaude(home)
	os.Setenv("_CCC_REAL_BIN", realBin)

	flags, err := parseWrapperArgs(os.Args[1:], cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	// Official is the default: "" names no provider and no transport.
	providerName, err := selectProvider(flags, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	// A provider only applies when the caller has not pinned a transport.
	applied := appliedProvider(providerName)

	infoOnly := isInfoOnlyInvocation(flags.passthrough)

	if flags.printPlan {
		if err := printPlan(cfg, providerName, applied, flags, realBin); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		return
	}

	// Info-only invocations never touch provider state or settings.
	if infoOnly {
		execClaude(realBin, flags.passthrough)
	}

	// ccc owns the transport in the process environment only, and Claude Code
	// merges settings.env over the process environment: any base URL left in
	// settings.json would beat everything ccc injects. Scrubbing first is what
	// makes an official run official, so it is fail-closed.
	if err := scrubSettings(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}

	environment, err := environmentFor(cfg, applied)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	for key, value := range environment {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintf(os.Stderr, "set %s: %v\n", key, err)
			os.Exit(2)
		}
	}

	execClaude(realBin, buildArgs(cfg, applied, flags))
}

// parseWrapperArgs consumes ccc's own flags and keeps everything else for
// claude.
//
// `--relay <name>` is only read as a name when <name> is a configured provider:
// anything else stays a claude argument, so `ccc --relay "fix the tests"` still
// works. Use `--relay=<name>` when a prompt could read as a provider.
func parseWrapperArgs(args []string, cfg *config) (wrapperFlags, error) {
	var flags wrapperFlags
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--provider":
			if i+1 >= len(args) {
				return flags, fmt.Errorf("--provider requires a value")
			}
			i++
			flags.provider = args[i]
		case strings.HasPrefix(arg, "--provider="):
			flags.provider = strings.TrimPrefix(arg, "--provider=")
		case arg == "--relay":
			flags.relay = true
			if i+1 < len(args) {
				if _, known := cfg.Providers[args[i+1]]; known {
					i++
					flags.relayName = args[i]
				}
			}
		case strings.HasPrefix(arg, "--relay="):
			flags.relay = true
			flags.relayName = strings.TrimPrefix(arg, "--relay=")
		case arg == "--official":
			// Official is the default; the flag exists so scripts can say it.
		case arg == "--safe":
			flags.safe = true
		case arg == "--print-plan":
			flags.printPlan = true
		default:
			flags.passthrough = append(flags.passthrough, arg)
		}
	}
	return flags, nil
}

// selectProvider names the provider this run asks for; "" is an official run.
func selectProvider(flags wrapperFlags, cfg *config) (string, error) {
	name := flags.provider
	if name == "" {
		name = flags.relayName
	}
	if name == "" && flags.relay {
		name = cfg.DefaultProvider
		if name == "" {
			if len(cfg.Providers) > 0 {
				return "", fmt.Errorf("no relay provider configured\n  Run: ccc provider set-default <name>\n  Or: ccc provider list")
			}
			return "", fmt.Errorf("no providers configured\n  Run: ccc provider add <name> --base-url <url>")
		}
	}
	if name == "" {
		return "", nil
	}
	if _, ok := cfg.Providers[name]; !ok {
		return "", fmt.Errorf("provider %q not found in config (%s)\n  Run: ccc provider list", name, configPath())
	}
	return name, nil
}

// appliedProvider is the provider whose transport reaches claude: none for an
// official run, and none when the caller already pinned ANTHROPIC_BASE_URL.
func appliedProvider(providerName string) string {
	if providerName == "" || os.Getenv("ANTHROPIC_BASE_URL") != "" {
		return ""
	}
	return providerName
}

// buildArgs assembles the command line: ccc's standing args, the provider's
// args when one applies, then whatever the user asked claude for. Duplicates
// are dropped so a provider's args do not double a flag ccc already adds.
func buildArgs(cfg *config, applied string, flags wrapperFlags) []string {
	seen := make(map[string]bool, len(flags.passthrough))
	for _, arg := range flags.passthrough {
		seen[arg] = true
	}

	var args []string
	add := func(arg string) {
		if flags.safe && arg == "--dangerously-skip-permissions" {
			return
		}
		if seen[arg] {
			return
		}
		seen[arg] = true
		args = append(args, arg)
	}
	for _, arg := range cfg.Args {
		add(arg)
	}
	if applied != "" {
		for _, arg := range cfg.Providers[applied].Args {
			add(arg)
		}
	}
	return append(args, flags.passthrough...)
}

// printPlan shows what ccc would do without doing it: no settings write, no
// environment mutation, no claude.
func printPlan(cfg *config, providerName, applied string, flags wrapperFlags, realBin string) error {
	mode := "official (claude's own login)"
	switch {
	case applied != "":
		mode = fmt.Sprintf("relay · %s", applied)
	case providerName != "":
		mode = fmt.Sprintf("caller transport (%s skipped: ANTHROPIC_BASE_URL is already set)", providerName)
	}
	fmt.Printf("mode:      %s\n", mode)
	fmt.Printf("claude:    %s\n", realBin)

	argv := append([]string{"claude"}, buildArgs(cfg, applied, flags)...)
	fmt.Printf("argv:      %s\n", strings.Join(argv, " "))

	environment, err := environmentFor(cfg, applied)
	if err != nil {
		return err
	}
	if len(environment) == 0 {
		fmt.Printf("env:       unchanged\n")
	} else {
		keys := sortedKeys(environment)
		first := true
		for _, key := range keys {
			value := environment[key]
			if isCredentialKey(key) {
				value = maskToken(value)
			}
			if first {
				fmt.Printf("env:       %s=%s\n", key, value)
				first = false
				continue
			}
			fmt.Printf("           %s=%s\n", key, value)
		}
	}

	path := claudeSettingsPath()
	if os.Getenv("CCC_NO_SYNC_SETTINGS") != "" {
		fmt.Printf("settings:  %s (not touched: CCC_NO_SYNC_SETTINGS is set)\n", path)
		return nil
	}
	stale, err := scrubPreview(cfg)
	if err != nil {
		return err
	}
	if len(stale) == 0 {
		fmt.Printf("settings:  %s (holds no transport)\n", path)
		return nil
	}
	fmt.Printf("settings:  %s (will drop %s)\n", path, strings.Join(stale, ", "))
	return nil
}

func isCredentialKey(key string) bool {
	return strings.Contains(key, "TOKEN") || strings.Contains(key, "KEY") || strings.Contains(key, "SECRET")
}

func execClaude(realBin string, args []string) {
	// Exec — replaces this process entirely
	argv := append([]string{realBin}, args...)
	env := os.Environ()

	if err := syscall.Exec(realBin, argv, env); err != nil {
		// Fallback: run as subprocess
		cmd := exec.Command(realBin, args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = env
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			}
			os.Exit(1)
		}
	}
	os.Exit(0)
}

func printUsage(w *os.File) {
	fmt.Fprint(w, `ccc — Claude Code Companion (config-driven wrapper)

Usage:
  ccc [--relay [name]] [--provider <name>] [--safe] [claude args...]
  ccc <subcommand> ...

By default ccc runs claude on its own login: no base URL, no token, official.
A provider — a relay — is opt-in for that one run.

Wrapper flags (consumed by ccc, not forwarded):
  --relay [name]      Use a relay: the config's default_provider, or <name>.
                      Relay state is never persisted, so `+"`claude`"+` stays official.
  --provider <name>   Use the named provider (same as --relay <name>).
  --official          Force the official run (the default; for scripts).
  --safe              Strip --dangerously-skip-permissions from ccc's args
  --print-plan        Show mode, argv, env and settings changes, then stop.
  -h, --help          Show this help

Subcommands:
  provider add|list|remove|set-default   Manage providers in config.json
  token    set|get|list|delete           Manage tokens in macOS Keychain

Pass-through:
  Any other args are forwarded to the real claude binary. Provider setup is
  skipped for info-only invocations (--version, update, doctor, mcp, config,
  login, logout) and when ANTHROPIC_BASE_URL is already set by the caller.

Config:  ~/.config/ccc/config.json
`)
}

func isInfoOnlyInvocation(args []string) bool {
	for _, a := range args {
		if a == "--version" || a == "-v" || a == "--help" || a == "-h" {
			return true
		}
	}
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "update", "doctor", "mcp", "config", "login", "logout":
		return true
	default:
		return false
	}
}
