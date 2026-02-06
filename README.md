# devtap

[![Go Reference](https://pkg.go.dev/badge/github.com/killme2008/devtap.svg)](https://pkg.go.dev/github.com/killme2008/devtap)
[![CI](https://github.com/killme2008/devtap/actions/workflows/ci.yml/badge.svg)](https://github.com/killme2008/devtap/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/killme2008/devtap)](https://goreportcard.com/report/github.com/killme2008/devtap)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Bridge build/dev process output to AI coding sessions automatically.

`devtap` captures stdout/stderr from build and development commands, then feeds them into AI coding tool sessions via [MCP](https://modelcontextprotocol.io/) (Model Context Protocol).

## The Problem

In vibe coding workflows, you run an AI coding tool in one terminal and build commands in another. When errors occur, you manually copy-paste logs into the coding session. `devtap` automates this feedback loop.

## Quick Start

### Install

```bash
go install github.com/killme2008/devtap/cmd/devtap@latest
```

Or download from [GitHub Releases](https://github.com/killme2008/devtap/releases).

### Setup

```bash
cd /path/to/your-project
devtap install --adapter claude-code
```

This configures the MCP server and injects devtap instructions into your project's instruction file (e.g., `CLAUDE.md`). See [Supported Tools](#supported-tools) for all available adapters.

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

**Tip:** Since devtap captures stdout from any command, you can send arbitrary messages to your coding agent:

```bash
devtap -- echo "Please refactor the auth module to use JWT"
```

This turns devtap into a general-purpose human→agent message channel — no copy-paste needed.

## How It Works

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

1. `devtap install` configures the MCP server for your AI tool
2. `devtap -- <cmd>` runs your command, captures stdout/stderr, fans out to all registered adapters
3. Each AI tool independently drains its own copy via `get_build_errors`
4. AI sees the errors and fixes them

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

For persistent history, SQL-based filtering, and richer statistics. With a remote GreptimeDB instance, Terminal A and Terminal B don't even need to be on the same machine — capture build output from a CI server or remote dev box and consume it from your local AI coding session.

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
# Configure in ~/.devtap/config.toml
cat > ~/.devtap/config.toml <<EOF
[store]
backend = "greptimedb"

[store.greptimedb]
endpoint = "127.0.0.1:4001"
mysql_endpoint = "127.0.0.1:4002"
database = "public"
EOF

# Or override per-command
devtap --store greptimedb -- cargo check

# SQL-based filtering
devtap drain --store greptimedb --filter-sql "content LIKE '%error%'"

# Query build error history
devtap history --since 24h

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
  -s, --session <id>         Target session ("auto", "pick", or UUID)
      --store <backend>      Storage backend ("file" or "greptimedb")
      --filter-regex <pat>   Regex filter for output lines
      --filter-invert        Invert filter (exclude matching lines)
      --max-lines <n>        Max lines per drain (default 10000)
      --tag <label>          Log tag prefix (default: command name)
      --debounce <dur>       Flush interval for captured output (default "2s", 0 to disable)

Subcommands:
  install     Configure AI tool integration
  mcp-serve   Start MCP stdio server
  drain       Read pending messages as plain text
  status      Show pending message counts
  history     Query build error history (GreptimeDB only)
                --since <dur>    Time range (default "24h")
                --tag <label>    Filter by tag
                --limit <n>      Max entries (default 20)
  gc          Remove expired session data (default TTL: 7 days)
```

Lines exceeding `--max-lines` are smart-truncated: head and tail preserved with omission notice. Consecutive duplicate lines are merged.

## Security & Privacy

- **All data stays local.** Build output is stored on your machine at `~/.devtap/` (file backend) or in a self-hosted GreptimeDB instance. Nothing is sent to external servers.
- **MCP communication is local stdio.** The MCP server runs as a child process of your AI tool, communicating via stdin/stdout JSON-RPC. No network sockets are opened.
- **No telemetry.** devtap collects no usage data, analytics, or crash reports.
- **`--filter-sql` is not a security boundary.** It includes a best-effort keyword blocklist but is designed for convenience, not adversarial input. The user already has full local and database access.
- **Cleanup:** Run `devtap gc` to remove expired session data, or delete `~/.devtap/` entirely to remove all stored data.

## License

MIT
