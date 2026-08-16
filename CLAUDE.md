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

`internal/tui/app.go` is the `App` struct, the `FocusTarget` enum, and the lifecycle around them: `NewApp`, `Run`, `loadInitialData`, `loadCurrentUser`, `applySettings`, `resetCachedState`, `buildLayout`, plus `selectedIssueID` and `parseLogLevel`. The rest splits by area: `theme_apply.go` (theme and density restyling, `rebuildModals`), `navigation_data.go` (tree fetch and build), `nav_search.go` (the pane shell, the query box, and its keys), `issue_search.go` (the search fetch and its model), `key_dispatch.go` (`bindGlobalKeys` and the per-pane handlers), `mouse_focus.go` (the click's pane hit test), `pane_focus.go` (Tab cycling, focus, pane titles), `issues_refresh.go` (search debounce, fetch, pagination merge, render), `issue_grouping.go` (columns, grouping, sort fields), `issue_filters.go` (`IssueFilters` and its summary formatters), `pickers.go` (`optionScope`, `issueFieldOptions` and the `Show*Picker` set), `issue_field_save.go` (`saveIssueField` and the per-field constructors), `modal_launchers.go` (the `Show*Modal` set), `app_accessors.go` (the getters commands call), `nav_cache.go` (the disk copy of the tree), `loading_pane.go` (the spinner frame loop and the waiting panes' messages), `details_comments.go` (the page render, the comment cards, their relative timestamps, and the thread rail), `comment_thread.go` (comments to threaded blocks and the activity merged into them), `details_activity.go` (the activity lines and their phrasing), `details_page.go` (the page primitive the pane is drawn as), `comment_actions.go` (the focus ring and the per-comment keys), `details_chooser.go` (the inline option list, its keys and its commit), `details_editor.go` (the inline text box, its keys and its commit), `details_compose.go` (the two writing boxes, the ring's focus targets, and the post path). The status bar updates sit in `status_bar.go` beside `buildStatusBar`, and `SortField` in `issue_sort.go` beside its comparator. `app_test.go` did not follow the split.

### Launch

`loadInitialData` paints the navigation tree from `internal/cache`'s `nav-cache.json` before the teams and favorites fetch answers, so the issue list starts loading on the saved place a round trip early. When the fetch matches the cached copy, **nothing rebuilds** — that is what keeps a rebuild from moving the cursor out from under a user who has already started navigating. When it differs, the tree rebuilds and the live place is re-resolved through `applySessionNavigation`, the same path session restore uses. A `hasCache` launch that fails keeps the cached tree rather than emptying the sidebar.

Panes that are waiting say so: `loading_pane.go` owns one ticker for the spinner and stops it when nothing is in flight. tview has no frame loop, so anything animated needs that ticker plus `QueueUpdateDraw`, and the loop has to stop or it queues draws forever.

**Never call `Application.GetFocus` or `SetFocus` from anything a draw can reach.** `Application.draw` holds the app's lock for the whole frame, so a draw func, a `SetBeforeDrawFunc`, or a `SetFocusFunc` that reads or moves focus blocks on a mutex its own goroutine already holds. Nothing draws after that and no key is read, Ctrl+C included: the process has to be killed. A draw func on the compose box did this once and froze the app on any draw of the details pane; `watchLayoutWidth` did it again from the before-draw hook, on a resize past a breakpoint while zoomed. Read state instead (`detailsFocus`, `focusedPane`); confine `GetFocus` to the key path, where the lock is free. A hook that needs the keyboard moved sets a flag and lets the next event do it (`layoutFocusStale`, `repairLayoutFocus`), the way `releaseStrandedCompose` recovers before routing. `TestDrawingTheCommentsPanelKeepsTheAppAlive` and `TestCrossingABreakpointWhileZoomedDoesNotFreezeTheApp` hang if either comes back.

**One mouse capture owns the pane.** `handleMouse` (`mouse_focus.go`) hit-tests a left press against the panes `contentFlex` mounts and sets `focusedPane` before tview delivers the click, so the press lands on the widget and the pane agrees with it. Three rules there. It walks the mounted items rather than the three panes' rects, since `Flex.Clear` leaves an unmounted pane's rect where the last draw put it. It swallows the whole of a click that reflowed the layout, because the rects are a frame behind and forwarding hands it to whatever moved in under the pointer — press and release arrive as separate reports, and the release is the half that selects, so a latch (`swallowingClick`) carries the swallow across both. And it redraws itself: tview paints after a mouse event only when a primitive consumed it, and a press on a pane's border consumes nothing. Sub-focus stays with the controls — `claimNavFocus` for the query box and the tree, `enterDetailsFocus` for the writing boxes.

### Issue field writes

`issue_field_save.go` builds the `UpdateIssueInput` for every single-field edit. A caller names a field and gets an `issueFieldSave` back; `saveIssueField` sends it. Two write paths are not on it yet and both are due to be deleted: `ShowEditDescriptionModal` and the issue form's edit mode.

The constructors take `linearapi.Issue` **by value, never the `*App`**, and the id is captured where the user chose the issue. A picker or a text modal outlives the background refresh that moves the selection, so a write that leaves the id to the send path lands on whatever is selected by then, which is a different issue. Six writes did exactly that until ZNL-115, and `runIssueUpdateWithResult` now refuses an empty id rather than resolving one.

`issueFieldOptions` in `pickers.go` is the matching loader for every field with a list behind it: state, assignee, priority, project, milestone, cycle, labels. A new field of that kind needs both a constructor and a case; the text fields have constructors only. Every single-value overlay goes through `ShowFieldPicker`, and the chosen `PickerItem` carries its own `Name` so a save message prints "Cycle 12" where the row read "Cycle 12 (active)". Labels is the one field whose overlay is the multi-select, so `ShowEditLabelsModal` calls the loader itself rather than `ShowFieldPicker`.

**Options are loaded for the scope that owns them, never the navigation tree's.** `optionScope` is the issue's team for states, users, cycles and projects, and its project for milestones; a filter passes `navOptionScope` instead, which is what a filter means by a team. Linear rejects a state or a label from another team, so a search result or a cross-team favorite offering the tree's states is a write that fails. The flat `App` caches are read **and written** only while `scope.teamID == metadataTeamID`; the write guard is the one that matters, because a chooser opened elsewhere would otherwise fill the nav team's cache with another team's rows and leave `metadataTeamID` still naming the nav team. Ownership of that field stays with `preloadTeamMetadata`. The cost is that a cross-team issue never hits the cache.

Every load reports through its `onFail`, and nothing else touches the status bar. An overlay has nowhere to fail to, so it just names the error; the inline chooser closes itself, since a modal that never opens is invisible but a chooser stuck on "Loading options" holds the keyboard.

Messages follow one rule, in `fieldSetMessage`, `fieldClearMessage` and `fieldUpdateMessage`: `Set <field>: <value>`, `Cleared <field>`, and `Updated <field>` where the new value is not one name to print. Team keeps a phrasing of its own, because the move renumbers the issue and the identifier the user picked it by is the only one that still means anything. `TestFieldSaveMessagesNameTheFieldAndItsValue` reads them off the rendered status corner.

### Config plumbing (the most common trap)

`internal/config` has a triple: `SettingsFile` (pointer fields, what's on disk) → `Settings` → `Config`. The settings modal saves via `settingsFromForm`, which rebuilds the file from form controls — **any config field without a form control (workspaces, default_workspace, group_by, subgroup_by, columns, keybindings) must be explicitly carried through there, or an in-app settings save silently strips it from the user's config**. Every new field needs: the triple, a validator in `settings.go`, and the carry-through.

Config, credentials, prompts, the session, and the log live under `~/.zen-linear`, resolved through `config.Dir()`. **Only the settings file is dual-homed**: `ConfigFilePath` prefers a `config.json` already sitting under `$XDG_CONFIG_HOME/zen-linear` (default `~/.config/zen-linear`) and falls back to `Dir()`, which is also where a first run creates one. Nothing else follows it there, because that copy is usually a symlink into a dotfiles repo and the rest is written on every quit. The resolution happens once, at launch, and rides on `App.settingsPath` (`UseSettingsFile`): the settings modal saves back to the file it was loaded from, since re-resolving would follow an XDG copy that appeared or went away mid-session.

`internal/session` owns `session.json` (last workspace, and per workspace the nav selection, focused issue, filters, and search). It sits outside `internal/config` because config models what the user writes and this is what the app writes: `config.json` is a symlink into Drew's dotfiles repo, and a per-quit write there would dirty that repo on every launch. `App.Run` flushes it after the event loop stops, `switchWorkspace` flushes the outgoing workspace before `resetCachedState` wipes it, and the restore is a locator resolved against the live tree, never a serialized node.

`internal/cache` owns `nav-cache.json` beside it: the teams and favorites the tree is built from, keyed by workspace, versioned and discarded rather than migrated. **A session with no configured workspace name gets no entry** (a bare `LINEAR_API_KEY` or OAuth): nothing on disk tells two Linear workspaces reached that way apart, one shared entry would paint the wrong teams, and fingerprinting the token to tell them apart puts a derivative of a live credential in a cache file. It is cached API data, not user state, which is why it sits outside `internal/session`. Both write through `config.WriteFileAtomic` (temp file plus rename, mode 0600).

Drew's live config is a symlink chain: `~/.zen-linear/config.json` → dotfiles repo (`drucial-dots/configs/linear-tui/config.json`). The app writes settings in place, so in-app saves show up as diffs in the dotfiles repo — intentional.

### Keybindings

`config.Keybindings` (id → key) is validated **once**, by `resolveKeybindings` in `internal/tui/keybindings.go`, into the `resolvedKeybindings` on `App.bindings`. A binding that names a movement rune (`h`, `l`, `j`, `k`, `g`, `G`), is not a single rune, or names neither a command nor a UI action is dropped there and logged. **Everything downstream reads the resolved set, never `config.Keybindings`.** Half-honoring a rejected binding is the bug this shape exists to prevent: it used to keep a command's default alive at one call site while taking a rune at another, so one dead key explained a second.

Both a command and a UI action can hold a rune, and both give a default up to an id the user named explicitly (`takenFrom`). The check reads scope, so a navigation command and an issue command may share one. `actionKey(action, fallback)` returns **0** when something else took the fallback, which no key event carries, so that action's `case` stops matching and the claimant answers. Anything printing an action key has to drop a 0 rather than render it.

Action ids and their scopes live in `uiActionScopes`; `TestUIActionScopesCoverEveryActionKeyCallSite` greps the package for `actionKey(` calls so the map cannot fall behind. **Never compare against a hardcoded rune in a key handler** — that's how the details pane's keys broke when they were rebound.

Deleting a command is a breaking change for anyone who bound it: the id stops resolving, and the user gets a logged warning instead of a silent theft. Say so in the PR.

Tab is not pane navigation. It walks a pane's own controls, a writing box to its Post button and the navigation pane's query box, and nothing else; `{`/`}` step the details page's comment ring, and `h`/`l` and the pane numbers move between panes.

### Modal dispatch

`modalBindings` in `internal/tui/modal_dispatch.go` is an ordered list of page name → modal. Order is dispatch priority, not stack order: `handleGlobalKey` gives the key to the first open page in that list, whichever modal opened last. A new modal needs one entry plus `HandleKey` and `Focus`; the dispatcher never grows a branch. Every `Hide` ends in `restoreModalFocus`, which raises the next modal still open and falls back to the panes. `repairModalFocus` runs it again before any key routes, because a click that misses the modal's panel falls through Pages to the pane beneath and the widget there focuses itself: the modal keeps the keys, so the typing goes somewhere the caret is not.

Adding a page moves focus. `Pages.AddPage` re-delegates focus to the top visible page whenever the Pages tree holds it, and that walks down to the pane `buildLayout` flagged, not the pane the user is in. Anything that adds or replaces a page mid-session has to end in `restoreModalFocus`; `applySettings` defers it so the restore runs after `resetCachedState` moves the active tab out from under it. `modal_dispatch_test.go` pins the order against its own literal copy, so reordering the registry fails there rather than agreeing with itself.

### The modal shell

`modal_shell.go` is what an overlay panel is made of, and **every new one composes with it rather than picking its own numbers**: `modalWidth(widest)` and `fitModalHeight(want)` clamp to the terminal, `modalPanel(title)` is the bordered box (fill restored, `BorderFocus`, one column of gutter, the title on the border in the accent and never in a content row), `modalRule()` is the line over a footer that meets that border in a tee, and `centerModal` places it. **Every modal title is on the accent and every pane title on the foreground**, so the color is what says overlay. A fixed size is the bug this replaced: the agent output modal was larger than a 100x30 terminal before it drew anything, and a two-option picker cost fifteen rows.

`centerModal` refills the wrapper it is given rather than returning a new one, because pages hold that pointer for as long as the modal is up. **Two Flexes, never three**: a third only looks centered, because tview hands its spacers a negative width when the panel is wider than the slot they share. That puts the panel where it belongs and its rect somewhere else, and a rect the pointer is not in takes no clicks.

`newListModal` builds the shell, and `showPlaceholder` is its empty state: one dim row with the cursor hidden behind it, so nothing reads as pickable when there is nothing to pick. `PickerModal` and `MultiSelectModal` are both built this way, one `layout()` per `Show` since the title and the height come from what the modal was handed.

**Editing an issue's labels is the multi-select with a context line**, not a modal of its own. It was a second copy of the same toggle list until 2026-08-14. The `edit_labels` command id is unchanged; only the type went.

### Theme system

Themes are structs in `internal/tui/theme.go` registered in `ThemeRegistry`. Optional fields (`InverseText`, `StatusReview`, `StatusTriage`, `AssigneeText`, `Success`) have fallback methods so legacy themes need no changes.

The status bar's message corner is colored by what it says (`toastTag`): `flashStatus` for a nudge (plain foreground), `flashSuccess` for an action that finished (`Success`), `flashError` for one that failed (`Error`), and the accent for the loading line behind them, which is the app working rather than a result. Plain is the default for a nudge so the other three mean something when they appear — a failure carrying an error value goes to `updateStatusBarWithError` instead, which keeps the whole thing on the hint line rather than in a corner sized to a few words.

Every color comes from the theme, never a literal. `RosePineMoonTheme` is built from named palette constants and `TestRosePineMoonStaysInItsPalette` fails on any field outside them: the palette has six hues and no green, so `Success` and `StatusReview` take iris rather than a hex borrowed from another theme. `RosePineMoonTheme` sets `Background: tcell.ColorDefault` (terminal transparency), which breaks tview defaults that assume an opaque `PrimitiveBackgroundColor` — selection styles, InputField inner fill, modal backgrounds all have explicit workarounds (`selectionStyle`, `newThemedInputField`, `ModalBackground`). When adding UI, style from the theme, not tview defaults.

Modal panels: `tview.NewFlex` (and Grid) set `dontClear`, so a Flex never paints its own background and the layer beneath bleeds through unpainted cells. Any modal panel needs `panel.Box = tview.NewBox()` before its other Box settings to restore the fill — `FormModal` does this for its shells; hand-built overlays must too.

### Navigation pane

The pane is `navigationPanel` (`nav_search.go`), a Flex owning the border, the pane title, and one column of border padding: the query box in a bordered frame of its own, and a borderless `navigationTree` beneath. `navSearchFocused` says which of the two controls holds the keyboard, and `navSearchActive()` gates key routing above the global runes in `handleGlobalKey` — without that gate, `q`, `:` and the pane numbers fire while you type. `buildNavigationPanel` rebuilds the box and the frame on a theme change and must end in `rebuildContentLayout`, or `contentFlex` keeps the old pointer.

Both controls call `claimNavFocus` from a `SetFocusFunc`, because a mouse click focuses a widget without ever reaching `updateFocus`. Three rules there. It must **never call `updateFocus`** — that calls `SetFocus`, which calls the callback back. It **stands down while an overlay is up**: tview re-delegates focus down the whole tree on every page add and remove, that walk reaches this pane, and the palette rebuilds its page on every keystroke. Without the guard the claim took the pane back mid-rebuild and the palette's own re-show check then failed silently, so every key closed it.

And it **records which of the two controls has the keyboard, never the pane**. `enterDetailsFocus` (`details_compose.go`) is the same primitive for the details page and holds the same rule, for the same reason. `handleMouse` owns `focusedPane` and sets it before the click is delivered, so claiming the pane in a focus callback as well let anything that focuses those widgets claim it. The two windows the overlay guard cannot see are opening the palette, which rebuilds its page before it records the pane to go back to, and closing a modal, which drops its page before `restoreModalFocus` reads one. A walk landing in either window took the pane, and Escape out of a picker or the palette went there instead of back where it came from.

**Which pane the walk reaches is not fixed**: it is whichever child `contentFlex` flagged, and that flag goes stale, because `updateFocus` only rebuilds the layout below the wide breakpoint. On a wide terminal, stepping off a pane leaves the flag on it, so the same bug surfaced first as the nav pane stealing focus and then as the details pane. `TestClosingAnOverlayGoesBackToThePaneItOpenedFrom` and `TestClosingAnOverlayLeavesTheDetailsPaneWhereItWas` are what catch it coming back.

One of the panel's two children must carry the Flex's focus flag. `SetRoot` and every added page delegate focus downward, and a Flex with nothing flagged keeps it on its own Box, which answers no keys: the pane looks focused and the arrows do nothing.

The search is workspace-wide and takes neither the tree's scope, the rich filters, nor the sort chain. That is why the issues context line skips it.

### Command palette

`palette_modal.go` is the panel, `palette_controller.go` is what it draws, `palette_search.go` ranks a query.

The panel is rebuilt on every keystroke — the query box, its frame and the list are reused, the centering wrappers are not — so `layoutPaletteModal` is the only place its shape is written down. Its chrome is the shared modal shell above; only the query box, the row layout and the row cap are the palette's own.

`PaletteRow` is a heading or a command, the way `IssueRow` is. **The cursor never rests on a heading**: `step` walks past them, so `Selected` cannot answer with one, and `filterCommands` lands the cursor on the first command rather than row zero. Headings only appear under an empty query; a query lists matches flat, since grouping would fight the ranking.

`rankCommands` scores a command by where the query sits — the start of the title, the start of a word in it, anywhere in it, then the start of a word in a keyword and anywhere in one — and a command has to score against every whitespace-separated token to be listed at all. A second pass matches the token's characters scattered through the title and keywords, and **runs only when the first found nothing**, so it can never dilute a real result. `commandGroupOrder` is the heading order; a command whose group is missing from it lists last under no heading, which `TestEveryCommandFilesUnderAHeading` is what stops happening by accident.

### Issues list

`BuildGroupedIssueRows` (`internal/tui/issue_tree.go`) produces a flat `[]IssueRow` where group/subgroup headers are rows with `IsHeader: true` and no issue. Headers are selectable (Enter/Space/click toggles collapse), so **any code walking table rows or moving selection must skip or special-case headers** (`nextIssueRow`, and the default-selection logic in `rebuildIssuesTables`). Columns are a registry in `issues_table.go` (`issueColumnSpecs`), rendered per the `columns` config.

`updateIssuesColumnLayout` swaps the mounted primitive between the active section's table and the placeholder. Focus lives on the primitive, so a swap has to carry it (`issuesPaneHasFocus`) or the pane goes dead with the keys on something off screen.

The pane has no tabs. `IssuesSection` is two values, `List` and `Search`, with a table and a row model each; `performIssueSearch` owns which one is active, so an empty query is what puts the list back. The pane title names what is on screen (`issuesTitleLabel`) and `issues_context.go` right-aligns the sort and filter line into the same top border row, measuring against that title so the two cannot collide.

### Details pane

The pane has no tabs. It is one scrolling page, not a layout: `detailsPage` (`details_page.go`) is text with holes in it. `renderDetailsPage` (`details_comments.go`) writes the whole thing into one text view — the metadata header, a rule, the description, a rule, the Activity label heading its section, the activity lines and comment cards in one stream, an open reply box, and the compose card that ends the page — and records a `pageSlot` for each live widget. The page's `Draw` paints the text, then draws the two `TextArea`s and their buttons over the rows their frames left empty. **A widget in that page is positioned by the render, not mounted in a Flex**, which is what keeps one scroll, one measure, and one set of borders across the lot; anything added there needs a slot and nothing else.

Every span and slot is offset by however many rows the issue took, and that offset is not a number anyone keeps: the header lines go into the same slice the cards are counted in, so `len(lines)` is the only source of it. A count held apart would have to be recomputed on every refit, since the header's height moves with the width.

The metadata header is `detailsHeaderRows []detailsRow` (`details_view.go`), not strings: a row carries the text read mode prints, the `issueField` it edits, and the column its value starts at. Every gridded row is built by `detailsGridRow`, which pads the label out to `detailsLabelGutter`, so **the grid is one constant rather than a format string per field**. Subscribers, Relations and Attachments outgrow the gutter, take a single space, and edit nothing. `detailsHeaderBlock` reports a `fieldSpan` per editable row as it emits it, which is the map the field cursor will move by — recorded at build time, since measuring at render would mean stripping the color tags first. **`valueColumn` is a column in the untruncated row**, and the same loop cuts that row to the measure, so a pane narrower than the gutter leaves the column past the end of what was drawn. Anything placing a cursor or a highlight by it bounds it against the drawn line.

**Edit mode is `detailsEdit` (`details_edit.go`)**, entered by `e` and left by Escape: `j`/`k` step the cursor over the `fieldSpan`s the last render recorded, held by field id so a background rebuild leaves it where it was. It is not a page, so `activeModal` cannot see it and nothing above it closes it. `handleGlobalKey` hands it every key ahead of the global runes and `handleDetailsEditKey` is **default-deny**, or `q` quits from under a field. Every way out is written down: the three pane moves, a click into a writing box, an issue change, and a layout change that took the keyboard off the pane. A command shortcut is not one of them — a modal takes the keys ahead of the mode, so the pickers run and the cursor is still where it was when the modal goes. Every header row gains `detailsCursorGutter` cells while it is on, `valueColumn` included. **`e` renders the selection before it enters** where the pane is still showing the issue before it, or the detail debounce that follows would drop the mode a tenth of a second later. And `updateDetailsView` scrolls to the top **only when the issue changed**, since every save and every refresh comes through it and a reset on those throws the reader back to line zero.

**Enter opens a chooser in the page's own rows** (`details_chooser.go`): up to `detailsChooserMaxRows` options and a count of the rest, framed with the comment card's own `cardEdge` and `cardRow` and hung off the field's `valueColumn`, injected in `detailsHeaderBlock`'s loop right after the row it belongs to. It is sized to its widest option rather than to the measure, and drops the frame below `commentCardMinWidth` the way a comment does. Everything below shifts for free, because every span, slot and card row is `len(lines)` where it is emitted. It is text and a cursor, not a `tview.List`: a List answers `MouseLeftClick` and not `MouseLeftDown`, so the press falls through to the TextView beneath and the focus stops agreeing with the state, and it does not bind `j`/`k` either. The cap is clamped against the pane's own height, since `scrollRowsIntoView` anchors a block taller than the viewport to its top and the bottom of one would be unreachable.

Five rules there. The render **clamps `choice` and `offset` itself**, because it runs inside `Draw` and an index past a list that came back shorter is a panic in a frame; it clamps the anchor column too, since `valueColumn` is a column of the untruncated row and a pane narrower than the gutter would draw the whole chooser past the drawn line, invisible and holding the keyboard. Every move re-renders and then **scrolls the lit row together with the frame under it** — `refitDetailsPage` early-returns on an unchanged size, so nothing else will draw the highlight, and the wheel goes straight through the pane, so the chooser can be scrolled off. The lit row alone parks on the last visible row, which puts the count and the closing edge below the fold on every step to the end; the pair is bounded so a chooser taller than the pane still anchors on the option the keys are on. Both ways out end in `scrollFieldIntoView`, or the rows going away leave the field above the viewport. The load's generation counter is `App.chooserGeneration`, **not** a field of `detailsEditState`, which every exit zeroes: a counter inside it restarts at nought and the next opening collides with the last one's in-flight load. And a commit refuses an option whose scope moved under it (`chooserScopeMoved`): the team for most fields, the project for a milestone, nothing for priority, which is a local list.

**`refitDetailsPage` guards on the height as well as the width**, and `chooserVisibleRows` reads `detailsFittedHeight` rather than the view's own rect, which during a draw is still the frame before. The cap is against the pane, so a height-only resize has to re-lay the page or an open chooser stays taller than the pane it is in. Only a width change re-runs the description, since that is the half glamour cares about.

The current option wears `themeTags.Selection`, the cursor line the tree and the issue tables paint with, padded inside the tag so it runs the width of the list rather than the width of the word — which is why the frame is measured **before** the line goes on, or every list comes out as wide as the pane. The field's own `❯` stays at the margin and dims, since the line inside the box is where the keyboard is. The highlight opens on the value the field already holds, which is the whole of marking it; a clear row heads the list only for a field that has something to clear. A commit re-reads the issue from the selection while the id still matches — the captured id is the write target, the rest is state a chooser holds far longer than a modal did.

**Labels is the same chooser as a multi-select**, marked with `multiSelectRow` and held in `detailsEdit.picked`. Space toggles and writes nothing; Enter sends the whole set, because `labelIds` is one field and a save per toggle races itself. Three rules of its own. The lit row takes a bare `multiSelectGlyph` rather than the colored mark, since the mark's `[-]` resets the foreground and would leave the text after it on the terminal default over the cursor line's background. The cursor opens on row zero, the marks being what says which labels the issue holds. And **the write is rebased at commit, not carried from the snapshot** (`labelWrite`): the reader's toggles decide the labels the list offered, and the freshly re-read issue decides any it did not. A label added while the chooser sat open has no row saying anything about it, so applying must not drop it; one taken off must not come back. Which is also why "unchanged" for a set is measured against the issue the chooser **opened** on rather than the fresh one — Enter after no toggle is a close, not a write that undoes what a refresh brought in.

**Title, due date and estimate open a box rather than a list** (`details_editor.go`), held by `detailsEdit.editing` beside the chooser's `open`. It is a real `tview.InputField` in a `pageSlot`, the only widget the header block contributes — which is why `detailsHeaderBlock` returns slots at all, and its lines starting the page is why they need no rebasing. One row that scrolls, never one that grows: a box sized to its text re-renders the page on every keystroke. Four rules. `handleFieldEditorKey` is **default-allow**, the inverse of the mode around it, so `q` is a letter; it keeps Enter, Escape, and Tab, which is swallowed because `InputField`'s done func would hand the keyboard to whatever tview walks to next. The chooser never took focus and this does, so **opening renders before it focuses and closing renders before it moves focus off** — the slot has to exist first, and a caret on a zeroed rect is left on the terminal. A teardown a focus callback can reach (`leaveDetailsEdit`, `updateDetailsView`) calls `releaseFieldEditor`, which drops the target and sets `layoutFocusStale` rather than moving focus itself. And the widget's own `SetFocusFunc` is `claimFieldEditorFocus`, not `enterDetailsFocus`, whose two-rings guard would drop edit mode the instant the box took a click.

An empty box is a clear for due date and estimate, checked before the parse, and a refusal for a title, which is what Linear does with one. `issueFieldDueDateSave` and `issueFieldEstimateSave` already returned errors for exactly this caller: on a refusal the text stays, the box stays, and the reason goes in `detailsEdit.err` under it rather than on a status bar the next cursor move wipes. **That covers what the app can tell on its own, not what Linear refuses later**: the box is closed by the time an async write answers, and a server-side rejection reports through `updateStatusBarWithError` like every other field's. Keeping it open across the round trip would need the chooser's generation stamp, since `updateDetailsView` zeroes `detailsEdit` on an issue change. `issueFieldTitleSave` reports `fieldUpdateMessage`, since a title is too long for the toast corner.

**The view has wrapping off**, because the page counts its own lines: a line the view wrapped is one page line drawn as two screen rows, and every card and box below it lands a row out. Everything written to it is fitted to the measure first. The description body goes through `commentBodyLines` then `wrapTagged`, the same two steps a comment body does — glamour wraps prose to the measure but cannot break a bare URL and does not wrap code blocks or tables. `TestLongDescriptionLinesWrapToTheMeasure` is what catches a regression there, and a short fixture will not.

`commentBlocks` (`comment_thread.go`) is the page's order: threads from `buildCommentRows`, the activity folded in by time by `mergeActivityBlocks`, the reply box at the end of the thread it answers, the compose card last. The ring `{` and `}` walk is the `commentSpans` the same render recorded, so **what the braces do follows what was drawn** — a stop that is not on the page cannot be focused. An activity line records no span, which is the whole of it not being a stop; nothing addresses one, so `activityBlock` gives it no id either.

**A thread is placed as a whole, by its root.** Events drain only where a depth-0 block starts, so an event stamped between a root and its reply lands after the thread's last reply. Splitting a thread would break the rail's closing corner (`isLastReply` reads the next block's depth) and leave it trailing into a line that is not a card. Inside a writing box the braces are prose; Esc is the way back to the cards. Linear rejects a `parentId` that is not top level, so a reply always posts against `threadRootID`.

The pane's vertical padding is written as blank lines in the text rather than held back from the content rect: `trailingPad` at the end, and the top rows inside `detailsHeaderBlock`, where the spans are counted. Mid-scroll the content runs to both borders, and each gap is an end of the issue rather than a margin that never moves. The pane keeps its left and right padding on the box, which has nothing to do with the scroll.

### Workspaces and auth

`workspaces` in config reference env vars (`api_key_env`) — keys are never stored in the file. A bare `LINEAR_API_KEY` env var overrides all auth unconditionally (`internal/auth/resolve.go`); never export one. Known upstream bug, unfixed: `applySettings` rebuilds the API client without `UseBearer`/`OnUnauthorized`, breaking OAuth sessions after an in-app settings save (doesn't affect API-key workspaces).

### GraphQL client

`internal/linearapi` splits by domain: `client.go` (construction, auth transport), `retry.go` (429/5xx backoff, wrapping the auth transport), `types.go` (domain structs plus every input type's `GetGraphQLType` and `MarshalJSON`), `teams.go`, `favorites.go`, `metadata.go` (projects, milestones, cycles, users, workflow states, labels), `issue_filters.go`, `issues_query.go`, `issue_parse.go` (`toIssue`/`toRef` conversions off the query node types), `issue_activity.go` (history entries to feed events), `issue_detail.go`, `issue_mutations.go`, `comments.go`. Tests did not follow the split: `client_test.go` covers most of the package, alongside `favorites_test.go`, `retry_test.go`, and `query_golden_test.go`.

Queries are struct-tagged shurcooL/graphql selections. A field the schema doesn't allow in that position makes Linear reject the entire query — one bad field in the `Attachments` node once silently broke attachments, comments, and GitHub links together. Verify field placement against the Linear schema when extending a selection, and live-test fetches.

One issue selection serves every call site. `issueQueryNode` (`issues_query.go`) is the list, search, custom-view, and mutation shape; `issueDetailNode` (`issue_detail.go`) **embeds it untagged and first**, which is what makes shurcooL inline its fields flat and in order, then adds the five connections only the details pane needs. Tag that embedded field or move it and the whole selection changes shape.

`testdata/*.graphql` holds the exact outgoing query for each of the package's `c.client.query`/`mutate` sites, and `query_golden_test.go` fails if any of them drifts. **A refactor of the query structs is only safe when that diff stays empty.** A new call site needs a case in `goldenQueryCases()`; `TestQueryGoldensCoverEveryCallSite` counts the sites in the sources and fails when the table falls behind. Regenerate with:

```sh
ZEN_UPDATE_GOLDENS=1 go test ./internal/linearapi -run TestQueryGoldens
```

### Issue activity

`Issue.history` rides on the detail query (`history(first: 50)` on `issueDetailNode`) and `issue_activity.go` turns each entry into feed events. Two things there are not obvious:

**One entry yields several events.** Linear records everything saved together as one entry, so a state move and an assignee change arrive in the same node. The feed draws one icon and one phrase per line, so `toActivity` emits an event per change and returns none for an entry carrying nothing renderable.

**Creation has no history entry.** It is derived in `issueDetailNode.toIssue` from `Issue.createdAt` plus `creator`/`botActor`, and dropped when the time is zero rather than sorted to the top as "now".

**`relationChanges.type` is an undocumented two-letter code**, decoded in `relationChangePhrases`: a relation letter (r related, x blocking, b blocked by, d duplicate, m duplicate of) prefixed with `a` when added and suffixed with `r` when removed. The five addition codes plus `xr` and `br` were read off a real workspace; the rest follow the pattern. **An unrecognized code drops the event** — if Linear extends the vocabulary the feed loses a line rather than printing a wrong one, and that is the intended failure.

### Agent providers

Each provider in `internal/agents` builds argv for a CLI that isn't in this repo, so nothing but a real run proves a flag exists. `--sandbox` and `--workspace` shipped for months against `cursor-agent`, which has neither, and every run died on the first one. Provider tests pin the whole argv against a literal for that reason; check a new flag against the installed CLI before adding it, and note that `--help` exits 0 on an unknown flag, so it settles nothing. `AgentRunOptions.Sandbox` is a portable intent, not a flag: claude maps it to `--permission-mode`, cursor to `--force`. The workspace rides on `cmd.Dir` in `runner.go`, never a flag.
