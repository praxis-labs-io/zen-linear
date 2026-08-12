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

// typeInCompose sends a key the way the running app does: the global capture
// first, then whatever it hands back goes to the root, which walks down the
// focus chain to deliver it.
//
// The walk is not an implementation detail to shortcut. tview hands a key to
// the root primitive, never to the focused one, so a container that answers for
// the wrong child swallows every key while the focus, the border and the status
// bar all say the box has the keyboard. Handing the event straight to
// app.GetFocus() here proved only that the box was focused, and passed for a
// whole branch against a box nothing could be typed into.
func typeInCompose(t *testing.T, app *App, event *tcell.EventKey) {
	t.Helper()
	remaining := app.handleGlobalKey(event)
	if remaining == nil {
		return
	}
	root := app.pages
	if !root.HasFocus() {
		t.Fatalf("the layout does not hold the focus, so no key can be delivered")
	}
	if handler := root.InputHandler(); handler != nil {
		handler(remaining, func(p tview.Primitive) { app.app.SetFocus(p) })
	}
}

func typeRunes(t *testing.T, app *App, text string) {
	t.Helper()
	for _, r := range text {
		typeInCompose(t, app, tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}
}

// newComposeTestApp opens an issue with comments, puts the keyboard in the
// compose box the way t does, and returns a channel that fires after each
// queued draw so a test can wait out the post goroutine.
func newComposeTestApp(t *testing.T) (*App, <-chan struct{}) {
	t.Helper()
	app := newCommentsTestApp(t)
	drawn := make(chan struct{}, 8)
	app.queueUpdateDraw = func(f func()) {
		f()
		select {
		case drawn <- struct{}{}:
		default:
		}
	}
	app.openComposeBox()
	return app, drawn
}

// postAndWait sends the chord and waits for the answer to land.
func postAndWait(t *testing.T, app *App, drawn <-chan struct{}) {
	t.Helper()
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModCtrl))
	waitForDraw(t, drawn)
}

// TestCommentShortcutOpensTheBoxFromAnIssuePane covers c reaching the box from
// the list as well as the details pane. It writes on the selected issue, so it
// answers where an issue is selected and nowhere else.
func TestCommentShortcutOpensTheBoxFromAnIssuePane(t *testing.T) {
	for _, pane := range []struct {
		name string
		from FocusTarget
	}{
		{"issues", FocusIssues},
		{"details", FocusDetails},
	} {
		t.Run(pane.name, func(t *testing.T) {
			app := newCommentsTestApp(t)
			app.focusedPane = pane.from
			app.focusedDetailsView = false
			app.updateFocus()

			app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModNone))

			if app.focusedPane != FocusDetails || !app.focusedDetailsView {
				t.Errorf("c left focus on pane %v tab %v, want the Comments tab", app.focusedPane, app.focusedDetailsView)
			}
			if !app.composeBoxActive() {
				t.Error("c did not put the keyboard in the compose box")
			}
			if got := app.app.GetFocus(); got != app.detailsComposeArea {
				t.Errorf("focus is on %T, want the compose area", got)
			}
		})
	}
}

// TestCommentShortcutIsDeadInTheNavigationPane pins the other half. A comment
// goes on the selected issue, and the navigation tree is not where that is
// chosen.
func TestCommentShortcutIsDeadInTheNavigationPane(t *testing.T) {
	app := newCommentsTestApp(t)
	app.focusedPane = FocusNavigation
	app.focusedDetailsView = false
	app.updateFocus()

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModNone))

	if app.focusedPane != FocusNavigation {
		t.Errorf("c moved focus to pane %v, want to stay in the tree", app.focusedPane)
	}
	if app.composeBoxActive() {
		t.Error("c opened the compose box from the navigation pane")
	}
}

