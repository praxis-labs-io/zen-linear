package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// descriptionFixture opens the details fixture in edit mode with the cursor on
// the description. Writes land on the channel and UI updates wait on the queue.
func descriptionFixture(t *testing.T) (*App, <-chan linearapi.UpdateIssueInput, <-chan func()) {
	t.Helper()

	app := newDetailsTestApp(t)
	seedChooserOptions(app)
	app.updateDetailsView()
	app.enterDetailsEdit()
	drawDetails(t, app, 90)
	cursorTo(t, app, issueFieldDescription)

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

// openDescription presses Enter and draws, which is what puts the box on the
// page and records where it landed.
func openDescription(t *testing.T, app *App) []string {
	t.Helper()
	pressFieldKey(app, tcell.KeyEnter)
	if app.detailsEdit.editing != issueFieldDescription {
		t.Fatalf("Enter opened %q, want the description box", app.detailsEdit.editing)
	}
	return drawDetails(t, app, 90)
}

// sendDescription is the chord, which is the only key that saves: Enter in
// prose is a newline.
func sendDescription(app *App) {
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModCtrl))
}

// descriptionSlot is the box's place on the page just rendered.
func descriptionSlot(t *testing.T, app *App) pageSlot {
	t.Helper()
	for _, slot := range app.detailsPage.slots {
		if slot.primitive == tview.Primitive(app.detailsDescArea) {
			return slot
		}
	}
	t.Fatal("the page carries no slot for the description box")
	return pageSlot{}
}

func TestTheDescriptionIsTheLastFieldTheCursorReaches(t *testing.T) {
	app := newDetailsTestApp(t)
	app.enterDetailsEdit()
	drawDetails(t, app, 90)

	spans := app.detailsFieldSpans
	if len(spans) == 0 {
		t.Fatal("no editable fields on the page")
	}
	if last := spans[len(spans)-1].field; last != issueFieldDescription {
		t.Errorf("last field is %q, want the description: it reads last on the page", last)
	}
	cursorTo(t, app, issueFieldEstimate)
	pressField(app, 'j')
	if app.detailsEdit.cursor != issueFieldDescription {
		t.Errorf("j from the estimate landed on %q, want the description", app.detailsEdit.cursor)
	}
}

func TestEnterOpensABoxHoldingTheRawMarkdown(t *testing.T) {
	app, _, _ := descriptionFixture(t)

	openDescription(t, app)

	want := app.selectedIssue.Description
	if got := app.detailsDescArea.GetText(); got != want {
		t.Errorf("box holds %q, want the description's own markdown", got)
	}
	slot := descriptionSlot(t, app)
	if slot.column != detailsCursorGutter || slot.width <= 0 || slot.height < composeRows {
		t.Errorf("slot = %+v, want it under the label with at least %d rows", slot, composeRows)
	}
}

// The marker on the label alone says a row is open. The description is a block,
// so the bar runs its height and the text clears it.
func TestTheWriteMarkerRunsTheHeightOfTheBox(t *testing.T) {
	app, _, _ := descriptionFixture(t)
	openDescription(t, app)

	slot := descriptionSlot(t, app)
	page := strings.Split(app.detailsPageView.GetText(false), "\n")
	label := app.detailsFieldSpans[app.fieldSpanIndex(issueFieldDescription)].row
	for row := label; row < slot.row+slot.height; row++ {
		if !strings.HasPrefix(page[row], app.themeTags.Accent+detailsWriteMarker) {
			t.Fatalf("row %d reads %q, want the bar down the whole block", row, page[row])
		}
	}
	if next := slot.row + slot.height; next < len(page) && strings.Contains(page[next], detailsWriteMarker) {
		t.Errorf("row %d still carries the bar, want it to stop with the box", next)
	}
}

// Opening the box must not lift the text a row, so it keeps the blank the read
// view puts under the label.
func TestTheBoxKeepsTheBlankUnderTheLabel(t *testing.T) {
	app, _, _ := descriptionFixture(t)

	label := app.detailsFieldSpans[app.fieldSpanIndex(issueFieldDescription)].row
	page := strings.Split(app.detailsPageView.GetText(false), "\n")
	if got := strings.TrimSpace(page[label+1]); got != "" {
		t.Fatalf("read row %d is %q, want the blank under the label", label+1, got)
	}

	openDescription(t, app)
	label = app.detailsFieldSpans[app.fieldSpanIndex(issueFieldDescription)].row

	if slot := descriptionSlot(t, app); slot.row != label+2 {
		t.Errorf("the box starts %d rows under the label, want 2", slot.row-label)
	}
}

