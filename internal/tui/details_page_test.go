package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// TestTheDetailsPaneIsOneScrollingPage is what the pane is now: the issue, its
// description and the conversation about it, in that order, in one scroll.
func TestTheDetailsPaneIsOneScrollingPage(t *testing.T) {
	app := newThreadedTestApp(t)
	page := strings.Join(drawComments(t, app, 90), "\n")

	at := 0
	for _, want := range []string{
		"ZNO-5",                       // the identifier
		"State:",                      // a metadata field
		"─────",                       // the rule closing the metadata
		"Description:",                // the description's own label
		"The debounce is the problem", // the first comment
		"write a comment",             // the compose card, at the end
	} {
		found := strings.Index(page[at:], want)
		if found < 0 {
			t.Fatalf("the page is missing %q after row %d:\n%s", want, at, page)
		}
		at += found + len(want)
	}
}

// TestThePaletteSurvivesAKeystrokeFromTheDetailsPane is the regression test for
// what folding the tab in cost. The page's focus func used to be reachable only
// while the Comments tab was mounted; it is the whole pane now, so tview's
// focus re-delegation on the palette's per-keystroke page rebuild walked into
// it and took focusedPane back. The palette then failed its own re-show check
// and went dead with nothing on screen to say so.
func TestThePaletteSurvivesAKeystrokeFromTheDetailsPane(t *testing.T) {
	app := newThreadedTestApp(t)
	app.openPalette()

	// What tview does on its own: the palette rebuilds its page on every
	// keystroke, and RemovePage re-delegates focus down the tree to the item
	// buildLayout flagged, which is this pane. Driven directly because the
	// delegation itself needs a running Application to fire.
	app.app.SetFocus(app.detailsPageView)

	if got := app.focusedPane; got != FocusPalette {
		t.Errorf("focus delegated to the details page took the pane to %v, want the palette to keep it", got)
	}
	if got := app.commentsFocus; got != commentsFocusCards {
		t.Errorf("the delegation moved the box focus to %v, want it untouched", got)
	}
}

// TestTabInAnotherPaneLeavesTheWritingBoxesAlone covers the other half of Tab
// narrowing to a box and its button: commentsFocus outlives the pane being
// left, so an unscoped Tab elsewhere stepped a box nobody was looking at.
func TestTabInAnotherPaneLeavesTheWritingBoxesAlone(t *testing.T) {
	app := newThreadedTestApp(t)
	app.commentsFocus = commentsFocusText
	app.focusedPane = FocusIssues

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))

	if got := app.commentsFocus; got != commentsFocusText {
		t.Errorf("Tab in the issues pane moved the box focus to %v, want it untouched", got)
	}
	if got := app.focusedPane; got != FocusIssues {
		t.Errorf("Tab in the issues pane landed on %v, want it to stay put", got)
	}
}

// TestTheCommentCountHeadsTheSection covers the number the tab strip used to
// carry. It heads the comments the way Description: heads the description, and
// it counts what was drawn rather than a total held somewhere else.
func TestTheCommentCountHeadsTheSection(t *testing.T) {
	app := newThreadedTestApp(t)

	page := strings.Join(drawComments(t, app, 90), "\n")
	want := fmt.Sprintf("Comments (%d)", len(app.detailsCommentsSource))
	if !strings.Contains(page, want) {
		t.Errorf("the page does not head the comments with %q:\n%s", want, page)
	}

	app.appendComment(app.selectedIssue.ID, linearapi.Comment{ID: "new", Body: "one more"})

	page = strings.Join(drawComments(t, app, 90), "\n")
	if got := fmt.Sprintf("Comments (%d)", len(app.detailsCommentsSource)); !strings.Contains(page, got) {
		t.Errorf("the count did not follow the posted comment, want %q:\n%s", got, page)
	}
}

// TestTheDescriptionScrollsWithTheComments pins the one scroll. Two views would
// each keep their own offset, and scrolling the conversation would leave the
// issue where it was.
func TestTheDescriptionScrollsWithTheComments(t *testing.T) {
	app := newThreadedTestApp(t)
	showComments(t, app, 80, 20)

	app.detailsPageView.ScrollToEnd()
	page := strings.Join(showComments(t, app, 80, 20), "\n")

	if strings.Contains(page, "State:") {
		t.Errorf("the metadata is still on screen at the end of the page:\n%s", page)
	}
	if !strings.Contains(page, "write a comment") {
		t.Errorf("the compose card is not on screen at the end of the page:\n%s", page)
	}
}

// TestLongDescriptionLinesWrapToTheMeasure is the one that catches the trap in
// merging the two views: the page counts its own lines, so nothing written to
// it may overrun the measure. Glamour wraps prose but cannot break a bare URL
// and does not wrap a code block, and a description that overran would push
// every card and every box below it a row out of place.
func TestLongDescriptionLinesWrapToTheMeasure(t *testing.T) {
	app := newDetailsTestApp(t)
	issue := detailsFixture()
	issue.Description = "https://example.com/a/path/long/enough/that/nothing/can/break/it/anywhere\n\n" +
		"```\nfunc unwrappable() { return \"a fenced line far past any reading measure at all\" }\n```\n"
	issue.Comments = threadedComments()
	app.selectedIssue = issue
	app.updateDetailsView()

	for _, width := range []int{20, 40, 70, 140} {
		drawComments(t, app, width)
		measure, _ := readingMeasure(width)
		for i, line := range strings.Split(app.detailsPageView.GetText(false), "\n") {
			if got := tview.TaggedStringWidth(line); got > measure {
				t.Errorf("width %d: line %d is %d cells in a %d pane: %q", width, i, got, measure, line)
			}
		}
	}
}

