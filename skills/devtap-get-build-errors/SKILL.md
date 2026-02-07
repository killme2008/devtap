---
name: devtap-get-build-errors
description: Fetch pending devtap build/dev output for the current turn using get_build_status and get_build_errors. Use when the user asks to check build errors, latest build logs, or "/get_build_errors"-style actions.
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
3. If build succeeded, acknowledge briefly (do not repeat the output).
4. If build failed, present the error output verbatim in a fenced code block.
5. Keep source warnings verbatim (for example, unreachable source warnings).
6. After the output, add one line: `Next action: <what you will do>`.

## Rules

- Do not fabricate or reinterpret error content.
- Do not call `get_build_errors` repeatedly in the same turn unless new output is reported.
- MCP tool names map to CLI subcommands: `get_build_status` → `devtap status`, `get_build_errors` → `devtap drain`.
- If MCP tools are unavailable, use `scripts/get_build_errors.sh` as CLI fallback.