// TestComposeBoxTakesLettersTheAppWouldOtherwiseClaim is the one that matters
// for a prose field: q quits, brackets cycle tabs, and braces toggle panes
// everywhere else.
func TestComposeBoxTakesLettersTheAppWouldOtherwiseClaim(t *testing.T) {
	app, _ := newComposeTestApp(t)

	typeRunes(t, app, "q[]{}/:")

	if got := app.detailsComposeArea.GetText(); got != "q[]{}/:" {
		t.Errorf("box holds %q, want every key typed into it", got)
	}
	if !app.focusedDetailsView {
		t.Error("a bracket cycled the details tabs instead of typing")
	}
	if app.navigationHidden || app.detailsHidden {
		t.Error("a brace toggled a pane instead of typing")
	}
}

// TestEnterIsANewlineAndTheChordPosts pins the split: a bare Enter cannot send
// a half-written comment.
func TestEnterIsANewlineAndTheChordPosts(t *testing.T) {
	posted := make(chan string, 1)
	app, drawn := newComposeTestApp(t)
	app.createCommentFunc = func(_ context.Context, input linearapi.CreateCommentInput) (linearapi.Comment, error) {
		posted <- input.Body
		return linearapi.Comment{ID: "new", Body: input.Body, Author: linearapi.User{DisplayName: "drew", IsMe: true}}, nil
	}

	typeRunes(t, app, "one")
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone))
	typeRunes(t, app, "two")

	if got := app.detailsComposeArea.GetText(); got != "one\ntwo" {
		t.Fatalf("box holds %q, want Enter to have added a line", got)
	}
	select {
	case body := <-posted:
		t.Fatalf("a bare Enter posted %q", body)
	default:
	}

	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModCtrl))

	if body := <-posted; body != "one\ntwo" {
		t.Errorf("posted %q, want the whole buffer", body)
	}
	waitForDraw(t, drawn)
}

// TestEscapeLeavesTheBoxAndKeepsTheWords covers handing the keys back without
// losing a draft.
func TestEscapeLeavesTheBoxAndKeepsTheWords(t *testing.T) {
	app, _ := newComposeTestApp(t)
	typeRunes(t, app, "half a thought")

	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	if app.composeBoxActive() {
		t.Error("Esc left the keyboard in the box")
	}
	if got := app.app.GetFocus(); got != app.detailsCommentsView {
		t.Errorf("focus is on %T, want the card stack", got)
	}
	if got := app.detailsComposeArea.GetText(); got != "half a thought" {
		t.Errorf("box holds %q, want the words kept", got)
	}

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModNone))
	if got := app.detailsComposeArea.GetText(); got != "half a thought" {
		t.Errorf("reopening the box holds %q, want the draft back", got)
	}
}

// TestPostedCommentLandsWithoutARefetch covers the card appearing off the
// mutation's own answer. A refetch here would blank the pane and cost a full
// issue query for one comment.
func TestPostedCommentLandsWithoutARefetch(t *testing.T) {
	app, drawn := newComposeTestApp(t)
	app.fetchIssueByID = func(context.Context, string) (linearapi.Issue, error) {
		t.Error("posting a comment refetched the issue")
		return linearapi.Issue{}, nil
	}
	app.createCommentFunc = func(_ context.Context, input linearapi.CreateCommentInput) (linearapi.Comment, error) {
		return linearapi.Comment{
			ID:     "comment-3",
			Body:   input.Body,
			Author: linearapi.User{ID: "u1", DisplayName: "drew", IsMe: true},
		}, nil
	}

	before := app.selectedIssueCommentCount()
	typeRunes(t, app, "  a third comment  ")
	postAndWait(t, app, drawn)

	if got := app.selectedIssueCommentCount(); got != before+1 {
		t.Errorf("issue has %d comments, want %d", got, before+1)
	}
	if got := app.detailsComposeArea.GetText(); got != "" {
		t.Errorf("box holds %q after posting, want it emptied", got)
	}
	if app.composeBoxActive() {
		t.Error("the keyboard stayed in the box after posting, want it back on the cards")
	}
	if got := app.app.GetFocus(); got != app.detailsCommentsView {
		t.Errorf("focus is on %T after posting, want the card stack", got)
	}
	if !strings.Contains(strings.Join(drawComments(t, app, 90), "\n"), "a third comment") {
		t.Error("the posted comment is not in the card stack")
	}
	if !strings.Contains(app.detailsTabsTitle(true), "Comments (3)") {
		t.Errorf("tab strip reads %q, want the count to have moved", app.detailsTabsTitle(true))
	}
}

