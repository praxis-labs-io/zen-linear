# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

Drew's fork of [roeyazroel/linear-tui](https://github.com/roeyazroel/linear-tui) (`upstream`), a Go/tview terminal client for Linear. `origin` is Drucial/zen-linear. **`main` is the product branch** — feature work and integration fixes commit straight to it, no fork-internal PRs. The installed binary is built from here to `~/.local/bin/linear-tui`, which shadows the stock brew install; **rebuild after changes or Drew keeps running the old code**:

```sh
go build -o ~/.local/bin/linear-tui ./cmd/linear-tui
```

### Upstream PR discipline

`feature/*` branches are cut from `upstream/main` and map to individual upstream PRs (#15 favorites, #16 workspace switching, #17 keybindings, #18 pane toggles pending as of 2026-07-31). Each must stay standalone-green (`make all` on the branch) — never fix a cross-feature interaction on a feature branch; integration fixes live only on `main`. #16 and #17 overlap; whichever lands second gets rebased (promised in the PR bodies). Held back as too opinionated for upstream: the Rose Pine Moon theme work, the Linear-style list layout, and pane tabs.

Anything published under Drew's name (PR bodies, issues, README) must be shown to him word-for-word before pushing. His voice: terse, considerate, stoic, no strong adverbs, no em-dashes.

## Commands

```sh
make all              # lint (gofmt + mod-tidy + golangci-lint) + test + build
make test             # go test -v -race -coverprofile ./...
make lint             # includes gofmt check and go.mod tidiness
make fmt-fix          # gofmt -w .
go test ./internal/tui/ -run TestName   # single test
```

Run checks directly, never through a pipe that swallows exit codes (`make lint | tail` reports success on failure — this let broken commits through repeatedly).

### Lint version pin

CI pins golangci-lint **v2.8.0** (`.github/workflows/ci.yml`). Newer local versions report false positives (goconst, staticcheck) on code CI accepts, and v2.8.0 panics under new Go toolchains. Replicate CI with a pinned binary plus `GOTOOLCHAIN=go1.24.4`, or scope a newer binary with `--new-from-rev=upstream/main`.

## Architecture

`cmd/linear-tui` is the entrypoint; everything lives in `internal/`: `linearapi` (GraphQL client via shurcooL/graphql), `tui` (tview app — the bulk), `config`, `auth`, `cache`, `agents`, `logger`.

### Config plumbing (the most common trap)

`internal/config` has a triple: `SettingsFile` (pointer fields, what's on disk) → `Settings` → `Config`. The settings modal saves via `settingsFromForm`, which rebuilds the file from form controls — **any config field without a form control (workspaces, default_workspace, group_by, subgroup_by, columns, keybindings) must be explicitly carried through there, or an in-app settings save silently strips it from the user's config**. Every new field needs: the triple, a validator in `settings.go`, and the carry-through.

Drew's live config is a symlink chain: `~/.linear-tui/config.json` → dotfiles repo (`drucial-dots/configs/linear-tui/config.json`). The app writes settings in place, so in-app saves show up as diffs in the dotfiles repo — intentional.

### Keybindings

Commands carry a single-rune shortcut; the `keybindings` config map (command id → rune) remaps them with steal semantics: an explicit mapping clears the colliding default (`applyCommandKeybindings` in `internal/tui/keybindings.go`). UI actions that aren't palette commands (tab_next/tab_prev, columns_left/right, quit, open_palette, search) resolve through `actionKey(action, fallback)`. **Never compare against a hardcoded rune in a key handler** — that's how the details pane's tab keys broke when tabs were rebound.

### Theme system

Themes are structs in `internal/tui/theme.go` registered in `ThemeRegistry`. Optional fields (`InverseText`, `StatusReview`) have fallback methods so legacy themes need no changes. `RosePineMoonTheme` sets `Background: tcell.ColorDefault` (terminal transparency), which breaks tview defaults that assume an opaque `PrimitiveBackgroundColor` — selection styles, InputField inner fill, modal backgrounds all have explicit workarounds (`selectionStyle`, `newThemedInputField`, `ModalBackground`). When adding UI, style from the theme, not tview defaults.

### Issues list

`BuildGroupedIssueRows` (`internal/tui/issue_tree.go`) produces a flat `[]IssueRow` where group/subgroup headers are rows with `IsHeader: true` and no issue. Headers are selectable (Enter/Space/click toggles collapse), so **any code walking table rows or moving selection must skip or special-case headers** (`nextIssueRow`, and the default-selection logic in `rebuildIssuesTables`). Columns are a registry in `issues_table.go` (`issueColumnSpecs`), rendered per the `columns` config.

### Workspaces and auth

`workspaces` in config reference env vars (`api_key_env`) — keys are never stored in the file. A bare `LINEAR_API_KEY` env var overrides all auth unconditionally (`internal/auth/resolve.go`); never export one. Known upstream bug, unfixed: `applySettings` rebuilds the API client without `UseBearer`/`OnUnauthorized`, breaking OAuth sessions after an in-app settings save (doesn't affect API-key workspaces).

### GraphQL client

Queries in `internal/linearapi/client.go` are struct-tagged shurcooL/graphql selections. A field the schema doesn't allow in that position makes Linear reject the entire query — one bad field in the `Attachments` node once silently broke attachments, comments, and GitHub links together. Verify field placement against the Linear schema when extending a selection, and live-test fetches.
