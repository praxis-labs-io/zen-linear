# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

Drew's Go/tview terminal client for Linear, at zen-linear/zen-linear (`origin`). It began as a fork of [roeyazroel/linear-tui](https://github.com/roeyazroel/linear-tui) and separated on 2026-08-02: the module is `github.com/zen-linear/zen-linear`, there is no `upstream` remote, and nothing here is written with an upstream PR in mind. The MIT license retains Roey Azroel's copyright alongside Drew's, and the README credits the original. Both stay.

**`main` is the product branch.** Feature work flows ticket → branch → PR on `origin` (see Project Management); genuinely trivial tweaks (a typo, a one-liner) still commit straight to `main`. A pre-push hook rejects pushes to `main`, so agent work always goes through a branch. The installed binary is built from here to `~/.local/bin/zen-linear`; **rebuild after changes or Drew keeps running the old code**:

```sh
go build -o ~/.local/bin/zen-linear ./cmd/zen-linear
```

The repo moved to the `zen-linear` org under the Praxis Labs enterprise on 2026-08-02, so the module path is `github.com/zen-linear/zen-linear` and the Homebrew tap lives at `zen-linear/homebrew-tap`. Releases install as `brew install zen-linear/tap/zen-linear`. **Keep the tap prefix**: Homebrew 6 refuses to load formulae from untrusted taps, and naming the tap on the command line is its consent signal, so a bare `brew install zen-linear` fails. The brew step needs a `HOMEBREW_TAP_TOKEN` secret with write access to the tap repo.

Anything published under Drew's name (PR bodies, issues, README) must be shown to him word-for-word before pushing. His voice: terse, considerate, stoic, no strong adverbs, no em-dashes.

## Conventions

@.claude/rules/code-quality.md

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

CI pins golangci-lint **v2.12.2** (`.github/workflows/ci.yml`), matching what brew currently ships. `make lint` needs no `GOTOOLCHAIN` override and reports what CI reports.

Keep the pin current with the local version. The old v2.8.0 pin drifted four versions behind, which meant local runs and CI disagreed and v2.8.0 panicked under newer Go toolchains.

## Project Management

Work is tracked in Linear: Praxis Labs workspace, **Zen Linear** team (key `ZNL`, tickets `ZNL-###`), reached through the `linear-zen-linear` MCP server declared in `.mcp.json`. Address projects and statuses **by name, never a UUID**; ids don't survive workspace moves.

### Projects

Five long-running buckets. They never complete; every ticket belongs to exactly one. Each bucket's Linear description holds a `File here when:` test and a routing list, and those descriptions are the tiebreaker when a ticket could fit two:

- **Polish & Bugs**: bugs and rough edges in surfaces that already ship. The dogfood inbox.
- **Feature Backlog**: net-new capabilities. Ideas live here until promoted.
- **Performance and Code-Quality**: improves the code, no user-visible change (perf, refactors, tests, hygiene).
- **Website**: the public site, its copy, its SEO.
- **Release & Distribution**: how the binary gets from `main` to a user and stays current (GoReleaser, Homebrew tap, versioning, CI release).

A body of work big enough to need milestones gets its own finite epic project, named for what it delivers, completed and closed when it ships. An epic is a Linear Project, never a tracking issue. When an epic closes, follow-ups move to the matching bucket.

### Tickets

- Every ticket gets the team, exactly one project, a priority, and a status. No orphans.
- Create tickets as we go; never dump a full backlog up front.
- PR-sized scoping: 1 ticket = 1 branch = 1 PR as the rule of thumb. Don't split tightly-related work that ships together; don't group independent slices.
- Keep descriptions lean: clear title, short goal and scope. No boilerplate acceptance criteria.
- Use Linear's generated branch name (`gitBranchName` from the MCP), never an invented one.
- Reference the ticket id in commits and the PR title/body so Linear auto-links.
- Status ladder: agent drives Backlog → Todo → In Progress. The GitHub integration owns In Review and Done; never write those by hand.

### Shipping