// The rendered body would otherwise sit behind the box, one line per two rows.
func TestTheOpenBoxTakesTheRenderedBodysPlace(t *testing.T) {
	app, _, _ := descriptionFixture(t)

	openDescription(t, app)

	page := app.detailsPageView.GetText(false)
	if strings.Contains(page, "rather than being cut off at the border") {
		t.Error("the rendered description is still on the page under the box")
	}
	label := findLine(t, strings.Split(page, "\n"), "Description:")
	if !strings.Contains(label, "▌") {
		t.Errorf("label row = %q, want the write-mode marker on it", label)
	}
}

func TestAnIssueWithNoDescriptionOpensAnEmptyBox(t *testing.T) {
	app, _, _ := descriptionFixture(t)
	app.selectedIssue.Description = ""
	app.updateDetailsView()
	drawDetails(t, app, 90)
	cursorTo(t, app, issueFieldDescription)

	lines := openDescription(t, app)

	if got := app.detailsDescArea.GetText(); got != "" {
		t.Errorf("box holds %q, want nothing", got)
	}
	if slot := descriptionSlot(t, app); slot.height != composeRows {
		t.Errorf("slot height = %d, want %d rows to write in", slot.height, composeRows)
	}
	findLine(t, lines, "Description:")
}

// The label needs a row to mark, so it draws whether or not there is a body.
func TestAnIssueWithNoDescriptionStillDrawsTheLabel(t *testing.T) {
	app := newDetailsTestApp(t)
	app.selectedIssue.Description = ""
	app.updateDetailsView()

	lines := drawDetails(t, app, 90)

	findLine(t, lines, "Description:")
	findLine(t, lines, "No description available")
}

// The comment boxes open on their tail. A description is read before it is
// rewritten, so a long one opening there hides everything it is about.
func TestALongDescriptionOpensAtItsHead(t *testing.T) {
	app, _, _ := descriptionFixture(t)
	app.selectedIssue.Description = "HEADLINE\n\n" + strings.Repeat("filler line\n\n", 60) + "TAILEND"
	app.updateDetailsView()
	drawDetails(t, app, 90)
	cursorTo(t, app, issueFieldDescription)

	openDescription(t, app)

	if row, column, _, _ := app.detailsDescArea.GetCursor(); row != 0 || column != 0 {
		t.Errorf("caret at row %d column %d, want the head of the description", row, column)
	}
	if row, _ := app.detailsDescArea.GetOffset(); row != 0 {
		t.Errorf("box scrolled to row %d, want the first line showing", row)
	}
}

func TestTheBoxGrowsWithWhatIsTypedIntoIt(t *testing.T) {
	app, _, _ := descriptionFixture(t)
	openDescription(t, app)
	before := descriptionSlot(t, app).height

	app.detailsDescArea.SetText(strings.Repeat("a line\n", before+4), true)
	drawDetails(t, app, 90)

	if after := descriptionSlot(t, app).height; after <= before {
		t.Errorf("slot height stayed at %d after %d lines went in", after, before+4)
	}
}

func TestTheChordWritesTheDescriptionAlone(t *testing.T) {
	app, writes, _ := descriptionFixture(t)
	openDescription(t, app)

	app.detailsDescArea.SetText("Rewritten.", true)
	sendDescription(app)

	input := awaitWrite(t, writes)
	if input.Description == nil || *input.Description != "Rewritten." {
		t.Fatalf("write carried %+v, want the new description", input)
	}
	if input.Title != nil || input.StateID != nil || input.LabelIDs != nil {
		t.Errorf("write carried more than the description: %+v", input)
	}
}

func TestSendingAnUnchangedDescriptionWritesNothing(t *testing.T) {
	app, writes, _ := descriptionFixture(t)
	openDescription(t, app)

	sendDescription(app)

	if app.detailsEdit.editing != "" {
		t.Error("the box stayed open on an unchanged send, which is a close")
	}
	select {
	case input := <-writes:
		t.Fatalf("an unchanged description was written: %+v", input)
	case <-time.After(200 * time.Millisecond):
	}
}

