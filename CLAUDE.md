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

The bucket names are shared with other teams, so `save_issue` resolving a bare project name can land on another team's copy and fail the call. Pass the Zen Linear project id in that one argument when it does.

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

`cmd/zen-linear` is the entrypoint; everything lives in `internal/`: `linearapi` (GraphQL client via shurcooL/graphql), `tui` (tview app — the bulk), `config`, `session`, `auth`, `cache`, `agents`, `logger`.

### App wiring

`internal/tui/app.go` is the `App` struct, the `FocusTarget` enum, and the lifecycle around them: `NewApp`, `Run`, `loadInitialData`, `loadCurrentUser`, `applySettings`, `resetCachedState`, `buildLayout`, plus `selectedIssueID` and `parseLogLevel`. The rest splits by area: `theme_apply.go` (theme and density restyling, `rebuildModals`), `navigation_data.go` (tree fetch and build), `nav_search.go` (the pane shell, the query box, and its keys), `issue_search.go` (the search fetch and its model), `key_dispatch.go` (`bindGlobalKeys` and the per-pane handlers), `pane_focus.go` (Tab cycling, focus, pane titles), `issues_refresh.go` (search debounce, fetch, pagination merge, render), `issue_grouping.go` (columns, grouping, sort fields), `issue_filters.go` (`IssueFilters` and its summary formatters), `pickers.go` (`loadPickerData` and the `Show*Picker` set), `modal_launchers.go` (the `Show*Modal` set), `app_accessors.go` (the getters commands call), `nav_cache.go` (the disk copy of the tree), `loading_pane.go` (the spinner frame loop and the waiting panes' messages), `details_comments.go` (the Comments tab's cards, their relative timestamps, and the thread rail), `comment_thread.go` (comments to threaded blocks), `comments_page.go` (the page primitive the tab is drawn as), `comment_actions.go` (the focus ring and the per-comment keys), `details_compose.go` (the two writing boxes, the ring's focus targets, and the post path). The status bar updates sit in `status_bar.go` beside `buildStatusBar`, and `SortField` in `issue_sort.go` beside its comparator. `app_test.go` did not follow the split.

### Launch

`loadInitialData` paints the navigation tree from `internal/cache`'s `nav-cache.json` before the teams and favorites fetch answers, so the issue list starts loading on the saved place a round trip early. When the fetch matches the cached copy, **nothing rebuilds** — that is what keeps a rebuild from moving the cursor out from under a user who has already started navigating. When it differs, the tree rebuilds and the live place is re-resolved through `applySessionNavigation`, the same path session restore uses. A `hasCache` launch that fails keeps the cached tree rather than emptying the sidebar.

Panes that are waiting say so: `loading_pane.go` owns one ticker for the spinner and stops it when nothing is in flight. tview has no frame loop, so anything animated needs that ticker plus `QueueUpdateDraw`, and the loop has to stop or it queues draws forever.

**Never call `Application.GetFocus` from anything a draw can reach.** `Application.draw` holds the app's lock for the whole frame, so a draw func, a `SetBeforeDrawFunc`, or a `SetFocusFunc` that reads live focus blocks on a mutex its own goroutine already holds. Nothing draws after that and no key is read, Ctrl+C included: the process has to be killed. A draw func on the compose box did this once and froze the app on any draw of the Comments tab. Read state instead (`commentsFocus`, `focusedPane`); confine `GetFocus` to the key path, where the lock is free. `TestDrawingTheCommentsPanelKeepsTheAppAlive` hangs if it comes back.

### Config plumbing (the most common trap)

`internal/config` has a triple: `SettingsFile` (pointer fields, what's on disk) → `Settings` → `Config`. The settings modal saves via `settingsFromForm`, which rebuilds the file from form controls — **any config field without a form control (workspaces, default_workspace, group_by, subgroup_by, columns, keybindings) must be explicitly carried through there, or an in-app settings save silently strips it from the user's config**. Every new field needs: the triple, a validator in `settings.go`, and the carry-through.

Config, credentials, prompts, the session, and the log live under `~/.zen-linear`, resolved through `config.Dir()`.

`internal/session` owns `session.json` (last workspace, and per workspace the nav selection, focused issue, tab, filters, and search). It sits outside `internal/config` because config models what the user writes and this is what the app writes: `config.json` is a symlink into Drew's dotfiles repo, and a per-quit write there would dirty that repo on every launch. `App.Run` flushes it after the event loop stops, `switchWorkspace` flushes the outgoing workspace before `resetCachedState` wipes it, and the restore is a locator resolved against the live tree, never a serialized node. `config.MigrateLegacyDir()` renames a leftover `~/.linear-tui` on startup; it uses `os.Rename` so symlinked files keep resolving to their targets.

`internal/cache` owns `nav-cache.json` beside it: the teams and favorites the tree is built from, keyed by workspace, versioned and discarded rather than migrated. **A session with no configured workspace name gets no entry** (a bare `LINEAR_API_KEY` or OAuth): nothing on disk tells two Linear workspaces reached that way apart, one shared entry would paint the wrong teams, and fingerprinting the token to tell them apart puts a derivative of a live credential in a cache file. It is cached API data, not user state, which is why it sits outside `internal/session`. Both write through `config.WriteFileAtomic` (temp file plus rename, mode 0600).

Drew's live config is a symlink chain: `~/.zen-linear/config.json` → dotfiles repo (`drucial-dots/configs/linear-tui/config.json`). The app writes settings in place, so in-app saves show up as diffs in the dotfiles repo — intentional.

### Keybindings

`config.Keybindings` (id → key) is validated **once**, by `resolveKeybindings` in `internal/tui/keybindings.go`, into the `resolvedKeybindings` on `App.bindings`. A binding that names a movement rune (`h`, `l`, `j`, `k`, `g`, `G`), is not a single rune, or names neither a command nor a UI action is dropped there and logged. **Everything downstream reads the resolved set, never `config.Keybindings`.** Half-honoring a rejected binding is the bug this shape exists to prevent: it used to keep a command's default alive at one call site while taking a rune at another, so one dead key explained a second.

Both a command and a UI action can hold a rune, and both give a default up to an id the user named explicitly (`takenFrom`). The check reads scope, so a navigation command and an issue command may share one. `actionKey(action, fallback)` returns **0** when something else took the fallback, which no key event carries, so that action's `case` stops matching and the claimant answers. Anything printing an action key has to drop a 0 rather than render it.

Action ids and their scopes live in `uiActionScopes`; `TestUIActionScopesCoverEveryActionKeyCallSite` greps the package for `actionKey(` calls so the map cannot fall behind. **Never compare against a hardcoded rune in a key handler** — that's how the details pane's tab keys broke when tabs were rebound.

Deleting a command is a breaking change for anyone who bound it: the id stops resolving, and the user gets a logged warning instead of a silent theft. Say so in the PR.

Tab is not pane navigation. It walks a pane's own controls, the Comments page's ring and the navigation pane's query box, and nothing else; `h`/`l` and the pane numbers move between panes.

### Modal dispatch

`modalBindings` in `internal/tui/modal_dispatch.go` is an ordered list of page name → modal. Order is dispatch priority, not stack order: `handleGlobalKey` gives the key to the first open page in that list, whichever modal opened last. A new modal needs one entry plus `HandleKey` and `Focus`; the dispatcher never grows a branch. Every `Hide` ends in `restoreModalFocus`, which raises the next modal still open and falls back to the panes.

Adding a page moves focus. `Pages.AddPage` re-delegates focus to the top visible page whenever the Pages tree holds it, and that walks down to the pane `buildLayout` flagged, not the pane the user is in. Anything that adds or replaces a page mid-session has to end in `restoreModalFocus`; `applySettings` defers it so the restore runs after `resetCachedState` moves the active tab out from under it. `modal_dispatch_test.go` pins the order against its own literal copy, so reordering the registry fails there rather than agreeing with itself.

### Theme system

Themes are structs in `internal/tui/theme.go` registered in `ThemeRegistry`. Optional fields (`InverseText`, `StatusReview`, `StatusTriage`, `AssigneeText`) have fallback methods so legacy themes need no changes. `RosePineMoonTheme` sets `Background: tcell.ColorDefault` (terminal transparency), which breaks tview defaults that assume an opaque `PrimitiveBackgroundColor` — selection styles, InputField inner fill, modal backgrounds all have explicit workarounds (`selectionStyle`, `newThemedInputField`, `ModalBackground`). When adding UI, style from the theme, not tview defaults.

Modal panels: `tview.NewFlex` (and Grid) set `dontClear`, so a Flex never paints its own background and the layer beneath bleeds through unpainted cells. Any modal panel needs `panel.Box = tview.NewBox()` before its other Box settings to restore the fill — `FormModal` does this for its shells; hand-built overlays must too.

### Navigation pane

The pane is `navigationPanel` (`nav_search.go`), a Flex owning the border, the pane title, and one column of border padding: the query box, a hairline rule, and a borderless `navigationTree` beneath. `navSearchFocused` says which of the two controls holds the keyboard, and `navSearchActive()` gates key routing above the global runes in `handleGlobalKey` — without that gate, `q`, `:` and the pane numbers fire while you type. The box **must be rebuilt, not restyled**, on a theme change (tview bakes `InputBg` at construction), and the rebuild has to end in `rebuildContentLayout` or `contentFlex` keeps the old pointer.

The search is workspace-wide and takes neither the tree's scope, the rich filters, nor the sort chain. That is why the issues context line skips it.

### Issues list

`BuildGroupedIssueRows` (`internal/tui/issue_tree.go`) produces a flat `[]IssueRow` where group/subgroup headers are rows with `IsHeader: true` and no issue. Headers are selectable (Enter/Space/click toggles collapse), so **any code walking table rows or moving selection must skip or special-case headers** (`nextIssueRow`, and the default-selection logic in `rebuildIssuesTables`). Columns are a registry in `issues_table.go` (`issueColumnSpecs`), rendered per the `columns` config.

`updateIssuesColumnLayout` swaps the mounted primitive between the active section's table and the placeholder. Focus lives on the primitive, so a swap has to carry it (`issuesPaneHasFocus`) or the pane goes dead with the keys on something off screen.

The pane has no tabs. `IssuesSection` is two values, `List` and `Search`, with a table and a row model each; `performIssueSearch` owns which one is active, so an empty query is what puts the list back. The pane title names what is on screen (`issuesTitleLabel`) and `issues_context.go` right-aligns the sort and filter line into the same top border row, measuring against that title so the two cannot collide.

### Comments tab

The tab is one scrolling page, not a layout: `commentsPage` (`comments_page.go`) is text with holes in it. `renderDetailsComments` writes every card's frame into one text view — comments, an open reply box, and the compose card that ends the page — and records a `pageSlot` for each live widget. The page's `Draw` paints the text, then draws the two `TextArea`s and their buttons over the rows their frames left empty. **A widget in that page is positioned by the render, not mounted in a Flex**, which is what keeps one scroll, one measure, and one set of borders across the lot; anything added there needs a slot and nothing else.

`commentBlocks` (`comment_thread.go`) is the page's order: threads from `buildCommentRows`, the reply box at the end of the thread it answers, the compose card last. The ring Tab walks is the `commentSpans` the same render recorded, so **what Tab does follows what was drawn** — a stop that is not on the page cannot be focused. Linear rejects a `parentId` that is not top level, so a reply always posts against `threadRootID`.

Bottom padding is written as blank lines at the end of the text rather than held back from the content rect (`trailingPad`), so mid-scroll the content runs to the border and the gap is the end of the scroll.

### Workspaces and auth

`workspaces` in config reference env vars (`api_key_env`) — keys are never stored in the file. A bare `LINEAR_API_KEY` env var overrides all auth unconditionally (`internal/auth/resolve.go`); never export one. Known upstream bug, unfixed: `applySettings` rebuilds the API client without `UseBearer`/`OnUnauthorized`, breaking OAuth sessions after an in-app settings save (doesn't affect API-key workspaces).

### GraphQL client

`internal/linearapi` splits by domain: `client.go` (construction, auth transport), `retry.go` (429/5xx backoff, wrapping the auth transport), `types.go` (domain structs plus every input type's `GetGraphQLType` and `MarshalJSON`), `teams.go`, `favorites.go`, `metadata.go` (projects, milestones, cycles, users, workflow states, labels), `issue_filters.go`, `issues_query.go`, `issue_parse.go` (`toIssue`/`toRef` conversions off the query node types), `issue_detail.go`, `issue_mutations.go`, `comments.go`. Tests did not follow the split: `client_test.go` covers most of the package, alongside `favorites_test.go`, `retry_test.go`, and `query_golden_test.go`.

Queries are struct-tagged shurcooL/graphql selections. A field the schema doesn't allow in that position makes Linear reject the entire query — one bad field in the `Attachments` node once silently broke attachments, comments, and GitHub links together. Verify field placement against the Linear schema when extending a selection, and live-test fetches.

One issue selection serves every call site. `issueQueryNode` (`issues_query.go`) is the list, search, custom-view, and mutation shape; `issueDetailNode` (`issue_detail.go`) **embeds it untagged and first**, which is what makes shurcooL inline its fields flat and in order, then adds the five connections only the details pane needs. Tag that embedded field or move it and the whole selection changes shape.

`testdata/*.graphql` holds the exact outgoing query for each of the package's `c.client.query`/`mutate` sites, and `query_golden_test.go` fails if any of them drifts. **A refactor of the query structs is only safe when that diff stays empty.** A new call site needs a case in `goldenQueryCases()`; `TestQueryGoldensCoverEveryCallSite` counts the sites in the sources and fails when the table falls behind. Regenerate with:

```sh
ZEN_UPDATE_GOLDENS=1 go test ./internal/linearapi -run TestQueryGoldens
```

### Agent providers

Each provider in `internal/agents` builds argv for a CLI that isn't in this repo, so nothing but a real run proves a flag exists. `--sandbox` and `--workspace` shipped for months against `cursor-agent`, which has neither, and every run died on the first one. Provider tests pin the whole argv against a literal for that reason; check a new flag against the installed CLI before adding it, and note that `--help` exits 0 on an unknown flag, so it settles nothing. `AgentRunOptions.Sandbox` is a portable intent, not a flag: claude maps it to `--permission-mode`, cursor to `--force`. The workspace rides on `cmd.Dir` in `runner.go`, never a flag.
