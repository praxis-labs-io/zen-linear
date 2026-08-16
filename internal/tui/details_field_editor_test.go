package tui

import (
	"context"
	"strings"
	"testing"
	"time"

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

	// Room for a write that should not have happened, so an extra one lands
	// where the test can see it instead of blocking its goroutine.
	writes := make(chan linearapi.UpdateIssueInput, 4)
	app.updateIssueFunc = func(_ context.Context, input linearapi.UpdateIssueInput) (linearapi.Issue, error) {
		writes <- input
		return linearapi.Issue{ID: input.ID}, nil
	}
	pending := make(chan func(), 64)
	app.queueUpdateDraw = func(f func()) { pending <- f }
	return app, writes, pending
}

// openEditor presses Enter and draws twice: the first draw is what gives the
// queued caret nudge a width, and the nudge is spent between them.
func openEditor(t *testing.T, app *App, pending <-chan func()) []string {
	t.Helper()
	pressFieldKey(app, tcell.KeyEnter)
	if app.detailsEdit.editing == "" {
		t.Fatal("Enter opened no editor")
	}
	drawDetails(t, app, 90)
	runQueuedUpdate(t, pending)
	return drawDetails(t, app, 90)
}

// typeInto replaces what the box holds. The widget owns its text; only the two
// keys the mode intercepts go through the handler.
func typeInto(app *App, text string) {
	app.detailsFieldInput.SetText(text)
}

// onlyWriteIs sends an edit that must land and returns it, having proven the
// attempt before it sent nothing: this one arrived first, and none followed.
func onlyWriteIs(t *testing.T, app *App, writes <-chan linearapi.UpdateIssueInput, pending <-chan func(), text string) linearapi.UpdateIssueInput {
	t.Helper()
	if app.detailsEdit.editing == "" {
		openEditor(t, app, pending)
	}
	typeInto(app, text)
	pressFieldKey(app, tcell.KeyEnter)
	input := awaitWrite(t, writes)
	// The skipped write's goroutine was launched first, so if it existed at all
	// it has had longer than this to reach a channel with room on it.
	select {
	case extra := <-writes:
		t.Fatalf("a second write followed: %+v", extra)
	case <-time.After(200 * time.Millisecond):
	}
	return input
}

func TestEnterOpensABoxHoldingTheTitle(t *testing.T) {
	app, _, pending := editorFixture(t, issueFieldTitle)

	openEditor(t, app, pending)

	if got := app.detailsFieldInput.GetText(); got != "M3: comment infrastructure" {
		t.Errorf("box holds %q, want the title it opened on", got)
	}
	slot, ok := app.fieldEditorSlot()
	if !ok {
		t.Fatal("the page carries no slot for the box")
	}
	if slot.height != 1 || slot.width <= 0 {
		t.Errorf("slot = %+v, want one row with a width to type in", slot)
	}
}

func TestTheBoxHangsOffTheFieldsValueColumn(t *testing.T) {
	app, _, pending := editorFixture(t, issueFieldDueDate)

	openEditor(t, app, pending)

	slot, ok := app.fieldEditorSlot()
	if !ok {
		t.Fatal("the page carries no slot for the box")
	}
	// The gutter and the cursor's two cells: the box is the value, in place.
	want := detailsLabelGutter + detailsCursorGutter
	if slot.column != want {
		t.Errorf("slot column = %d, want %d, the value's own column", slot.column, want)
	}
	span := app.detailsFieldSpans[app.fieldSpanIndex(issueFieldDueDate)]
	if slot.row != span.row {
		t.Errorf("slot row = %d, want %d, the field's own row", slot.row, span.row)
	}
}

// The row prints its value under the box otherwise, and a value longer than the
// box shows its tail past the end of the field.
func TestTheRowBeingEditedKeepsItsLabelAndDropsItsValue(t *testing.T) {
	app, _, pending := editorFixture(t, issueFieldDueDate)

	openEditor(t, app, pending)

	// The page text under the widget, not the screen: the box paints the value
	// back on top. A value longer than the box would show its tail past it.
	page := strings.Split(app.detailsPageView.GetText(false), "\n")
	row := findLine(t, page, "Due date:")
	if strings.Contains(row, "2026-08-20") {
		t.Errorf("page row = %q, want only the label: the box owns the value", row)
	}
}

