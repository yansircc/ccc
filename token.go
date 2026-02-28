package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func handleTokenCommand(args []string) {
	if len(args) == 0 {
		tokenUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "set":
		if len(args) != 3 {
			fmt.Fprintln(os.Stderr, "Usage: ccc token set <provider> <value>")
			os.Exit(1)
		}
		name, value := args[1], args[2]
		cfg, err := loadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}
		if _, ok := cfg.Providers[name]; !ok {
			fmt.Fprintf(os.Stderr, "Unknown provider: %s (not in config)\n", name)
			os.Exit(1)
		}
		if err := keychainSet(name, value); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Token for %q stored in keychain.\n", name)

	case "get":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "Usage: ccc token get <provider>")
			os.Exit(1)
		}
		token, err := keychainGet(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "No token found for %q: %v\n", args[1], err)
			os.Exit(1)
		}
		fmt.Println(token)

	case "list":
		cfg, err := loadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}
		names := make([]string, 0, len(cfg.Providers))
		for k := range cfg.Providers {
			names = append(names, k)
		}
		sort.Strings(names)
		if len(names) == 0 {
			fmt.Println("No providers configured.")
			return
		}
		for _, name := range names {
			token, err := keychainGet(name)
			if err != nil || token == "" {
				fmt.Printf("  %-12s (not set)\n", name)
			} else {
				fmt.Printf("  %-12s %s\n", name, maskToken(token))
			}
		}

	case "delete":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "Usage: ccc token delete <provider>")
			os.Exit(1)
		}
		if err := keychainDelete(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Token for %q deleted from keychain.\n", args[1])

	default:
		tokenUsage()
		os.Exit(1)
	}
}


func maskToken(s string) string {
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}

func tokenUsage() {
	fmt.Fprintln(os.Stderr, `Usage: ccc token <command>

Commands:
  set <provider> <value>   Store a token in keychain
  get <provider>           Retrieve a token from keychain
  list                     List all provider tokens (masked)
  delete <provider>        Remove a token from keychain`)
}
