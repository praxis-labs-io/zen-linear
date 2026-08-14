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
	mineReplyID  = "reply-2"
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

// The edit box stands mid-page, so scrolling puts it off screen while it still
// holds the keyboard. A key that scrolls somewhere else leaves the user typing
// blind into a box they cannot see.
func TestTypingBringsTheEditBoxBackOnScreen(t *testing.T) {
	app, _ := newEditableCommentsApp(t)
	stepToComment(t, app, mineID)
	pressInComments(t, app, 'e')
	// Short enough that the box mid-page and the compose card at the end of it
	// cannot both be on screen.
	drawPrimitiveAt(t, app.detailsPage, 80, 14)
	app.detailsPageView.ScrollToEnd()
	drawPrimitiveAt(t, app.detailsPage, 80, 14)

	index := app.commentSpanIndex(mineID)
	if index < 0 {
		t.Fatal("the edit box is not on the page")
	}
	if app.commentSpanVisible(app.commentSpans[index]) {
		t.Fatal("the page never scrolled off the box, so there is nothing to bring back")
	}

	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone))
	drawPrimitiveAt(t, app.detailsPage, 80, 14)

	index = app.commentSpanIndex(mineID)
	if index < 0 || !app.commentSpanVisible(app.commentSpans[index]) {
		t.Error("typing scrolled to a box other than the one being typed in")
	}
}

// A save is slow enough to Esc out of and start another. The answer belongs to
// the edit that sent it, not to whatever box is open when it lands.
func TestASlowSaveLeavesALaterEditAlone(t *testing.T) {
	app, drawn := newEditableCommentsApp(t)
	release := make(chan struct{})
	app.updateCommentFunc = func(_ context.Context, input linearapi.UpdateCommentInput) (linearapi.Comment, error) {
		<-release
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
	fillWritingBox(app.detailsEditArea, "Sent, and slow.")
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModCtrl))

	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	stepToComment(t, app, mineReplyID)
	pressInComments(t, app, 'e')
	const later = "Written while the first was still out."
	fillWritingBox(app.detailsEditArea, later)

	close(release)
	waitForDraw(t, drawn)

	if got := app.editingCommentID(); got != mineReplyID {
		t.Fatalf("the open box is on %q, want %s: the first save closed a box it did not open", got, mineReplyID)
	}
	if got := app.detailsEditArea.GetText(); got != later {
		t.Errorf("the box holds %q, want the words written after the save went out", got)
	}
	// The keyboard is physically in the box, so the state that says where it is
	// has to agree. Tab, the growth refit and the hints all read this.
	if app.commentsFocus != commentsFocusEdit {
		t.Errorf("focus reads %v, want the box the keys are going to", app.commentsFocus)
	}
	if got := app.focusedCommentID; got != mineReplyID {
		t.Errorf("the ring moved to %q, want it left on the box being written in", got)
	}
}

// The box stays open until Linear answers, so Ctrl+Enter twice inside one round
// trip sends two rewrites. The loser closes the box under the winner, and out
// of order it pins the older body to the card.
func TestASecondSaveWaitsForTheFirst(t *testing.T) {
	app, drawn := newEditableCommentsApp(t)
	release := make(chan struct{})
	sent := make(chan string, 4)
	app.updateCommentFunc = func(_ context.Context, input linearapi.UpdateCommentInput) (linearapi.Comment, error) {
		sent <- input.Body
		<-release
		return linearapi.Comment{ID: input.ID, Body: input.Body, Author: linearapi.User{ID: "u1", IsMe: true}}, nil
	}

	stepToComment(t, app, mineID)
	pressInComments(t, app, 'e')
	fillWritingBox(app.detailsEditArea, "First.")
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModCtrl))
	<-sent

	fillWritingBox(app.detailsEditArea, "Second.")
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModCtrl))

	close(release)
	waitForDraw(t, drawn)
	if extra := len(sent); extra != 0 {
		t.Errorf("the mutation ran %d more times, want the second refused while the first was out", extra)
	}
}

// A card being deleted stays actionable for the length of the round trip, so a
// box can be opened on it. The render then pulls the box's slot away from a
// widget that still owns the keyboard.
func TestDeletingACommentClosesTheBoxOpenOnIt(t *testing.T) {
	app, drawn := newEditableCommentsApp(t)
	release := make(chan struct{})
	app.deleteCommentFunc = func(context.Context, string) error {
		<-release
		return nil
	}
	stepToComment(t, app, mineID)
	pressInComments(t, app, 'd')
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone))

	pressInComments(t, app, 'e')
	if app.editingCommentID() != mineID {
		t.Fatal("e did not open a box on the card being deleted")
	}

	close(release)
	waitForDraw(t, drawn)

	if got := app.editingCommentID(); got != "" {
		t.Errorf("the box on %q is still open after its comment left the page", got)
	}
	if app.commentsFocus != commentsFocusCards {
		t.Errorf("focus reads %v, want the cards: the box it names is not drawn", app.commentsFocus)
	}
}

