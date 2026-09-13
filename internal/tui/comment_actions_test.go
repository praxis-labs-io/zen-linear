package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

func newThreadedTestApp(t *testing.T) *App {
	t.Helper()

	app := newDetailsTestApp(t)
	issue := detailsFixture()
	issue.Comments = threadedComments()
	issue.URL = "https://linear.app/praxis-labs/issue/ZNO-1"
	app.selectedIssue = issue
	app.updateDetailsView()
	focusCommentCards(app)
	drawComments(t, app, 80)
	return app
}

func focusCommentCards(app *App) {
	app.detailsHidden = false
	app.focusedPane = FocusDetails
	app.detailsFocus = detailsFocusCards
	app.rebuildContentLayout()
	app.updateFocus()
}

func pressInComments(t *testing.T, app *App, r rune) {
	t.Helper()
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
}

func stepComments(t *testing.T, app *App, backward bool) {
	t.Helper()
	key := '}'
	if backward {
		key = '{'
	}
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, key, tcell.ModNone))
}

func TestTheDetailsPageOpensWithNothingPicked(t *testing.T) {
	app := newThreadedTestApp(t)

	if got := app.focusedCommentID; got != "" {
		t.Errorf("the page opened with %q picked, want nothing", got)
	}
}

func TestBracesStepThroughTheThreadIntoTheBox(t *testing.T) {
	app := newThreadedTestApp(t)

	for _, want := range []string{"root-1", "reply-1", "reply-2", "root-2", "orphan"} {
		stepComments(t, app, false)
		if got := app.focusedCommentID; got != want {
			t.Fatalf("} picked %q, want %q", got, want)
		}
		if a := app.detailsFocus; a != detailsFocusCards {
			t.Fatalf("} left the stack at %q", want)
		}
	}

	stepComments(t, app, false)
	if got := app.focusedCommentID; got != blockIDCompose {
		t.Fatalf("} past the last card picked %q, want the compose card", got)
	}
	if app.composeBoxActive() {
		t.Fatal("} armed the box for typing, want it shut until c opens it")
	}

	pressInComments(t, app, 'c')
	if !app.composeBoxActive() {
		t.Fatal("c did not open the box")
	}
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if app.detailsFocus != detailsFocusPost {
		t.Errorf("Tab from the box went to %v, want the Post button", app.detailsFocus)
	}
}

func TestEscLeavesTheComposeCardOnTheRing(t *testing.T) {
	app := newThreadedTestApp(t)
	for i := 0; i < 6; i++ {
		stepComments(t, app, false)
	}
	pressInComments(t, app, 'c')

	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if app.composeBoxActive() {
		t.Fatal("esc left the box holding the keyboard")
	}
	if got := app.focusedCommentID; got != blockIDCompose {
		t.Fatalf("esc landed the ring on %q, want the compose card", got)
	}

	stepComments(t, app, true)
	if got := app.focusedCommentID; got != "orphan" {
		t.Errorf("{ off the compose card picked %q, want the last comment", got)
	}
}

func TestTheShutComposeCardNamesTheKeyThatOpensIt(t *testing.T) {
	app := newThreadedTestApp(t)

	page := strings.Join(drawComments(t, app, 80), "\n")
	if !strings.Contains(page, "Press c to leave a comment") {
		t.Error("the shut card does not name the key that opens it")
	}

	for i := 0; i < 6; i++ {
		stepComments(t, app, false)
	}
	if page = strings.Join(drawComments(t, app, 80), "\n"); !strings.Contains(page, "c write") {
		t.Error("the lit card does not name the key along its border")
	}

	pressInComments(t, app, 'c')
	if got := composeCue(t, app.detailsComposeArea); got != composePlaceholder {
		t.Errorf("the open box reads %q, want %q", got, composePlaceholder)
	}
}

