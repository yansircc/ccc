package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

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

	// Parse wrapper-specific flags; everything else is passed through
	var providerName string
	safe := false
	var userArgs []string

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case arg == "--provider" || strings.HasPrefix(arg, "--provider="):
			name := strings.TrimPrefix(arg, "--provider=")
			if name == "" && i+1 < len(os.Args) {
				name = os.Args[i+1]
				i++
			}
			if name == "" {
				fmt.Fprintln(os.Stderr, "--provider requires a value")
				os.Exit(2)
			}
			providerName = name
		case arg == "--safe":
			safe = true
		default:
			userArgs = append(userArgs, arg)
		}
	}

	// Determine provider: --provider flag > config default
	if providerName == "" {
		providerName = cfg.DefaultProvider
	}

	infoOnly := isInfoOnlyInvocation(userArgs)

	// Without a default provider and no external base URL, running ccc would
	// silently pass through to claude with whatever settings.json still pins.
	// Fail loudly instead of inheriting stale provider state.
	if !infoOnly && providerName == "" && os.Getenv("ANTHROPIC_BASE_URL") == "" {
		if len(cfg.Providers) > 0 {
			fmt.Fprintf(os.Stderr, "No default provider configured.\n  Run: ccc provider set-default <name>\n")
		} else {
			fmt.Fprintln(os.Stderr, "No providers configured.\n  Run: ccc provider add <name> --base-url <url>")
		}
		os.Exit(2)
	}

	var finalArgs []string

	if !infoOnly && providerName != "" && os.Getenv("ANTHROPIC_BASE_URL") == "" {
		// Setup provider environment
		if err := setupProvider(providerName, cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}

		// Build args from provider config
		if p, ok := cfg.Providers[providerName]; ok {
			for _, a := range p.Args {
				if safe && a == "--dangerously-skip-permissions" {
					continue
				}
				finalArgs = append(finalArgs, a)
			}
		}
	}

	finalArgs = append(finalArgs, userArgs...)

	// Exec — replaces this process entirely
	argv := append([]string{realBin}, finalArgs...)
	env := os.Environ()

	if err := syscall.Exec(realBin, argv, env); err != nil {
		// Fallback: run as subprocess
		cmd := exec.Command(realBin, finalArgs...)
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
}

func printUsage(w *os.File) {
	fmt.Fprint(w, `ccc — Claude Code Companion (config-driven wrapper)

Usage:
  ccc [--provider <name>] [--safe] [claude args...]
  ccc <subcommand> ...

Wrapper flags (consumed by ccc, not forwarded):
  --provider <name>   Use the named provider (overrides default_provider)
  --safe              Strip --dangerously-skip-permissions from provider args
  -h, --help          Show this help

Subcommands:
  provider add|list|remove|set-default   Manage providers in config.json
  token    set|get|list|delete           Manage tokens in macOS Keychain

Pass-through:
  Any other args are forwarded to the real claude binary. Provider setup is
  skipped for info-only invocations (--version, update, doctor, mcp, config,
  login, logout) and when ANTHROPIC_BASE_URL is already set.

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
