package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// newThreadedTestApp opens an issue whose comments form a thread, with the card
// stack holding the keyboard.
func newThreadedTestApp(t *testing.T) *App {
	t.Helper()

	app := newDetailsTestApp(t)
	issue := detailsFixture()
	issue.Comments = threadedComments()
	app.selectedIssue = issue
	app.updateDetailsView()
	focusCommentCards(app)
	// The spans come off a real draw: the ring moves and scrolls by where the
	// cards landed, and nothing has landed anywhere until the pane has a width.
	drawComments(t, app, 80)
	return app
}

// focusCommentCards puts the keyboard on the card stack. The pane has to be on
// screen first: updateFocus bounces focus off a hidden details pane, which
// leaves the keys going to the issues list.
func focusCommentCards(app *App) {
	app.detailsHidden = false
	app.focusedPane = FocusDetails
	app.focusedDetailsView = true
	app.commentsFocus = commentsFocusCards
	app.updateFocus()
}

func pressInComments(t *testing.T, app *App, r rune) {
	t.Helper()
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
}

// TestTheRingStartsOnTheFirstCard covers the stack taking the keyboard with
// nothing focused: an action key needs a card to act on before j is ever
// pressed.
func TestTheRingStartsOnTheFirstCard(t *testing.T) {
	app := newThreadedTestApp(t)

	if got := app.focusedCommentID; got != "root-1" {
		t.Errorf("the ring is on %q, want the first card", got)
	}
}

// TestTheRingStepsThroughTheThread covers j, k, g and G walking the cards in
// the order they are drawn, replies included, and stopping at either end.
func TestTheRingStepsThroughTheThread(t *testing.T) {
	app := newThreadedTestApp(t)
	order := []string{"root-1", "reply-1", "reply-2", "root-2", "orphan"}

	for _, want := range order[1:] {
		pressInComments(t, app, 'j')
		if got := app.focusedCommentID; got != want {
			t.Fatalf("j moved the ring to %q, want %q", got, want)
		}
	}
	pressInComments(t, app, 'j')
	if got := app.focusedCommentID; got != "orphan" {
		t.Errorf("j past the last card moved the ring to %q, want it to stay", got)
	}

	pressInComments(t, app, 'k')
	if got := app.focusedCommentID; got != "root-2" {
		t.Errorf("k moved the ring to %q, want %q", got, "root-2")
	}

	pressInComments(t, app, 'g')
	if got := app.focusedCommentID; got != "root-1" {
		t.Errorf("g moved the ring to %q, want the first card", got)
	}
	pressInComments(t, app, 'k')
	if got := app.focusedCommentID; got != "root-1" {
		t.Errorf("k before the first card moved the ring to %q, want it to stay", got)
	}
	pressInComments(t, app, 'G')
	if got := app.focusedCommentID; got != "orphan" {
		t.Errorf("G moved the ring to %q, want the last card", got)
	}
}

// showComments draws the card stack in a pane too short for the thread, which
// is where the ring has to scroll to stay visible. drawComments cannot: it
// draws forty rows, and everything fits in forty rows.
func showComments(t *testing.T, app *App, width, height int) {
	t.Helper()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(width, height)
	app.detailsCommentsView.SetRect(0, 0, width, height)
	app.detailsCommentsView.Draw(screen)
}

// TestTheRingScrollsTheCardIntoView covers the half of the ring a full-height
// draw cannot show: stepping past the bottom of the pane has to bring the card
// down, or the ring moves onto something off screen.
func TestTheRingScrollsTheCardIntoView(t *testing.T) {
	app := newThreadedTestApp(t)
	// Two cards' worth of rows, against five cards of thread.
	showComments(t, app, 80, 12)

	for i := 0; i < 4; i++ {
		pressInComments(t, app, 'j')
	}
	if got := app.focusedCommentID; got != "orphan" {
		t.Fatalf("the ring is on %q, want the last card", got)
	}

	span := app.commentSpans[app.commentSpanIndex("orphan")]
	row, _ := app.detailsCommentsView.GetScrollOffset()
	if span.start < row || span.end >= row+12 {
		t.Errorf("card at rows %d..%d is off a pane showing %d..%d", span.start, span.end, row, row+11)
	}

	pressInComments(t, app, 'g')
	if row, _ := app.detailsCommentsView.GetScrollOffset(); row != 0 {
		t.Errorf("g left the pane scrolled to row %d, want the top", row)
	}
}

