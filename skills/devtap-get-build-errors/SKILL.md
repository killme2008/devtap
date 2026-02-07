---
name: devtap-get-build-errors
description: Fetch devtap output for the current turn. Default to concise summaries and print raw logs only on explicit request.
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
3. If build succeeded, acknowledge briefly (do not repeat output).
4. If build failed, provide a concise summary by default:
   - source label(s)
   - final failure signal (for example exit code or test summary)
   - most actionable failing case/file/error line(s)
5. Show raw output verbatim only when the user explicitly asks for full/raw/original logs.
6. Keep source warnings verbatim (for example, unreachable source warnings).
7. After the output (or summary), add one line: `Next action: <what you will do>`.

## Rules

- Do not fabricate or reinterpret error content.
- Do not call `get_build_errors` repeatedly in the same turn unless new output is reported.
- MCP tool names map to CLI subcommands: `get_build_status` -> `devtap status`, `get_build_errors` -> `devtap drain`.
- If MCP tools are unavailable, use `scripts/get_build_errors.sh` as CLI fallback.
