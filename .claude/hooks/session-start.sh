#!/bin/bash
# SessionStart hook: point git at the tracked hooks, and download modules so
# `make all` works immediately in fresh Claude Code on the web containers.
set -euo pipefail

cd "${CLAUDE_PROJECT_DIR:-.}"

# Untracked .git/hooks files don't survive a clone, so the hooks are tracked in
# .githooks and this is what points git at them. Every session, so a fresh
# clone is covered.
git config core.hooksPath .githooks

# Module download is only worth it in remote (web) containers; local sessions
# manage their own deps.
if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

go mod download