// TestTheRingCountsThePageFromTheTop covers the offset the merge introduced:
// the cards start however many rows down the issue took, and the spans the ring
// moves by have to agree with where they landed. Read off the screen, because
// spans that agreed only with themselves is exactly the failure.
func TestTheRingCountsThePageFromTheTop(t *testing.T) {
	app := newThreadedTestApp(t)
	stepComments(t, app, false)

	lines := drawComments(t, app, 90)
	index := app.commentStopIndex()
	if index < 0 {
		t.Fatal("the ring lit no card")
	}
	span := app.commentSpans[index]
	if span.start == 0 {
		t.Error("the first card starts at row 0, want it under the issue")
	}
	if !strings.Contains(lines[span.start], "╭") {
		t.Errorf("the lit card's first row is %q, want the top of a card", lines[span.start])
	}
	if !strings.Contains(lines[span.end], "╰") {
		t.Errorf("the lit card's last row is %q, want the bottom of a card", lines[span.end])
	}
}

// TestTheComposeBoxIsDrawnOverItsHoleBelowTheDescription is the other half of
// that offset: the widgets are placed by the render, not mounted in a layout,
// so a slot counted from the wrong row paints the box over the wrong card.
func TestTheComposeBoxIsDrawnOverItsHoleBelowTheDescription(t *testing.T) {
	app := newThreadedTestApp(t)
	drawPrimitiveAt(t, app.detailsPage, 90, 160)

	index := app.commentSpanIndex(blockIDCompose)
	if index < 0 {
		t.Fatal("the compose card is not on the page")
	}
	span := app.commentSpans[index]

	_, y, _, height := app.detailsComposeArea.GetRect()
	if height == 0 {
		t.Fatal("the compose area was not drawn")
	}
	if y <= span.start || y+height > span.end+1 {
		t.Errorf("the compose area sits at rows %d..%d, want it inside the card's %d..%d",
			y, y+height-1, span.start, span.end)
	}
}

// TestBracesAnchorFromTheDescription covers the reader who has scrolled up into
// the issue with nothing lit. The ring has no stop to step from, so it takes
// the first card they can see rather than hauling them somewhere else.
func TestBracesAnchorFromTheDescription(t *testing.T) {
	app := newThreadedTestApp(t)
	showComments(t, app, 80, 16)
	app.detailsPageView.ScrollToBeginning()
	if got := app.focusedCommentID; got != "" {
		t.Fatalf("the page opened with %q lit", got)
	}

	stepComments(t, app, false)

	if got := app.focusedCommentID; got == "" {
		t.Error("} from the description lit nothing")
	}
	if index := app.commentStopIndex(); index < 0 || !app.commentSpanVisible(app.commentSpans[index]) {
		t.Error("} lit a card it did not scroll onto the screen")
	}
}

// TestTheEmptyPaneSaysSoAndDropsTheRing covers clearing the selection: the page
// has nothing to write on and nothing to write in, so the ring cannot be left
// aimed at a card that is no longer drawn.
func TestTheEmptyPaneSaysSoAndDropsTheRing(t *testing.T) {
	app := newThreadedTestApp(t)
	stepComments(t, app, false)

	app.issuesMu.Lock()
	app.selectedIssue = nil
	app.issuesMu.Unlock()
	app.updateDetailsView()

	if got := app.focusedCommentID; got != "" {
		t.Errorf("the ring is still on %q with no issue selected", got)
	}
	if got := app.commentsFocus; got != commentsFocusCards {
		t.Errorf("the sub-focus is %v with no issue selected, want the cards", got)
	}
	if got := app.detailsPageView.GetText(true); strings.Contains(got, "write a comment") {
		t.Errorf("the empty pane still draws the compose card:\n%s", got)
	}
	if len(app.detailsPage.slots) != 0 {
		t.Error("the boxes are still slotted on an empty page")
	}
}

// TestAWritingBoxDropsItsFrameInANarrowPane covers a pane too small to frame a
// card. The box keeps its rows and its button; only the border goes, because
// two border cells and two pad cells out of a pane this size leave nothing to
// write in, and a frame drawn anyway ran past the measure.
func TestAWritingBoxDropsItsFrameInANarrowPane(t *testing.T) {
	app := newDetailsTestApp(t)
	app.selectedIssue = detailsFixture()
	app.updateDetailsView()

	for _, width := range []int{0, 1, 3, 5, 11} {
		lines := drawComments(t, app, width)
		for i, line := range lines {
			if strings.ContainsAny(line, "╭╰│") {
				t.Errorf("width %d: row %d frames a card in a pane too narrow for one: %q", width, i, line)
			}
		}
	}
}
