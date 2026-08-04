# CCC — Claude Code Companion

A lightweight wrapper for Claude Code with config-driven multi-provider switching and macOS Keychain token management.

## Install

### From release (recommended)

```bash
# macOS Apple Silicon
curl -Lo ccc https://github.com/yansircc/ccc/releases/latest/download/ccc-darwin-arm64
chmod +x ccc && mv ccc ~/.local/bin/

# macOS Intel
curl -Lo ccc https://github.com/yansircc/ccc/releases/latest/download/ccc-darwin-amd64
chmod +x ccc && mv ccc ~/.local/bin/

# Linux amd64
curl -Lo ccc https://github.com/yansircc/ccc/releases/latest/download/ccc-linux-amd64
chmod +x ccc && mv ccc ~/.local/bin/
```

### From source

```bash
git clone https://github.com/yansircc/ccc.git
cd ccc
make install                 # installs to ~/.local/bin/ccc
# or: make install PREFIX=/usr/local
```

Ensure the install dir is in `PATH` and takes priority over any existing `claude` binary.

## Configuration

Path: `~/.config/ccc/config.json` (managed via `ccc provider` commands, no manual editing needed)

```json
{
  "default_provider": "myproxy",
  "providers": {
    "myproxy": {
      "base_url": "https://your-proxy.example.com",
      "args": ["--dangerously-skip-permissions"],
      "env": { "CLAUDE_CODE_DISABLE_1M_CONTEXT": "1" }
    },
    "minimax": {
      "base_url": "https://api.minimax.io/anthropic",
      "env": { "ANTHROPIC_DEFAULT_SONNET_MODEL": "MiniMax-M2.5-highspeed" }
    }
  }
}
```

## Provider Management

```bash
# Add a provider
ccc provider add <name> --base-url <url> [--arg <arg>]... [--env KEY=VAL]...

# List all providers (* marks default)
ccc provider list

# Set default provider
ccc provider set-default <name>

# Remove a provider
ccc provider remove <name>
```

## Token Management

Tokens are stored securely in macOS Keychain. Environment variable
`CCC_<NORMALIZED_NAME>_TOKEN` takes priority over Keychain. Normalization
uppercases ASCII letters and replaces every non-alphanumeric character with
`_`, so `kimi-coding` resolves to `CCC_KIMI_CODING_TOKEN`.

```bash
ccc token set <provider> <value>   # Store in Keychain
ccc token get <provider>           # Retrieve
ccc token list                     # List all (masked)
ccc token delete <provider>        # Delete
```

## Usage

```bash
ccc                        # Use default provider
ccc --provider minimax     # Specify provider
ccc --safe                 # Filter --dangerously-skip-permissions from provider args
ccc -h                     # Show ccc's own help (wrapper flags + subcommands)
ccc --version              # Info-only, skip provider setup, pass through directly
```

## Behavior

- `--provider` flag and `default_provider` in config are the only provider sources
- If `ANTHROPIC_BASE_URL` is already set by the caller, provider setup is skipped
- Provider `args` are prepended to the claude command; `--safe` filters out `--dangerously-skip-permissions`
- Provider `env` entries are set as environment variables before launch (e.g. model overrides, feature flags)
- Info-only invocations (`--version`/`--help`/`update`/`doctor` etc.) skip provider and token resolution

## Settings sync

Recent Claude Code versions give `~/.claude/settings.json` env precedence over the
process environment, so env injection alone cannot switch the base URL or token.
On every non-info invocation ccc therefore **also persists the active provider
into `~/.claude/settings.json`** (env section):

- `ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN` are always managed by ccc
- All `ANTHROPIC_DEFAULT_*` / `ANTHROPIC_MODEL` keys are swept and replaced with
  the active provider's values, so stale model locks never leak between providers
- Other settings (permissions, hooks, MCP, unrelated env) are preserved
- Direct `claude` runs — wechat bridges, pawl, scripts — follow the same provider
- The file is replaced atomically and the run aborts on any read/write error
  (fail closed); set `CCC_NO_SYNC_SETTINGS=1` to disable syncing

> Tokens are written into settings.json in plain text (same as cc-switch does);
> they are also kept in the macOS Keychain as the source of truth.