func TestBracesWalkTheRingTheOtherWay(t *testing.T) {
	app := newThreadedTestApp(t)
	for i := 0; i < 5; i++ {
		stepComments(t, app, false)
	}
	if got := app.focusedCommentID; got != "orphan" {
		t.Fatalf("five } landed on %q, want the last card", got)
	}

	for _, want := range []string{"root-2", "reply-2", "reply-1", "root-1"} {
		stepComments(t, app, true)
		if got := app.focusedCommentID; got != want {
			t.Fatalf("{ picked %q, want %q", got, want)
		}
	}
	stepComments(t, app, true)
	if got := app.focusedCommentID; got != "root-1" {
		t.Errorf("{ before the first card picked %q, want it to stay", got)
	}
}

func TestScrollKeysStayWithTheStack(t *testing.T) {
	app := newThreadedTestApp(t)
	showComments(t, app, 80, 12)
	stepComments(t, app, false)

	pressInComments(t, app, 'j')

	if got := app.focusedCommentID; got != "root-1" {
		t.Errorf("j moved the ring to %q, want the ring left alone", got)
	}
}

func TestEscapeLetsGoOfTheCard(t *testing.T) {
	app := newThreadedTestApp(t)
	stepComments(t, app, false)
	if app.focusedCommentID == "" {
		t.Fatal("Tab picked nothing")
	}

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	if got := app.focusedCommentID; got != "" {
		t.Errorf("Esc left %q picked", got)
	}
	if app.focusedPane != FocusDetails {
		t.Error("Esc left the details pane as well as the card")
	}
}

func showComments(t *testing.T, app *App, width, height int) []string {
	t.Helper()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(width, height)
	app.detailsPage.SetRect(0, 0, width, height)
	app.detailsPage.Draw(screen)
	screen.Show()

	cells, screenWidth, screenHeight := screen.GetContents()
	lines := make([]string, 0, screenHeight)
	for y := 0; y < screenHeight; y++ {
		row := make([]rune, 0, screenWidth)
		for x := 0; x < screenWidth; x++ {
			runes := cells[y*screenWidth+x].Runes
			if len(runes) == 0 || runes[0] == 0 {
				row = append(row, ' ')
				continue
			}
			row = append(row, runes[0])
		}
		lines = append(lines, strings.TrimRight(string(row), " "))
	}
	return lines
}

func TestAPickedCardCarriesItsKeysInItsBorder(t *testing.T) {
	app := newThreadedTestApp(t)
	if picked := cardTextFor(t, app, "root-1"); strings.Contains(picked, "reply") {
		t.Errorf("a card nobody picked names keys in its border:\n%s", picked)
	}

	stepComments(t, app, false)

	card := cardTextFor(t, app, "root-1")
	footer := strings.Split(card, "\n")
	last := footer[len(footer)-1]
	for _, want := range []string{"r reply", "Q quote"} {
		if !strings.Contains(last, want) {
			t.Errorf("the border under the card = %q, want it to name %q", last, want)
		}
	}
	for _, gone := range []string{"copy link", "o open"} {
		if strings.Contains(last, gone) {
			t.Errorf("the border under the card = %q, want it to leave out %q", last, gone)
		}
	}
	if !strings.HasSuffix(strings.TrimRight(last, " "), "─╯[-:-:-]") {
		t.Errorf("the border under the card = %q, want the hints inside it", last)
	}
}

func TestABoxNamesItsKeysBesideThePostButton(t *testing.T) {
	app := newThreadedTestApp(t)
	stepComments(t, app, false)
	pressInComments(t, app, 'r')

	rows := strings.Split(cardTextFor(t, app, blockIDReply), "\n")
	button := rows[len(rows)-2]
	for _, want := range []string{"ctrl+enter post", "tab post button", "esc close"} {
		if !strings.Contains(button, want) {
			t.Errorf("the button row = %q, want it to name %q", button, want)
		}
	}

	compose := strings.Split(cardTextFor(t, app, blockIDCompose), "\n")
	if got := compose[len(compose)-2]; strings.Contains(got, "post") {
		t.Errorf("the compose card names keys nobody is pressing: %q", got)
	}
}

