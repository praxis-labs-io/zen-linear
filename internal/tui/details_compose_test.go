package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

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

func postAndWait(t *testing.T, app *App, drawn <-chan struct{}) {
	t.Helper()
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModCtrl))
	waitForDraw(t, drawn)
}

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
			app.updateFocus()

			app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModNone))

			if app.focusedPane != FocusDetails {
				t.Errorf("c left focus on pane %v, want the details pane", app.focusedPane)
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

func TestCommentShortcutIsDeadInTheNavigationPane(t *testing.T) {
	app := newCommentsTestApp(t)
	app.focusedPane = FocusNavigation
	app.updateFocus()

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModNone))

	if app.focusedPane != FocusNavigation {
		t.Errorf("c moved focus to pane %v, want to stay in the tree", app.focusedPane)
	}
	if app.composeBoxActive() {
		t.Error("c opened the compose box from the navigation pane")
	}
}

func TestComposeBoxTakesLettersTheAppWouldOtherwiseClaim(t *testing.T) {
	app, _ := newComposeTestApp(t)
	lit := app.focusedCommentID

	typeRunes(t, app, "q{}<>/:")

	if got := app.detailsComposeArea.GetText(); got != "q{}<>/:" {
		t.Errorf("box holds %q, want every key typed into it", got)
	}
	if got := app.focusedCommentID; got != lit {
		t.Errorf("a brace stepped the ring to %q instead of typing", got)
	}
	if app.navigationHidden || app.detailsHidden {
		t.Error("an angle toggled a pane instead of typing")
	}
}

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

func TestEscapeLeavesTheBoxAndKeepsTheWords(t *testing.T) {
	app, _ := newComposeTestApp(t)
	typeRunes(t, app, "half a thought")

	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	if app.composeBoxActive() {
		t.Error("Esc left the keyboard in the box")
	}
	if got := app.app.GetFocus(); got != app.detailsPageView {
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

	before := len(app.selectedIssue.Comments)
	typeRunes(t, app, "  a third comment  ")
	postAndWait(t, app, drawn)

	if got := len(app.selectedIssue.Comments); got != before+1 {
		t.Errorf("issue has %d comments, want %d", got, before+1)
	}
	if got := app.detailsComposeArea.GetText(); got != "" {
		t.Errorf("box holds %q after posting, want it emptied", got)
	}
	if app.composeBoxActive() {
		t.Error("the keyboard stayed in the box after posting, want it back on the cards")
	}
	if got := app.app.GetFocus(); got != app.detailsPageView {
		t.Errorf("focus is on %T after posting, want the card stack", got)
	}
	if !strings.Contains(strings.Join(drawComments(t, app, 90), "\n"), "a third comment") {
		t.Error("the posted comment is not in the card stack")
	}
}

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
	if got := len(app.selectedIssue.Comments); got != 2 {
		t.Errorf("issue has %d comments, want the failed post not to have landed", got)
	}
}

func TestFailedPostKeepsACommentStartedInTheMeantime(t *testing.T) {
	release := make(chan struct{})
	app, drawn := newComposeTestApp(t)
	app.createCommentFunc = func(context.Context, linearapi.CreateCommentInput) (linearapi.Comment, error) {
		<-release
		return linearapi.Comment{}, errors.New("create comment: connection reset")
	}

	typeRunes(t, app, "first")
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModCtrl))
	app.openComposeBox()
	typeRunes(t, app, "second")

	close(release)
	waitForDraw(t, drawn)

	if got := app.detailsComposeArea.GetText(); got != "first\n\nsecond" {
		t.Errorf("box holds %q, want both comments", got)
	}
}

