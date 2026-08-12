package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// newThreadedTestApp opens an issue whose comments form a thread, with the card
// stack holding the keyboard.
func newThreadedTestApp(t *testing.T) *App {
	t.Helper()

	app := newDetailsTestApp(t)
	issue := detailsFixture()
	issue.Comments = threadedComments()
	// The issue's own URL is what the comment keys fall back to when no card is
	// picked, so it has to be there for that fallback to be visible.
	issue.URL = "https://linear.app/praxis-labs/issue/ZNO-1"
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

// tabComments sends Tab, or Shift+Tab backwards, the way the running app does.
func tabComments(t *testing.T, app *App, backward bool) {
	t.Helper()
	key := tcell.KeyTab
	if backward {
		key = tcell.KeyBacktab
	}
	app.handleGlobalKey(tcell.NewEventKey(key, 0, tcell.ModNone))
}

// TestTheCommentsTabOpensWithNothingPicked covers the stack taking the keyboard
// without lighting a card. Nothing is picked until Tab says so, and until then
// the pane's own keys are the issue's.
func TestTheCommentsTabOpensWithNothingPicked(t *testing.T) {
	app := newThreadedTestApp(t)

	if got := app.focusedCommentID; got != "" {
		t.Errorf("the tab opened with %q picked, want nothing", got)
	}
}

// TestTabStepsThroughTheThreadIntoTheBox covers the ring: every card in the
// order it is drawn, replies included, then the compose box and its button.
func TestTabStepsThroughTheThreadIntoTheBox(t *testing.T) {
	app := newThreadedTestApp(t)

	for _, want := range []string{"root-1", "reply-1", "reply-2", "root-2", "orphan"} {
		tabComments(t, app, false)
		if got := app.focusedCommentID; got != want {
			t.Fatalf("Tab picked %q, want %q", got, want)
		}
		if a := app.commentsFocus; a != commentsFocusCards {
			t.Fatalf("Tab left the stack at %q", want)
		}
	}

	tabComments(t, app, false)
	if !app.composeBoxActive() {
		t.Fatal("Tab past the last card did not reach the compose box")
	}
	tabComments(t, app, false)
	if app.commentsFocus != commentsFocusPost {
		t.Errorf("Tab from the box went to %v, want the Post button", app.commentsFocus)
	}
}

// TestBacktabWalksTheRingTheOtherWay covers coming back: out of the box onto
// the card Tab left, and up the stack to the first, where it stops.
func TestBacktabWalksTheRingTheOtherWay(t *testing.T) {
	app := newThreadedTestApp(t)
	for i := 0; i < 6; i++ {
		tabComments(t, app, false)
	}
	if !app.composeBoxActive() {
		t.Fatal("six tabs did not reach the compose box")
	}

	tabComments(t, app, true)
	if got := app.focusedCommentID; got != "orphan" {
		t.Errorf("Shift+Tab out of the box picked %q, want the card it left", got)
	}
	for _, want := range []string{"root-2", "reply-2", "reply-1", "root-1"} {
		tabComments(t, app, true)
		if got := app.focusedCommentID; got != want {
			t.Fatalf("Shift+Tab picked %q, want %q", got, want)
		}
	}
	tabComments(t, app, true)
	if got := app.focusedCommentID; got != "root-1" {
		t.Errorf("Shift+Tab before the first card picked %q, want it to stay", got)
	}
}

// TestScrollKeysStayWithTheStack covers j and k keeping their job. The ring is
// Tab's, so a reader scrolling a long thread never has to think about it.
func TestScrollKeysStayWithTheStack(t *testing.T) {
	app := newThreadedTestApp(t)
	showComments(t, app, 80, 12)
	tabComments(t, app, false)

	pressInComments(t, app, 'j')

	if got := app.focusedCommentID; got != "root-1" {
		t.Errorf("j moved the ring to %q, want the ring left alone", got)
	}
}

// TestEscapeLetsGoOfTheCard covers backing out of the ring without closing the
// pane, which is the other half of nothing being picked by default.
func TestEscapeLetsGoOfTheCard(t *testing.T) {
	app := newThreadedTestApp(t)
	tabComments(t, app, false)
	if app.focusedCommentID == "" {
		t.Fatal("Tab picked nothing")
	}

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	if got := app.focusedCommentID; got != "" {
		t.Errorf("Esc left %q picked", got)
	}
	if app.focusedPane != FocusDetails || !app.focusedDetailsView {
		t.Error("Esc left the Comments tab as well as the card")
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
	app.detailsCommentsPage.SetRect(0, 0, width, height)
	app.detailsCommentsPage.Draw(screen)
}

// TestTheComposeCardScrollsWithThePage covers the box being in the flow rather
// than pinned to the foot of the pane. Scrolled to the top of a long thread it
// is not on screen at all, and the rows it would have covered are conversation.
func TestTheComposeCardScrollsWithThePage(t *testing.T) {
	app := newThreadedTestApp(t)
	showComments(t, app, 80, 12)
	app.detailsCommentsView.ScrollToBeginning()
	showComments(t, app, 80, 12)

	compose := app.commentSpans[app.commentSpanIndex(blockIDCompose)]
	if app.commentSpanVisible(compose) {
		t.Errorf("the compose card at rows %d..%d is on screen at the top of the page", compose.start, compose.end)
	}

	// The last row of the pane belongs to a comment, not to a box sitting over
	// it.
	lines := drawComments(t, app, 80)
	if last := strings.TrimSpace(lines[11]); last == "" {
		t.Errorf("the pane's last row is empty, want the conversation running to the bottom:\n%s",
			strings.Join(lines[:12], "\n"))
	}
}

// TestTabScrollsTheCardIntoView covers the half of the ring a full-height draw
// cannot show: stepping past the bottom of the pane has to bring the card down,
// or Tab picks something off screen.
func TestTabScrollsTheCardIntoView(t *testing.T) {
	app := newThreadedTestApp(t)
	// Two cards' worth of rows, against five cards of thread.
	showComments(t, app, 80, 12)

	for i := 0; i < 5; i++ {
		tabComments(t, app, false)
	}
	if got := app.focusedCommentID; got != "orphan" {
		t.Fatalf("Tab picked %q, want the last card", got)
	}

	span := app.commentSpans[app.commentSpanIndex("orphan")]
	row, _ := app.detailsCommentsView.GetScrollOffset()
	if span.start < row || span.end >= row+12 {
		t.Errorf("card at rows %d..%d is off a pane showing %d..%d", span.start, span.end, row, row+11)
	}
}

// TestTheRingReanchorsAfterAFreeScroll covers Ctrl+D leaving the ring behind:
// the next Tab picks up on screen rather than hauling the pane back to the card
// the reader scrolled away from.
func TestTheRingReanchorsAfterAFreeScroll(t *testing.T) {
	app := newThreadedTestApp(t)
	showComments(t, app, 80, 12)
	tabComments(t, app, false)

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModCtrl))
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModCtrl))
	scrolled, _ := app.detailsCommentsView.GetScrollOffset()
	if scrolled == 0 {
		t.Fatal("Ctrl+D did not scroll the stack")
	}

	tabComments(t, app, false)

	if got := app.focusedCommentID; got == "root-1" {
		t.Error("Tab went back to the card it was left on, want the one on screen")
	}
	if row, _ := app.detailsCommentsView.GetScrollOffset(); row != scrolled {
		t.Errorf("Tab scrolled the pane to row %d, want it left at %d", row, scrolled)
	}
}

