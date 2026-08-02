#!/bin/bash
# SessionStart hook: download modules so `make all` works immediately in fresh
# Claude Code on the web containers.
set -euo pipefail

# Only run in remote (web) environments — local sessions manage their own deps.
if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

cd "${CLAUDE_PROJECT_DIR:-.}"
go mod download