func TestComposeBoxAlignsWithTheCards(t *testing.T) {
	for _, width := range []int{60, 100, 180} {
		app, _ := newComposeTestApp(t)
		lines := drawPrimitive(t, app.detailsView, width)

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

func TestTheComposeBoxGrowsWithWhatIsTyped(t *testing.T) {
	app, _ := newComposeTestApp(t)
	frameRows := func() int {
		t.Helper()
		top, bottom := -1, -1
		for i, line := range drawPrimitive(t, app.detailsView, 100) {
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

	const chrome = 5
	if empty := frameRows(); empty != composeRows+chrome {
		t.Errorf("an empty box draws %d rows, want %d", empty, composeRows+chrome)
	}

	const written = 11
	typeRunes(t, app, "one")
	for i := 0; i < written-1; i++ {
		typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone))
		typeRunes(t, app, "more")
	}
	if got := frameRows(); got != written+chrome {
		t.Errorf("box draws %d rows for %d lines, want %d", got, written, written+chrome)
	}

	fillWritingBox(app.detailsComposeArea, "")
	if got := frameRows(); got != composeRows+chrome {
		t.Errorf("box draws %d rows once emptied, want back to %d", got, composeRows+chrome)
	}
}

func TestANewLineStaysOnThePage(t *testing.T) {
	app, _ := newComposeTestApp(t)
	const height = 16
	showComments(t, app, 80, height)

	bottom := func() (span commentSpan, fold int) {
		t.Helper()
		index := app.commentSpanIndex(blockIDCompose)
		if index < 0 {
			t.Fatal("the compose card is not on the page")
		}
		row, _ := app.detailsPageView.GetScrollOffset()
		return app.commentSpans[index], row + viewHeight(app.detailsPageView)
	}

	for i := 0; i < 12; i++ {
		typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone))
		showComments(t, app, 80, height)
		span, fold := bottom()
		if span.end-span.start+1 > viewHeight(app.detailsPageView) {
			return
		}
		if span.end >= fold {
			t.Fatalf("line %d put the box's last row at %d, below the fold at %d", i+1, span.end, fold)
		}
	}
}

func TestAGrownBoxShowsWhatWasWrittenFirst(t *testing.T) {
	app, _ := newComposeTestApp(t)
	typeRunes(t, app, "first")
	for i := 0; i < 10; i++ {
		typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone))
		typeRunes(t, app, "more")
	}
	page := strings.Join(drawPrimitive(t, app.detailsView, 100), "\n")

	if row, _ := app.detailsComposeArea.GetOffset(); row != 0 {
		t.Errorf("the box is scrolled %d rows down, want the top of what was written", row)
	}
	if !strings.Contains(page, "first") {
		t.Error("the first line written is not on the page")
	}
}

func TestABracketedBodyIsMeasuredAsItIsDrawn(t *testing.T) {
	area := tview.NewTextArea()
	area.SetText("See the [docs](https://example.com/a/very/long/path/that/keeps/going) for the rest of it.", false)

	const measure = 20
	drawn := len(drawPrimitiveAt(t, area, measure, 40))
	rows := writingBoxRows(area, measure)
	written := 0
	for _, line := range drawPrimitiveAt(t, area, measure, drawn) {
		if strings.TrimSpace(line) != "" {
			written++
		}
	}
	if rows < written {
		t.Errorf("the box is sized to %d rows for text the TextArea draws in %d", rows, written)
	}
}

func TestAFullLastLineLeavesTheCursorARow(t *testing.T) {
	area := tview.NewTextArea()
	const measure = 20
	full := strings.Repeat("a", measure)
	area.SetText(strings.Repeat(full+"\n", 4)+full, false)

	if got := writingBoxRows(area, measure); got != 6 {
		t.Errorf("five full lines measure %d rows, want 6: one spare for the cursor", got)
	}
}

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

func TestClickingTheBoxTakesTheKeyboard(t *testing.T) {
	app, _ := newComposeTestApp(t)
	app.leaveComposeBox()
	app.focusedPane = FocusIssues
	app.updateFocus()

	app.app.SetFocus(app.detailsComposeArea)
	typeRunes(t, app, "clicked in")

	if got := app.detailsComposeArea.GetText(); got != "clicked in" {
		t.Errorf("box holds %q, want the keys that followed the click", got)
	}
}

func TestBracesWalkThePageAndStopAtTheEnd(t *testing.T) {
	app, _ := newComposeTestApp(t)
	app.leaveComposeBox()
	app.detailsPageView.ScrollToBeginning()

	next := func() { stepComments(t, app, false) }

	for range app.detailsCommentsSource {
		next()
		if got := app.app.GetFocus(); got != app.detailsPageView {
			t.Fatalf("} through the cards focused %T, want the card stack", got)
		}
	}
	next()
	if got := app.focusedCommentID; got != blockIDCompose {
		t.Fatalf("} past the last card picked %q, want the compose card", got)
	}
	if got := app.app.GetFocus(); got != app.detailsPageView {
		t.Fatalf("} past the last card focused %T, want the card stack", got)
	}
	next()
	if got := app.focusedCommentID; got != blockIDCompose {
		t.Errorf("} off the end of the ring moved to %q, want it to stay put", got)
	}
	if app.focusedPane != FocusDetails {
		t.Errorf("} left the details pane for %v", app.focusedPane)
	}
}

