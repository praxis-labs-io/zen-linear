package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

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

func pressField(app *App, r rune) *tcell.EventKey {
	return app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
}

func pressFieldKey(app *App, key tcell.Key) *tcell.EventKey {
	return app.handleGlobalKey(tcell.NewEventKey(key, 0, tcell.ModNone))
}

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

	state := findLine(t, lines, "Status:")
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
	row := []rune(strings.TrimLeft(state, " "))
	if len(row) <= span.valueColumn {
		t.Fatalf("state row = %q, want a value at column %d", state, span.valueColumn)
	}
	if row[span.valueColumn-1] != ' ' || row[span.valueColumn] == ' ' {
		t.Errorf("state row = %q, want its value at column %d", string(row), span.valueColumn)
	}
}

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
	state := findLine(t, lines, "Status:")
	row := []rune(strings.TrimLeft(state, " "))
	if row[detailsLabelGutter-1] != ' ' || row[detailsLabelGutter] == ' ' {
		t.Errorf("state row = %q, want its value back at column %d", string(row), detailsLabelGutter)
	}
}

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

func TestTheFieldCursorDropsOnAnotherIssue(t *testing.T) {
	app := newFieldEditApp(t)

	app.selectedIssue = &linearapi.Issue{ID: "issue-2", Identifier: "ZNO-6", Title: "Another", State: "Todo"}
	app.updateDetailsView()
	if app.detailsEdit.on {
		t.Error("edit mode followed the selection onto another issue")
	}
}

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

func TestEditModeSwallowsTheKeysThatWouldLeaveIt(t *testing.T) {
	app := newFieldEditApp(t)

	for _, r := range []rune{':', '1', 'q', '/'} {
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

func TestACommandRunsWithoutEndingTheMode(t *testing.T) {
	app := newFieldEditApp(t)
	app.selectedIssue.URL = "https://linear.app/praxis-labs/issue/ZNO-5"
	pressField(app, 'j')
	want := app.detailsEdit.cursor
	copied := ""
	app.copyToClipboardFunc = func(text string) error {
		copied = text
		return nil
	}

	pressField(app, 'y')
	if copied != app.selectedIssue.URL {
		t.Errorf("copied %q, want the issue URL", copied)
	}
	if !app.detailsEdit.on {
		t.Fatal("the command ended edit mode")
	}
	if app.detailsEdit.cursor != want {
		t.Errorf("cursor moved to %q, want %q", app.detailsEdit.cursor, want)
	}
}

func TestEnteringTwiceKeepsTheCursor(t *testing.T) {
	app := newFieldEditApp(t)
	pressField(app, 'j')
	pressField(app, 'j')
	want := app.detailsEdit.cursor

	pressField(app, 'e')
	if !app.detailsEdit.on {
		t.Fatal("e left edit mode")
	}
	if app.detailsEdit.cursor != want {
		t.Errorf("cursor reset to %q, want %q", app.detailsEdit.cursor, want)
	}
}

func TestEnteringInsideTheDebounceWindowHoldsTheMode(t *testing.T) {
	app := newDetailsTestApp(t)
	app.selectedIssue = &linearapi.Issue{ID: "issue-2", Identifier: "ZNO-6", Title: "Another", State: "Todo"}

	app.enterDetailsEdit()
	if !app.detailsEdit.on {
		t.Fatal("e did not enter edit mode")
	}

	app.updateDetailsView()
	if !app.detailsEdit.on {
		t.Error("the deferred render dropped the mode it was entered in")
	}
}