func TestThePaddingIsTheEndOfTheScroll(t *testing.T) {
	app := newThreadedTestApp(t)
	if app.density.DetailsPadding.Bottom == 0 {
		t.Skip("this density has no bottom padding to place")
	}

	bottomRow := func() string {
		t.Helper()
		lines := showComments(t, app, 60, 12)
		return strings.TrimSpace(lines[len(lines)-1])
	}

	app.detailsPageView.ScrollToBeginning()
	if bottomRow() == "" {
		t.Error("the pane's last row is blank mid-scroll, want the conversation running to it")
	}
	app.detailsPageView.ScrollToEnd()
	if got := bottomRow(); got != "" {
		t.Errorf("the last row at the end of the scroll = %q, want the padding", got)
	}
}

func TestTheComposeCardScrollsWithThePage(t *testing.T) {
	app := newThreadedTestApp(t)
	showComments(t, app, 80, 12)
	app.detailsPageView.ScrollToBeginning()
	showComments(t, app, 80, 12)

	compose := app.commentSpans[app.commentSpanIndex(blockIDCompose)]
	if app.commentSpanVisible(compose) {
		t.Errorf("the compose card at rows %d..%d is on screen at the top of the page", compose.start, compose.end)
	}

	lines := showComments(t, app, 80, 12)
	if last := strings.TrimSpace(lines[len(lines)-1]); last == "" {
		t.Errorf("the pane's last row is empty, want the conversation running to the bottom:\n%s",
			strings.Join(lines, "\n"))
	}
}

func TestTabScrollsTheCardIntoView(t *testing.T) {
	app := newThreadedTestApp(t)
	showComments(t, app, 80, 12)

	for i := 0; i < 5; i++ {
		stepComments(t, app, false)
	}
	if got := app.focusedCommentID; got != "orphan" {
		t.Fatalf("Tab picked %q, want the last card", got)
	}

	span := app.commentSpans[app.commentSpanIndex("orphan")]
	row, _ := app.detailsPageView.GetScrollOffset()
	if span.start < row || span.end >= row+12 {
		t.Errorf("card at rows %d..%d is off a pane showing %d..%d", span.start, span.end, row, row+11)
	}
}

func TestTheRingReanchorsAfterAFreeScroll(t *testing.T) {
	app := newThreadedTestApp(t)
	showComments(t, app, 80, 12)
	stepComments(t, app, false)

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModCtrl))
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyCtrlD, 0, tcell.ModCtrl))
	scrolled, _ := app.detailsPageView.GetScrollOffset()
	if scrolled == 0 {
		t.Fatal("Ctrl+D did not scroll the stack")
	}

	stepComments(t, app, false)

	if got := app.focusedCommentID; got == "root-1" {
		t.Error("Tab went back to the card it was left on, want the one on screen")
	}
	if row, _ := app.detailsPageView.GetScrollOffset(); row != scrolled {
		t.Errorf("Tab scrolled the pane to row %d, want it left at %d", row, scrolled)
	}
}

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

func TestTheFocusedCardWearsTheFocusBorder(t *testing.T) {
	app := newThreadedTestApp(t)
	stepComments(t, app, false)

	focused := cardTextFor(t, app, "root-1")
	if !strings.Contains(focused, app.themeTags.BorderFocus) {
		t.Errorf("the focused card carries no focus color:\n%s", focused)
	}
	unfocused := cardTextFor(t, app, "root-2")
	if strings.Contains(unfocused, app.themeTags.BorderFocus) {
		t.Errorf("an unfocused card wears the focus color:\n%s", unfocused)
	}
}