func TestTheDateBoxOpensOnWhatTheIssueHolds(t *testing.T) {
	app, _, pending := editorFixture(t, issueFieldDueDate)

	openEditor(t, app, pending)

	if got := app.detailsFieldInput.GetText(); got != "2026-08-20" {
		t.Errorf("box holds %q, want the due date", got)
	}
}

func TestTheEstimateBoxOpensEmptyRatherThanOnADash(t *testing.T) {
	app, _, pending := editorFixture(t, issueFieldEstimate)

	// The read row prints "-" for no estimate, which typed back in is not a
	// number. The box has to open empty instead.
	if got := fieldEditorText(issueFieldEstimate, linearapi.Issue{}); got != "" {
		t.Errorf("box would hold %q on an issue with no estimate, want it empty", got)
	}
	openEditor(t, app, pending)
	if got := app.detailsFieldInput.GetText(); got != "3" {
		t.Errorf("box holds %q, want the estimate", got)
	}
}

func TestSavingATitleWritesThatFieldAlone(t *testing.T) {
	app, writes, pending := editorFixture(t, issueFieldTitle)
	openEditor(t, app, pending)

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
	app, writes, pending := editorFixture(t, issueFieldDueDate)
	openEditor(t, app, pending)

	pressFieldKey(app, tcell.KeyEnter)

	if app.detailsEdit.editing != "" {
		t.Fatal("Enter left the box open")
	}
	if input := onlyWriteIs(t, app, writes, pending, "2026-09-01"); input.DueDate == nil || *input.DueDate != "2026-09-01" {
		t.Fatalf("first write = %+v, want nothing sent for a value nobody changed", input)
	}
}

func TestEmptyingTheDateClearsIt(t *testing.T) {
	app, writes, pending := editorFixture(t, issueFieldDueDate)
	openEditor(t, app, pending)

	typeInto(app, "")
	pressFieldKey(app, tcell.KeyEnter)

	input := awaitWrite(t, writes)
	if input.DueDate == nil || *input.DueDate != "" {
		t.Fatalf("write = %+v, want the empty string Linear reads as a null date", input)
	}
}

func TestEmptyingTheEstimateClearsItWithoutSendingAZero(t *testing.T) {
	app, writes, pending := editorFixture(t, issueFieldEstimate)
	openEditor(t, app, pending)

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
	app, writes, pending := editorFixture(t, issueFieldTitle)
	openEditor(t, app, pending)

	typeInto(app, "   ")
	pressFieldKey(app, tcell.KeyEnter)

	if app.detailsEdit.editing != issueFieldTitle {
		t.Fatal("the box closed on a refusal, losing what was typed")
	}
	if got := app.detailsFieldInput.GetText(); got != "   " {
		t.Errorf("box holds %q, want what the reader typed", got)
	}
	findLine(t, drawDetails(t, app, 90), "title is required")

	if input := onlyWriteIs(t, app, writes, pending, "A real title"); input.Title == nil || *input.Title != "A real title" {
		t.Fatalf("first write = %+v, want the empty title refused rather than sent", input)
	}
}

func TestAMalformedDateIsRefusedUnderTheBox(t *testing.T) {
	app, writes, pending := editorFixture(t, issueFieldDueDate)
	openEditor(t, app, pending)

	typeInto(app, "next tuesday")
	pressFieldKey(app, tcell.KeyEnter)

	if app.detailsEdit.editing != issueFieldDueDate {
		t.Fatal("the box closed on a refusal")
	}
	if got := app.detailsFieldInput.GetText(); got != "next tuesday" {
		t.Errorf("box holds %q, want what the reader typed", got)
	}
	findLine(t, drawDetails(t, app, 90), "date must be YYYY-MM-DD")

	if input := onlyWriteIs(t, app, writes, pending, "2026-09-01"); input.DueDate == nil || *input.DueDate != "2026-09-01" {
		t.Fatalf("first write = %+v, want the bad date stopped here rather than sent", input)
	}
}

