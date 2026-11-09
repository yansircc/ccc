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
  "default_provider": "relay210k",
  "args": ["--dangerously-skip-permissions"],
  "env": { "CLAUDE_CODE_MAX_RETRIES": "2" },
  "providers": {
    "relay210k": {
      "base_url": "https://ccc.210k.cc",
      "args": [],
      "env": { "DISABLE_TELEMETRY": "1" }
    },
    "minimax": {
      "base_url": "https://api.minimax.io/anthropic",
      "env": { "ANTHROPIC_DEFAULT_SONNET_MODEL": "MiniMax-M2.5-highspeed" }
    }
  }
}
```

- `default_provider` names the relay `ccc --relay` uses. It is **not** the default run.
- `args` / `env` at the top level are ccc's standing arguments and environment:
  they apply to an official run and a relay run alike.
- A provider's `args` / `env` apply only to the run that uses that provider.

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
ccc                        # official: claude on its own login, nothing injected
ccc --relay                # the config's default_provider, for this run only
ccc --relay glm            # that named provider, for this run only
ccc --provider glm         # same thing
ccc --official             # explicit official run (for scripts)
ccc --safe                 # drop --dangerously-skip-permissions from ccc's args
ccc --print-plan           # show mode, argv, env and settings changes, then stop
ccc -h                     # show ccc's own help (wrapper flags + subcommands)
ccc --version              # info-only, skip provider setup, pass through directly
```

## Behavior

- **Official is the default.** `ccc` adds no base URL and no token, so it uses
  whatever claude is logged in with. So does a plain `claude`.
- A relay is opt-in per run: `--relay` (the `default_provider`) or
  `--provider <name>`. The relay lives in the environment of the process ccc
  launches; nothing is persisted, so the next `claude` is official again.
- `--provider` wins over `--relay`, which wins over official.
- Naming a provider that does not exist is an error, not a silent official run.
- If `ANTHROPIC_BASE_URL` is already set by the caller, ccc injects no transport
  and passes through: a nested call (a subagent shell, a bridge script) keeps
  the transport its parent chose.
- Provider `args` are appended to ccc's standing `args`; duplicates are dropped,
  and `--safe` filters out `--dangerously-skip-permissions`.
- Provider `env` entries are set as environment variables before launch (e.g.
  model overrides, feature flags).
- Info-only invocations (`--version`/`--help`/`update`/`doctor` etc.) skip
  provider and token resolution, and never touch settings.json.

## Settings sync

Claude Code merges `~/.claude/settings.json`'s `env` section **over** the process
environment, so a base URL parked there beats anything ccc injects — and it keeps
a plain `claude` run on that relay too. ccc therefore keeps no transport in
settings.json at all:

- on every non-info run, ccc removes the env keys it owns — `ANTHROPIC_BASE_URL`,
  `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_MODEL`, everything in the
  `ANTHROPIC_DEFAULT_*` namespace, and every key a provider declares — so the
  next launch, ccc's or plain `claude`, is official
- the top-level `env` is not scrubbed: it is your standing preference, the same
  in every mode, so it may stay in settings.json (and plain `claude` keeps it);
  a provider may not contradict it with a different value for the same key
- other settings (permissions, hooks, MCP, `ANTHROPIC_API_KEY`, unrelated env)
  are preserved, and a file that is already clean is not rewritten
- the file is replaced atomically and private, and the run aborts on any
  read/write error (fail closed); set `CCC_NO_SYNC_SETTINGS=1` to keep ccc's
  hands off settings.json entirely
- relay tokens live in the macOS Keychain (`ccc token set <provider>`), never in
  settings.json
