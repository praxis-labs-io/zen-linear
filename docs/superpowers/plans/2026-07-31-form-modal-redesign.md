# Form Modal Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the copy-pasted `tview.Form` pattern in all nine modals with one `FormModal` builder producing the approved Linear-style layout (caps label above framed field, content-driven sizing, focus-only accent buttons, dim hint line).

**Architecture:** New `internal/tui/form_modal.go` owns the shell (border title, width/height computation, hint), field rows (caps label + framed stock tview editor), the button row, and all shared keys (Tab cycle, Esc, Ctrl/Cmd+Enter, Show-resets-focus). The nine modals declare fields and keep their existing public `Show`/`Hide`/`HandleKey` APIs so callers don't change. Spec: `docs/superpowers/specs/2026-07-31-form-modal-redesign-design.md`.

**Tech Stack:** Go 1.24, tview v0.42 / tcell v2.13. Tests use the existing `newUXTestApp()` helper and `app.app.GetFocus()` assertions (see `internal/tui/ux_remediation_test.go`).

## Global Constraints

- Style ONLY from `app.theme` (`ModalBackground()`, `InputBg`, `Foreground`, `SecondaryText`, `Border`, `BorderFocus`, `Accent`, `InverseTextColor()`). Never rely on `tview.Styles` defaults — the Rose Pine Moon theme has `Background: tcell.ColorDefault` and defaults break on it.
- Field and modal widths derive from the screen, never hardcoded field widths (the old `AddInputField(…, 60, …)` overflow bug).
- Nothing is silently clipped: height fits content, clamps to screen, textareas shrink first, and when still too tall the row window scrolls to keep the focused row visible (the old missing-Priority bug).
- `Show()` always resets focus to the first field (tview forms remember their last-focused item across shows; see memory `tview-form-focus-reset`).
- Public modal APIs (`Show` signatures, `HandleKey`, `GetModal`) stay unchanged.
- Verify with `make all` run directly — never through a pipe. CI-parity lint: `GOTOOLCHAIN=go1.24.4 <scratchpad>/bin/golangci-lint run ./...` (v2.8.0 pin).
- Commit to `main` (product branch), conventional prefixes (`feat:`, `refactor:`, `test:`).
- Single test: `go test ./internal/tui/ -run TestName`.

---

### Task 1: FormModal builder — shell, text fields, buttons, keys

**Files:**
- Create: `internal/tui/form_modal.go`
- Test: `internal/tui/form_modal_test.go`

**Interfaces (produced for all later tasks):**

```go
type FormButton struct {
    Label   string
    OnPress func()
}

func NewFormModal(app *App, title string) *FormModal
func (fm *FormModal) SetMaxWidth(w int)                                  // default clamp 76; settings uses 110
func (fm *FormModal) SetHint(hint string)                                // dim line inside the bottom border
func (fm *FormModal) SetOnCancel(fn func())                              // Esc
func (fm *FormModal) SetOnSubmit(fn func())                              // Ctrl+Enter / Cmd+Enter
func (fm *FormModal) AddInput(label, initial string) *tview.InputField   // caps label + framed 1-line editor
func (fm *FormModal) AddTextArea(label, initial string, rows int) *tview.TextArea
func (fm *FormModal) AddButtons(buttons ...FormButton)                   // one row; Cancel last by convention
func (fm *FormModal) Show(pageName string)                              // sizes, resets focus to first field, AddPage+SendToFront+SetFocus
func (fm *FormModal) Hide(pageName string)                              // RemovePage + app.updateFocus()
func (fm *FormModal) Root() *tview.Flex                                 // for GetModal()
func (fm *FormModal) HandleKey(event *tcell.EventKey) *tcell.EventKey   // Esc/submit for the modal-level dispatcher
```

Internal structure:

```go
type formRow struct {
    container  *tview.Flex       // label row + widget stack
    height     int               // rows at full size (label included)
    minHeight  int               // shrink floor when clamped
    flexible   bool              // textareas shrink first
    focusables []tview.Primitive // widgets inside, in tab order
    frame      *tview.Flex       // framed wrapper to recolor on focus (nil for pickers/static)
}

type FormModal struct {
    app      *App
    title    string
    maxWidth int // 0 → default 76
    root     *tview.Flex // centering wrapper
    frame    *tview.Flex // bordered column
    rowsBox  *tview.Flex // holds row containers; scrolled by zero-heighting rows above/below the window
    hintView *tview.TextView
    rows     []formRow
    order    []tview.Primitive // fields then buttons, tab order
    buttons  []*tview.Button
    onCancel func()
    onSubmit func()
}
```