// TestPostTrimsTheBody covers a box holding nothing but newlines: there is no
// comment in it to send.
func TestPostTrimsTheBody(t *testing.T) {
	app, _ := newComposeTestApp(t)
	app.createCommentFunc = func(context.Context, linearapi.CreateCommentInput) (linearapi.Comment, error) {
		t.Error("posted an empty comment")
		return linearapi.Comment{}, nil
	}

	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone))
	typeRunes(t, app, "   ")
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModCtrl))
}

// TestFailedPostPutsTheWordsBack covers the network eating a comment. The
// words are the writer's, and a request that failed is not a reason to lose
// them.
func TestFailedPostPutsTheWordsBack(t *testing.T) {
	app, drawn := newComposeTestApp(t)
	app.createCommentFunc = func(context.Context, linearapi.CreateCommentInput) (linearapi.Comment, error) {
		return linearapi.Comment{}, errors.New("create comment: connection reset")
	}

	typeRunes(t, app, "worth keeping")
	postAndWait(t, app, drawn)

	if got := app.detailsComposeArea.GetText(); got != "worth keeping" {
		t.Errorf("box holds %q, want the failed comment back", got)
	}
	if !app.composeBoxActive() {
		t.Error("the keyboard did not come back to the box")
	}
	if got := app.selectedIssueCommentCount(); got != 2 {
		t.Errorf("issue has %d comments, want the failed post not to have landed", got)
	}
}

// TestFailedPostKeepsACommentStartedInTheMeantime covers the answer arriving
// after the writer moved on. Overwriting would destroy the new comment to
// rescue the old one, which is the same loss the other way round.
func TestFailedPostKeepsACommentStartedInTheMeantime(t *testing.T) {
	release := make(chan struct{})
	app, drawn := newComposeTestApp(t)
	app.createCommentFunc = func(context.Context, linearapi.CreateCommentInput) (linearapi.Comment, error) {
		<-release
		return linearapi.Comment{}, errors.New("create comment: connection reset")
	}

	typeRunes(t, app, "first")
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModCtrl))
	// Posting hands the keyboard back, so writing the next comment means
	// opening the box again, which is what a writer would do here.
	app.openComposeBox()
	typeRunes(t, app, "second")

	close(release)
	waitForDraw(t, drawn)

	if got := app.detailsComposeArea.GetText(); got != "first\n\nsecond" {
		t.Errorf("box holds %q, want both comments", got)
	}
}

// TestComposeBoxAlignsWithTheCards covers the frame the box is drawn in. It is
// the last card in the stack, so it starts in the same column and runs to the
// same width as the ones above it.
func TestComposeBoxAlignsWithTheCards(t *testing.T) {
	for _, width := range []int{60, 100, 180} {
		app, _ := newComposeTestApp(t)
		lines := drawPrimitive(t, app.detailsCommentsPanel, width)

		var tops []int
		var widths []int
		for _, line := range lines {
			start := strings.Index(line, "╭")
			if start < 0 {
				continue
			}
			end := strings.Index(line, "╮")
			if end < 0 {
				t.Fatalf("width %d: a box opened and did not close: %q", width, line)
			}
			tops = append(tops, len([]rune(line[:start])))
			widths = append(widths, len([]rune(line[start:end]))+1)
		}
		if len(tops) < 3 {
			t.Fatalf("width %d drew %d boxes, want two cards and the compose box", width, len(tops))
		}
		for i := range tops {
			if tops[i] != tops[0] || widths[i] != widths[0] {
				t.Errorf("width %d: box %d starts at column %d and runs %d cells, want %d and %d",
					width, i, tops[i], widths[i], tops[0], widths[0])
			}
		}
	}
}

