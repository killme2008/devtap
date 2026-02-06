# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
# Build
go build ./cmd/devtap

# Test (unit only)
go test ./...

# Test with race detection (as CI runs)
go test ./... -race -count=1

# Run a single test
go test ./internal/store/file/ -run TestDrain

# Integration tests (requires local GreptimeDB on gRPC :4001, MySQL :4002)
go test -tags=integration ./internal/store/greptimedb/

# Lint
golangci-lint run ./...
```

## Architecture

devtap captures stdout/stderr from build/dev commands and delivers them to AI coding tools via MCP (Model Context Protocol).

### Data Flow

```
devtap -- <cmd>  →  capture (runner/longrun)  →  store.Write()  →  fan-out to all adapters
AI tool          ←  MCP server (get_build_errors) ←  store.Drain()  ←  per-adapter queue
```

### Core Interfaces

**Store** (`internal/store/store.go`): Write/Drain/Status/Close. Two backends:
- **File** (default): JSONL at `~/.devtap/<session>/<adapter>/pending.jsonl`, atomic rename for IPC
- **GreptimeDB** (optional): SQL queries + watermark cursor, `tag` is a reserved keyword and must be backtick-quoted in all SQL

**Adapter** (`internal/adapter/adapter.go`): Name/DiscoverSessions/Install. Five implementations:
- **claudecode**: `.mcp.json` + optional Stop hook in `~/.claude/settings.json`
- **codex**: `.codex/config.toml`
- **opencode**: `opencode.json`
- **gemini**: `.gemini/settings.json`
- **aider**: lint wrapper script (no MCP)

### Key Patterns

- **Multi-adapter fan-out**: Writers discover adapters via `store.DiscoverAdapters()`, write to all. Each tool drains independently.
- **File store IPC**: `pending.jsonl` → atomic rename to `pending.jsonl.draining` → read → delete. Leftover lines written back to prevent data loss.
- **Config merge**: `.mcp.json` / `settings.json` / `opencode.json` reads existing → upserts devtap entry → writes back. Never overwrites other tools' config.
- **Instruction injection**: Appends `<!-- devtap:start -->` / `<!-- devtap:end -->` block to project instruction files. Idempotent via marker detection.
- **Session encoding**: `session.EncodeDir("/foo/bar")` → `"-foo-bar"`, shared across adapters.
- **Capture modes**: `runner.go` (batch, flush every 50 lines) vs `longrun.go` (debounce timer, for dev servers).
- **Scanner buffers**: 64KB initial / 1MB max (`internal/capture/errors.go`). On scanner error (line >1MB), pipe is drained to discard to prevent child process deadlock.
- **Line-level truncation**: `mcp.TruncateMessages()` allocates line budget proportionally across messages. Applied in both MCP server and drain command.

### GreptimeDB Specifics

- Composite `PRIMARY KEY (session_id, \`tag\`, stream, adapter)` clause (not inline per-column)
- `TIMESTAMP(6)` microsecond precision to avoid PK collisions
- `append_mode=true` allows duplicate PKs
- SQL injection protection via `validateFilterSQL` in drain (best-effort blocklist, not a security boundary)
- Integration tests gated behind `//go:build integration`

<!-- devtap:start -->
## devtap

Get pending build errors and output captured by devtap. Call this before writing or editing code to check for build failures that need fixing.
<!-- devtap:end -->