// A comment can be deleted upstream while it is being rewritten here. The box
// is then drawn nowhere while the compose card keeps composeBoxOnScreen true,
// so every key goes to an editor nothing paints.
func TestARefreshWithoutTheEditedCommentClosesTheBox(t *testing.T) {
	app, _ := newEditableCommentsApp(t)
	stepToComment(t, app, mineID)
	pressInComments(t, app, 'e')
	fillWritingBox(app.detailsEditArea, "Rewriting something that is about to go.")

	app.issuesMu.Lock()
	app.selectedIssue.Comments = app.selectedIssue.Comments[1:]
	app.issuesMu.Unlock()
	app.updateDetailsView()

	if got := app.editingCommentID(); got != "" {
		t.Errorf("the box on %q is still open after the refresh dropped its comment", got)
	}
	if app.commentsFocus != commentsFocusCards {
		t.Errorf("focus reads %v, want the cards", app.commentsFocus)
	}
	if app.commentSpanIndex(mineID) >= 0 {
		t.Error("the deleted comment is still on the page")
	}
}

// The ring belongs to the reader, not to a mutation answering late. A delete
// landing after they stepped away must not haul them back to its neighbor.
func TestADeleteLandingLateLeavesTheRingAlone(t *testing.T) {
	app, drawn := newEditableCommentsApp(t)
	release := make(chan struct{})
	app.deleteCommentFunc = func(context.Context, string) error {
		<-release
		return nil
	}
	stepToComment(t, app, mineID)
	pressInComments(t, app, 'd')
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone))

	stepToComment(t, app, "root-2")
	close(release)
	waitForDraw(t, drawn)

	if got := app.focusedCommentID; got != "root-2" {
		t.Errorf("the ring landed on %q, want the card the reader stepped to", got)
	}
}

// Linear reads leading whitespace as an indented code block, so trimming the
// body would rewrite a comment its author only looked at.
func TestOpeningAndSavingAnIndentedCommentSendsNothing(t *testing.T) {
	app, _ := newEditableCommentsApp(t)
	sent := make(chan string, 1)
	app.updateCommentFunc = func(_ context.Context, input linearapi.UpdateCommentInput) (linearapi.Comment, error) {
		sent <- input.Body
		return linearapi.Comment{ID: input.ID, Body: input.Body}, nil
	}
	app.issuesMu.Lock()
	app.selectedIssue.Comments[0].Body = "    code := run()\n    return code\n"
	app.issuesMu.Unlock()
	app.updateDetailsView()
	drawComments(t, app, 80)

	stepToComment(t, app, mineID)
	pressInComments(t, app, 'e')
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModCtrl))

	select {
	case body := <-sent:
		t.Fatalf("sent %q, want nothing sent: nobody changed the comment", body)
	case <-time.After(100 * time.Millisecond):
	}
	if app.editingCommentID() != "" {
		t.Error("the box stayed open on a save with nothing to send")
	}
}

// The card stays on the page until Linear answers, so it can be confirmed twice
// inside one round trip. The loser reports a failure for a comment that went.
func TestASecondDeleteWaitsForTheFirst(t *testing.T) {
	app, drawn := newEditableCommentsApp(t)
	release := make(chan struct{})
	calls := make(chan string, 4)
	app.deleteCommentFunc = func(_ context.Context, id string) error {
		calls <- id
		<-release
		return nil
	}
	stepToComment(t, app, mineID)

	pressInComments(t, app, 'd')
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone))
	<-calls

	pressInComments(t, app, 'd')
	if app.pages.HasPage("confirmation") {
		t.Fatal("a second delete was offered while the first was still out")
	}

	close(release)
	waitForDraw(t, drawn)
	if extra := len(calls); extra != 0 {
		t.Errorf("the mutation ran %d more times than the one confirmation asked for", extra)
	}
}

// A write that lands after the user has moved on belongs to nobody. Canceling
// there would kill the fetch filling in the issue they moved to.
func TestAWriteForAnIssueLeftBehindLeavesTheLiveFetchAlone(t *testing.T) {
	app, _ := newEditableCommentsApp(t)
	before := app.detailFetchGeneration.Load()

	app.replaceComment("some-other-issue", linearapi.Comment{ID: mineID, Body: "Rewritten."})
	app.removeComment("some-other-issue", mineID)

	if got := app.detailFetchGeneration.Load(); got != before {
		t.Errorf("the fetch generation moved to %d from %d, so a live fetch was canceled by a write for an issue nobody is looking at", got, before)
	}
}
