# Code Quality (zen-linear)

Hard conventions for this repo. The stack is Go 1.24, tview/tcell, shurcooL/graphql.

## Anti-defaults (counter common Claude tendencies)

- No premature abstractions. Three similar lines beats a helper used once.
- Don't add features or improvements beyond what was asked.
- Don't refactor adjacent code while fixing a bug.
- No dead code or commented-out blocks.
- WHY comments only, never WHAT. If code needs a "what" comment, rename instead.
- Don't modify generated files.

## Naming

- **Packages**: short, lowercase, no underscores (`tui`, `linearapi`, `config`). One package per directory.
- **Files**: snake_case, matching the repo (`issues_table.go`, `form_modal.go`). Tests are `foo_test.go` beside `foo.go`.
- **Identifiers**: exported PascalCase, unexported camelCase. No stutter (`config.Load`, not `config.LoadConfig`).
- **Booleans**: `is` / `has` / `should` / `can` prefix.
- **Functions**: verb-first (`buildGroupedIssueRows`, `applySettings`).
- **Constants**: Go style (`MixedCaps`), not SCREAMING_SNAKE.

## No defer markers

- Never leave `TODO`, `FIXME`, `HACK`, `XXX`, `TEMP`, or `REMOVEME` in code. A marker is a sign the thing should be fixed, so fix it now.
- Never add an inline `//nolint` to silence a lint violation. The violation is the signal to fix the code. The scoped exclusions in `.golangci.yml` are config, not a precedent for inline suppression.
- Genuinely out-of-scope work gets a Linear ticket in the right bucket, never a comment in the code.

## Go specifics

- No `any` or `interface{}` where a concrete type or a small interface works. Accept interfaces, return structs.
- Wrap errors with `%w` and context: `fmt.Errorf("loading settings: %w", err)`. Never discard an error silently.
- No naked returns in anything longer than a few lines.
- Table-driven tests for anything with more than two cases.
- Keep goroutine use explicit and justified. UI mutation happens on the tview event loop (`QueueUpdateDraw` from anywhere else).

## Reuse before build (this codebase's primitives)

Search these before hand-building anything:

- **Styling**: everything styles from the active theme (`internal/tui/theme.go` accessors), never tview defaults. `RosePineMoonTheme` uses a transparent background that breaks default styling; the workarounds (`selectionStyle`, `newThemedInputField`, `ModalBackground`) already exist, use them.
- **Modals**: `FormModal` (`internal/tui/form_modal.go`) for forms; any hand-built overlay panel needs `panel.Box = tview.NewBox()` first or the layer beneath bleeds through.
- **Issue list**: columns live in the `issueColumnSpecs` registry (`internal/tui/issues_table.go`); rows come from `BuildGroupedIssueRows` and header rows must be skipped or special-cased when walking or selecting.
- **Keybindings**: resolve through `actionKey(action, fallback)` or the command shortcut map. Never compare against a hardcoded rune in a key handler.
- **GraphQL**: extend the struct-tagged selections in the matching `internal/linearapi` domain file (`issues_query.go`, `issue_detail.go`, `issue_mutations.go`, `metadata.go`, `favorites.go`, `comments.go`, `teams.go`); verify field placement against the Linear schema, one bad field silently kills the whole query.

## Tests

- Tests ship in the same PR as the logic, never a follow-up.
- Test through the real interface: drive key events and read rendered state, not internal fields, where practical.
- Respect the existing patterns in `internal/tui/*_test.go` before inventing a new harness.

## File size

- Keep files focused. `app.go` and `client_test.go` are already too big; don't make that worse, and don't grow a new file past what one sitting can review.