// Linear reads the empty string as a cleared description, which is what the
// retired modal did too.
func TestEmptyingTheBoxClearsTheDescription(t *testing.T) {
	app, writes, _ := descriptionFixture(t)
	openDescription(t, app)

	app.detailsDescArea.SetText("", true)
	sendDescription(app)

	input := awaitWrite(t, writes)
	if input.Description == nil || *input.Description != "" {
		t.Fatalf("write carried %+v, want an empty description", input)
	}
}

// Prose has nowhere else to be held, so the box is what holds it until Linear
// has taken it.
func TestTheBoxHoldsTheWordsUntilLinearAnswers(t *testing.T) {
	app, writes, pending := descriptionFixture(t)
	openDescription(t, app)

	app.detailsDescArea.SetText("Rewritten.", true)
	sendDescription(app)
	awaitWrite(t, writes)

	if app.detailsEdit.editing != issueFieldDescription {
		t.Fatal("the box closed before the write answered")
	}
	runQueuedUpdate(t, pending)
	if app.detailsEdit.editing != "" {
		t.Error("the box stayed open after the write landed")
	}
	if !app.detailsEdit.on {
		t.Error("saving left edit mode, which only Escape does")
	}
}

func TestARefusedWriteLeavesTheWordsInTheBox(t *testing.T) {
	app, _, pending := descriptionFixture(t)
	app.updateIssueFunc = func(_ context.Context, _ linearapi.UpdateIssueInput) (linearapi.Issue, error) {
		return linearapi.Issue{}, errors.New("description is too long")
	}
	openDescription(t, app)

	app.detailsDescArea.SetText("Rewritten.", true)
	sendDescription(app)
	runQueuedUpdate(t, pending)

	if app.detailsEdit.editing != issueFieldDescription {
		t.Fatal("the box closed on a refused write, taking the rewrite with it")
	}
	if got := app.detailsDescArea.GetText(); got != "Rewritten." {
		t.Errorf("box holds %q, want the words that failed to send", got)
	}
}

func TestASecondSendWhileOneIsInFlightWritesNothing(t *testing.T) {
	app, writes, pending := descriptionFixture(t)
	openDescription(t, app)

	app.detailsDescArea.SetText("Rewritten.", true)
	sendDescription(app)
	awaitWrite(t, writes)
	sendDescription(app)

	select {
	case input := <-writes:
		t.Fatalf("a second write followed the first: %+v", input)
	case <-time.After(200 * time.Millisecond):
	}
	runQueuedUpdate(t, pending)
}

func TestEscapePutsTheRenderedBodyBack(t *testing.T) {
	app, writes, _ := descriptionFixture(t)
	openDescription(t, app)
	app.detailsDescArea.SetText("Half a thought", true)

	pressFieldKey(app, tcell.KeyEscape)
	lines := drawDetails(t, app, 90)

	if app.detailsEdit.editing != "" {
		t.Fatal("Escape left the box open")
	}
	if !app.detailsEdit.on {
		t.Error("Escape left edit mode as well as the box")
	}
	findLine(t, lines, "rather than being cut off at the border")
	select {
	case input := <-writes:
		t.Fatalf("Escape wrote something: %+v", input)
	case <-time.After(200 * time.Millisecond):
	}
}

// Default-allow, the inverse of the mode around it: the box is prose.
func TestTheBoxTakesLettersAndNewlines(t *testing.T) {
	app, _, _ := descriptionFixture(t)
	openDescription(t, app)

	for _, event := range []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone),
	} {
		if app.handleGlobalKey(event) == nil {
			t.Fatalf("the mode swallowed %v, want it in the box", event.Key())
		}
	}
	if app.detailsEdit.editing != issueFieldDescription {
		t.Error("typing closed the box")
	}
}

func TestTabMovesNoFocusOutOfTheBox(t *testing.T) {
	app, _, _ := descriptionFixture(t)
	openDescription(t, app)

	if event := pressFieldKey(app, tcell.KeyTab); event != nil {
		t.Error("Tab escaped the box, and tview would land it on any primitive")
	}
	if app.detailsFocus != detailsFocusDescription {
		t.Errorf("focus = %v, want it still in the box", app.detailsFocus)
	}
}

