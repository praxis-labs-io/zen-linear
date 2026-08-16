package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// editorFixture is chooserFixture with the typed fields filled in, since two of
// the three read as empty on the shared one.
func editorFixture(t *testing.T, field issueField) (*App, <-chan linearapi.UpdateIssueInput, <-chan func()) {
	t.Helper()

	app := newDetailsTestApp(t)
	seedChooserOptions(app)
	due, estimate := "2026-08-20", 3.0
	app.selectedIssue.DueDate = &due
	app.selectedIssue.Estimate = &estimate
	app.updateDetailsView()
	app.enterDetailsEdit()
	drawDetails(t, app, 90)
	cursorTo(t, app, field)

	writes := make(chan linearapi.UpdateIssueInput, 1)
	app.updateIssueFunc = func(_ context.Context, input linearapi.UpdateIssueInput) (linearapi.Issue, error) {
		writes <- input
		return linearapi.Issue{ID: input.ID}, nil
	}
	pending := make(chan func(), 64)
	app.queueUpdateDraw = func(f func()) { pending <- f }
	return app, writes, pending
}

// openEditor presses Enter and draws, which is what puts the box on the page
// and records where it landed.
func openEditor(t *testing.T, app *App) []string {
	t.Helper()
	pressFieldKey(app, tcell.KeyEnter)
	if app.detailsEdit.editing == "" {
		t.Fatal("Enter opened no editor")
	}
	return drawDetails(t, app, 90)
}

// typeInto replaces what the box holds. The widget owns its text; only the two
// keys the mode intercepts go through the handler.
func typeInto(app *App, text string) {
	app.detailsFieldInput.SetText(text)
}

// nextWriteIs sends an edit that must land and returns the first write on the
// channel. A skipped write is proven by this one arriving, not by an empty read.
func nextWriteIs(t *testing.T, app *App, writes <-chan linearapi.UpdateIssueInput, text string) linearapi.UpdateIssueInput {
	t.Helper()
	if app.detailsEdit.editing == "" {
		openEditor(t, app)
	}
	typeInto(app, text)
	pressFieldKey(app, tcell.KeyEnter)
	return awaitWrite(t, writes)
}

// editorSlot is the box's place on the page, and whether the render gave it one.
func editorSlot(app *App) (pageSlot, bool) {
	for _, slot := range app.detailsPage.slots {
		if slot.primitive == app.detailsFieldInput {
			return slot, true
		}
	}
	return pageSlot{}, false
}

func TestEnterOpensABoxHoldingTheTitle(t *testing.T) {
	app, _, _ := editorFixture(t, issueFieldTitle)

	openEditor(t, app)

	if got := app.detailsFieldInput.GetText(); got != "M3: comment infrastructure" {
		t.Errorf("box holds %q, want the title it opened on", got)
	}
	slot, ok := editorSlot(app)
	if !ok {
		t.Fatal("the page carries no slot for the box")
	}
	if slot.height != 1 || slot.width <= 0 {
		t.Errorf("slot = %+v, want one row with a width to type in", slot)
	}
}

func TestTheBoxHangsOffTheFieldsValueColumn(t *testing.T) {
	app, _, _ := editorFixture(t, issueFieldDueDate)

	openEditor(t, app)

	slot, ok := editorSlot(app)
	if !ok {
		t.Fatal("the page carries no slot for the box")
	}
	// The gutter, the cursor's two cells, and the card's border and pad.
	want := detailsLabelGutter + detailsCursorGutter + commentCardChrome/2
	if slot.column != want {
		t.Errorf("slot column = %d, want %d, the value column plus the card's inset", slot.column, want)
	}
}

func TestTheDateBoxOpensOnWhatTheIssueHolds(t *testing.T) {
	app, _, _ := editorFixture(t, issueFieldDueDate)

	openEditor(t, app)

	if got := app.detailsFieldInput.GetText(); got != "2026-08-20" {
		t.Errorf("box holds %q, want the due date", got)
	}
}

func TestTheEstimateBoxOpensEmptyRatherThanOnADash(t *testing.T) {
	app, _, _ := editorFixture(t, issueFieldEstimate)

	// The read row prints "-" for no estimate, which typed back in is not a
	// number. The box has to open empty instead.
	if got := fieldEditorText(issueFieldEstimate, linearapi.Issue{}); got != "" {
		t.Errorf("box would hold %q on an issue with no estimate, want it empty", got)
	}
	openEditor(t, app)
	if got := app.detailsFieldInput.GetText(); got != "3" {
		t.Errorf("box holds %q, want the estimate", got)
	}
}