// TestComposeBoxIsAFixedHeight covers the box not growing with what is typed,
// which would push the card stack around mid-sentence.
func TestComposeBoxIsAFixedHeight(t *testing.T) {
	app, _ := newComposeTestApp(t)
	// The box is the last thing on the page, so the bottom-most frame is its.
	frameRows := func() int {
		t.Helper()
		top, bottom := -1, -1
		for i, line := range drawPrimitive(t, app.detailsCommentsPanel, 100) {
			if strings.Contains(line, "\u256d") {
				top = i
			}
			if strings.Contains(line, "\u2570") {
				bottom = i
			}
		}
		if top < 0 || bottom < top {
			t.Fatalf("no compose frame drawn: top %d bottom %d", top, bottom)
		}
		return bottom - top + 1
	}

	// The writing, the button row under it, and the card's own frame and byline
	// around both.
	const composeCardRows = composeRows + 5
	empty := frameRows()
	if empty != composeCardRows {
		t.Errorf("box draws %d rows, want %d", empty, composeCardRows)
	}

	typeRunes(t, app, "one")
	for i := 0; i < 10; i++ {
		typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone))
		typeRunes(t, app, "more")
	}

	if got := frameRows(); got != empty {
		t.Errorf("box draws %d rows after typing and %d empty, want a fixed height", got, empty)
	}
}

// TestDraftFollowsItsIssue covers the box being one widget over a changing
// selection. A comment written for one issue must not be posted to whichever
// one happens to be on screen when the chord lands.
func TestDraftFollowsItsIssue(t *testing.T) {
	app, _ := newComposeTestApp(t)
	first := app.selectedIssue

	typeRunes(t, app, "meant for the first issue")

	second := commentedIssueFixture()
	second.ID = "issue-2"
	second.Identifier = "ZNL-2"
	app.selectedIssue = second
	app.updateDetailsView()

	if got := app.detailsComposeArea.GetText(); got != "" {
		t.Errorf("the second issue's box holds %q, want it empty", got)
	}

	app.selectedIssue = first
	app.updateDetailsView()

	if got := app.detailsComposeArea.GetText(); got != "meant for the first issue" {
		t.Errorf("coming back, the box holds %q, want the draft", got)
	}
}

// TestFailedPostGoesBackToItsOwnIssue covers the answer arriving after the
// reader moved on: the words wait on the issue they were written for, not in a
// box about a different one.
func TestFailedPostGoesBackToItsOwnIssue(t *testing.T) {
	release := make(chan struct{})
	app, drawn := newComposeTestApp(t)
	app.createCommentFunc = func(context.Context, linearapi.CreateCommentInput) (linearapi.Comment, error) {
		<-release
		return linearapi.Comment{}, errors.New("create comment: connection reset")
	}
	first := app.selectedIssue

	typeRunes(t, app, "for the first issue")
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModCtrl))

	second := commentedIssueFixture()
	second.ID = "issue-2"
	app.selectedIssue = second
	app.updateDetailsView()

	close(release)
	waitForDraw(t, drawn)

	if got := app.detailsComposeArea.GetText(); got != "" {
		t.Errorf("the second issue's box holds %q, want the failed post kept off it", got)
	}

	app.selectedIssue = first
	app.updateDetailsView()

	if got := app.detailsComposeArea.GetText(); got != "for the first issue" {
		t.Errorf("the first issue's box holds %q, want the failed post back", got)
	}
}

// TestClickingTheBoxTakesTheKeyboard covers the mouse path. A click lands focus
// on the text area without going through updateFocus, and a sub-focus field is
// the only thing that would know: tested on the field alone, every letter typed
// after a click fired a command shortcut instead.
func TestClickingTheBoxTakesTheKeyboard(t *testing.T) {
	app, _ := newComposeTestApp(t)
	app.leaveComposeBox()
	app.focusedPane = FocusIssues
	app.updateFocus()

	// What tview's TextArea does on a left click.
	app.app.SetFocus(app.detailsComposeArea)
	typeRunes(t, app, "clicked in")

	if got := app.detailsComposeArea.GetText(); got != "clicked in" {
		t.Errorf("box holds %q, want the keys that followed the click", got)
	}
}