Layout rules (exact):

- Row heights: input = 1 label + 3 framed = 4. Textarea(rows) = 1 + rows + 2, `flexible: true`, `minHeight` = 1 + 3 + 2.
- Width: `w := screenW - 8; if max := fm.effectiveMaxWidth(); w > max { w = max }`; inner field frames span the full inner width.
- Height: sum of row heights + 1 blank + 1 buttons + 1 hint + 2 border + `density.ModalPadding` top/bottom. Clamp to `screenH - 4`; overflow is absorbed by shrinking flexible rows down to `minHeight`, evenly; any remaining overflow enables row-window scrolling: `ensureVisible(rowIndex)` sets rows outside the window to height 0 via `rowsBox.ResizeItem` so the focused row is always on screen.
- Caps label: `strings.ToUpper(label)` in a 1-row TextView, `theme.SecondaryText` on `theme.ModalBackground()`.
- Frame: `tview.NewFlex` wrapping the editor, `SetBorder(true)`, `theme.Border` normally, `theme.BorderFocus` while its editor has focus (recolored in the editor's `SetFocusFunc`/`SetBlurFunc`, which also call `ensureVisible`).
- Editors: `SetFieldBackgroundColor(theme.InputBg)` … wait, inside a frame the editor sits on the modal background; use `theme.ModalBackground()` as field background and `theme.Foreground` text so the frame is the affordance, not a fill.
- Buttons: `tview.NewButton(label)`, `SetStyle` = `theme.SecondaryText` on `theme.ModalBackground()`, `SetActivatedStyle` = `theme.InverseTextColor()` on `theme.Accent`. Laid out in one row with 3-cell gaps, left-aligned.
- Hint: `theme.SecondaryText`, left-aligned, 1 row at the bottom inside the border.
- Border: `SetBorder(true)`, `theme.BorderFocus` color, title `" "+title+" "`, `theme.Foreground` title color, `density.ModalPadding` border padding. (Rounded runes come from `applyThemeStyles` globals — nothing to do here.)

Keys, on `fm.frame.SetInputCapture`:

- `Tab` → focus next in `fm.order` (wrap); `Backtab` → previous (wrap).
- `Esc` → if any `*tview.DropDown` in order `IsOpen()`, forward the event to it (close) — else call `onCancel`. (DropDowns arrive in Task 2; write the loop now, it's a no-op until then.)
- `Ctrl+Enter` / `Enter` with `ModMeta` → `onSubmit` when set.
- `Enter` on a focused single-line `InputField` → focus next (builder sets `SetDoneFunc`; callers may override on the returned primitive).

- [ ] **Step 1: Write failing tests**

```go
package tui

import (
    "testing"

    "github.com/gdamore/tcell/v2"
)

func TestFormModalShowResetsFocusToFirstField(t *testing.T) {
    app := newUXTestApp()
    fm := NewFormModal(app, "Test")
    first := fm.AddInput("Title", "")
    fm.AddTextArea("Body", "", 5)
    fm.AddButtons(FormButton{Label: "OK"}, FormButton{Label: "Cancel"})

    fm.Show("form_test")
    if app.app.GetFocus() != first {
        t.Fatal("first Show did not focus the first field")
    }
    // Simulate a prior session ending focused on a button, then reopen.
    app.app.SetFocus(fm.order[len(fm.order)-1])
    fm.Hide("form_test")
    fm.Show("form_test")
    if app.app.GetFocus() != first {
        t.Fatal("reopen did not reset focus to the first field")
    }
}

func TestFormModalTabCyclesFieldsThenButtonsAndWraps(t *testing.T) {
    app := newUXTestApp()
    fm := NewFormModal(app, "Test")
    fm.AddInput("Title", "")
    fm.AddTextArea("Body", "", 5)
    fm.AddButtons(FormButton{Label: "OK"}, FormButton{Label: "Cancel"})
    fm.Show("form_test")

    capture := fm.frame.GetInputCapture()
    for i := 1; i <= len(fm.order); i++ {
        capture(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
        want := fm.order[i%len(fm.order)]
        if app.app.GetFocus() != want {
            t.Fatalf("after %d tabs focus = %T, want order[%d]", i, app.app.GetFocus(), i%len(fm.order))
        }
    }
}

func TestFormModalEscCancelsAndCtrlEnterSubmits(t *testing.T) {
    app := newUXTestApp()
    fm := NewFormModal(app, "Test")
    fm.AddInput("Title", "")
    var canceled, submitted bool
    fm.SetOnCancel(func() { canceled = true })
    fm.SetOnSubmit(func() { submitted = true })
    fm.Show("form_test")

    capture := fm.frame.GetInputCapture()
    capture(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
    if !canceled {
        t.Fatal("Esc did not call onCancel")
    }
    capture(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModCtrl))
    if !submitted {
        t.Fatal("Ctrl+Enter did not call onSubmit")
    }
}

func TestFormModalHeightFitsContentAndClampsToScreen(t *testing.T) {
    app := newUXTestApp()
    fm := NewFormModal(app, "Test")
    fm.AddInput("Title", "")      // 4
    fm.AddTextArea("Body", "", 10) // 13, flexible (min 6)
    fm.AddButtons(FormButton{Label: "OK"})

    pad := app.density.ModalPadding
    chrome := 1 + 1 + 1 + 2 + pad.Top + pad.Bottom // blank + buttons + hint + border + padding
    if got, want := fm.contentHeight(100), 4+13+chrome; got != want {
        t.Fatalf("unclamped height = %d, want %d", got, want)
    }
    // Screen of 20 rows → clamp to 16; textarea absorbs the loss.
    if got := fm.contentHeight(20); got != 16 {
        t.Fatalf("clamped height = %d, want 16", got)
    }
}
```

`contentHeight(screenH int) int` is the pure sizing function `Show` uses; exporting it package-internally keeps the math testable without a screen.

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./internal/tui/ -run TestFormModal`
Expected: FAIL — `NewFormModal` undefined.

- [ ] **Step 3: Implement `form_modal.go`**

Write the builder per the structure and layout rules above. Order of implementation inside the file: types, `NewFormModal`, `AddInput`/`AddTextArea` (both delegate to an internal `addFramedRow(label string, editor tview.Primitive, editorRows int, flexible bool) *tview.Flex`), `AddButtons`, `SetHint`/`SetOnCancel`/`SetOnSubmit`/`SetMaxWidth`, sizing (`contentHeight`, `effectiveMaxWidth`, `layout()` called from `Show`), focus (`focusIndex`, `focusNext`/`focusPrev`, `ensureVisible`), keys (`frame.SetInputCapture`), `Show`/`Hide`/`Root`/`HandleKey`. `HandleKey` mirrors Esc/submit for the app-level modal dispatcher: Esc → `onCancel` (dropdown check first), Ctrl/Cmd+Enter → `onSubmit`, else pass through.

- [ ] **Step 4: Run tests, verify they pass**

Run: `go test ./internal/tui/ -run TestFormModal`
Expected: PASS. Also run the full package: `go test ./internal/tui/`.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/form_modal.go internal/tui/form_modal_test.go
git commit -m "feat: FormModal builder for Linear-style form modals"
```

---

### Task 2: FormModal pickers and static rows

**Files:**
- Modify: `internal/tui/form_modal.go`
- Test: `internal/tui/form_modal_test.go`

**Interfaces (produced):**

```go
// AddPicker appends a dropdown. Consecutive AddPicker calls (no other Add*
// between them) share one two-row unit: caps labels on the first row, the
// dropdowns (value + ▾) on the second, equal widths.
func (fm *FormModal) AddPicker(label string, options []string, selected int, onChange func(text string, index int)) *tview.DropDown

// AddStatic appends a one-row read-only line (e.g. "Parent: ZEN-101 …"),
// SecondaryText, not focusable.
func (fm *FormModal) AddStatic(text string) *tview.TextView
```

Picker styling: `SetListStyles(unselected: theme.Foreground on theme.ModalBackground(), selected: theme.InverseTextColor() on theme.Accent)`, field background `theme.ModalBackground()`, `theme.InputBg` while focused (recolor in focus/blur funcs), current option text `theme.Foreground`, `SetFieldWidth(0)` — width comes from the row split, never the option text.

- [ ] **Step 1: Write failing tests**

```go
func TestFormModalConsecutivePickersShareARow(t *testing.T) {
    app := newUXTestApp()
    fm := NewFormModal(app, "Test")
    fm.AddInput("Title", "")
    a := fm.AddPicker("Assignee", []string{"Unassigned"}, 0, nil)
    c := fm.AddPicker("Cycle", []string{"No cycle"}, 0, nil)
    p := fm.AddPicker("Priority", []string{"Normal"}, 0, nil)
    if len(fm.rows) != 2 {
        t.Fatalf("rows = %d, want 2 (input row + one shared picker row)", len(fm.rows))
    }
    if h := fm.rows[1].height; h != 2 {
        t.Fatalf("picker row height = %d, want 2", h)
    }
    for i, dd := range []tview.Primitive{a, c, p} {
        if fm.order[1+i] != dd {
            t.Fatalf("tab order position %d is not picker %d", 1+i, i)
        }
    }
}

func TestFormModalEscClosesOpenDropdownBeforeCanceling(t *testing.T) {
    app := newUXTestApp()
    fm := NewFormModal(app, "Test")
    dd := fm.AddPicker("Priority", []string{"Normal", "High"}, 0, nil)
    var canceled bool
    fm.SetOnCancel(func() { canceled = true })
    fm.Show("form_test")

    // Open the dropdown by sending Enter through its handler, then Esc.
    handler := dd.InputHandler()
    handler(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) { app.app.SetFocus(p) })
    if !dd.IsOpen() {
        t.Fatal("dropdown did not open")
    }
    fm.frame.GetInputCapture()(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
    if canceled {
        t.Fatal("Esc canceled the modal while a dropdown was open")
    }
    if dd.IsOpen() {
        t.Fatal("Esc did not close the open dropdown")
    }
}
```

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./internal/tui/ -run TestFormModal`
Expected: FAIL — `AddPicker` undefined.

- [ ] **Step 3: Implement AddPicker, AddStatic, and picker-row packing**

Track `lastRowIsPickers bool`; `AddPicker` appends into the open picker row (rebuilding its two inner Flexes with equal `proportion: 1` columns) or starts a new one. Any other `Add*` closes it. Wire each dropdown into `fm.order` and the Esc loop from Task 1.

- [ ] **Step 4: Run tests, verify they pass**

Run: `go test ./internal/tui/ -run TestFormModal`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/form_modal.go internal/tui/form_modal_test.go
git commit -m "feat: FormModal pickers and static rows"
```

---

### Task 3: Convert the simple text modals

**Files:**
- Modify: `internal/tui/edit_title_modal.go`, `internal/tui/edit_description_modal.go`, `internal/tui/create_comment_modal.go`, `internal/tui/text_input_modal.go`
- Test: existing tests in `internal/tui/commands_test.go`, `internal/tui/ux_remediation_test.go` (update internals references), plus the files' own tests if any

**Interfaces:**
- Consumes: Task 1 builder API.
- Produces: unchanged public APIs — `EditTitleModal.Show(issueID, currentTitle string, onUpdate func(issueID, title string))`, `EditDescriptionModal.Show(issueID, currentDescription string, onUpdate func(issueID, description string))`, `CreateCommentModal.Show(...)` (keep its current signature), `TextInputModal.Show(title, label, initial string, onSubmit func(string))`, and each type's `Hide`/`HandleKey`/`GetModal`.

Conversion pattern — `edit_description_modal.go` becomes:

```go
type EditDescriptionModal struct {
    app       *App
    fm        *FormModal
    bodyField *tview.TextArea
    issueID   string
    onUpdate  func(issueID, description string)
}

func NewEditDescriptionModal(app *App) *EditDescriptionModal {
    edm := &EditDescriptionModal{app: app}
    edm.fm = NewFormModal(app, "Edit Description")
    edm.bodyField = edm.fm.AddTextArea("Description", "", 10)
    submit := func() {
        description := edm.bodyField.GetText()
        edm.Hide()
        if edm.onUpdate != nil && edm.issueID != "" {
            edm.onUpdate(edm.issueID, description)
        }
    }
    edm.fm.AddButtons(
        FormButton{Label: "Update", OnPress: submit},
        FormButton{Label: "Cancel", OnPress: edm.Hide},
    )
    edm.fm.SetOnSubmit(submit)
    edm.fm.SetOnCancel(edm.Hide)
    edm.fm.SetHint("Esc cancel · ⌃⏎ submit")
    return edm
}

func (edm *EditDescriptionModal) Show(issueID, currentDescription string, onUpdate func(issueID, description string)) {
    edm.issueID = issueID
    edm.onUpdate = onUpdate
    edm.bodyField.SetText(currentDescription, true)
    edm.fm.Show("edit_description")
}

func (edm *EditDescriptionModal) Hide()                       { edm.fm.Hide("edit_description") }
func (edm *EditDescriptionModal) HandleKey(e *tcell.EventKey) *tcell.EventKey { return edm.fm.HandleKey(e) }
func (edm *EditDescriptionModal) GetModal() *tview.Flex       { return edm.fm.Root() }
```

Apply the same shape to the others:

- **Edit title**: `AddInput("Title", "")`; buttons Update/Cancel; hint `"Esc cancel · ⏎ next · ⌃⏎ submit"`. Empty title still refuses to submit (keep the `title != ""` guard from the old button callback).
- **Create comment**: `AddTextArea("Comment", "", 5)`; buttons Comment/Cancel; keep its current submit guard behavior exactly (read the old file's button callback before rewriting).
- **Text input**: single `AddInput` with the label passed at `Show` time — recreate the field label per Show via `input.SetLabel("")` (no inline label; the caps label row is set through a builder accessor on the row's TextView, store it on the struct). Keep Enter-submits: after Task 1 the builder's DoneFunc moves focus next, so `Show` overrides `SetDoneFunc` with the current submit-on-Enter behavior verbatim. No buttons; hint `"⏎ save · Esc cancel"`.

Update tests that reach into removed internals: `TestEditDescriptionModalShowResetsFocusToTextArea` uses `modal.form.SetFocus(1)` — replace with focusing the last item via `app.app.SetFocus(modal.fm.order[len(modal.fm.order)-1])` then `Show`, asserting `app.app.GetFocus() == modal.bodyField` (semantics identical: reopen must focus the textarea).

- [ ] **Step 1: Convert `edit_title_modal.go`, run `go test ./internal/tui/`** — fix compile errors in tests that referenced its internals. Expected: PASS.
- [ ] **Step 2: Convert `edit_description_modal.go` (code above), run `go test ./internal/tui/ -run TestEditDescription`** — update the two commands_test.go tests' internals references. Expected: PASS.
- [ ] **Step 3: Convert `create_comment_modal.go`, run `go test ./internal/tui/`.** Expected: PASS.
- [ ] **Step 4: Convert `text_input_modal.go`, run `go test ./internal/tui/`.** Expected: PASS.
- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "refactor: text modals on the FormModal builder"
```

---

### Task 4: Convert the create-issue modal

**Files:**
- Modify: `internal/tui/create_issue_modal.go`
- Test: `internal/tui/ux_remediation_test.go` (`TestCreateIssueModalShowWithOptionsResetsFocusAndShowsParentContext` — update internals references, keep semantics)

**Interfaces:**
- Consumes: Tasks 1–2 (`AddInput`, `AddTextArea`, `AddPicker`, `AddStatic`, `AddButtons`).
- Produces: unchanged `Show`, `ShowWithOptions`, `Hide`, `HandleKey`, `GetModal`.

Field mapping (order): static parent line (only text set when sub-issue; keep the `parentView` TextView returned by `AddStatic` on the struct and set `"Parent: %s - %s"` or `""` in `ShowWithOptions`), `AddInput("Title", "")`, `AddTextArea("Description", "", 4)`, then three consecutive pickers `Assignee` / `Cycle` / `Priority` sharing one row. Buttons Create/Cancel; `SetOnSubmit` mirrors Create. Hint: `"Esc cancel · Tab next · ⏎ open dropdown · ⌃⏎ create"`.

Keep verbatim: the async `loadUsers`/`loadCycles` flow (`SetOptions([]string{"Loading..."}, nil)` at Show, `populateAssigneeDropdown`/`populateCycleDropdown` unchanged — they operate on the same `*tview.DropDown`s the builder returns), the priority default (index 3 = Normal), the reset logic in `ShowWithOptions`, and the title-required guard in Create. Delete `closeOpenDropdown` — the builder's Esc handling replaces it (verify by running the modal's Esc path test if present).

The old duplicate heading (`headerView` "Create New Issue"/"Create Sub-Issue") is gone; the border title is now set per show: add `fm.SetTitle(title string)` to the builder (one-liner updating the frame's title) and call it with `" New Issue "` / `" New Sub-Issue "`.

- [ ] **Step 1: Convert, adding `FormModal.SetTitle`**
- [ ] **Step 2: Run `go test ./internal/tui/ -run TestCreateIssue` and fix the internals references in the ux test.** Expected: PASS with unchanged assertions about focus reset and parent context.
- [ ] **Step 3: Run the full package: `go test ./internal/tui/`.** Expected: PASS.
- [ ] **Step 4: Commit**

```bash
git add internal/tui/
git commit -m "refactor: create-issue modal on the FormModal builder"
```

---

### Task 5: Convert the agent modals

**Files:**
- Modify: `internal/tui/agent_prompt_modal.go`, `internal/tui/agent_prompt_templates_modal.go`
- Test: `internal/tui/agent_command_test.go` (update internals references only)

**Interfaces:**
- Consumes: Tasks 1–2.
- Produces: unchanged public APIs of both modals.

**Agent prompt** (`" Ask Agent "`): `AddInput("Workspace", "")` (keep the struct's `workspaceField`), optional `AddPicker("Template", labels, 0, onChange)` only when `len(app.agentPromptTemplates) > 0` (keep `applyTemplatePrompt(index)` as the onChange), `AddTextArea("Prompt", "", 5)`. Buttons Run/Cancel; `SetOnSubmit` = Run. Hint: `"Esc cancel · ⌃⏎ run · template fills prompt · blank workspace uses CWD"` (the old help line's context notes move to the hint, shortened; includes-title/description/comments note drops — it described payload, not keys, and the modal stays honest without it).

**Prompt templates** (`" Edit Agent Prompts "`): keeps its two-pane body — the template `tview.List` on the left is untouched. The right column becomes builder rows: `AddInput("Name", "")`, `AddTextArea("Prompt", "", 8)`, `AddButtons(Add, Delete, Save, Cancel)` wired to the existing `addTemplate`/`deleteSelected`/`saveTemplates`/`Hide`. Because the builder owns a full modal shell but this modal composes list+form side by side, add one builder accessor: `fm.ContentBody() *tview.Flex` returning the rows+buttons column *without* the centering root, and let this modal keep its own outer bordered flex that places `pm.list` beside `fm.ContentBody()`. The builder still provides focus order and keys for its own widgets; the modal's existing list-focus handling stays. Hint: keep the current `"a: add | d: delete | Ctrl+S: save | Esc: cancel"` content in the new dim style.

- [ ] **Step 1: Convert `agent_prompt_modal.go`, run `go test ./internal/tui/ -run TestAgent`.** Expected: PASS after updating internals references.
- [ ] **Step 2: Convert `agent_prompt_templates_modal.go` (adding `ContentBody`), run `go test ./internal/tui/`.** Expected: PASS.
- [ ] **Step 3: Commit**

```bash
git add internal/tui/
git commit -m "refactor: agent modals on the FormModal builder"
```

---

### Task 6: Confirmation modal button treatment

**Files:**
- Modify: `internal/tui/confirmation_modal.go`

**Interfaces:** unchanged; stays `tview.Modal`.

Replace the always-accent button colors with the shared treatment:

```go
cm.modal.SetButtonStyle(tcell.StyleDefault.
    Background(cm.app.theme.ModalBackground()).
    Foreground(cm.app.theme.SecondaryText))
cm.modal.SetButtonActivatedStyle(tcell.StyleDefault.
    Background(cm.app.theme.Accent).
    Foreground(cm.app.theme.InverseTextColor()))
```

(Removing `SetButtonBackgroundColor`/`SetButtonTextColor`.) Border color moves from `Accent` to `theme.BorderFocus` for consistency with the builder shell.

- [ ] **Step 1: Apply, run `go test ./internal/tui/`.** Expected: PASS.
- [ ] **Step 2: Commit**

```bash
git add internal/tui/confirmation_modal.go
git commit -m "refactor: confirmation modal adopts focus-only accent buttons"
```

---

### Task 7: Convert the settings modal — with the config-stripping regression test first

**Files:**
- Modify: `internal/tui/settings_modal.go`
- Test: `internal/tui/settings_modal_test.go` (create)

**Interfaces:**
- Consumes: Tasks 1–2, `FormModal.SetMaxWidth(110)`.
- Produces: unchanged `Show`, `Hide`, `HandleKey`, `saveSettings`, `settingsFromForm`.

This is the trap task: `settingsFromForm` rebuilds the config file from controls. The regression test is written BEFORE the conversion and must pass on both the old and new form.

- [ ] **Step 1: Write the round-trip test against the CURRENT modal, verify it passes**

```go
func TestSettingsFormRoundTripPreservesConfig(t *testing.T) {
    app := newUXTestApp()
    app.config.GroupBy = "status"
    app.config.SubgroupBy = "project"
    app.config.Columns = []string{"priority", "id", "title"}
    app.config.Keybindings = map[string]string{"switch_workspace": "w"}
    app.config.Workspaces = []config.Workspace{{Name: "Zenterm", APIKeyEnv: "LINEAR_API_KEY_FAKE"}}
    app.config.DefaultWorkspace = "Zenterm"

    sm := app.settingsModal
    sm.Show()
    settings, err := sm.settingsFromForm()
    if err != nil {
        t.Fatalf("settingsFromForm: %v", err)
    }
    if settings.GroupBy != "status" || settings.SubgroupBy != "project" {
        t.Fatalf("grouping stripped: %+v", settings)
    }
    if len(settings.Columns) != 3 || len(settings.Workspaces) != 1 ||
        settings.DefaultWorkspace != "Zenterm" || settings.Keybindings["switch_workspace"] != "w" {
        t.Fatalf("config stripped on save: %+v", settings)
    }
}
```

(Adjust field names to `config.Workspace`'s actual struct tags by reading `internal/config/config.go` first; use a fake env var name, never a real key.)

Run: `go test ./internal/tui/ -run TestSettingsFormRoundTrip`
Expected: PASS on the unconverted modal. Commit this alone:

```bash
git add internal/tui/settings_modal_test.go
git commit -m "test: settings save round-trip guard"
```

- [ ] **Step 2: Convert the modal**

`NewFormModal(app, "Settings")`, `SetMaxWidth(110)`. Fields in the current order, all struct pointers kept: inputs for endpoint / timeout / page size / cache TTL / search debounce / log file, pickers for log level / theme / density (theme+density keep their label↔value parallel slices and `current*Value` helpers), a checkbox — add `fm.AddCheckbox(label string, checked bool) *tview.Checkbox` to the builder (1-row unit, same caps-label-inline treatment as pickers), pickers for agent provider / sandbox / model (keep `setAgentModelOptionsForProvider` wiring), inputs for agent workspace / default team / default project. Buttons Save/Cancel; `SetOnSubmit` = save. The `settingsModalHeight`/`settingsFormHeight` helpers and the `modalBody` machinery die — the builder sizes and scrolls (this is the modal that exercises row-window scrolling; the height clamp test from Task 1 covers the math).

`settingsFromForm` body is UNCHANGED — it reads the same field pointers.

- [ ] **Step 3: Run the round-trip test and full package**

Run: `go test ./internal/tui/`
Expected: PASS, including `TestSettingsFormRoundTripPreservesConfig` against the converted form.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/
git commit -m "refactor: settings modal on the FormModal builder"
```

---

### Task 8: Full verification and install

- [ ] **Step 1: `make all`** — run directly, read the real exit status. Expected: lint, tests (race), build all green.
- [ ] **Step 2: CI-parity lint** — `GOTOOLCHAIN=go1.24.4 <scratchpad>/bin/golangci-lint run ./...`. Expected: 0 issues.
- [ ] **Step 3: Rebuild the installed binary** — `go build -o ~/.local/bin/linear-tui ./cmd/linear-tui`.
- [ ] **Step 4: Manual pass (Drew, transparent terminal)** — open every modal on Rose Pine Moon: edit description, edit title, new issue (top-level and sub-issue), comment, agent prompt, prompt templates, a confirmation, text input (e.g. set due date), settings. Check: no overflow past borders, all fields visible (Priority!), one accent element max, reopen-after-submit types into the first field, settings save diffs cleanly in the dotfiles repo.
- [ ] **Step 5: Commit anything the manual pass shakes out; otherwise done.**
