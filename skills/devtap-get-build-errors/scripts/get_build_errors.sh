#!/usr/bin/env sh
set -eu

MAX_LINES="${1:-10000}"

if ! command -v devtap >/dev/null 2>&1; then
  echo "devtap not found in PATH" >&2
  exit 127
fi

pending_count=$(devtap status --quiet 2>/dev/null || echo 0)
if [ "$pending_count" -gt 0 ] 2>/dev/null; then
  devtap drain --max-lines "$MAX_LINES"
fi