// TestTheRingReanchorsAfterAFreeScroll covers Ctrl+D leaving the ring behind:
// the next j takes the ring to what is on screen rather than jumping the pane
// back to where it was left.
func TestTheRingReanchorsAfterAFreeScroll(t *testing.T) {
	app := newThreadedTestApp(t)
	showComments(t, app, 80, 12)

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModCtrl))
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModCtrl))
	scrolled, _ := app.detailsCommentsView.GetScrollOffset()
	if scrolled == 0 {
		t.Fatal("Ctrl+D did not scroll the stack")
	}

	pressInComments(t, app, 'j')

	if got := app.focusedCommentID; got == "root-1" {
		t.Error("j took the ring back to the card it was left on, want the one on screen")
	}
	if row, _ := app.detailsCommentsView.GetScrollOffset(); row != scrolled {
		t.Errorf("j scrolled the pane to row %d, want it left at %d", row, scrolled)
	}
}

// TestTheFocusedCardWearsTheFocusBorder covers the ring being visible. The
// border color is in the card's text, so it is read there rather than off the
// screen, where the harness has already dropped the styling.
func TestTheFocusedCardWearsTheFocusBorder(t *testing.T) {
	app := newThreadedTestApp(t)

	focused := cardTextFor(t, app, "root-1")
	if !strings.Contains(focused, app.themeTags.BorderFocus) {
		t.Errorf("the focused card carries no focus color:\n%s", focused)
	}
	unfocused := cardTextFor(t, app, "root-2")
	if strings.Contains(unfocused, app.themeTags.BorderFocus) {
		t.Errorf("an unfocused card wears the focus color:\n%s", unfocused)
	}
}

// TestTheRingGivesUpTheBorderWithTheKeyboard covers the pane losing focus. A
// ring left painted says the keys are somewhere they are not.
func TestTheRingGivesUpTheBorderWithTheKeyboard(t *testing.T) {
	app := newThreadedTestApp(t)

	app.focusPane(FocusIssues)

	if card := cardTextFor(t, app, "root-1"); strings.Contains(card, app.themeTags.BorderFocus) {
		t.Errorf("the ring kept the focus color after the pane lost the keyboard:\n%s", card)
	}
}

// cardTextFor returns one comment's rendered card, tags and all. It slices by
// the span the render recorded rather than searching for the body, which
// glamour has already broken into styled runs.
func cardTextFor(t *testing.T, app *App, id string) string {
	t.Helper()
	index := app.commentSpanIndex(id)
	if index < 0 {
		t.Fatalf("comment %q drew no card:\n%s", id, app.detailsCommentsView.GetText(false))
	}
	span := app.commentSpans[index]
	lines := strings.Split(app.detailsCommentsView.GetText(false), "\n")
	return strings.Join(lines[span.start:span.end+1], "\n")
}