func TestTheStatusLineNamesTheChord(t *testing.T) {
	app, _, _ := descriptionFixture(t)
	app.focusedPane = FocusDetails
	openDescription(t, app)
	app.updateStatusBar()

	hints := app.statusBar.GetText(true)
	if !strings.Contains(hints, "⌃⏎") || !strings.Contains(hints, "save") {
		t.Errorf("status line = %q, want the chord that sends it", hints)
	}
	if !strings.Contains(hints, "Editing description") {
		t.Errorf("status line = %q, want it to name the field", hints)
	}
}

// The issue the box was about is gone, and the keyboard cannot stay in a widget
// this page no longer draws.
func TestChangingIssueDropsTheOpenBox(t *testing.T) {
	app, _, _ := descriptionFixture(t)
	openDescription(t, app)

	moveSelection(app)
	app.updateDetailsView()

	if app.detailsEdit.editing != "" || app.detailsEdit.on {
		t.Fatalf("the mode survived the move: editing=%q on=%v", app.detailsEdit.editing, app.detailsEdit.on)
	}
	if app.detailsFocus == detailsFocusDescription {
		t.Error("the keyboard is still aimed at a box that is gone")
	}
}

// The command keeps its id so a binding survives, and now opens the box.
func TestTheDescriptionCommandOpensTheBox(t *testing.T) {
	app := newUXTestApp(t)
	app.selectedIssue = detailsFixture()
	app.detailsHidden = false
	app.focusedPane = FocusNavigation
	app.updateDetailsView()
	drawDetails(t, app, 90)

	var cmd Command
	for _, c := range app.paletteCtrl.commands {
		if c.ID == "edit_description" {
			cmd = c
			break
		}
	}
	if cmd.ID == "" {
		t.Fatal("edit_description command not registered")
	}
	cmd.Run(app)

	if app.detailsEdit.editing != issueFieldDescription {
		t.Fatalf("the command opened %q, want the description box", app.detailsEdit.editing)
	}
	if focus := app.app.GetFocus(); focus == tview.Primitive(app.navigationTree) {
		t.Error("the keyboard is on the navigation tree, want it in the box")
	}
}

// TextView.MouseHandler holds that view's own lock while it moves focus, so a
// teardown reached from a focus callback wedges the process rather than failing.
func TestARealClickOnThePageBodyClosesTheDescriptionBox(t *testing.T) {
	app := newMouseTestApp(t)
	app.selectedIssue = detailsFixture()
	app.updateDetailsView()
	layOut(t, app, 180, 40, FocusDetails)
	app.enterDetailsEdit()
	app.detailsEdit.cursor = issueFieldDescription
	app.openFieldEditor()
	app.app.ForceDraw()
	if app.detailsEdit.editing != issueFieldDescription {
		t.Fatal("no box open to click off")
	}

	left, top, width, height := app.detailsView.GetRect()
	clickAt(t, app, left+width/2, top+height-3)

	if app.detailsEdit.editing != "" || app.detailsEdit.on {
		t.Fatalf("edit mode survived the click: editing=%q on=%v", app.detailsEdit.editing, app.detailsEdit.on)
	}
	if focus := app.app.GetFocus(); focus == tview.Primitive(app.detailsDescArea) {
		t.Error("the keyboard is still in a box that is gone")
	}
}

// A press in the box is placing the caret, not leaving.
func TestAClickInsideTheDescriptionBoxKeepsIt(t *testing.T) {
	app := newMouseTestApp(t)
	app.selectedIssue = detailsFixture()
	app.updateDetailsView()
	layOut(t, app, 180, 40, FocusDetails)
	app.enterDetailsEdit()
	app.detailsEdit.cursor = issueFieldDescription
	app.openFieldEditor()
	app.app.ForceDraw()

	x, y, _, _ := app.detailsDescArea.GetRect()
	clickAt(t, app, x+1, y)

	if app.detailsEdit.editing != issueFieldDescription {
		t.Fatal("clicking into the box closed it")
	}
}
