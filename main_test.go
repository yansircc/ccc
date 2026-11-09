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

func parseTestArgs(t *testing.T, args ...string) (wrapperFlags, error) {
	t.Helper()
	cfg := &config{Providers: map[string]providerConfig{
		"glm":   {BaseURL: "https://glm.example"},
		"relay": {BaseURL: "https://relay.example"},
	}}
	return parseWrapperArgs(args, cfg)
}

func TestParseWrapperArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want wrapperFlags
	}{
		{
			name: "nothing",
			args: nil,
			want: wrapperFlags{},
		},
		{
			name: "relay is opt-in and unflagged args pass through",
			args: []string{"--relay", "-p", "hi"},
			want: wrapperFlags{relay: true, passthrough: []string{"-p", "hi"}},
		},
		{
			name: "relay with a named provider",
			args: []string{"--relay=glm"},
			want: wrapperFlags{relay: true, relayName: "glm"},
		},
		{
			name: "relay takes a configured provider name after it",
			args: []string{"--relay", "glm"},
			want: wrapperFlags{relay: true, relayName: "glm"},
		},
		{
			name: "a prompt that is not a provider stays a prompt",
			args: []string{"--relay", "fix the tests"},
			want: wrapperFlags{relay: true, passthrough: []string{"fix the tests"}},
		},
		{
			name: "a version-looking argument is never eaten",
			args: []string{"--relay", "--version"},
			want: wrapperFlags{relay: true, passthrough: []string{"--version"}},
		},
		{
			name: "provider space and equals forms",
			args: []string{"--provider", "glm", "--provider=kimi"},
			want: wrapperFlags{provider: "kimi"},
		},
		{
			name: "official is accepted and consumed",
			args: []string{"--official", "--resume"},
			want: wrapperFlags{passthrough: []string{"--resume"}},
		},
		{
			name: "safe and print-plan are wrapper-only",
			args: []string{"--safe", "--print-plan"},
			want: wrapperFlags{safe: true, printPlan: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTestArgs(t, tt.args...)
			if err != nil {
				t.Fatalf("parseWrapperArgs(%v) error = %v", tt.args, err)
			}
			if got.provider != tt.want.provider || got.relay != tt.want.relay ||
				got.relayName != tt.want.relayName || got.safe != tt.want.safe ||
				got.printPlan != tt.want.printPlan {
				t.Errorf("parseWrapperArgs(%v) = %+v, want %+v", tt.args, got, tt.want)
			}
			if len(got.passthrough) != len(tt.want.passthrough) {
				t.Fatalf("passthrough = %v, want %v", got.passthrough, tt.want.passthrough)
			}
			for i := range tt.want.passthrough {
				if got.passthrough[i] != tt.want.passthrough[i] {
					t.Errorf("passthrough[%d] = %q, want %q", i, got.passthrough[i], tt.want.passthrough[i])
				}
			}
		})
	}
}

func TestParseWrapperArgs_ProviderNeedsValue(t *testing.T) {
	if _, err := parseTestArgs(t, "--provider"); err == nil {
		t.Fatal("--provider without a value was accepted")
	}
}

func TestSelectProvider(t *testing.T) {
	cfg := &config{
		DefaultProvider: "relay",
		Providers: map[string]providerConfig{
			"relay": {BaseURL: "https://relay.example"},
			"glm":   {BaseURL: "https://glm.example"},
		},
	}
	tests := []struct {
		name  string
		flags wrapperFlags
		want  string
	}{
		{"default run is official", wrapperFlags{}, ""},
		{"relay uses the configured default", wrapperFlags{relay: true}, "relay"},
		{"named relay wins over the default", wrapperFlags{relay: true, relayName: "glm"}, "glm"},
		{"provider wins over relay", wrapperFlags{relay: true, relayName: "glm", provider: "relay"}, "relay"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectProvider(tt.flags, cfg)
			if err != nil {
				t.Fatalf("selectProvider() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("selectProvider() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSelectProvider_Errors(t *testing.T) {
	t.Run("unknown provider is refused, not ignored", func(t *testing.T) {
		cfg := &config{Providers: map[string]providerConfig{"relay": {}}}
		if _, err := selectProvider(wrapperFlags{provider: "typo"}, cfg); err == nil {
			t.Fatal("unknown --provider was accepted")
		}
	})
	t.Run("relay without a default provider is refused", func(t *testing.T) {
		cfg := &config{Providers: map[string]providerConfig{"relay": {}}}
		if _, err := selectProvider(wrapperFlags{relay: true}, cfg); err == nil {
			t.Fatal("--relay without default_provider was accepted")
		}
	})
	t.Run("relay without providers is refused", func(t *testing.T) {
		cfg := &config{Providers: map[string]providerConfig{}}
		if _, err := selectProvider(wrapperFlags{relay: true}, cfg); err == nil {
			t.Fatal("--relay without providers was accepted")
		}
	})
}

func TestAppliedProvider(t *testing.T) {
	t.Run("official", func(t *testing.T) {
		t.Setenv("ANTHROPIC_BASE_URL", "")
		if got := appliedProvider(""); got != "" {
			t.Errorf("appliedProvider(\"\") = %q, want empty", got)
		}
	})
	t.Run("relay", func(t *testing.T) {
		t.Setenv("ANTHROPIC_BASE_URL", "")
		if got := appliedProvider("relay"); got != "relay" {
			t.Errorf("appliedProvider(relay) = %q, want relay", got)
		}
	})
	t.Run("caller pinned the transport", func(t *testing.T) {
		t.Setenv("ANTHROPIC_BASE_URL", "https://pinned.example")
		if got := appliedProvider("relay"); got != "" {
			t.Errorf("appliedProvider(relay) = %q, want empty when the caller pinned a base URL", got)
		}
	})
}

func TestBuildArgs(t *testing.T) {
	cfg := &config{
		Args: []string{"--dangerously-skip-permissions"},
		Providers: map[string]providerConfig{
			"relay": {Args: []string{"--dangerously-skip-permissions", "--model", "relay-model"}},
		},
	}

	t.Run("official run carries standing args only", func(t *testing.T) {
		got := buildArgs(cfg, "", wrapperFlags{passthrough: []string{"-p", "hi"}})
		want := []string{"--dangerously-skip-permissions", "-p", "hi"}
		assertArgs(t, got, want)
	})

	t.Run("relay adds provider args without duplicating", func(t *testing.T) {
		got := buildArgs(cfg, "relay", wrapperFlags{})
		want := []string{"--dangerously-skip-permissions", "--model", "relay-model"}
		assertArgs(t, got, want)
	})

	t.Run("--safe strips skip-permissions", func(t *testing.T) {
		got := buildArgs(cfg, "relay", wrapperFlags{safe: true})
		want := []string{"--model", "relay-model"}
		assertArgs(t, got, want)
	})

	t.Run("a user-supplied flag is not injected twice", func(t *testing.T) {
		got := buildArgs(cfg, "relay", wrapperFlags{passthrough: []string{"--dangerously-skip-permissions", "--resume"}})
		want := []string{"--model", "relay-model", "--dangerously-skip-permissions", "--resume"}
		assertArgs(t, got, want)
	})
}

func assertArgs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}
