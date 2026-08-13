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

// root-1 is Drew's, reply-1 is not. The two are what the authorship gate is
// checked against.
const (
	mineID       = "root-1"
	mineBody     = "The debounce is the problem."
	somebodyElse = "reply-1"
	elseBody     = "Which one?"
)

// newEditableCommentsApp opens the threaded issue with the cards holding the
// keyboard, and returns a channel that fires after each queued draw so a test
// can wait out a write goroutine.
func newEditableCommentsApp(t *testing.T) (*App, <-chan struct{}) {
	t.Helper()
	app := newThreadedTestApp(t)
	drawn := make(chan struct{}, 8)
	app.queueUpdateDraw = func(f func()) {
		f()
		select {
		case drawn <- struct{}{}:
		default:
		}
	}
	return app, drawn
}

// focusComment steps the ring forward until it lands on id, so a test names the
// card it means rather than counting braces.
func stepToComment(t *testing.T, app *App, id string) {
	t.Helper()
	for i := 0; i < 8; i++ {
		if app.focusedCommentID == id {
			return
		}
		stepComments(t, app, false)
	}
	t.Fatalf("the ring never reached %s, stopped on %q", id, app.focusedCommentID)
}

func TestEditOpensABoxWhereTheCardWas(t *testing.T) {
	app, _ := newEditableCommentsApp(t)
	before := len(commentCards(drawComments(t, app, 80)))
	stepToComment(t, app, mineID)

	pressInComments(t, app, 'e')

	if got := app.editingCommentID(); got != mineID {
		t.Fatalf("editing %q, want %s", got, mineID)
	}
	if app.commentsFocus != commentsFocusEdit {
		t.Errorf("focus = %v, want the edit box", app.commentsFocus)
	}
	if got := app.detailsEditArea.GetText(); got != mineBody {
		t.Errorf("the box holds %q, want the comment's body", got)
	}

	lines := drawComments(t, app, 80)
	page := strings.Join(lines, "\n")
	if !strings.Contains(page, "edit this comment") {
		t.Error("the page does not say which card is being rewritten")
	}
	if !strings.Contains(page, "Save") {
		t.Error("the box has no Save button")
	}
	if after := len(commentCards(lines)); after != before {
		t.Errorf("the page holds %d cards, want %d: the box replaces the card rather than joining it", after, before)
	}
}

// The page counts its own rows and the widgets are placed by the render rather
// than mounted in a layout, so a box opened mid-page is where a row of drift
// would show: over the card below it, and over everything after that.
func TestTheEditBoxIsDrawnOverTheCardItReplaced(t *testing.T) {
	app, _ := newEditableCommentsApp(t)
	stepToComment(t, app, mineID)
	pressInComments(t, app, 'e')
	drawPrimitiveAt(t, app.detailsPage, 90, 160)

	for _, box := range []struct {
		name string
		id   string
		area *tview.TextArea
	}{
		{"the edit box", mineID, app.detailsEditArea},
		{"the compose box below it", blockIDCompose, app.detailsComposeArea},
	} {
		index := app.commentSpanIndex(box.id)
		if index < 0 {
			t.Fatalf("%s is not on the page", box.name)
		}
		span := app.commentSpans[index]

		_, y, _, height := box.area.GetRect()
		if height == 0 {
			t.Fatalf("%s was not drawn", box.name)
		}
		if y <= span.start || y+height > span.end+1 {
			t.Errorf("%s sits at rows %d..%d, want it inside the card's %d..%d",
				box.name, y, y+height-1, span.start, span.end)
		}
	}
}

// A box sized like the compose box would collapse a long comment to four rows
// the moment the key was pressed, taking the words being rewritten off screen
// and reflowing every card under them.
func TestEditingALongCommentKeepsTheCardsHeight(t *testing.T) {
	app, _ := newEditableCommentsApp(t)
	long := strings.Repeat("A sentence that runs on, and on, and keeps running. ", 12)
	app.issuesMu.Lock()
	app.selectedIssue.Comments[0].Body = long
	app.issuesMu.Unlock()
	app.updateDetailsView()
	drawComments(t, app, 80)
	stepToComment(t, app, mineID)

	before := cardHeight(t, drawComments(t, app, 80), 0)
	pressInComments(t, app, 'e')
	after := cardHeight(t, drawComments(t, app, 80), 0)

	if after < before-1 {
		t.Errorf("the card was %d rows and the box is %d, want the box no shorter", before, after)
	}
}

// cardHeight is how many rows the card at index runs to, frame included.
func cardHeight(t *testing.T, lines []string, index int) int {
	t.Helper()
	cards := commentCards(lines)
	if index >= len(cards) {
		t.Fatalf("the page drew %d cards, want at least %d", len(cards), index+1)
	}
	return len(cards[index])
}

func TestEditAndDeleteAreOfferedOnYourOwnCommentsOnly(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		wantHint bool
	}{
		{"your own comment", mineID, true},
		{"somebody else's", somebodyElse, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := newEditableCommentsApp(t)
			app.deleteCommentFunc = func(context.Context, string) error {
				t.Fatal("the mutation ran on a card that offers no delete")
				return nil
			}
			stepToComment(t, app, tt.id)

			page := strings.Join(drawComments(t, app, 80), "\n")
			if got := strings.Contains(page, "e edit") && strings.Contains(page, "d delete"); got != tt.wantHint {
				t.Errorf("card names the edit and delete keys = %v, want %v", got, tt.wantHint)
			}

			pressInComments(t, app, 'e')
			if opened := app.editingCommentID() != ""; opened != tt.wantHint {
				t.Errorf("e opened a box = %v, want %v", opened, tt.wantHint)
			}
			if tt.wantHint {
				return
			}
			pressInComments(t, app, 'd')
			if app.pages.HasPage("confirmation") {
				t.Error("d asked about a comment it cannot delete")
			}
		})
	}
}

