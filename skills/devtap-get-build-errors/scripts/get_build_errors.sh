#!/usr/bin/env sh
set -eu

MAX_LINES="${1:-10000}"

if ! command -v devtap >/dev/null 2>&1; then
  echo "devtap not found in PATH" >&2
  exit 127
fi

devtap status --quiet
devtap drain --max-lines "$MAX_LINES"