// TestTabWalksTheCommentsTabAndStopsAtTheEnd covers the focus ring: each card
// in turn, then the box, then the button. Tab is not pane navigation, so the
// end of the ring is the end of the walk.
func TestTabWalksTheCommentsTabAndStopsAtTheEnd(t *testing.T) {
	app, _ := newComposeTestApp(t)
	app.leaveComposeBox()
	// Opening the box scrolled the page to it, at the end. The ring picks up
	// from what is on screen, so the walk starts where the reader is looking.
	app.detailsCommentsView.ScrollToBeginning()

	tab := func() { app.handleGlobalKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)) }

	// One Tab per comment before the ring reaches the box.
	for range app.detailsCommentsSource {
		tab()
		if got := app.app.GetFocus(); got != app.detailsCommentsView {
			t.Fatalf("Tab through the cards focused %T, want the card stack", got)
		}
	}
	tab()
	if got := app.app.GetFocus(); got != app.detailsComposeArea {
		t.Fatalf("Tab past the last card focused %T, want the compose area", got)
	}
	tab()
	if got := app.app.GetFocus(); got != app.detailsComposePost {
		t.Fatalf("two Tabs focused %T, want the Post button", got)
	}
	tab()
	if got := app.app.GetFocus(); got != app.detailsComposePost {
		t.Fatalf("three Tabs focused %T, want to stay on the Post button", got)
	}
	if app.focusedPane != FocusDetails {
		t.Errorf("Tab left the details pane for %v", app.focusedPane)
	}
}

// TestBacktabWalksTheRingBackwards covers the ring in reverse, stopping on the
// cards rather than wrapping or leaving the pane.
func TestBacktabWalksTheRingBackwards(t *testing.T) {
	app, _ := newComposeTestApp(t)
	app.commentsFocus = commentsFocusPost
	app.updateFocus()

	backtab := func() { app.handleGlobalKey(tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone)) }

	backtab()
	if got := app.app.GetFocus(); got != app.detailsComposeArea {
		t.Fatalf("one Backtab from the button focused %T, want the compose area", got)
	}
	backtab()
	if got := app.app.GetFocus(); got != app.detailsCommentsView {
		t.Fatalf("two Backtabs focused %T, want the card stack", got)
	}
	// One per card, and then the ring is out of cards to give.
	for range app.detailsCommentsSource {
		backtab()
	}
	if got := app.app.GetFocus(); got != app.detailsCommentsView {
		t.Fatalf("Backtab off the top of the stack focused %T, want to stay", got)
	}
	if got := app.focusedCommentID; got != app.detailsCommentsSource[0].ID {
		t.Errorf("Backtab stopped on %q, want the first card", got)
	}
	if app.focusedPane != FocusDetails {
		t.Errorf("Backtab left the details pane for %v", app.focusedPane)
	}
}

// TestEnterOnThePostButtonSends covers the control a terminal that cannot send
// Ctrl+Enter has to post with.
func TestEnterOnThePostButtonSends(t *testing.T) {
	posted := make(chan string, 1)
	app, drawn := newComposeTestApp(t)
	app.createCommentFunc = func(_ context.Context, input linearapi.CreateCommentInput) (linearapi.Comment, error) {
		posted <- input.Body
		return linearapi.Comment{ID: "new", Body: input.Body}, nil
	}

	typeRunes(t, app, "sent from the button")
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if got := app.app.GetFocus(); got != app.detailsComposePost {
		t.Fatalf("Tab focused %T, want the Post button", got)
	}

	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone))

	if body := <-posted; body != "sent from the button" {
		t.Errorf("posted %q, want the buffer", body)
	}
	waitForDraw(t, drawn)
}