// composeCue reads the first row of the drawn compose box, which on an empty
// box is the placeholder. tview takes a placeholder and never hands it back, so
// the drawn box is the only place to read it.
func composeCue(t *testing.T, app *App) string {
	t.Helper()
	for _, line := range drawPrimitive(t, app.detailsComposeArea, 60) {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// TestReplyAimsTheBoxAtTheThread covers r: the keyboard moves to the box, the
// placeholder names who is being answered, and the aim is the thread's root
// rather than the reply the ring happened to be on.
func TestReplyAimsTheBoxAtTheThread(t *testing.T) {
	app := newThreadedTestApp(t)
	pressInComments(t, app, 'j')
	pressInComments(t, app, 'j') // reply-2, a reply to a reply

	pressInComments(t, app, 'r')

	if got := app.replyParentID(); got != "root-1" {
		t.Errorf("the box is aimed at %q, want the thread's root", got)
	}
	if !app.composeBoxActive() {
		t.Error("r left the keyboard on the cards")
	}
	if got := composeCue(t, app); got != "Reply to drew" {
		t.Errorf("the box reads %q, want it to name who is being answered", got)
	}
	if card := cardTextFor(t, app, "root-1"); !strings.Contains(card, app.themeTags.Accent) {
		t.Errorf("the card being answered carries no accent border:\n%s", card)
	}
}

// TestReplyPostsWithItsParent is the one that keeps a reply out of the top
// level: the aim has to reach the mutation.
func TestReplyPostsWithItsParent(t *testing.T) {
	app := newThreadedTestApp(t)
	drawn := make(chan struct{}, 4)
	app.queueUpdateDraw = func(f func()) {
		f()
		select {
		case drawn <- struct{}{}:
		default:
		}
	}
	posted := make(chan linearapi.CreateCommentInput, 1)
	app.createCommentFunc = func(_ context.Context, input linearapi.CreateCommentInput) (linearapi.Comment, error) {
		posted <- input
		return linearapi.Comment{
			ID: "posted", Body: input.Body, ParentID: input.ParentID,
			Author: linearapi.User{ID: "u1", DisplayName: "drew", IsMe: true},
		}, nil
	}

	pressInComments(t, app, 'j')
	pressInComments(t, app, 'r')
	typeRunes(t, app, "the debounce")
	postAndWait(t, app, drawn)

	input := <-posted
	if input.ParentID != "root-1" {
		t.Errorf("posted with parent %q, want the thread's root", input.ParentID)
	}
	if input.Body != "the debounce" {
		t.Errorf("posted %q, want the words in the box", input.Body)
	}
	if got := app.replyParentID(); got != "" {
		t.Errorf("the box is still aimed at %q after posting, want it cleared", got)
	}
	if got := composeCue(t, app); got != composePlaceholder {
		t.Errorf("the box reads %q, want it back to a top-level comment", got)
	}
	if got := app.focusedCommentID; got != "posted" {
		t.Errorf("the ring is on %q, want the comment just posted", got)
	}
}

// TestQuoteFillsTheBoxWithTheComment covers Q: the body goes in as markdown
// quote, and the reply is aimed at the same thread.
func TestQuoteFillsTheBoxWithTheComment(t *testing.T) {
	app := newThreadedTestApp(t)

	pressInComments(t, app, 'Q')

	if got, want := app.detailsComposeArea.GetText(), "> The debounce is the problem.\n"; got != want {
		t.Errorf("the box holds %q, want %q", got, want)
	}
	if got := app.replyParentID(); got != "root-1" {
		t.Errorf("the quote is aimed at %q, want the quoted comment", got)
	}
}

func TestQuoteBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "one line", body: "A thought.", want: "> A thought."},
		{
			name: "a blank line stays quoted",
			body: "First.\n\nSecond.",
			// Unmarked, the gap would end the quote and drop the rest of the
			// comment back into the reply as the writer's own words.
			want: "> First.\n>\n> Second.",
		},
		{name: "trailing newlines go", body: "A thought.\n\n", want: "> A thought."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quoteBody(tt.body); got != tt.want {
				t.Errorf("quoteBody(%q) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}

// TestEscapeDropsTheAimBeforeTheBox covers backing out of a reply without
// losing the words or the box in one keystroke.
func TestEscapeDropsTheAimBeforeTheBox(t *testing.T) {
	app := newThreadedTestApp(t)
	pressInComments(t, app, 'r')
	typeRunes(t, app, "half a thought")

	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	if got := app.replyParentID(); got != "" {
		t.Errorf("Esc left the box aimed at %q", got)
	}
	if !app.composeBoxActive() {
		t.Error("Esc left the box on the first press, want it to drop the aim first")
	}
	if got := app.detailsComposeArea.GetText(); got != "half a thought" {
		t.Errorf("the box holds %q, want the words kept", got)
	}

	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if app.composeBoxActive() {
		t.Error("the second Esc did not hand the keyboard back")
	}
}

// TestCopyAndOpenActOnTheFocusedCard covers the two actions reaching for the
// ring's comment rather than the issue.
func TestCopyAndOpenActOnTheFocusedCard(t *testing.T) {
	app := newThreadedTestApp(t)
	copied := make(chan string, 1)
	opened := make(chan string, 1)
	app.copyToClipboardFunc = func(text string) error { copied <- text; return nil }
	app.openURLFunc = func(url string) error { opened <- url; return nil }

	pressInComments(t, app, 'j')
	pressInComments(t, app, 'y')
	if got := <-copied; got != "https://linear.app/c/reply-1" {
		t.Errorf("copied %q, want the focused comment's link", got)
	}

	pressInComments(t, app, 'o')
	if got := <-opened; got != "https://linear.app/c/reply-1" {
		t.Errorf("opened %q, want the focused comment's link", got)
	}
}

// TestCommentKeysStayOutOfTheDescriptionTab covers the shadow being scoped: r
// refreshes everywhere the ring is not.
func TestCommentKeysStayOutOfTheDescriptionTab(t *testing.T) {
	app := newThreadedTestApp(t)
	app.focusedDetailsView = false
	app.updateFocus()

	pressInComments(t, app, 'r')

	if got := app.replyParentID(); got != "" {
		t.Errorf("r aimed the box at %q from the description tab", got)
	}
}

// TestCommentKeysDoNothingWithoutComments covers the issue nobody has written
// on: j and k go back to scrolling and the actions have nothing to act on.
func TestCommentKeysDoNothingWithoutComments(t *testing.T) {
	app := newDetailsTestApp(t)
	app.selectedIssue = detailsFixture()
	app.updateDetailsView()
	focusCommentCards(app)
	drawComments(t, app, 80)

	for _, key := range []rune{'j', 'r', 'Q', 'y', 'o'} {
		pressInComments(t, app, key)
	}

	if got := app.focusedCommentID; got != "" {
		t.Errorf("the ring landed on %q with no comments to land on", got)
	}
	if got := app.replyParentID(); got != "" {
		t.Errorf("the box is aimed at %q with no comments to answer", got)
	}
}