func TestEscapeDropsTheEditAndLeavesTheRow(t *testing.T) {
	app, writes, pending := editorFixture(t, issueFieldTitle)
	openEditor(t, app, pending)

	typeInto(app, "Not this one")
	pressFieldKey(app, tcell.KeyEscape)

	if app.detailsEdit.editing != "" {
		t.Fatal("Escape left the box open")
	}
	if !app.detailsEdit.on {
		t.Error("Escape left the whole mode, want it to close the box only")
	}
	findLine(t, drawDetails(t, app, 90), "M3: comment infrastructure")

	if input := onlyWriteIs(t, app, writes, pending, "A later title"); input.Title == nil || *input.Title != "A later title" {
		t.Fatalf("first write = %+v, want Escape to have written nothing", input)
	}
}

func TestALetterInTheBoxTypesRatherThanQuits(t *testing.T) {
	app, _, pending := editorFixture(t, issueFieldTitle)
	openEditor(t, app, pending)

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
	app, _, pending := editorFixture(t, issueFieldTitle)
	openEditor(t, app, pending)

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
	app, _, pending := editorFixture(t, issueFieldEstimate)
	openEditor(t, app, pending)
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
	// Drawn in the row, not just held in the widget.
	if row := findLine(t, drawDetails(t, app, 90), "Estimate:"); !strings.Contains(row, "8") {
		t.Errorf("estimate row = %q, want the digit drawn in it", row)
	}
}

func TestTheHintNamesWhatEnterOpens(t *testing.T) {
	app, _, pending := editorFixture(t, issueFieldState)

	if text := statusText(app); !strings.Contains(text, "⏎ open") {
		t.Fatalf("status bar = %q, want Enter named on a field with a list", text)
	}

	cursorTo(t, app, issueFieldDueDate)
	if text := statusText(app); !strings.Contains(text, "⏎ edit") {
		t.Fatalf("status bar = %q, want Enter named on a field with a box", text)
	}

	openEditor(t, app, pending)
	text := statusText(app)
	if !strings.Contains(text, "⏎ save") || !strings.Contains(text, "Esc cancel") {
		t.Errorf("status bar = %q, want the two keys the box does not get", text)
	}
	if !strings.Contains(text, "Editing due date") {
		t.Errorf("status bar = %q, want the field named", text)
	}
}

func TestTheIssueChangingDropsTheBoxAndTheKeyboard(t *testing.T) {
	app, _, pending := editorFixture(t, issueFieldTitle)
	openEditor(t, app, pending)

	moveSelection(app)
	app.updateDetailsView()

	if app.detailsEdit.editing != "" || app.detailsEdit.on {
		t.Error("the box outlived the issue it was opened on")
	}
	if app.detailsFocus == detailsFocusField {
		t.Error("the keyboard is still aimed at a box that is no longer drawn")
	}
	if _, ok := app.fieldEditorSlot(); ok {
		t.Error("the page still carries a slot for the box")
	}
	// The flag alone is half of it. The next event is what has to spend it.
	pressField(app, 'j')
	if app.app.GetFocus() == app.detailsFieldInput {
		t.Error("a key later the keyboard is still in the box")
	}
}

// Entering the mode reaches enterDetailsFocus on the cards, so the guard above
// cannot be unconditional.
func TestEnteringEditModeSurvivesItsOwnFocusCallback(t *testing.T) {
	app := newDetailsTestApp(t)
	seedChooserOptions(app)
	app.updateDetailsView()

	app.enterDetailsEdit()

	if !app.detailsEdit.on {
		t.Fatal("entering edit mode closed it again on the way in")
	}
}

