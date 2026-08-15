package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// newFieldEditApp opens the details fixture in edit mode the way e does, drawn
// so the spans and the scroll are the ones a reader would be looking at.
func newFieldEditApp(t *testing.T) *App {
	t.Helper()

	app := newDetailsTestApp(t)
	app.enterDetailsEdit()
	if !app.detailsEdit.on {
		t.Fatal("the pane did not enter edit mode")
	}
	drawDetails(t, app, 90)
	return app
}

// pressField sends a rune the way the dispatcher does and reports what was
// left over, which is what tview would go on to act on.
func pressField(app *App, r rune) *tcell.EventKey {
	return app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
}

// TestTheFieldCursorMarksTheRowItIsOn covers the marker and the gutter it sits
// in: the whole header shifts, and the spans say by how much.
func TestTheFieldCursorMarksTheRowItIsOn(t *testing.T) {
	app := newFieldEditApp(t)

	title := findLine(t, drawDetails(t, app, 90), "M3: comment infrastructure")
	if !strings.Contains(title, "❯") {
		t.Errorf("title row = %q, want the cursor on it", title)
	}

	pressField(app, 'j')
	lines := drawDetails(t, app, 90)
	if moved := findLine(t, lines, "M3: comment infrastructure"); strings.Contains(moved, "❯") {
		t.Errorf("title row = %q, want the cursor to have left it", moved)
	}

	state := findLine(t, lines, "State:")
	if !strings.Contains(state, "❯") {
		t.Errorf("state row = %q, want the cursor on it", state)
	}
	index := app.fieldSpanIndex(issueFieldState)
	if index < 0 {
		t.Fatal("no span for the state field")
	}
	span := app.detailsFieldSpans[index]
	if want := detailsLabelGutter + detailsCursorGutter; span.valueColumn != want {
		t.Fatalf("state value column = %d, want %d", span.valueColumn, want)
	}
	// Off the pane's own left padding. The marker is what the trim stops on, so
	// column 0 is the cursor and the value column is a column of this row.
	row := []rune(strings.TrimLeft(state, " "))
	if len(row) <= span.valueColumn {
		t.Fatalf("state row = %q, want a value at column %d", state, span.valueColumn)
	}
	if row[span.valueColumn-1] != ' ' || row[span.valueColumn] == ' ' {
		t.Errorf("state row = %q, want its value at column %d", string(row), span.valueColumn)
	}
}

// TestReadModeDrawsNoCursorGutter is the other half: leaving the mode puts the
// header back where it was.
func TestReadModeDrawsNoCursorGutter(t *testing.T) {
	app := newFieldEditApp(t)
	sendKey(app, tcell.KeyEscape)
	if app.detailsEdit.on {
		t.Fatal("escape left the pane in edit mode")
	}

	lines := drawDetails(t, app, 90)
	for _, line := range lines {
		if strings.Contains(line, "❯") {
			t.Errorf("read mode drew a cursor: %q", line)
		}
	}
	state := findLine(t, lines, "State:")
	row := []rune(strings.TrimLeft(state, " "))
	if row[detailsLabelGutter-1] != ' ' || row[detailsLabelGutter] == ' ' {
		t.Errorf("state row = %q, want its value back at column %d", string(row), detailsLabelGutter)
	}
}

// TestTheFieldCursorDoesNotWrapPastTheEnds covers both ends of the walk. A
// cursor that wrapped would leave the reader at the far end of the header.
func TestTheFieldCursorDoesNotWrapPastTheEnds(t *testing.T) {
	app := newFieldEditApp(t)
	if app.detailsEdit.cursor != issueFieldTitle {
		t.Fatalf("edit mode opened on %q, want the title", app.detailsEdit.cursor)
	}

	pressField(app, 'k')
	if app.detailsEdit.cursor != issueFieldTitle {
		t.Errorf("k off the top landed on %q, want the title", app.detailsEdit.cursor)
	}

	last := app.detailsFieldSpans[len(app.detailsFieldSpans)-1].field
	for range len(app.detailsFieldSpans) + 3 {
		pressField(app, 'j')
	}
	if app.detailsEdit.cursor != last {
		t.Errorf("j off the bottom landed on %q, want %q", app.detailsEdit.cursor, last)
	}
}