// TestClickingThePostButtonSends covers the same control with the mouse, which
// never reaches the key path at all.
func TestClickingThePostButtonSends(t *testing.T) {
	posted := make(chan string, 1)
	app, drawn := newComposeTestApp(t)
	app.createCommentFunc = func(_ context.Context, input linearapi.CreateCommentInput) (linearapi.Comment, error) {
		posted <- input.Body
		return linearapi.Comment{ID: "new", Body: input.Body}, nil
	}

	typeRunes(t, app, "sent by mouse")

	app.detailsComposePost.SetRect(0, 0, 8, 1)
	click := tcell.NewEventMouse(1, 0, tcell.Button1, tcell.ModNone)
	consumed, _ := app.detailsComposePost.MouseHandler()(tview.MouseLeftClick, click, func(tview.Primitive) {})
	if !consumed {
		t.Fatal("the button did not take the click")
	}

	if body := <-posted; body != "sent by mouse" {
		t.Errorf("posted %q, want the buffer", body)
	}
	waitForDraw(t, drawn)
}

// TestPostGoesToTheDraftsOwnIssue covers the window a selection move opens: it
// writes selectedIssue at once but defers the draft sync behind the detail
// debounce, so reading the selection at post time sends one issue's words to
// another.
func TestPostGoesToTheDraftsOwnIssue(t *testing.T) {
	posted := make(chan string, 1)
	app, drawn := newComposeTestApp(t)
	app.createCommentFunc = func(_ context.Context, input linearapi.CreateCommentInput) (linearapi.Comment, error) {
		posted <- input.IssueID
		return linearapi.Comment{ID: "new", Body: input.Body}, nil
	}
	first := app.selectedIssue.ID

	typeRunes(t, app, "written for the first issue")

	// The selection moves; the draft sync has not run yet.
	second := commentedIssueFixture()
	second.ID = "issue-2"
	app.issuesMu.Lock()
	app.selectedIssue = second
	app.issuesMu.Unlock()

	app.postComment()

	if got := <-posted; got != first {
		t.Errorf("posted to issue %q, want %q, the issue the words were written for", got, first)
	}
	waitForDraw(t, drawn)
}

// TestSavingSettingsKeepsTheDraft covers a settings save running through
// resetCachedState. It empties the pane, so the draft is held against its issue
// rather than shown, and it comes back when the issue does. Saving settings is
// no reason to lose what someone wrote.
func TestSavingSettingsKeepsTheDraft(t *testing.T) {
	app, _ := newComposeTestApp(t)
	issue := app.selectedIssue
	typeRunes(t, app, "still being written")

	app.resetCachedState()

	if got := app.composeDrafts[issue.ID]; got != "still being written" {
		t.Fatalf("held draft for %s is %q, want it kept across the save", issue.Identifier, got)
	}

	app.issuesMu.Lock()
	app.selectedIssue = issue
	app.issuesMu.Unlock()
	app.updateDetailsView()

	if got := app.detailsComposeArea.GetText(); got != "still being written" {
		t.Errorf("box holds %q with the issue back, want the draft", got)
	}
}

// TestSwitchingWorkspaceDropsDrafts covers the case that does clear them: the
// issues they belong to are not in the new workspace.
func TestSwitchingWorkspaceDropsDrafts(t *testing.T) {
	app, _ := newComposeTestApp(t)
	typeRunes(t, app, "for the old workspace")

	app.clearComposeDrafts()

	if got := app.detailsComposeArea.GetText(); got != "" {
		t.Errorf("box holds %q after a workspace switch, want it emptied", got)
	}
	if len(app.composeDrafts) != 0 {
		t.Errorf("%d drafts held after a workspace switch, want none", len(app.composeDrafts))
	}
}

// TestCommentsPanelDelegatesFocusToTheCards covers tview handing the panel
// focus on its own. With no item flagged, Flex.Focus falls through to the
// panel's own Box, whose InputHandler is nil, and the tab goes dead.
func TestCommentsPanelDelegatesFocusToTheCards(t *testing.T) {
	app, _ := newComposeTestApp(t)

	app.app.SetFocus(app.detailsCommentsPanel)

	if got := app.app.GetFocus(); got != app.detailsCommentsView {
		t.Errorf("focusing the panel landed on %T, want the card stack", got)
	}
}

