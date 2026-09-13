package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

func TestTheDetailsPaneIsOneScrollingPage(t *testing.T) {
	app := newThreadedTestApp(t)
	page := strings.Join(drawComments(t, app, 90), "\n")

	at := 0
	for _, want := range []string{
		"ZNO-5",
		"Status:",
		"─────",
		"Description:",
		"The debounce is the problem",
		"write a comment",
	} {
		found := strings.Index(page[at:], want)
		if found < 0 {
			t.Fatalf("the page is missing %q after row %d:\n%s", want, at, page)
		}
		at += found + len(want)
	}
}

func TestThePaletteSurvivesAKeystrokeFromTheDetailsPane(t *testing.T) {
	app := newThreadedTestApp(t)
	app.openPalette()

	app.app.SetFocus(app.detailsPageView)

	if got := app.focusedPane; got != FocusPalette {
		t.Errorf("focus delegated to the details page took the pane to %v, want the palette to keep it", got)
	}
	if got := app.detailsFocus; got != detailsFocusCards {
		t.Errorf("the delegation moved the box focus to %v, want it untouched", got)
	}
}

func TestTabInAnotherPaneLeavesTheWritingBoxesAlone(t *testing.T) {
	app := newThreadedTestApp(t)
	app.detailsFocus = detailsFocusText
	app.focusedPane = FocusIssues

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))

	if got := app.detailsFocus; got != detailsFocusText {
		t.Errorf("Tab in the issues pane moved the box focus to %v, want it untouched", got)
	}
	if got := app.focusedPane; got != FocusIssues {
		t.Errorf("Tab in the issues pane landed on %v, want it to stay put", got)
	}
}

func TestTheActivityLabelHeadsTheSection(t *testing.T) {
	app := newThreadedTestApp(t)

	page := strings.Join(drawComments(t, app, 90), "\n")
	if !strings.Contains(page, "Activity") {
		t.Errorf("the page does not head the feed with Activity:\n%s", page)
	}
	if strings.Contains(page, "Comments (") {
		t.Errorf("the heading still carries a comment count:\n%s", page)
	}

	app.appendComment(app.selectedIssue.ID, linearapi.Comment{ID: "new", Body: "one more"})

	page = strings.Join(drawComments(t, app, 90), "\n")
	if !strings.Contains(page, "one more") {
		t.Errorf("a posted comment did not reach the feed:\n%s", page)
	}
}

func TestTheDescriptionScrollsWithTheComments(t *testing.T) {
	app := newThreadedTestApp(t)
	showComments(t, app, 80, 20)

	app.detailsPageView.ScrollToEnd()
	page := strings.Join(showComments(t, app, 80, 20), "\n")

	if strings.Contains(page, "Status:") {
		t.Errorf("the metadata is still on screen at the end of the page:\n%s", page)
	}
	if !strings.Contains(page, "write a comment") {
		t.Errorf("the compose card is not on screen at the end of the page:\n%s", page)
	}
}

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
	if got := app.detailsFocus; got != detailsFocusCards {
		t.Errorf("the sub-focus is %v with no issue selected, want the cards", got)
	}
	if got := app.detailsPageView.GetText(true); strings.Contains(got, "write a comment") {
		t.Errorf("the empty pane still draws the compose card:\n%s", got)
	}
	if len(app.detailsPage.slots) != 0 {
		t.Error("the boxes are still slotted on an empty page")
	}
}

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

func TestAStyledDescriptionLineDoesNotBleedDownThePage(t *testing.T) {
	app := newDetailsTestApp(t)
	issue := detailsFixture()
	issue.Description = "> A quote above it.\n\n" +
		"[a link whose text is long enough that it has to wrap onto a second row of the pane before it ends](https://example.com/a)\n\n" +
		"![image.png](https://uploads.linear.app/9e76ca0c-0fe9-4629-b86a-5a701c1c48de/51d67805-0f7c-492b-aea5-e6681d89bc2e)"
	issue.Comments = threadedComments()
	app.selectedIssue = issue
	app.updateDetailsView()

	const width, height = 105, 60
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(width, height)
	app.detailsView.SetRect(0, 0, width, height)
	app.detailsView.Draw(screen)
	screen.Show()

	cells, screenWidth, screenHeight := screen.GetContents()
	below := false
	for y := 0; y < screenHeight; y++ {
		row := make([]rune, 0, screenWidth)
		styled := ""
		for x := 0; x < screenWidth; x++ {
			cell := cells[y*screenWidth+x]
			if cell.Style.GetUnderlineStyle() != tcell.UnderlineStyleNone {
				styled = "underlined"
			}
			if _, _, attr := cell.Style.Decompose(); attr&tcell.AttrBold != 0 {
				styled = "bold"
			}
			if len(cell.Runes) == 0 || cell.Runes[0] == 0 {
				row = append(row, ' ')
				continue
			}
			row = append(row, cell.Runes[0])
		}
		line := strings.TrimRight(string(row), " ")
		if strings.Contains(line, "Activity") {
			below = true
		}
		if below && styled != "" {
			t.Errorf("row %d is %s below the description: %q", y, styled, line)
		}
	}
}
