package main

import (
	"os"
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

func TestSetupProvider_Success(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("CCC_MYCLOUD_TOKEN", "secret-tok")
	// Clear env vars that setupProvider will set
	t.Setenv("ANTHROPIC_BASE_URL", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("CUSTOM_HEADER", "")

	cfg := &config{
		Providers: map[string]providerConfig{
			"mycloud": {
				BaseURL: "https://mycloud.example.com/api",
				Env:     map[string]string{"CUSTOM_HEADER": "X-Custom: yes"},
			},
		},
	}

	err := setupProvider("mycloud", cfg)
	if err != nil {
		t.Fatalf("setupProvider() error = %v", err)
	}

	if got := os.Getenv("ANTHROPIC_BASE_URL"); got != "https://mycloud.example.com/api" {
		t.Errorf("ANTHROPIC_BASE_URL = %q, want %q", got, "https://mycloud.example.com/api")
	}
	if got := os.Getenv("ANTHROPIC_AUTH_TOKEN"); got != "secret-tok" {
		t.Errorf("ANTHROPIC_AUTH_TOKEN = %q, want %q", got, "secret-tok")
	}
	if got := os.Getenv("CUSTOM_HEADER"); got != "X-Custom: yes" {
		t.Errorf("CUSTOM_HEADER = %q, want %q", got, "X-Custom: yes")
	}
}

func TestSetupProvider_NotFound(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg := &config{
		Providers: map[string]providerConfig{},
	}

	err := setupProvider("nonexist", cfg)
	if err == nil {
		t.Fatal("setupProvider() expected error for missing provider, got nil")
	}
}

func TestSetupProvider_EmptyToken(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("CCC_GHOST_TOKEN", "")

	cfg := &config{
		Providers: map[string]providerConfig{
			"ghost": {BaseURL: "https://ghost.example.com"},
		},
	}

	err := setupProvider("ghost", cfg)
	if err == nil {
		t.Fatal("setupProvider() expected error for empty token, got nil")
	}
}