func TestAKeyBringsABoxScrolledOffTheTopBack(t *testing.T) {
	app, _, pending := editorFixture(t, issueFieldTitle)
	openEditor(t, app, pending)

	// What the wheel does: the page scrolls under a box that keeps the keyboard.
	app.detailsPageView.ScrollTo(app.detailsEditorSpan.end+20, 0)
	pressField(app, 'x')

	if top, _ := app.detailsPageView.GetScrollOffset(); top > app.detailsEditorSpan.start {
		t.Fatalf("scrolled to row %d with the box at %d, want the key to bring it back", top, app.detailsEditorSpan.start)
	}
}

// The chooser dims the marker because the keyboard moved into a list below. A
// box is in the row, so it stays lit and says the row is being written in.
func TestTheMarkerSaysTheRowIsBeingWrittenIn(t *testing.T) {
	app, _, pending := editorFixture(t, issueFieldTitle)

	page := strings.Split(app.detailsPageView.GetText(false), "\n")
	if marker := findLine(t, page, "❯"); !strings.HasPrefix(marker, app.themeTags.Accent+"❯") {
		t.Fatalf("read-mode row = %q, want the pointing cursor", marker)
	}

	openEditor(t, app, pending)

	page = strings.Split(app.detailsPageView.GetText(false), "\n")
	marker := findLine(t, page, "▌")
	if !strings.HasPrefix(marker, app.themeTags.Accent+"▌") {
		t.Errorf("write-mode row = %q, want the bar, lit: the caret is in this row", marker)
	}
	if strings.Contains(strings.Join(page, "\n"), "❯") {
		t.Error("both markers on the page at once")
	}
}

func TestClickingAWritingBoxClosesTheEditor(t *testing.T) {
	app, _, pending := editorFixture(t, issueFieldTitle)
	openEditor(t, app, pending)

	// What a click into the compose box does: the widget focuses itself and its
	// callback records the stop.
	app.enterDetailsFocus(detailsFocusText)

	if app.detailsEdit.on || app.detailsEdit.editing != "" {
		t.Error("edit mode survived a click into a writing box")
	}
}

// The value column can fall past a pane this narrow, which would put the box
// past the drawn line: invisible, and holding the keyboard.
func TestANarrowPaneStillLeavesSomethingToTypeIn(t *testing.T) {
	app, _, _ := editorFixture(t, issueFieldDueDate)
	pressFieldKey(app, tcell.KeyEnter)
	drawDetails(t, app, fieldEditorMinWidth+6)

	slot, ok := app.fieldEditorSlot()
	if !ok {
		t.Fatal("the page dropped the box on a narrow pane")
	}
	if slot.width < fieldEditorMinWidth || slot.column+slot.width > app.detailsFittedWidth {
		t.Errorf("slot = %+v at measure %d, want it inside the drawn line", slot, app.detailsFittedWidth)
	}
}

// Editing an issue should look like the issue. A filled field reads as a form,
// and the title stops being bold the moment you type in it.
func TestTheBoxDrawsTheValueTheWayTheRowReadsIt(t *testing.T) {
	app, _, pending := editorFixture(t, issueFieldTitle)
	openEditor(t, app, pending)

	fg, bg, attrs := app.detailsFieldInput.GetFieldStyle().Decompose()
	if fg != app.theme.Foreground || bg != app.theme.Background {
		t.Errorf("field = %v on %v, want the row's %v on %v", fg, bg, app.theme.Foreground, app.theme.Background)
	}
	if attrs&tcell.AttrBold == 0 {
		t.Error("the title lost its weight on the way into the box")
	}

	// The box owns the keys, so the cursor cannot be walked out from under it.
	pressFieldKey(app, tcell.KeyEscape)
	cursorTo(t, app, issueFieldEstimate)
	openEditor(t, app, pending)
	if _, _, attrs := app.detailsFieldInput.GetFieldStyle().Decompose(); attrs&tcell.AttrBold != 0 {
		t.Error("the estimate came out bold, want the weight on the title alone")
	}
}

