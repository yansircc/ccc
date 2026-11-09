package main

import (
	"testing"
)

func TestResolveToken_EnvVar(t *testing.T) {
	t.Setenv("CCC_ACME_TOKEN", "env-token-123")
	got := resolveToken("acme")
	if got != "env-token-123" {
		t.Errorf("resolveToken(acme) = %q, want %q", got, "env-token-123")
	}
}

func TestTokenEnvironmentNameNormalizesProviderIdentity(t *testing.T) {
	for input, want := range map[string]string{
		"acme":        "CCC_ACME_TOKEN",
		"kimi-coding": "CCC_KIMI_CODING_TOKEN",
		"a.b/c d":     "CCC_A_B_C_D_TOKEN",
		"模型-1":        "CCC____1_TOKEN",
	} {
		if got := tokenEnvironmentName(input); got != want {
			t.Errorf("tokenEnvironmentName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestResolveToken_NormalizedEnvVar(t *testing.T) {
	t.Setenv("CCC_KIMI_CODING_TOKEN", "env-token-456")
	if got := resolveToken("kimi-coding"); got != "env-token-456" {
		t.Errorf("resolveToken(kimi-coding) = %q, want normalized env token", got)
	}
}

func TestResolveToken_EnvVarEmpty_NoKeychain(t *testing.T) {
	t.Setenv("CCC_ACME_TOKEN", "")
	// Keychain won't have an entry in test env, so we expect ""
	got := resolveToken("acme")
	if got != "" {
		t.Errorf("resolveToken(acme) = %q, want empty", got)
	}
}

func TestEnvironmentFor_RelayCarriesTransport(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CCC_MYCLOUD_TOKEN", "secret-tok")

	cfg := &config{
		Env: map[string]string{"STANDING": "1", "SHARED": "standing"},
		Providers: map[string]providerConfig{
			"mycloud": {
				BaseURL: "https://mycloud.example.com/api",
				Env:     map[string]string{"CUSTOM_HEADER": "X-Custom: yes", "SHARED": "provider"},
			},
		},
	}

	environment, err := environmentFor(cfg, "mycloud")
	if err != nil {
		t.Fatalf("environmentFor() error = %v", err)
	}

	want := map[string]string{
		"ANTHROPIC_BASE_URL":   "https://mycloud.example.com/api",
		"ANTHROPIC_AUTH_TOKEN": "secret-tok",
		"CUSTOM_HEADER":        "X-Custom: yes",
		"STANDING":             "1",
		"SHARED":               "provider",
	}
	if len(environment) != len(want) {
		t.Fatalf("environment = %v, want %v", environment, want)
	}
	for key, value := range want {
		if environment[key] != value {
			t.Errorf("environment[%s] = %q, want %q", key, environment[key], value)
		}
	}
}

// An official run adds ccc's standing env and no transport whatsoever.
func TestEnvironmentFor_OfficialAddsNoTransport(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := &config{
		Env:       map[string]string{"DISABLE_TELEMETRY": "1"},
		Providers: map[string]providerConfig{"relay": {BaseURL: "https://relay.example"}},
	}
	environment, err := environmentFor(cfg, "")
	if err != nil {
		t.Fatalf("environmentFor() error = %v", err)
	}
	if _, ok := environment["ANTHROPIC_BASE_URL"]; ok {
		t.Error("official run carries a base URL")
	}
	if _, ok := environment["ANTHROPIC_AUTH_TOKEN"]; ok {
		t.Error("official run carries a token")
	}
	if environment["DISABLE_TELEMETRY"] != "1" {
		t.Error("standing env was dropped from an official run")
	}
}

func TestEnvironmentFor_NotFound(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := &config{Providers: map[string]providerConfig{}}

	if _, err := environmentFor(cfg, "nonexist"); err == nil {
		t.Fatal("environmentFor() expected error for missing provider, got nil")
	}
}

func TestEnvironmentFor_EmptyToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CCC_GHOST_TOKEN", "")

	cfg := &config{
		Providers: map[string]providerConfig{
			"ghost": {BaseURL: "https://ghost.example.com"},
		},
	}

	if _, err := environmentFor(cfg, "ghost"); err == nil {
		t.Fatal("environmentFor() expected error for empty token, got nil")
	}
}
