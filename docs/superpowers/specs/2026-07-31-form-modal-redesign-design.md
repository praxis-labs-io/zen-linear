# Form modal redesign

Date: 2026-07-31
Status: approved direction, pending implementation plan

## Goal

All nine form modals converge on one Linear-style layout: a caps label above a
framed field, modal size driven by content, quiet buttons, a dim hint line.
This replaces the current copy-pasted `tview.Form` pattern, which has a
duplicate heading inside the border, side labels, a fixed 75x20 shell that
clips fields (New Issue loses Priority) while hardcoded 60-cell field widths
overflow the border, and every button filled with the accent color regardless
of focus.

Decisions made with Drew:

- Direction: Linear-style rebuild (caps label row above framed fields), chosen
  over a palette-style compact layout after seeing both mocked.
- Buttons: accent fill on the focused button only; unfocused buttons are dim
  text. At most one loud element on screen, and it always marks keyboard focus.
- Scope: all nine modals in one round, including Settings.
- Architecture: hybrid builder (approach 1). Custom shell and field layout,
  stock tview editors inside the frames. A full custom widget kit (own text
  editing, inline pickers) was rejected as risk without visual payoff behind
  the frames; restyling `tview.Form` in place was rejected because it cannot
  place labels above fields.

## Target layout

```
╭─ New Issue ─────────────────────────────────────╮
│ TITLE                                           │
│ ┌─────────────────────────────────────────────┐ │
│ │ Fix palette focus restore                   │ │
│ └─────────────────────────────────────────────┘ │
│ DESCRIPTION                                     │
│ ┌─────────────────────────────────────────────┐ │
│ │ Repro: open palette from details pane…      │ │
│ └─────────────────────────────────────────────┘ │
│ ASSIGNEE            CYCLE          PRIORITY     │
│ Drew White (me) ▾   No cycle ▾     Normal ▾     │
│                                                 │
│ █ Create █   Cancel                             │
│ Esc cancel · Tab next · ⌃⏎ submit               │
╰─────────────────────────────────────────────────╯
```

- Border title is the only title. The inner heading rows
  ("Create New Issue", "Edit Issue Description") are removed.
- Text fields (input, textarea) get a caps label row plus a framed box.
- Pickers (dropdowns) render compact: caps label row, value + `▾` beneath,
  several sharing one row where they fit.
- The hint line sits at the bottom inside the border, SecondaryText, built
  from the keys the modal actually binds.

## Components

### FormModal builder — new `internal/tui/form_modal.go`

One builder owns everything the nine modals currently copy-paste.

Shell:

- Bordered flex on `theme.ModalBackground()`, border runes per
  `config.RoundedBorders`, title in the border, `density.ModalPadding`.
- Width: clamp(60, screen-8, 76). Every field frame spans the full inner
  width — field widths derive from the modal, never the reverse (kills the
  Title overflow).
- Height: computed from content — per-field rows plus chrome — and clamped to
  screen height minus 4. When the clamp bites, textarea rows shrink first.
  Nothing is ever silently clipped (kills the missing-Priority bug).

Field API (each returns the underlying tview primitive so callers keep their
current read/write code):

- `AddInput(label, initial) *tview.InputField`
- `AddTextArea(label, initial, rows) *tview.TextArea`
- `AddPicker(label, options, selected, onChange) *tview.DropDown` — pickers
  added consecutively share a row when their labels and values fit
- `AddStatic(label, text)` — read-only row (e.g. "Parent: ZEN-101 …")
- `AddButtons(buttons ...FormButton)` — variadic `{Label, OnPress}`; the
  prompt-templates modal needs four (Add / Delete / Save / Cancel). With
  focus-only accent there is no persistent "primary" look — order carries the
  emphasis, Cancel goes last, Esc always cancels regardless of buttons.

Field rendering: caps label row in SecondaryText; beneath it a framed box
(`theme.Border`; `theme.BorderFocus` while the editor inside holds focus)
wrapping the stock editor. Editors are restyled from the theme only — no
`tview.Styles` defaults — so the transparent Rose Pine Moon background keeps
working.

Buttons: builder-drawn row, not `tview.Form` buttons. Unfocused:
SecondaryText on the modal background. Focused: `theme.Accent` background,
`theme.InverseTextColor()` text. Order: primary, then Cancel.

Keys, handled once in the builder:

- Tab / Shift+Tab cycle fields → primary → cancel → wrap.
- Esc cancels; if a dropdown is open it closes the dropdown instead
  (preserves today's `closeOpenDropdown` behavior).
- Ctrl+Enter / Cmd+Enter submits from anywhere.
- Enter in a single-line input moves to the next field; Enter in a textarea
  stays a newline; Enter on a button activates it.
- `Show()` always resets focus to the first field. Root cause, proven in the
  live app: tview forms remember their last-focused item across shows, so
  after one button submit a reopened modal delegates focus to that button and
  typing goes dead. Edit-description just fixed this locally with
  `form.SetFocus(0)` plus a regression test; edit-title and create-comment
  still carry the latent copy. The builder owns Show once, so the class dies
  here instead of being patched modal by modal.

### Per-modal conversion

| Modal | Fields |
| --- | --- |
| Edit description | textarea (10 rows) |
| Edit title | input |
| Create comment | textarea (5 rows) |
| New issue | input, textarea (4), pickers: assignee / cycle / priority, static parent row when sub-issue |
| Agent prompt | input (workspace), picker (template, only when templates exist), textarea (5) |
| Prompt templates | keeps its two-pane body (template list beside the form); the form column adopts builder field rows (name input, prompt textarea) and the shared button row (Add / Delete / Save / Cancel) |
| Text input | single input, no label row (border title carries it), keeps its small footprint |
| Confirmation | keeps `tview.Modal`, but buttons restyled to the same focus-only accent treatment |
| Settings | all current controls through the builder, grouped in their existing order |

Async population (assignee/cycle "Loading…" then options) is untouched — the
builder returns the same `DropDown` primitives today's code populates.

The in-flight edit-description feature (uncommitted in the working tree) lands
first as-is; its modal is then converted with the others. The conversion
sweep is one change; no modal ships half-converted.

### Settings save path (the known trap)

`settingsFromForm` rebuilds `config.json` from form controls; a control lost
in conversion silently strips fields from the user's config. The conversion
keeps `settingsFromForm` reading from the builder-returned primitives, and
adds a regression test: build the settings form from a full `SettingsFile`
(workspaces, default_workspace, group_by, subgroup_by, columns, keybindings,
agent fields), save without touching anything, assert the result round-trips
identically.

## Testing

- Builder unit tests: height computation (fits content, clamps to screen,
  textarea shrinks first), width clamp, Tab cycle order including wrap,
  button focus styles, Esc-with-open-dropdown, Show() focus reset.
- Settings round-trip regression test as above.
- Existing modal tests (`commands_test.go`, `agent_command_test.go`, others)
  keep passing.
- Manual pass in a transparent terminal: every modal opened on the Rose Pine
  Moon theme — no opaque bleed, no overflow, focused-button accent correct.

## Out of scope

- Command palette, picker/multi-select list modals — already restyled in the
  earlier theme work.
- Inline expanding pickers (Linear-app style) — revisit only if the dropdown
  overlay feels wrong after the rebuild.
- Any behavior change to what the forms submit.