func TestBracesWalkThePageBackwards(t *testing.T) {
	app, _ := newComposeTestApp(t)
	app.leaveComposeBox()
	app.focusComment(app.detailsCommentsSource[len(app.detailsCommentsSource)-1].ID)

	for range app.detailsCommentsSource {
		stepComments(t, app, true)
	}
	if got := app.app.GetFocus(); got != app.detailsPageView {
		t.Fatalf("{ off the top of the stack focused %T, want to stay", got)
	}
	if got := app.focusedCommentID; got != app.detailsCommentsSource[0].ID {
		t.Errorf("{ stopped on %q, want the first card", got)
	}
	if app.focusedPane != FocusDetails {
		t.Errorf("{ left the details pane for %v", app.focusedPane)
	}
}

func TestTabWalksABoxToItsPostButton(t *testing.T) {
	app, _ := newComposeTestApp(t)
	tab := func() { app.handleGlobalKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)) }

	tab()
	if got := app.app.GetFocus(); got != app.detailsComposePost {
		t.Fatalf("Tab in the box focused %T, want the Post button", got)
	}
	tab()
	if got := app.app.GetFocus(); got != app.detailsComposeArea {
		t.Fatalf("Tab from the button focused %T, want back in the box", got)
	}

	app.leaveComposeBox()
	lit := app.focusedCommentID
	tab()
	if got := app.focusedCommentID; got != lit {
		t.Errorf("Tab on the cards stepped the ring to %q, want it left to the braces", got)
	}
}

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

func TestPostGoesToTheDraftsOwnIssue(t *testing.T) {
	posted := make(chan string, 1)
	app, drawn := newComposeTestApp(t)
	app.createCommentFunc = func(_ context.Context, input linearapi.CreateCommentInput) (linearapi.Comment, error) {
		posted <- input.IssueID
		return linearapi.Comment{ID: "new", Body: input.Body}, nil
	}
	first := app.selectedIssue.ID

	typeRunes(t, app, "written for the first issue")

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

func TestCommentsPanelDelegatesFocusToTheCards(t *testing.T) {
	app, _ := newComposeTestApp(t)

	app.app.SetFocus(app.detailsView)

	if got := app.app.GetFocus(); got != app.detailsPageView {
		t.Errorf("focusing the panel landed on %T, want the card stack", got)
	}
}

func TestOpeningTheBoxSyncsToTheSelection(t *testing.T) {
	app, _ := newComposeTestApp(t)
	typeRunes(t, app, "for the first issue")
	app.leaveComposeBox()

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

func TestPostedCommentSurvivesAnInFlightRefetch(t *testing.T) {
	app, drawn := newComposeTestApp(t)
	issue := app.selectedIssue
	stale := *issue
	stale.Comments = append([]linearapi.Comment(nil), issue.Comments...)

	app.createCommentFunc = func(_ context.Context, input linearapi.CreateCommentInput) (linearapi.Comment, error) {
		return linearapi.Comment{ID: "comment-3", Body: input.Body, CreatedAt: time.Now()}, nil
	}
	started, release := make(chan struct{}, 1), make(chan struct{})
	app.fetchIssueByID = func(context.Context, string) (linearapi.Issue, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return stale, nil
	}
	app.loadIssueDetailsByID(issue.ID)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("the detail fetch never went out")
	}

	typeRunes(t, app, "must not vanish")
	postAndWait(t, app, drawn)

	close(release)
	waitForDraw(t, drawn)

	if got := len(app.selectedIssue.Comments); got != 3 {
		t.Errorf("issue has %d comments after the refetch, want the posted one kept", got)
	}
	if !strings.Contains(strings.Join(drawComments(t, app, 90), "\n"), "must not vanish") {
		t.Error("the refetch took the posted card off the stack")
	}
}

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

func TestFailedPostLeavesTheErrorOnScreen(t *testing.T) {
	app, drawn := newComposeTestApp(t)
	app.createCommentFunc = func(context.Context, linearapi.CreateCommentInput) (linearapi.Comment, error) {
		return linearapi.Comment{}, errors.New("connection reset")
	}

	typeRunes(t, app, "words worth keeping")
	postAndWait(t, app, drawn)

	text := statusText(app)
	if !strings.Contains(text, "connection reset") {
		t.Errorf("status bar reads %q, want the failure", text)
	}
	if strings.Contains(text, "Posting comment") {
		t.Errorf("status bar reads %q, want the posting flash gone", text)
	}
}

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

func TestTypingBringsTheBoxBack(t *testing.T) {
	app, _ := newComposeTestApp(t)
	showComments(t, app, 80, 12)
	app.detailsPageView.ScrollToBeginning()
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

func TestCtrlCCopiesRatherThanQuitting(t *testing.T) {
	copied := make(chan string, 1)
	app, _ := newComposeTestApp(t)
	app.copyToClipboardFunc = func(text string) error { copied <- text; return nil }
	typeRunes(t, app, "worth keeping")
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyCtrlL, 0, tcell.ModCtrl))
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