// TestOpeningTheBoxSyncsToTheSelection covers opening inside the detail
// debounce window. The selection moves at once but the draft sync rides the
// debounce, so the box would open on the previous issue's words and swap them
// out mid-sentence when the debounce fired.
func TestOpeningTheBoxSyncsToTheSelection(t *testing.T) {
	app, _ := newComposeTestApp(t)
	typeRunes(t, app, "for the first issue")
	app.leaveComposeBox()

	// A selection move writes selectedIssue and defers updateDetailsView.
	second := commentedIssueFixture()
	second.ID = "issue-2"
	app.issuesMu.Lock()
	app.selectedIssue = second
	app.issuesMu.Unlock()

	app.openComposeBox()

	if got := app.detailsComposeArea.GetText(); got != "" {
		t.Errorf("box opened holding %q, want the new issue's empty draft", got)
	}
	if app.composeDraftIssueID != "issue-2" {
		t.Errorf("draft is keyed to %q, want the issue just selected", app.composeDraftIssueID)
	}
}

// TestPostedCommentSurvivesAnInFlightRefetch covers a detail fetch started
// before the post and answering after it. The id guard cannot catch this:
// posting does not move the selection.
func TestPostedCommentSurvivesAnInFlightRefetch(t *testing.T) {
	app, drawn := newComposeTestApp(t)
	issue := app.selectedIssue
	stale := *issue
	stale.Comments = append([]linearapi.Comment(nil), issue.Comments...)

	app.createCommentFunc = func(_ context.Context, input linearapi.CreateCommentInput) (linearapi.Comment, error) {
		return linearapi.Comment{ID: "comment-3", Body: input.Body, CreatedAt: time.Now()}, nil
	}
	app.fetchIssueByID = func(context.Context, string) (linearapi.Issue, error) { return stale, nil }

	typeRunes(t, app, "must not vanish")
	postAndWait(t, app, drawn)

	// The fetch that was already out answers now, with pre-post comments.
	app.loadIssueDetailsByID(issue.ID)
	waitForDraw(t, drawn)

	if got := app.selectedIssueCommentCount(); got != 3 {
		t.Errorf("issue has %d comments after the refetch, want the posted one kept", got)
	}
	if !strings.Contains(strings.Join(drawComments(t, app, 90), "\n"), "must not vanish") {
		t.Error("the refetch took the posted card off the stack")
	}
}

// TestCommentsLandInTimestampOrder covers two posts in flight answering out of
// order. The card stack reads oldest first, so arrival order is not the order
// to render in.
func TestCommentsLandInTimestampOrder(t *testing.T) {
	app, _ := newComposeTestApp(t)
	issue := app.selectedIssue
	base := time.Now()

	app.appendComment(issue.ID, linearapi.Comment{ID: "second", Body: "written second", CreatedAt: base.Add(time.Second)})
	app.appendComment(issue.ID, linearapi.Comment{ID: "first", Body: "written first", CreatedAt: base})

	app.issuesMu.RLock()
	got := []string{}
	for _, c := range app.selectedIssue.Comments[len(app.selectedIssue.Comments)-2:] {
		got = append(got, c.ID)
	}
	app.issuesMu.RUnlock()

	if got[0] != "first" || got[1] != "second" {
		t.Errorf("cards ordered %v, want first then second", got)
	}
}

// TestFailedPostLeavesTheErrorOnScreen covers the restore rebuilding the status
// bar on its way through updateFocus, which would repaint the posting flash
// over the error the writer needs to see.
func TestFailedPostLeavesTheErrorOnScreen(t *testing.T) {
	app, drawn := newComposeTestApp(t)
	app.createCommentFunc = func(context.Context, linearapi.CreateCommentInput) (linearapi.Comment, error) {
		return linearapi.Comment{}, errors.New("connection reset")
	}

	typeRunes(t, app, "words worth keeping")
	postAndWait(t, app, drawn)

	text := app.statusBar.GetText(true)
	if !strings.Contains(text, "connection reset") {
		t.Errorf("status bar reads %q, want the failure", text)
	}
	if strings.Contains(text, "Posting comment") {
		t.Errorf("status bar reads %q, want the posting flash gone", text)
	}
}