// TestTheFieldCursorSurvivesARefreshOfTheSameIssue covers the rebuild every
// fetch and every save runs: the cursor is held by id, so it stays put.
func TestTheFieldCursorSurvivesARefreshOfTheSameIssue(t *testing.T) {
	app := newFieldEditApp(t)
	pressField(app, 'j')
	pressField(app, 'j')
	want := app.detailsEdit.cursor
	if want != issueFieldAssignee {
		t.Fatalf("two steps landed on %q, want the assignee", want)
	}

	app.selectedIssue.Assignee = "Ada Lovelace"
	app.updateDetailsView()
	if !app.detailsEdit.on {
		t.Fatal("a refresh of the same issue dropped edit mode")
	}
	if app.detailsEdit.cursor != want {
		t.Errorf("cursor moved to %q on a refresh, want %q", app.detailsEdit.cursor, want)
	}
}

// TestTheFieldCursorDropsOnAnotherIssue is the other half: the cursor is aimed
// at one issue's fields and cannot follow the selection to another's.
func TestTheFieldCursorDropsOnAnotherIssue(t *testing.T) {
	app := newFieldEditApp(t)

	app.selectedIssue = &linearapi.Issue{ID: "issue-2", Identifier: "ZNO-6", Title: "Another", State: "Todo"}
	app.updateDetailsView()
	if app.detailsEdit.on {
		t.Error("edit mode followed the selection onto another issue")
	}
}

// TestOnlyANewIssueScrollsThePageToTheTop covers the reset that used to run on
// every rebuild, which throws a reader eight rows down back to line zero.
func TestOnlyANewIssueScrollsThePageToTheTop(t *testing.T) {
	app := newDetailsTestApp(t)
	drawDetails(t, app, 90)
	app.detailsPageView.ScrollTo(5, 0)

	app.selectedIssue.Assignee = "Ada Lovelace"
	app.updateDetailsView()
	if row, _ := app.detailsPageView.GetScrollOffset(); row != 5 {
		t.Errorf("a refresh scrolled to row %d, want the page held at 5", row)
	}

	app.selectedIssue = &linearapi.Issue{ID: "issue-2", Identifier: "ZNO-6", Title: "Another", State: "Todo"}
	app.updateDetailsView()
	if row, _ := app.detailsPageView.GetScrollOffset(); row != 0 {
		t.Errorf("a new issue opened at row %d, want the top", row)
	}
}

// TestEditModeSwallowsTheKeysThatWouldLeaveIt covers the default deny. Every
// key that leaves has to be named in the mode, or q quits from under a field.
func TestEditModeSwallowsTheKeysThatWouldLeaveIt(t *testing.T) {
	app := newFieldEditApp(t)

	for _, r := range []rune{':', '1', 'q', '/'} {
		// Swallowed, not handed back: tview quits on the Ctrl+C it is given and
		// hands every other key to the primitive under the focus.
		if left := pressField(app, r); left != nil {
			t.Errorf("%q was handed on rather than swallowed", string(r))
		}
		if !app.detailsEdit.on {
			t.Fatalf("%q left edit mode", string(r))
		}
		if app.focusedPane != FocusDetails {
			t.Fatalf("%q moved the keyboard to pane %d", string(r), app.focusedPane)
		}
	}

	sendKey(app, tcell.KeyEscape)
	if app.detailsEdit.on {
		t.Error("escape did not leave edit mode")
	}
}

// TestLeavingThePaneLeavesEditMode covers the ways out that are not the mode's
// own key. A cursor left drawn reads as a live mode on a pane nobody is in.
func TestLeavingThePaneLeavesEditMode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		leave func(*App)
	}{
		{"h to the issues list", func(a *App) { a.stepPane(-1) }},
		{"the issues pane's number", func(a *App) { a.focusPane(FocusIssues) }},
		{"a click on another pane", func(a *App) { a.claimPaneFocus(FocusIssues) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newFieldEditApp(t)
			tc.leave(app)
			if app.detailsEdit.on {
				t.Error("edit mode stayed on after the pane was left")
			}
			for _, line := range drawDetails(t, app, 90) {
				if strings.Contains(line, "❯") {
					t.Errorf("the cursor is still drawn: %q", line)
				}
			}
		})
	}
}

// TestACommandStandsEditModeDown covers the overlay pickers, which keep their
// shortcuts. The mode is not a page, so nothing else would take it off screen.
func TestACommandStandsEditModeDown(t *testing.T) {
	app := newFieldEditApp(t)
	app.selectedIssue.URL = "https://linear.app/praxis-labs/issue/ZNO-5"
	copied := ""
	app.copyToClipboardFunc = func(text string) error {
		copied = text
		return nil
	}

	pressField(app, 'y')
	if copied != app.selectedIssue.URL {
		t.Errorf("copied %q, want the issue URL", copied)
	}
	if app.detailsEdit.on {
		t.Error("edit mode stayed on under the command")
	}
}