Feature-complete work ships via the `ship-feature` skill (`.claude/skills/ship-feature/SKILL.md`): `make all` green, push, draft PR, Copilot + `/code-review`, triage with no tech debt, push then mark ready as separate actions. Manual invocation only.

### Specs and plans

Scratch, never committed. `docs/` describes only what is true today. Durable context lives in Linear project descriptions and tickets.

## Architecture

`cmd/zen-linear` is the entrypoint; everything lives in `internal/`: `linearapi` (GraphQL client via shurcooL/graphql), `tui` (tview app — the bulk), `config`, `auth`, `cache`, `agents`, `logger`.

### Config plumbing (the most common trap)

`internal/config` has a triple: `SettingsFile` (pointer fields, what's on disk) → `Settings` → `Config`. The settings modal saves via `settingsFromForm`, which rebuilds the file from form controls — **any config field without a form control (workspaces, default_workspace, group_by, subgroup_by, columns, keybindings) must be explicitly carried through there, or an in-app settings save silently strips it from the user's config**. Every new field needs: the triple, a validator in `settings.go`, and the carry-through.

Config, credentials, prompts, and the log live under `~/.zen-linear`, resolved through `config.Dir()`. `config.MigrateLegacyDir()` renames a leftover `~/.linear-tui` on startup; it uses `os.Rename` so symlinked files keep resolving to their targets.

Drew's live config is a symlink chain: `~/.zen-linear/config.json` → dotfiles repo (`drucial-dots/configs/linear-tui/config.json`). The app writes settings in place, so in-app saves show up as diffs in the dotfiles repo — intentional.

### Keybindings

Commands carry a single-rune shortcut; the `keybindings` config map (command id → rune) remaps them with steal semantics: an explicit mapping clears the colliding default (`applyCommandKeybindings` in `internal/tui/keybindings.go`). UI actions that aren't palette commands (tab_next/tab_prev, columns_left/right, quit, open_palette, search) resolve through `actionKey(action, fallback)`. **Never compare against a hardcoded rune in a key handler** — that's how the details pane's tab keys broke when tabs were rebound.

### Theme system

Themes are structs in `internal/tui/theme.go` registered in `ThemeRegistry`. Optional fields (`InverseText`, `StatusReview`) have fallback methods so legacy themes need no changes. `RosePineMoonTheme` sets `Background: tcell.ColorDefault` (terminal transparency), which breaks tview defaults that assume an opaque `PrimitiveBackgroundColor` — selection styles, InputField inner fill, modal backgrounds all have explicit workarounds (`selectionStyle`, `newThemedInputField`, `ModalBackground`). When adding UI, style from the theme, not tview defaults.

Modal panels: `tview.NewFlex` (and Grid) set `dontClear`, so a Flex never paints its own background and the layer beneath bleeds through unpainted cells. Any modal panel needs `panel.Box = tview.NewBox()` before its other Box settings to restore the fill — `FormModal` does this for its shells; hand-built overlays must too.

### Issues list

`BuildGroupedIssueRows` (`internal/tui/issue_tree.go`) produces a flat `[]IssueRow` where group/subgroup headers are rows with `IsHeader: true` and no issue. Headers are selectable (Enter/Space/click toggles collapse), so **any code walking table rows or moving selection must skip or special-case headers** (`nextIssueRow`, and the default-selection logic in `rebuildIssuesTables`). Columns are a registry in `issues_table.go` (`issueColumnSpecs`), rendered per the `columns` config.

### Workspaces and auth

`workspaces` in config reference env vars (`api_key_env`) — keys are never stored in the file. A bare `LINEAR_API_KEY` env var overrides all auth unconditionally (`internal/auth/resolve.go`); never export one. Known upstream bug, unfixed: `applySettings` rebuilds the API client without `UseBearer`/`OnUnauthorized`, breaking OAuth sessions after an in-app settings save (doesn't affect API-key workspaces).

### GraphQL client

Queries in `internal/linearapi/client.go` are struct-tagged shurcooL/graphql selections. A field the schema doesn't allow in that position makes Linear reject the entire query — one bad field in the `Attachments` node once silently broke attachments, comments, and GitHub links together. Verify field placement against the Linear schema when extending a selection, and live-test fetches.