func TestTheRingGivesUpTheBorderWithTheKeyboard(t *testing.T) {
	app := newThreadedTestApp(t)
	stepComments(t, app, false)

	app.focusPane(FocusIssues)

	if card := cardTextFor(t, app, "root-1"); strings.Contains(card, app.themeTags.BorderFocus) {
		t.Errorf("the ring kept the focus color after the pane lost the keyboard:\n%s", card)
	}
}

func cardTextFor(t *testing.T, app *App, id string) string {
	t.Helper()
	index := app.commentSpanIndex(id)
	if index < 0 {
		t.Fatalf("comment %q drew no card:\n%s", id, app.detailsPageView.GetText(false))
	}
	span := app.commentSpans[index]
	lines := strings.Split(app.detailsPageView.GetText(false), "\n")
	return strings.Join(lines[span.start:span.end+1], "\n")
}

func composeCue(t *testing.T, area *tview.TextArea) string {
	t.Helper()
	for _, line := range drawPrimitive(t, area, 60) {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func TestReplyOpensABoxInsideTheThread(t *testing.T) {
	app := newThreadedTestApp(t)
	for i := 0; i < 3; i++ {
		stepComments(t, app, false)
	}

	pressInComments(t, app, 'r')

	if got := app.replyParentID(); got != "root-1" {
		t.Errorf("the box answers %q, want the thread's root", got)
	}
	if !app.composeBoxActive() || app.detailsFocus != detailsFocusReply {
		t.Error("r left the keyboard off the reply box")
	}
	if got := composeCue(t, app.detailsReplyArea); got != replyPlaceholder {
		t.Errorf("the box reads %q, want %q", got, replyPlaceholder)
	}

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

func TestTheComposeCardEndsThePage(t *testing.T) {
	app := newThreadedTestApp(t)

	spans := app.commentSpans
	if len(spans) < 2 {
		t.Fatal("the page drew no compose card")
	}
	if got := spans[len(spans)-1].id; got != blockIDCompose {
		t.Errorf("the page ends on %q, want the compose card", got)
	}
	if got := spans[len(spans)-2].id; got != "orphan" {
		t.Errorf("the block before the compose card is %q, want the last comment", got)
	}
}

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

	stepComments(t, app, false)
	stepComments(t, app, false)
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
	if got := composeCue(t, app.detailsComposeArea); got != app.composePrompt() {
		t.Errorf("the compose card reads %q, want it empty and ready", got)
	}
	if got := app.focusedCommentID; got != "posted" {
		t.Errorf("the ring is on %q, want the comment just posted", got)
	}
}

func TestQuoteFillsTheBoxWithTheComment(t *testing.T) {
	app := newThreadedTestApp(t)
	stepComments(t, app, false)

	pressInComments(t, app, 'Q')

	if got, want := app.detailsReplyArea.GetText(), "> The debounce is the problem.\n\n"; got != want {
		t.Errorf("the box holds %q, want %q", got, want)
	}
	if got := app.replyParentID(); got != "root-1" {
		t.Errorf("the quote answers %q, want the quoted comment", got)
	}

	drawn := strings.Join(showComments(t, app, 80, 80), "\n")
	if !strings.Contains(drawn, "> The debounce is the problem.") {
		t.Errorf("the quote is in the box and not on the screen:\n%s", drawn)
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

func TestEscapeClosesTheReplyBoxAndKeepsTheWords(t *testing.T) {
	app := newThreadedTestApp(t)
	stepComments(t, app, false)
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

	pressInComments(t, app, 'r')
	if got := app.detailsReplyArea.GetText(); got != "half a thought" {
		t.Errorf("the reopened box holds %q, want the words kept", got)
	}
}

func TestCopyAndOpenActOnTheFocusedCard(t *testing.T) {
	app := newThreadedTestApp(t)
	copied := make(chan string, 1)
	opened := make(chan string, 1)
	app.copyToClipboardFunc = func(text string) error { copied <- text; return nil }
	app.openURLFunc = func(url string) error { opened <- url; return nil }

	stepComments(t, app, false)
	stepComments(t, app, false)
	pressInComments(t, app, 'y')
	if got := <-copied; got != "https://linear.app/c/reply-1" {
		t.Errorf("copied %q, want the focused comment's link", got)
	}

	pressInComments(t, app, 'o')
	if got := <-opened; got != "https://linear.app/c/reply-1" {
		t.Errorf("opened %q, want the focused comment's link", got)
	}
}

func TestCommentKeysStayOffAnUnlitPage(t *testing.T) {
	app := newThreadedTestApp(t)
	stepComments(t, app, false)
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	pressInComments(t, app, 'r')

	if got := app.replyParentID(); got != "" {
		t.Errorf("r aimed the box at %q with no card lit", got)
	}
}

func TestCommentKeysDoNothingWithoutComments(t *testing.T) {
	app := newDetailsTestApp(t)
	app.selectedIssue = detailsFixture()
	app.updateDetailsView()
	focusCommentCards(app)
	drawComments(t, app, 80)

	stepComments(t, app, false)
	for _, key := range []rune{'r', 'Q', 'y', 'o'} {
		pressInComments(t, app, key)
	}

	if got := app.focusedCommentID; got != blockIDCompose {
		t.Errorf("Tab landed on %q, want the compose card", got)
	}
	if got := app.replyParentID(); got != "" {
		t.Errorf("the box is aimed at %q with no comments to answer", got)
	}
}

func TestAReplyStaysWithItsOwnIssue(t *testing.T) {
	app := newThreadedTestApp(t)
	stepComments(t, app, false)
	pressInComments(t, app, 'r')
	typeRunes(t, app, "meant for the first issue")

	second := detailsFixture()
	second.ID, second.Identifier = "issue-2", "ZNL-2"
	second.Comments = threadedComments()
	app.selectedIssue = second
	app.updateDetailsView()

	if got := app.detailsReplyArea.GetText(); got != "" {
		t.Errorf("the second issue's reply box holds %q", got)
	}
	if got := app.replyParentID(); got != "" {
		t.Errorf("the second issue has a box open on %q", got)
	}
	if app.composeBoxActive() {
		t.Error("the keyboard stayed in a reply box the new page does not draw")
	}
	if app.detailsFocus == detailsFocusReply || app.detailsFocus == detailsFocusReplyPost {
		t.Errorf("the ring still points at a reply box: %v", app.detailsFocus)
	}

	first := detailsFixture()
	first.Comments = threadedComments()
	app.selectedIssue = first
	app.updateDetailsView()
	pressInComments(t, app, 'r')
	if got := app.detailsReplyArea.GetText(); got != "meant for the first issue" {
		t.Errorf("the first issue's box came back holding %q", got)
	}
}

func TestAnUndrawnBoxIsNoMouseTarget(t *testing.T) {
	app := newThreadedTestApp(t)
	showComments(t, app, 80, 12)
	app.detailsPageView.ScrollToEnd()
	showComments(t, app, 80, 12)

	_, _, width, height := app.detailsComposeArea.GetRect()
	if width == 0 || height == 0 {
		t.Fatal("the compose area was not drawn at the end of the page")
	}

	app.detailsPageView.ScrollToBeginning()
	showComments(t, app, 80, 12)

	if _, _, width, height := app.detailsComposeArea.GetRect(); width != 0 || height != 0 {
		t.Errorf("the scrolled-away box kept a %dx%d rectangle to be clicked in", width, height)
	}
}

func TestABoxCutOffAtTheTopIsNotRedrawn(t *testing.T) {
	app := newThreadedTestApp(t)
	stepComments(t, app, false)
	pressInComments(t, app, 'r')
	showComments(t, app, 80, 12)

	var area pageSlot
	for _, slot := range app.detailsPage.slots {
		if slot.primitive == app.detailsReplyArea {
			area = slot
		}
	}
	if area.height == 0 {
		t.Fatal("the compose area has no slot on the page")
	}
	app.detailsPageView.ScrollTo(area.row+1, 0)
	showComments(t, app, 80, 12)

	if _, _, width, height := app.detailsReplyArea.GetRect(); width != 0 || height != 0 {
		t.Errorf("a box cut off at the top drew %dx%d, want it to wait until it fits", width, height)
	}
}

func TestOnlyOneCardIsLitAtATime(t *testing.T) {
	app := newThreadedTestApp(t)
	stepComments(t, app, false)
	stepComments(t, app, false)

	pressInComments(t, app, 'r')

	if got := app.focusedCommentID; got != blockIDReply {
		t.Errorf("the ring names %q, want the box it just opened", got)
	}
	lit := 0
	for _, span := range app.commentSpans {
		if strings.Contains(cardTextFor(t, app, span.id), app.themeTags.BorderFocus) {
			lit++
		}
	}
	if lit != 2 {
		t.Errorf("%d cards carry the focus color, want the box alone", lit)
	}
	if card := cardTextFor(t, app, "reply-1"); strings.Contains(card, app.themeTags.BorderFocus) {
		t.Errorf("the card the reader came from is still lit:\n%s", card)
	}
	if card := cardTextFor(t, app, "root-1"); strings.Contains(card, app.themeTags.Accent) {
		t.Errorf("the comment being answered took a second color:\n%s", card)
	}
}

func TestQuotingTwiceKeepsBothQuotes(t *testing.T) {
	app := newThreadedTestApp(t)
	stepComments(t, app, false)
	pressInComments(t, app, 'Q')
	typeRunes(t, app, "mine")

	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	stepComments(t, app, false)
	stepComments(t, app, false)
	pressInComments(t, app, 'Q')

	got := app.detailsReplyArea.GetText()
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("the box holds %q, want one blank line between the quotes", got)
	}
	first := strings.Index(got, "> The debounce is the problem.")
	mine := strings.Index(got, "mine")
	second := strings.Index(got, "> The detail one.")
	if first < 0 || mine < 0 || second < 0 {
		t.Fatalf("the box holds %q, want both quotes and the words between them", got)
	}
	if first >= mine || mine >= second {
		t.Errorf("the box holds %q, want the new quote under what was written", got)
	}
}

func TestAnUnwrittenQuoteDoesNotFollowYou(t *testing.T) {
	app := newThreadedTestApp(t)
	stepComments(t, app, false)
	pressInComments(t, app, 'Q')
	typeInCompose(t, app, tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	stepComments(t, app, false)
	stepComments(t, app, false)
	pressInComments(t, app, 'Q')

	got := app.detailsReplyArea.GetText()
	if strings.Contains(got, "The debounce is the problem.") {
		t.Errorf("the box holds %q, want only the comment just quoted", got)
	}
	if !strings.Contains(got, "> The detail one.") {
		t.Errorf("the box holds %q, want the comment just quoted", got)
	}
}

func TestClosingAnOverlayLeavesTheDetailsPaneWhereItWas(t *testing.T) {
	app := newCommentsTestApp(t)
	showComments(t, app, 80, 24)
	app.detailsHidden = false
	app.layoutMode = layoutWide
	app.app.SetRoot(app.pages, true)

	app.focusedPane = FocusDetails
	app.rebuildContentLayout()
	app.stepPane(-1)
	if app.focusedPane != FocusIssues {
		t.Fatalf("the step landed on %v, want the issues pane", app.focusedPane)
	}

	app.pickerModal.Show("Set Priority", []PickerItem{{ID: "1", Label: "Urgent"}}, func(PickerItem) {})
	app.pickerModal.Hide()

	if app.focusedPane != FocusIssues {
		t.Fatalf("closing the picker left the pane on %v, want the issues pane it opened from", app.focusedPane)
	}
}