// TestAnActionNeedsAPickedCard covers the keys the ring owns falling back to
// the issue's own the moment no card is picked: nothing lit, nothing shadowed.
func TestAnActionNeedsAPickedCard(t *testing.T) {
	app := newThreadedTestApp(t)
	copied := make(chan string, 1)
	app.copyToClipboardFunc = func(text string) error { copied <- text; return nil }

	pressInComments(t, app, 'y')

	select {
	case got := <-copied:
		if got != "https://linear.app/praxis-labs/issue/ZNO-1" {
			t.Errorf("y copied %q, want the issue URL it falls back to", got)
		}
	default:
		t.Error("y copied nothing at all, want the issue URL it falls back to")
	}
	if got := app.replyParentID(); got != "" {
		t.Errorf("something aimed the box at %q with no card picked", got)
	}
}

// TestTheFocusedCardWearsTheFocusBorder covers the ring being visible. The
// border color is in the card's text, so it is read there rather than off the
// screen, where the harness has already dropped the styling.
func TestTheFocusedCardWearsTheFocusBorder(t *testing.T) {
	app := newThreadedTestApp(t)
	tabComments(t, app, false)

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
	tabComments(t, app, false)

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

// composeCue reads the first row of a drawn box, which while it is empty is the
// placeholder. tview takes a placeholder and never hands it back, so the drawn
// box is the only place to read it.
func composeCue(t *testing.T, area *tview.TextArea) string {
	t.Helper()
	for _, line := range drawPrimitive(t, area, 60) {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// TestReplyOpensABoxInsideTheThread covers r: a box appears at the end of the
// thread, the keyboard is in it, and it answers the thread's root rather than
// the reply the ring happened to be on.
func TestReplyOpensABoxInsideTheThread(t *testing.T) {
	app := newThreadedTestApp(t)
	for i := 0; i < 3; i++ {
		tabComments(t, app, false) // reply-2, a reply to a reply
	}

	pressInComments(t, app, 'r')

	if got := app.replyParentID(); got != "root-1" {
		t.Errorf("the box answers %q, want the thread's root", got)
	}
	if !app.composeBoxActive() || app.commentsFocus != commentsFocusReply {
		t.Error("r left the keyboard off the reply box")
	}
	if got := composeCue(t, app.detailsReplyArea); got != "Reply to drew" {
		t.Errorf("the box reads %q, want it to name who is being answered", got)
	}

	// The box goes at the end of the thread, before the next root, so the
	// answer is written where it is going to appear.
	at := app.commentSpanIndex(blockIDReply)
	if at < 0 {
		t.Fatal("no reply box on the page")
	}
	if before := app.commentSpans[at-1].id; before != "reply-2" {
		t.Errorf("the box follows %q, want the last comment of the thread", before)
	}
	if after := app.commentSpans[at+2].id; after != "root-2" {
		t.Errorf("the block after the box is %q, want the next thread", after)
	}
}

// TestTheComposeCardEndsThePage covers the box that is always there: it is the
// last card on the page, and it scrolls with everything else rather than
// sitting over the conversation.
func TestTheComposeCardEndsThePage(t *testing.T) {
	app := newThreadedTestApp(t)

	spans := app.commentSpans
	if len(spans) < 2 {
		t.Fatal("the page drew no compose card")
	}
	if got := spans[len(spans)-1].id; got != blockIDCompose {
		t.Errorf("the page ends on %q, want the compose card", got)
	}
	if got := spans[len(spans)-3].id; got != "orphan" {
		t.Errorf("the block before the compose card is %q, want the last comment", got)
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

	tabComments(t, app, false)
	tabComments(t, app, false) // reply-1
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
	if got := composeCue(t, app.detailsComposeArea); got != composePlaceholder {
		t.Errorf("the compose card reads %q, want it ready for the next comment", got)
	}
	if got := app.focusedCommentID; got != "posted" {
		t.Errorf("the ring is on %q, want the comment just posted", got)
	}
}

// TestQuoteFillsTheBoxWithTheComment covers Q: the body goes into the thread's
// own box as markdown quote.
func TestQuoteFillsTheBoxWithTheComment(t *testing.T) {
	app := newThreadedTestApp(t)
	tabComments(t, app, false)

	pressInComments(t, app, 'Q')

	if got, want := app.detailsReplyArea.GetText(), "> The debounce is the problem.\n"; got != want {
		t.Errorf("the box holds %q, want %q", got, want)
	}
	if got := app.replyParentID(); got != "root-1" {
		t.Errorf("the quote answers %q, want the quoted comment", got)
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

// TestEscapeClosesTheReplyBoxAndKeepsTheWords covers backing out of an answer.
// The box goes, because an empty box left open in the middle of a conversation
// is rows of nothing between two comments; the words stay against the thread.
func TestEscapeClosesTheReplyBoxAndKeepsTheWords(t *testing.T) {
	app := newThreadedTestApp(t)
	tabComments(t, app, false)
	pressInComments(t, app, 'r')
	typeRunes(t, app, "half a thought")

	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	if got := app.replyParentID(); got != "" {
		t.Errorf("Esc left a box open on %q", got)
	}
	if app.commentSpanIndex(blockIDReply) >= 0 {
		t.Error("Esc left the box on the page")
	}
	if got := app.focusedCommentID; got != "root-1" {
		t.Errorf("Esc put the ring on %q, want the comment being answered", got)
	}

	// Reopening on the same thread finds them again.
	pressInComments(t, app, 'r')
	if got := app.detailsReplyArea.GetText(); got != "half a thought" {
		t.Errorf("the reopened box holds %q, want the words kept", got)
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

	tabComments(t, app, false)
	tabComments(t, app, false) // reply-1
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
	tabComments(t, app, false)
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

	tabComments(t, app, false)
	for _, key := range []rune{'r', 'Q', 'y', 'o'} {
		pressInComments(t, app, key)
	}

	if got := app.focusedCommentID; got != "" {
		t.Errorf("the ring landed on %q with no comments to land on", got)
	}
	if got := app.replyParentID(); got != "" {
		t.Errorf("the box is aimed at %q with no comments to answer", got)
	}
}
