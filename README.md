# CCC — Claude Code Companion

A lightweight wrapper for Claude Code with config-driven multi-provider switching and macOS Keychain token management.

## Install

```bash
git clone https://github.com/yanxir/ccc.git
cd ccc
go build -o ccc .
cp ccc ~/.local/bin/  # ensure it's in PATH and takes priority over claude
```

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

Tokens are stored securely in macOS Keychain. Environment variable `CCC_<NAME>_TOKEN` takes priority over Keychain.

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
ccc --version              # Info-only, skip provider setup, pass through directly
```

## Behavior

- `--provider` flag and `default_provider` in config are the only provider sources
- If `ANTHROPIC_BASE_URL` is already set by the caller, provider setup is skipped
- Provider `args` are prepended to the claude command; `--safe` filters out `--dangerously-skip-permissions`
- Provider `env` entries are set as environment variables before launch (e.g. model overrides, feature flags)
- Info-only invocations (`--version`/`--help`/`update`/`doctor` etc.) skip provider and token resolution
