---
name: devtap-get-build-errors
description: Fetch pending devtap build/dev output for the current turn using get_build_status and get_build_errors, then present the captured output verbatim. Use when the user asks to check build errors, latest build logs, or "/get_build_errors"-style actions.
metadata:
  short-description: Fetch devtap output
---

# devtap-get-build-errors

Use this skill when the user asks for build errors, latest build output, or an equivalent quick action.

## Workflow

1. Call `get_build_status` once.
2. Call `get_build_errors` once when any of these is true:
   - status reports pending output
   - user explicitly asks to fetch/check logs now
   - user says new build/test/dev output arrived
3. Present `get_build_errors` content verbatim in a fenced code block.
4. Keep source warnings verbatim (for example, unreachable source warnings).
5. After the block, add one line: `Next action: <what you will do>`.

## Rules

- Do not summarize or rewrite build output.
- Do not call `get_build_errors` repeatedly in the same turn unless new output is reported.
- MCP tool names map to CLI subcommands: `get_build_status` → `devtap status`, `get_build_errors` → `devtap drain`.
- If MCP tools are unavailable, use `scripts/get_build_errors.sh` as CLI fallback.
