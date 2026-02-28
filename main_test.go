package main

import "testing"

func TestIsInfoOnlyInvocation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"version flag", []string{"--version"}, true},
		{"short version", []string{"-v"}, true},
		{"help flag", []string{"--help"}, true},
		{"short help", []string{"-h"}, true},
		{"update subcommand", []string{"update"}, true},
		{"doctor subcommand", []string{"doctor"}, true},
		{"mcp subcommand", []string{"mcp"}, true},
		{"config subcommand", []string{"config"}, true},
		{"login subcommand", []string{"login"}, true},
		{"logout subcommand", []string{"logout"}, true},
		{"empty args", []string{}, false},
		{"regular args", []string{"--prompt", "hello"}, false},
		{"version in middle", []string{"--prompt", "--version"}, true},
		{"unknown subcommand", []string{"something"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInfoOnlyInvocation(tt.args)
			if got != tt.want {
				t.Errorf("isInfoOnlyInvocation(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