func TestSavingAnEditRedrawsTheCard(t *testing.T) {
	app, drawn := newEditableCommentsApp(t)
	var sent linearapi.UpdateCommentInput
	app.updateCommentFunc = func(_ context.Context, input linearapi.UpdateCommentInput) (linearapi.Comment, error) {
		sent = input
		return linearapi.Comment{
			ID:        input.ID,
			Body:      input.Body,
			CreatedAt: time.Now().Add(-time.Hour),
			UpdatedAt: time.Now(),
			Author:    linearapi.User{ID: "u1", DisplayName: "drew", IsMe: true},
		}, nil
	}

	stepToComment(t, app, mineID)
	pressInComments(t, app, 'e')
	fillWritingBox(app.detailsEditArea, "Rewritten.")
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModCtrl))
	waitForDraw(t, drawn)

	if sent.ID != mineID || sent.Body != "Rewritten." {
		t.Errorf("sent %+v, want the rewrite against %s", sent, mineID)
	}
	if sent.IssueID != app.selectedIssue.ID {
		t.Errorf("sent issue %q, want %q", sent.IssueID, app.selectedIssue.ID)
	}
	if app.editingCommentID() != "" {
		t.Error("the box is still open after the save landed")
	}
	if app.focusedCommentID != mineID {
		t.Errorf("the ring landed on %q, want the card just rewritten", app.focusedCommentID)
	}

	page := strings.Join(drawComments(t, app, 80), "\n")
	if !strings.Contains(page, "Rewritten.") {
		t.Error("the card does not show the new body")
	}
	if strings.Contains(page, mineBody) {
		t.Error("the card still shows the old body")
	}
	if !strings.Contains(page, "edited") {
		t.Error("the byline does not say the comment was edited")
	}
}

// A rewrite has nowhere to be held, so a failed save leaves it where it was
// written rather than dropping it on the floor.
func TestAFailedSaveKeepsTheWords(t *testing.T) {
	app, drawn := newEditableCommentsApp(t)
	app.updateCommentFunc = func(context.Context, linearapi.UpdateCommentInput) (linearapi.Comment, error) {
		return linearapi.Comment{}, errors.New("Linear said no")
	}

	stepToComment(t, app, mineID)
	pressInComments(t, app, 'e')
	fillWritingBox(app.detailsEditArea, "Rewritten.")
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModCtrl))
	waitForDraw(t, drawn)

	if app.editingCommentID() != mineID {
		t.Fatal("a failed save closed the box")
	}
	if got := app.detailsEditArea.GetText(); got != "Rewritten." {
		t.Errorf("the box holds %q, want the rewrite that failed to send", got)
	}
}

func TestEscDiscardsTheEdit(t *testing.T) {
	app, _ := newEditableCommentsApp(t)
	app.updateCommentFunc = func(context.Context, linearapi.UpdateCommentInput) (linearapi.Comment, error) {
		t.Fatal("Esc sent the edit")
		return linearapi.Comment{}, nil
	}

	stepToComment(t, app, mineID)
	pressInComments(t, app, 'e')
	fillWritingBox(app.detailsEditArea, "Scrap this.")
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	if app.editingCommentID() != "" {
		t.Fatal("Esc left the box open")
	}
	if !strings.Contains(strings.Join(drawComments(t, app, 80), "\n"), mineBody) {
		t.Error("the card did not come back as the comment stands")
	}

	// Reopening starts from the comment, not from what was thrown away.
	pressInComments(t, app, 'e')
	if got := app.detailsEditArea.GetText(); got != mineBody {
		t.Errorf("the box reopened holding %q, want the comment's body", got)
	}
}

func TestDeleteAsksBeforeItActs(t *testing.T) {
	app, _ := newEditableCommentsApp(t)
	app.deleteCommentFunc = func(context.Context, string) error {
		t.Fatal("the mutation ran before the prompt was answered")
		return nil
	}
	stepToComment(t, app, mineID)

	pressInComments(t, app, 'd')

	if !app.pages.HasPage("confirmation") {
		t.Fatal("d deleted without asking")
	}
}

// Deleting a thread root leaves its replies. Linear keeps them, and the page
// draws a reply whose parent it does not have as a root of its own.
func TestConfirmingDeleteTakesTheCardOffThePage(t *testing.T) {
	app, drawn := newEditableCommentsApp(t)
	deleted := ""
	app.deleteCommentFunc = func(_ context.Context, id string) error {
		deleted = id
		return nil
	}
	stepToComment(t, app, mineID)

	pressInComments(t, app, 'd')
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone))
	waitForDraw(t, drawn)

	if deleted != mineID {
		t.Fatalf("deleted %q, want %s", deleted, mineID)
	}
	if got := len(app.selectedIssue.Comments); got != 4 {
		t.Errorf("the issue holds %d comments, want 4", got)
	}
	if app.focusedCommentID != somebodyElse {
		t.Errorf("the ring landed on %q, want the card that took its place", app.focusedCommentID)
	}

	page := strings.Join(drawComments(t, app, 80), "\n")
	if strings.Contains(page, mineBody) {
		t.Error("the deleted card is still on the page")
	}
	if !strings.Contains(page, elseBody) {
		t.Error("a reply left the page with the root it answered")
	}
}