// TestTypingWorksOnAnIssueWithNoComments is the one this branch shipped
// without. The box took the focus, lit its border and said so in the status
// bar, and every keystroke went into the page under it and was dropped: the
// page answered for the wrong child, and a key is delivered from the root down
// the focus chain rather than to the focused widget.
func TestTypingWorksOnAnIssueWithNoComments(t *testing.T) {
	posted := make(chan linearapi.CreateCommentInput, 1)
	app := newDetailsTestApp(t)
	app.selectedIssue = detailsFixture()
	app.updateDetailsView()
	drawn := make(chan struct{}, 4)
	app.queueUpdateDraw = func(f func()) {
		f()
		select {
		case drawn <- struct{}{}:
		default:
		}
	}
	app.createCommentFunc = func(_ context.Context, input linearapi.CreateCommentInput) (linearapi.Comment, error) {
		posted <- input
		return linearapi.Comment{ID: "first", Body: input.Body}, nil
	}

	if !app.openComposeBox() {
		t.Fatal("the box would not open on an issue with no comments")
	}
	typeRunes(t, app, "the first word on this issue")

	if got := app.detailsComposeArea.GetText(); got != "the first word on this issue" {
		t.Fatalf("the box holds %q, want what was typed into it", got)
	}

	postAndWait(t, app, drawn)
	if got := (<-posted).Body; got != "the first word on this issue" {
		t.Errorf("posted %q", got)
	}
}

// TestTypingBringsTheBoxBack covers a box scrolled off the page while it still
// holds the keyboard. Words going into something off screen are words the
// writer cannot read back, so the first key returns it to view.
func TestTypingBringsTheBoxBack(t *testing.T) {
	app, _ := newComposeTestApp(t)
	showComments(t, app, 80, 12)
	app.detailsCommentsView.ScrollToBeginning()
	showComments(t, app, 80, 12)

	compose := app.commentSpans[app.commentSpanIndex(blockIDCompose)]
	if app.commentSpanVisible(compose) {
		t.Fatal("the box is still on screen, so there is nothing to bring back")
	}

	typeRunes(t, app, "x")

	if !app.commentSpanVisible(app.commentSpans[app.commentSpanIndex(blockIDCompose)]) {
		t.Error("typing left the box off screen")
	}
	if got := app.detailsComposeArea.GetText(); got != "x" {
		t.Errorf("the box holds %q", got)
	}
}

// TestCtrlCCopiesRatherThanQuitting covers the reflex that used to end the
// session: Ctrl+C in a box with a selection is a copy, and the words and the
// app both survive it.
func TestCtrlCCopiesRatherThanQuitting(t *testing.T) {
	copied := make(chan string, 1)
	app, _ := newComposeTestApp(t)
	app.copyToClipboardFunc = func(text string) error { copied <- text; return nil }
	typeRunes(t, app, "worth keeping")
	// Select it all, the way the box's own key does.
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyCtrlL, 0, tcell.ModCtrl))
	// Straight through the capture, because what matters is what it hands back:
	// tview stops the app on a Ctrl+C it gets to see.
	left := app.handleGlobalKey(tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModCtrl))
	if left != nil {
		t.Error("Ctrl+C was handed back to tview, which stops the app on it")
	}

	select {
	case got := <-copied:
		if got != "worth keeping" {
			t.Errorf("copied %q, want the selection", got)
		}
	default:
		t.Error("Ctrl+C copied nothing")
	}
	if got := app.detailsComposeArea.GetText(); got != "worth keeping" {
		t.Errorf("the box holds %q, want the words untouched", got)
	}
}