func TestSavingATitleWritesThatFieldAlone(t *testing.T) {
	app, writes, pending := editorFixture(t, issueFieldTitle)
	openEditor(t, app)

	typeInto(app, "A better title")
	pressFieldKey(app, tcell.KeyEnter)

	input := awaitWrite(t, writes)
	if input.Title == nil || *input.Title != "A better title" {
		t.Fatalf("write = %+v, want the typed title", input)
	}
	if input.DueDate != nil || input.Estimate != nil || input.StateID != nil {
		t.Errorf("write = %+v, want only the title on it", input)
	}
	if app.detailsEdit.editing != "" {
		t.Error("the box is still open after a save")
	}
	runQueuedUpdate(t, pending)
	if text := statusText(app); !strings.Contains(text, "Updated title") {
		t.Errorf("status bar = %q, want the field named without its value", text)
	}
}

func TestSavingAnUnchangedValueSendsNothing(t *testing.T) {
	app, writes, _ := editorFixture(t, issueFieldDueDate)
	openEditor(t, app)

	pressFieldKey(app, tcell.KeyEnter)

	if app.detailsEdit.editing != "" {
		t.Fatal("Enter left the box open")
	}
	if input := nextWriteIs(t, app, writes, "2026-09-01"); input.DueDate == nil || *input.DueDate != "2026-09-01" {
		t.Fatalf("first write = %+v, want nothing sent for a value nobody changed", input)
	}
}

func TestEmptyingTheDateClearsIt(t *testing.T) {
	app, writes, _ := editorFixture(t, issueFieldDueDate)
	openEditor(t, app)

	typeInto(app, "")
	pressFieldKey(app, tcell.KeyEnter)

	input := awaitWrite(t, writes)
	if input.DueDate == nil || *input.DueDate != "" {
		t.Fatalf("write = %+v, want the empty string Linear reads as a null date", input)
	}
}

func TestEmptyingTheEstimateClearsItWithoutSendingAZero(t *testing.T) {
	app, writes, _ := editorFixture(t, issueFieldEstimate)
	openEditor(t, app)

	typeInto(app, "")
	pressFieldKey(app, tcell.KeyEnter)

	input := awaitWrite(t, writes)
	if !input.ClearEstimate {
		t.Fatalf("write = %+v, want ClearEstimate", input)
	}
	if input.Estimate != nil {
		t.Errorf("write carries Estimate %v, want no pointer: Linear reads a zero as an estimate", *input.Estimate)
	}
}

func TestAnEmptyTitleIsRefusedAndTheBoxKeepsTheText(t *testing.T) {
	app, writes, _ := editorFixture(t, issueFieldTitle)
	openEditor(t, app)

	typeInto(app, "   ")
	pressFieldKey(app, tcell.KeyEnter)

	if app.detailsEdit.editing != issueFieldTitle {
		t.Fatal("the box closed on a refusal, losing what was typed")
	}
	if got := app.detailsFieldInput.GetText(); got != "   " {
		t.Errorf("box holds %q, want what the reader typed", got)
	}
	findLine(t, drawDetails(t, app, 90), "title is required")

	if input := nextWriteIs(t, app, writes, "A real title"); input.Title == nil || *input.Title != "A real title" {
		t.Fatalf("first write = %+v, want the empty title refused rather than sent", input)
	}
}

func TestAMalformedDateIsRefusedUnderTheBox(t *testing.T) {
	app, writes, _ := editorFixture(t, issueFieldDueDate)
	openEditor(t, app)

	typeInto(app, "next tuesday")
	pressFieldKey(app, tcell.KeyEnter)

	if app.detailsEdit.editing != issueFieldDueDate {
		t.Fatal("the box closed on a refusal")
	}
	if got := app.detailsFieldInput.GetText(); got != "next tuesday" {
		t.Errorf("box holds %q, want what the reader typed", got)
	}
	findLine(t, drawDetails(t, app, 90), "date must be YYYY-MM-DD")

	if input := nextWriteIs(t, app, writes, "2026-09-01"); input.DueDate == nil || *input.DueDate != "2026-09-01" {
		t.Fatalf("first write = %+v, want the bad date stopped here rather than sent", input)
	}
}

func TestEscapeDropsTheEditAndLeavesTheRow(t *testing.T) {
	app, writes, _ := editorFixture(t, issueFieldTitle)
	openEditor(t, app)

	typeInto(app, "Not this one")
	pressFieldKey(app, tcell.KeyEscape)

	if app.detailsEdit.editing != "" {
		t.Fatal("Escape left the box open")
	}
	if !app.detailsEdit.on {
		t.Error("Escape left the whole mode, want it to close the box only")
	}
	findLine(t, drawDetails(t, app, 90), "M3: comment infrastructure")

	if input := nextWriteIs(t, app, writes, "A later title"); input.Title == nil || *input.Title != "A later title" {
		t.Fatalf("first write = %+v, want Escape to have written nothing", input)
	}
}

