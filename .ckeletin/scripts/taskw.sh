#!/usr/bin/env bash
set -euo pipefail

if command -v task >/dev/null 2>&1; then
  exec task "$@"
fi

if command -v go-task >/dev/null 2>&1; then
  exec go-task "$@"
fi

echo "Error: neither 'task' nor 'go-task' found in PATH" >&2
echo "Install Task first (e.g. 'go-task setup' or your OS package manager)." >&2
exit 127

