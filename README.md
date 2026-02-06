# devtap

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

# Claude Code — writes .mcp.json, injects instructions into CLAUDE.md
devtap install --adapter claude-code

# Codex CLI — writes .codex/config.toml, injects instructions into AGENTS.md
devtap install --adapter codex

# OpenCode — writes opencode.json, injects instructions into AGENTS.md
devtap install --adapter opencode

# aider (no MCP) — creates lint wrapper script, injects instructions into CONVENTIONS.md
devtap install --adapter aider
```

### Usage

**Terminal A** — capture build output:

```bash
devtap -- cargo check
devtap -- go build ./...
devtap --filter-regex "error|warning" -- npm run build

# Long-running with debounce
devtap --debounce 2s -- npm run dev
```

**Terminal B** — use your AI coding tool as usual. It will automatically call `get_build_errors` via MCP to fetch captured build errors.

## Instruction Injection

`devtap install` automatically appends a devtap instruction block to your project's instruction file (e.g., `CLAUDE.md`, `AGENTS.md`, `CONVENTIONS.md`). This ensures the AI tool proactively checks for build errors on every turn.

- If the instruction file exists and has no devtap block — the block is appended.
- If the instruction file already has a devtap block — you are prompted to confirm before overwriting.
- If no instruction file is found — the highest priority file is created automatically (e.g., `CLAUDE.md`).

The block is wrapped in `<!-- devtap:start -->` / `<!-- devtap:end -->` HTML comment markers for idempotent detection.

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
                                 │                            │
Terminal C (Codex)               │                            │
┌──────────────────┐             │                            │
│  MCP tool call:  │   stdio     │                            │
│  get_build_errors├─────────────┤                            │
│                  │  JSON-RPC   │                            │
└──────────────────┘             └────────────────────────────┘
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
| [aider](https://aider.chat) | `aider` | `--lint-cmd` wrapper | `.devtap-aider-lint.sh` | `CONVENTIONS.md` |

Any MCP-compatible tool can use `devtap mcp-serve` directly:

```bash
# Generic MCP server (stdio, JSON-RPC 2.0)
devtap mcp-serve
```

## MCP Tools

The MCP server exposes two tools:

- **`get_build_errors`** — Drain pending build errors and output. Call this before writing code to check for failures.
- **`get_build_status`** — Get a summary of pending message counts across all sessions.

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

For persistent history, SQL-based filtering, and richer statistics. See [GreptimeDB installation guide](https://docs.greptime.com/getting-started/installation/greptimedb-standalone/) for more options.

Quick start with Docker:

```bash
docker run -p 127.0.0.1:4000-4002:4000-4002 \
  -v ~/.devtap/greptimedb_data:/greptimedb_data \
  --name greptime --rm \
  greptime/greptimedb:latest standalone start \
  --http-addr 0.0.0.0:4000 \
  --rpc-bind-addr 0.0.0.0:4001 \
  --mysql-addr 0.0.0.0:4002
```

The `-v` flag mounts `~/.devtap/greptimedb_data/` into the container for persistent storage. Without it, data is lost when the container stops.

```bash
# Configure in ~/.devtap/config.toml
cat > ~/.devtap/config.toml <<EOF
[store]
backend = "greptimedb"

[store.greptimedb]
endpoint = "127.0.0.1:4001"
mysql_endpoint = "127.0.0.1:4002"
database = "devtap"
EOF

# Or override per-command
devtap --store greptimedb -- cargo check

# SQL-based filtering
devtap drain --store greptimedb --filter-sql "content LIKE '%error%'"

# Query build error history
devtap history --since 24h
```

Credentials via environment variables:
```bash
export DEVTAP_GREPTIMEDB_USERNAME=...
export DEVTAP_GREPTIMEDB_PASSWORD=...
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
      --max-lines <n>        Max lines per drain (default 100)
      --tag <label>          Log tag prefix (default: command name)
      --debounce <dur>       Aggregation interval for long-running mode

Subcommands:
  install     Configure AI tool integration
  mcp-serve   Start MCP stdio server
  drain       Read pending messages as plain text
  status      Show pending message counts
  history     Query build error history (GreptimeDB only)
                --since <dur>    Time range (default "24h")
                --tag <label>    Filter by tag
                --limit <n>      Max entries (default 20)
  gc          Remove expired session data
                --ttl <dur>      Time-to-live (default "7d")
```

## Filtering

```bash
# Only capture errors and warnings
devtap --filter-regex "error|warning" -- cargo check

# Exclude noisy lines
devtap --filter-regex "Downloading|Compiling" --filter-invert -- cargo build
```

Lines exceeding `--max-lines` are smart-truncated: head and tail preserved with omission notice. Consecutive duplicate lines are merged.

## Multiple Instances

Use `--tag` to run multiple `devtap` instances for the same session:

```bash
# Terminal B: build watcher
devtap --tag cargo-check --debounce 2s -- cargo watch -x check

# Terminal C: test watcher
devtap --tag cargo-test --debounce 5s -- cargo watch -x test
```

Both write to the same session; the MCP server collects all tagged output.

## Multi-Adapter Fan-out

When multiple AI tools are active for the same project, `devtap` automatically fans out build output to all registered adapters. Each tool independently consumes its own copy:

```bash
# Setup both adapters
devtap install --adapter claude-code
devtap install --adapter codex

# Build errors are delivered to both tools
devtap -- cargo check
```

An adapter is registered when its MCP server starts (via `mcp-serve` or `drain`). Writers discover all registered adapters by scanning `~/.devtap/<session>/` for subdirectories.

## Garbage Collection

Clean up expired session data:

```bash
# Remove data older than 7 days (default)
devtap gc

# Custom TTL
devtap gc --ttl 24h
```

## License

Apache-2.0