func TestALetterInTheBoxTypesRatherThanQuits(t *testing.T) {
	app, _, _ := editorFixture(t, issueFieldTitle)
	openEditor(t, app)

	// Handed back rather than swallowed is what puts it in the widget: the mode
	// around the box is default-deny and would have quit on this.
	if event := pressField(app, 'q'); event == nil {
		t.Fatal("q was swallowed, want it handed to the box")
	}
	if app.detailsEdit.editing == "" {
		t.Error("q closed the box")
	}
}

func TestTabInTheBoxMovesNoFocus(t *testing.T) {
	app, _, _ := editorFixture(t, issueFieldTitle)
	openEditor(t, app)

	if event := pressFieldKey(app, tcell.KeyTab); event != nil {
		t.Fatal("Tab was handed on, want it swallowed: the box has no second control")
	}
	if app.detailsFocus != detailsFocusField {
		t.Errorf("focus = %v, want it still in the box", app.detailsFocus)
	}
}

// The keys the mode hands back have to land somewhere. Everything else here
// fills the box directly, which would pass with the widget wired to nothing.
func TestATypedLetterReachesTheBoxAndIsDrawnInIt(t *testing.T) {
	app, _, _ := editorFixture(t, issueFieldEstimate)
	openEditor(t, app)
	typeInto(app, "")

	event := pressField(app, '8')
	if event == nil {
		t.Fatal("the digit was swallowed before it could reach the box")
	}
	handler := app.detailsPage.InputHandler()
	handler(event, func(tview.Primitive) {})

	if got := app.detailsFieldInput.GetText(); got != "8" {
		t.Fatalf("box holds %q, want the digit routed into it", got)
	}
	// Drawn over the blank row the frame left, not just held in the widget.
	findLine(t, drawDetails(t, app, 90), "│ 8")
}

func TestTheHintNamesWhatEnterOpens(t *testing.T) {
	app, _, _ := editorFixture(t, issueFieldState)

	if text := statusText(app); !strings.Contains(text, "⏎ open") {
		t.Fatalf("status bar = %q, want Enter named on a field with a list", text)
	}

	cursorTo(t, app, issueFieldDueDate)
	if text := statusText(app); !strings.Contains(text, "⏎ edit") {
		t.Fatalf("status bar = %q, want Enter named on a field with a box", text)
	}

	openEditor(t, app)
	text := statusText(app)
	if !strings.Contains(text, "⏎ save") || !strings.Contains(text, "Esc cancel") {
		t.Errorf("status bar = %q, want the two keys the box does not get", text)
	}
	if !strings.Contains(text, "Editing due date") {
		t.Errorf("status bar = %q, want the field named", text)
	}
}

func TestTheIssueChangingDropsTheBoxAndTheKeyboard(t *testing.T) {
	app, _, _ := editorFixture(t, issueFieldTitle)
	openEditor(t, app)

	moveSelection(app)
	app.updateDetailsView()

	if app.detailsEdit.editing != "" || app.detailsEdit.on {
		t.Error("the box outlived the issue it was opened on")
	}
	if app.detailsFocus == detailsFocusField {
		t.Error("the keyboard is still aimed at a box that is no longer drawn")
	}
	if _, ok := editorSlot(app); ok {
		t.Error("the page still carries a slot for the box")
	}
	// The flag alone is half of it. The next event is what has to spend it.
	pressField(app, 'j')
	if app.app.GetFocus() == app.detailsFieldInput {
		t.Error("a key later the keyboard is still in the box")
	}
}

func TestClickingAWritingBoxClosesTheEditor(t *testing.T) {
	app, _, _ := editorFixture(t, issueFieldTitle)
	openEditor(t, app)

	// What a click into the compose box does: the widget focuses itself and its
	// callback records the stop.
	app.enterDetailsFocus(detailsFocusText)

	if app.detailsEdit.on || app.detailsEdit.editing != "" {
		t.Error("edit mode survived a click into a writing box")
	}
}

func TestANarrowPaneDropsTheBoxFrameRatherThanTheBox(t *testing.T) {
	app, _, _ := editorFixture(t, issueFieldTitle)
	pressFieldKey(app, tcell.KeyEnter)
	drawDetails(t, app, commentCardMinWidth+4)

	slot, ok := editorSlot(app)
	if !ok {
		t.Fatal("the page dropped the box on a narrow pane")
	}
	if slot.width <= 0 {
		t.Errorf("slot = %+v, want something left to type in", slot)
	}
}

func TestTheBoxIsFramedInTheFocusBorder(t *testing.T) {
	app, _, _ := editorFixture(t, issueFieldTitle)
	openEditor(t, app)

	page := app.detailsPageView.GetText(false)
	for _, line := range strings.Split(page, "\n") {
		if strings.Contains(line, "╭") {
			if !strings.Contains(line, app.themeTags.BorderFocus) {
				t.Errorf("box edge = %q, want the accent that says it holds the keyboard", line)
			}
			return
		}
	}
	t.Fatalf("no framed box on the page:\n%s", stripTags(page))
}