// Driven through the mouse capture rather than the focus callback: the
// shortcut proved the guard and not the path that reaches it.
func TestARealClickOnThePageBodyClosesTheEditor(t *testing.T) {
	app := newMouseTestApp(t)
	layOut(t, app, 180, 40, FocusDetails)
	app.enterDetailsEdit()
	app.openFieldEditor()
	app.app.ForceDraw()
	if app.detailsEdit.editing == "" {
		t.Fatal("no box open to click off")
	}

	left, top, width, height := app.detailsView.GetRect()
	clickAt(t, app, left+width/2, top+height-3)

	if app.detailsEdit.editing != "" || app.detailsEdit.on {
		t.Fatalf("edit mode survived the click: editing=%q on=%v", app.detailsEdit.editing, app.detailsEdit.on)
	}
	if focus := app.app.GetFocus(); focus == tview.Primitive(app.detailsFieldInput) {
		t.Error("the keyboard is still in a box that is gone")
	}
}

// A press in the box is placing the caret, not leaving.
func TestAClickInsideTheBoxKeepsIt(t *testing.T) {
	app := newMouseTestApp(t)
	layOut(t, app, 180, 40, FocusDetails)
	app.enterDetailsEdit()
	app.openFieldEditor()
	app.app.ForceDraw()

	x, y, _, _ := app.detailsFieldInput.GetRect()
	clickAt(t, app, x+1, y)

	if app.detailsEdit.editing == "" {
		t.Fatal("clicking into the box closed it")
	}
}

// A title longer than the row opened on its head with the caret past the right
// edge, so nothing on screen said where typing would land.
func TestALongValueOpensScrolledToItsCaret(t *testing.T) {
	app, _, pending := editorFixture(t, issueFieldTitle)
	app.selectedIssue.Title = "HEAD" + strings.Repeat(" middle", 18) + " TAILEND"
	app.updateDetailsView()
	drawDetails(t, app, 60)
	cursorTo(t, app, issueFieldTitle)

	row := openEditor(t, app, pending)[3]

	if !strings.Contains(row, "TAILEND") || strings.Contains(row, "HEAD") {
		t.Fatalf("title row = %q, want the end of the value on screen", row)
	}
	// Where a keystroke lands is where the caret is, and the only way to read
	// it: the caret itself is the terminal's, not the page's.
	event := pressField(app, 'X')
	app.detailsPage.InputHandler()(event, func(tview.Primitive) {})
	if got := app.detailsFieldInput.GetText(); !strings.HasSuffix(got, "TAILENDX") {
		t.Errorf("typing gave %q, want the caret on the part of the value shown", got[len(got)-20:])
	}
}

// Both freezes this feature caused were invisible to the stubbed queue: they
// need a real loop, because the loop is what the blocking call waits on.
func TestOpeningAFieldBoxKeepsTheAppAlive(t *testing.T) {
	app := newDetailsTestApp(t)
	seedChooserOptions(app)
	app.updateDetailsView()
	// The real queue, not the harness stub that runs it inline.
	app.queueUpdateDraw = nil
	app.app.SetRoot(app.detailsView, true)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init screen: %v", err)
	}
	screen.SetSize(120, 40)
	app.app.SetScreen(screen)
	app.focusedPane = FocusDetails

	go func() { _ = app.app.Run() }()
	t.Cleanup(func() { app.app.Stop() })

	// On the loop, which is where the freeze lives: work running there cannot
	// wait for the loop to run anything else.
	opened := make(chan bool, 1)
	go app.app.QueueUpdateDraw(func() {
		app.enterDetailsEdit()
		app.openFieldEditor()
		opened <- app.detailsEdit.editing != ""
	})

	select {
	case ok := <-opened:
		if !ok {
			t.Fatal("no box opened, so this proves nothing")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("the event loop never came back: opening a field box queued on the loop it runs on")
	}

	alive := make(chan struct{})
	go func() { app.app.QueueUpdateDraw(func() { close(alive) }) }()
	select {
	case <-alive:
	case <-time.After(4 * time.Second):
		t.Fatal("the event loop never came back after a field box opened")
	}
}
