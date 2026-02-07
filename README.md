# devtap

[![Go Reference](https://pkg.go.dev/badge/github.com/killme2008/devtap.svg)](https://pkg.go.dev/github.com/killme2008/devtap)
[![Latest Release](https://img.shields.io/github/v/release/killme2008/devtap)](https://github.com/killme2008/devtap/releases)
[![CI](https://github.com/killme2008/devtap/actions/workflows/ci.yml/badge.svg)](https://github.com/killme2008/devtap/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/killme2008/devtap)](https://goreportcard.com/report/github.com/killme2008/devtap)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Bridge build/dev process output to AI coding sessions automatically.

`devtap` captures stdout/stderr from build and development commands, then feeds them into AI coding tool sessions via [MCP](https://modelcontextprotocol.io/) (Model Context Protocol). It can also fan out the same output to multiple coding agents. Each agent drains its own copy so parallel sessions don't interfere.

## The Problem

In vibe coding workflows, you run an AI coding tool in one terminal and build commands in another. When errors occur, you manually copy-paste logs into the coding session. `devtap` automates this feedback loop.

Three common cases make this especially painful:

1. Multiple local dev processes (frontend + backend + worker) that you need to keep an eye on and fix quickly.
2. Multiple coding agents working on the same project, where you want them to analyze the same failures in parallel and compare findings.
3. Build/test runs on remote machines (CI, dev boxes) whose output you need to feed back to a local coding agent.

## Quick Start

### Install

```bash
go install github.com/killme2008/devtap/cmd/devtap@latest
```

Or install via Homebrew:

```bash
brew install killme2008/tap/devtap
```

Or download from [GitHub Releases](https://github.com/killme2008/devtap/releases).

### Setup

```bash
cd /path/to/your-project
devtap install --adapter claude-code
```

This configures the MCP server and injects devtap instructions into your project's instruction file (e.g., `CLAUDE.md`). See [Supported Tools](#supported-tools) for all available adapters.

### Skills (Optional)

devtap ships with a built-in [skill](https://docs.anthropic.com/en/docs/claude-code/skills) that works as a CLI fallback when MCP is unavailable. Install it to let the agent fetch build output on demand:

```bash
# Claude Code
mkdir -p ~/.claude/skills && cp -r skills/devtap-get-build-errors ~/.claude/skills/

# Codex CLI
mkdir -p ~/.codex/skills && cp -r skills/devtap-get-build-errors ~/.codex/skills/
```

Or fetch directly from the repo without cloning:

```bash
DEST=~/.claude/skills/devtap-get-build-errors
mkdir -p "$DEST" && cd "$DEST"
for f in SKILL.md scripts/get_build_errors.sh; do
  mkdir -p "$(dirname "$f")"
  curl -sL "https://raw.githubusercontent.com/killme2008/devtap/main/skills/devtap-get-build-errors/$f" -o "$f"
done
chmod +x scripts/get_build_errors.sh
```

### Usage

**Terminal A** — capture build output:

```bash
devtap -- cargo check
devtap -- go build ./...
devtap --filter-regex "error|warning" -- npm run build

# Long-running dev servers (output is flushed every 2s by default)
devtap -- npm run dev
```

**Terminal B** — use your AI coding tool as usual. It will automatically call `get_build_errors` via MCP to fetch captured build errors.

If you want to verify without MCP, run:

```bash
devtap drain
```

Typical output:

```text
[devtap: cargo check] Build failed (exit code 101):
...
```

**Tip:** Since devtap captures stdout from any command, you can send arbitrary messages to your coding agent:

```bash
devtap -- echo "Please refactor the auth module to use JWT"
```

This turns devtap into a general-purpose human→agent message channel — no copy-paste needed.

## How It Works

**Local mode** (default, file-based):

```
Terminal A (Claude Code)          Terminal B (build/dev)
┌──────────────────┐             ┌────────────────────────────┐
│  MCP tool call:  │   stdio     │  devtap -- cargo check     │
│  get_build_errors├─────────────┤                            │
│                  │  JSON-RPC   │  captures stdout/stderr,   │
│  receives errors,│             │  fans out to all adapters: │
│  fixes code      │             │  ~/.devtap/<s>/claude-code/│
└──────────────────┘             │  ~/.devtap/<s>/codex/      │
                                 └────────────────────────────┘
```

**Cross-machine mode** (with [GreptimeDB](#greptimedb-optional)):

```
Your laptop                       CI / remote build server
┌──────────────────┐             ┌────────────────────────────┐
│  Claude Code     │             │  devtap -- make            │
│  get_build_errors│             │                            │
│                  │             │  captures stdout/stderr    │
│  receives errors,│             └─────────────┬──────────────┘
│  fixes code      │                           │ write
└────────┬─────────┘                           ▼
         │ drain              ┌────────────────────────────┐
         └───────────────────►│        GreptimeDB          │
                              │  (shared session store)    │
                              └────────────────────────────┘
```

1. `devtap install` configures the MCP server for your AI tool (pass `--session` and `--store` for cross-machine setup)
2. `devtap -- <cmd>` runs your command, captures stdout/stderr, fans out to all registered adapters
3. Each AI tool independently drains its own copy via `get_build_errors`
4. AI sees the errors and fixes them

When `mcp-serve`/`drain` is started with explicit `--session` or `--store`, devtap can merge output from two sources:

- `local`: auto-detected project session from your default backend
- `configured`: the explicit `--session`/`--store` target

If both resolve to the same backend+session, devtap uses a single source. Otherwise it drains both, deduplicates identical messages, and prefixes tags with source info (for example `myhost/local |`).

## Supported Tools

| Tool | Adapter | Integration | Config File | Instruction File |
|------|---------|-------------|-------------|------------------|
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | `claude-code` | MCP server | `.mcp.json` | `CLAUDE.md` |
| [Codex CLI](https://github.com/openai/codex) | `codex` | MCP server | `.codex/config.toml` | `AGENTS.md` |
| [OpenCode](https://opencode.ai) | `opencode` | MCP server | `opencode.json` | `AGENTS.md` |
| [Gemini CLI](https://github.com/google-gemini/gemini-cli) | `gemini` | MCP server | `.gemini/settings.json` | `GEMINI.md` |
| [aider](https://aider.chat) | `aider` | `--lint-cmd` wrapper | `.devtap-aider-lint.sh` | `CONVENTIONS.md` |

Any MCP-compatible tool can use `devtap mcp-serve` directly.

## Auto-loop Mode (Claude Code)

Claude Code supports a Stop hook that can block Claude from stopping when errors remain:

```bash
devtap install --adapter claude-code --auto-loop --max-retries 5
```

This configures:
- MCP server for on-demand error queries
- Stop hook that blocks Claude from finishing if build errors are pending
- Safety limit of 5 retries before allowing stop

## Storage Backends

### File (default)

Zero-dependency JSONL files at `~/.devtap/<session>/<adapter>/pending.jsonl`. Each adapter gets its own queue for independent consumption. Atomic rename for concurrency safety.

### [GreptimeDB](https://github.com/GreptimeTeam/greptimedb) (optional)

For persistent history, SQL-based filtering, and richer statistics. With a remote GreptimeDB instance, the build and the AI tool don't even need to be on the same machine — see [Cross-machine builds](#advanced-usage) for details.

See [GreptimeDB installation guide](https://docs.greptime.com/getting-started/installation/greptimedb-standalone/) for more options.

Quick start with Docker:

```bash
docker run -d \
  --name greptime-devtap \
  --restart unless-stopped \
  -p 127.0.0.1:4000-4002:4000-4002 \
  -v ~/.devtap/greptimedb_data:/greptimedb_data \
  greptime/greptimedb:latest standalone start \
  --http-addr 0.0.0.0:4000 \
  --rpc-bind-addr 0.0.0.0:4001 \
  --mysql-addr 0.0.0.0:4002
```

The container runs in the background (`-d`) and auto-starts with Docker (`--restart unless-stopped`). The `-v` flag mounts `~/.devtap/greptimedb_data/` for persistent storage.

```bash
# Configure in ~/.devtap/config.toml (this becomes the default store)
cat > ~/.devtap/config.toml <<EOF
[store]
backend = "greptimedb"

[store.greptimedb]
endpoint = "127.0.0.1:4001"
mysql_endpoint = "127.0.0.1:4002"
database = "public"
EOF

# Now devtap uses GreptimeDB by default
devtap -- cargo check

# SQL-based filtering
devtap drain --filter-sql "content LIKE '%error%'"

# Query build error history
devtap history --since 24h

# Or override per-command (e.g., fall back to file store)
devtap --store file -- cargo check

# View logs in GreptimeDB dashboard
open http://127.0.0.1:4000/dashboard/#/dashboard/logs-query
```

Credentials via environment variables:
```bash
export DEVTAP_GREPTIMEDB_USERNAME=...
export DEVTAP_GREPTIMEDB_PASSWORD=...
```

## Advanced Usage

**Multiple instances** — use `--tag` to run parallel watchers for the same session:

```bash
devtap --tag cargo-check -- cargo watch -x check
devtap --tag cargo-test --debounce 5s -- cargo watch -x test
```

**Multi-adapter fan-out** — when multiple AI tools are installed, build output is automatically delivered to all of them. Each tool independently consumes its own copy.

**Cross-machine builds** — with a shared [GreptimeDB](#greptimedb-optional) instance, the build and the AI tool can run on different machines. Use `--session` to give both sides the same logical session name:

```bash
# Machine B (your laptop) — install once, bakes --session and --store into MCP config
devtap install --adapter claude-code --session myproject --store greptimedb

# Machine A (CI / remote build server)
devtap --store greptimedb --session myproject -- make
```

`devtap install` writes the `--session` and `--store` flags into the MCP config file (e.g. `.mcp.json`), so the AI tool's MCP server automatically connects to the right GreptimeDB instance and session.

Multiple build machines can write to the same session simultaneously — each entry is tagged with its source, and the AI tool drains them all.

**Local + remote merged drain** — keep your local loop and a remote shared session visible at the same time:

```bash
# Laptop: local output (default source)
devtap -- cargo check

# Remote machine: shared session output (configured source)
devtap --store greptimedb --session myproject -- make

# Laptop: install MCP with explicit remote session
devtap install --adapter claude-code --store greptimedb --session myproject

# Optional manual check (without MCP client)
devtap drain --store greptimedb --session myproject
```

`devtap drain` shows a merged header (`Draining from N sources`), emits source-unreachable warnings when partial failures occur, and still returns available output from reachable sources.

Typical merged header:

```text
[devtap] Draining from 2 sources (2 reachable)
```

**Session auto-detection** — when `--session auto` (default), devtap resolves the project directory like this:
1. Git root (nearest parent with `.git`)
2. Project marker files (nearest parent with one of: `go.mod`, `package.json`, `pyproject.toml`, `Cargo.toml`, `pom.xml`, `build.gradle`, `build.gradle.kts`, `composer.json`, `Gemfile`, `setup.py`)
3. Current working directory

**Chaining commands** — `devtap` captures a single command after `--`. If you need shell operators like `&&`, wrap the command in a shell:

```bash
devtap -- sh -c "npm install && npm run dev"
```

Or run them separately:

```bash
devtap -- npm install
devtap -- npm run dev
```

## CLI Reference

```
devtap [flags] -- <command> [args...]

Flags:
  -a, --adapter <name>       AI tool adapter (default "claude-code")
  -s, --session <id>         Target session ("auto", "pick", or explicit name)
      --store <backend>      Storage backend ("file" or "greptimedb")
      --filter-regex <pat>   Regex filter for output lines
      --filter-invert        Invert filter (exclude matching lines)
      --max-lines <n>        Max lines per drain (default 10000)
      --tag <label>          Log tag prefix (default: command name)
      --debounce <dur>       Flush interval for captured output (default "2s", 0 to disable)

Subcommands:
  install     Configure AI tool integration (--session and --store are forwarded to MCP config)
  mcp-serve   Start MCP stdio server
  drain       Read pending messages as plain text
                -q, --quiet      Raw output without source/tag headers
  status      Show pending message counts
                -q, --quiet      Compact output (pending count only)
  history     Query build error history (GreptimeDB only)
                --since <dur>    Time range (default "24h")
                --tag <label>    Filter by tag
                --limit <n>      Max entries (default 20)
  gc          Remove expired session data (default TTL: 7 days)
```

Lines exceeding `--max-lines` are smart-truncated: head and tail preserved with omission notice. Consecutive duplicate lines are merged.

`devtap mcp-serve` and `devtap drain` can aggregate multiple sources (local + configured) as described above. `devtap drain --filter-sql` is a single-source mode and requires `--store greptimedb`.

## Troubleshooting

- No output but you expect logs: run `devtap status` first, then `devtap drain --max-lines 200`.
- Using an unexpected session: run with `--session pick` once to confirm the target session.
- Multi-source drain shows warnings: reachable sources are still returned; warnings indicate one source is unavailable.
- MCP tool not being called: re-run `devtap install --adapter <adapter>` in the project root and restart the AI tool session.

## Security & Privacy

- **All data stays local.** Build output is stored on your machine at `~/.devtap/` (file backend) or in a self-hosted GreptimeDB instance. Nothing is sent to external servers.
- **MCP communication is local stdio.** The MCP server runs as a child process of your AI tool, communicating via stdin/stdout JSON-RPC. No network sockets are opened.
- **No telemetry.** devtap collects no usage data, analytics, or crash reports.
- **`--filter-sql` is not a security boundary.** It includes a best-effort keyword blocklist but is designed for convenience, not adversarial input. The user already has full local and database access.
- **Cleanup:** Run `devtap gc` to remove expired session data, or delete `~/.devtap/` entirely to remove all stored data.

## License

MIT
