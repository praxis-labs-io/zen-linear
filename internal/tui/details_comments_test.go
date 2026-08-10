package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// commentedIssueFixture is an issue whose comments exercise the card: a body
// written a line per thought, one long enough to wrap, an edit, and a comment
// of the signed-in user's own.
func commentedIssueFixture() *linearapi.Issue {
	issue := detailsFixture()
	now := time.Now()
	issue.Comments = []linearapi.Comment{
		{
			ID:        "comment-1",
			Body:      "First line of the comment.\nSecond line, one newline above it.",
			CreatedAt: now.Add(-73 * time.Hour),
			UpdatedAt: now.Add(-73 * time.Hour),
			Author:    linearapi.User{ID: "u1", Name: "Drew White", DisplayName: "drew", IsMe: true},
		},
		{
			ID:        "comment-2",
			Body:      "A body long enough that it has to wrap once the card has taken its own four columns out of the reading measure, and then keep wrapping past that.",
			CreatedAt: now.Add(-5 * time.Hour),
			UpdatedAt: now.Add(-2 * time.Hour),
			Author:    linearapi.User{ID: "u2", Name: "Roey Azroel", DisplayName: "roey"},
		},
	}
	return issue
}

func newCommentsTestApp(t *testing.T) *App {
	t.Helper()

	app := newDetailsTestApp(t)
	app.selectedIssue = commentedIssueFixture()
	app.updateDetailsView()
	return app
}

// drawComments renders the comments tab at a width and returns its rows.
func drawComments(t *testing.T, app *App, width int) []string {
	t.Helper()
	return drawTextView(t, app.detailsCommentsView, width)
}

// commentCards groups the drawn rows into cards, each running from its top
// border to its bottom one. Rows are trimmed of the centering gutter so the
// card's own edges sit at the ends.
func commentCards(lines []string) [][]string {
	var cards [][]string
	var card []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "╭"):
			card = []string{trimmed}
		case card == nil:
		case strings.HasPrefix(trimmed, "╰"):
			cards = append(cards, append(card, trimmed))
			card = nil
		default:
			card = append(card, trimmed)
		}
	}
	return cards
}

// TestCommentsRenderAsCards pins the shape: a byline, a rule under it, and the
// body inside a rounded box that closes.
func TestCommentsRenderAsCards(t *testing.T) {
	app := newCommentsTestApp(t)
	cards := commentCards(drawComments(t, app, 80))

	if len(cards) != 2 {
		t.Fatalf("drew %d cards, want one per comment:\n%s", len(cards), strings.Join(drawComments(t, app, 80), "\n"))
	}

	for i, card := range cards {
		if len(card) < 5 {
			t.Fatalf("card %d has %d rows, want a byline, a rule and a body: %q", i, len(card), card)
		}
		if !strings.HasSuffix(card[0], "╮") {
			t.Errorf("card %d top border = %q, want it to close with ╮", i, card[0])
		}
		if !strings.HasPrefix(card[2], "├") || !strings.HasSuffix(card[2], "┤") {
			t.Errorf("card %d row 2 = %q, want the rule under the byline", i, card[2])
		}
		if !strings.HasSuffix(card[len(card)-1], "╯") {
			t.Errorf("card %d bottom border = %q, want it to close with ╯", i, card[len(card)-1])
		}
		for row, line := range card[1:] {
			if row == 1 || row == len(card)-2 {
				continue
			}
			if !strings.HasPrefix(line, "│") || !strings.HasSuffix(line, "│") {
				t.Errorf("card %d row %d = %q, want it framed by the box sides", i, row+1, line)
			}
		}
		width := len([]rune(card[0]))
		for row, line := range card {
			if got := len([]rune(line)); got != width {
				t.Errorf("card %d row %d is %d cells, want %d to match the top border: %q", i, row, got, width, line)
			}
		}
	}
}

// TestCommentCardsFillTheMeasure covers the card taking the pane's measure: the
// reading cap on a wide pane, the whole pane on a narrow one.
func TestCommentCardsFillTheMeasure(t *testing.T) {
	app := newCommentsTestApp(t)

	cardWidth := func(paneWidth int) int {
		t.Helper()
		cards := commentCards(drawComments(t, app, paneWidth))
		if len(cards) == 0 {
			t.Fatalf("width %d drew no cards", paneWidth)
		}
		return len([]rune(cards[0][0]))
	}

	if got := cardWidth(180); got != detailsMeasure {
		t.Errorf("card is %d cells at pane width 180, want the %d measure", got, detailsMeasure)
	}

	padding := app.density.DetailsPadding
	if want := 60 - 2 - padding.Left - padding.Right; cardWidth(60) != want {
		t.Errorf("card is %d cells at pane width 60, want %d to fill the pane", cardWidth(60), want)
	}
}

// TestCommentCardRefitsOnResize covers a widened pane. Cards laid out at the
// first width would leave a narrow box inside a wide pane.
func TestCommentCardRefitsOnResize(t *testing.T) {
	app := newCommentsTestApp(t)

	drawComments(t, app, 60)
	narrow := len([]rune(commentCards(drawComments(t, app, 60))[0][0]))
	wide := len([]rune(commentCards(drawComments(t, app, 120))[0][0]))

	if wide <= narrow {
		t.Errorf("card is %d cells at pane width 120 and %d at 60, want it to follow the measure", wide, narrow)
	}
}

// TestLongCommentBodyStaysInsideTheCard is the one that catches a padding or
// measure mismatch: a body that wraps must not push a row past the box.
func TestLongCommentBodyStaysInsideTheCard(t *testing.T) {
	app := newCommentsTestApp(t)
	lines := drawComments(t, app, 80)
	cards := commentCards(lines)

	if len(cards) < 2 {
		t.Fatalf("drew %d cards, want the wrapping one:\n%s", len(cards), strings.Join(lines, "\n"))
	}
	wrapping := cards[1]
	if len(wrapping) < 6 {
		t.Fatalf("the long body did not wrap: %q", wrapping)
	}
	// The box sides sit on every row, so they come off before the wrapped
	// sentence can be read back as one.
	body := strings.Join(strings.Fields(strings.ReplaceAll(strings.Join(wrapping, " "), "│", " ")), " ")
	if !strings.Contains(body, "keep wrapping past that.") {
		t.Errorf("the long body did not render in full:\n%s", strings.Join(wrapping, "\n"))
	}
}

// TestCommentBylineIsRelative covers the byline replacing the absolute
// timestamp the pane used to print.
func TestCommentBylineIsRelative(t *testing.T) {
	app := newCommentsTestApp(t)
	cards := commentCards(drawComments(t, app, 80))

	byline := cards[0][1]
	for _, want := range []string{"drew (me)", "·", "said", "3d"} {
		if !strings.Contains(byline, want) {
			t.Errorf("byline = %q, want it to carry %q", byline, want)
		}
	}
	if strings.Contains(byline, "2026") || strings.Contains(byline, "PM") {
		t.Errorf("byline = %q, want a relative time, not an absolute one", byline)
	}
	if edited := cards[1][1]; !strings.Contains(edited, "edited") {
		t.Errorf("byline of the edited comment = %q, want it marked", edited)
	}
}

// TestSingleNewlinesAreHardBreaks covers Linear's markdown, where one newline
// breaks the line. CommonMark folds it into the paragraph.
func TestSingleNewlinesAreHardBreaks(t *testing.T) {
	app := newCommentsTestApp(t)
	card := commentCards(drawComments(t, app, 80))[0]

	first := -1
	for i, line := range card {
		if strings.Contains(line, "First line of the comment.") {
			first = i
		}
	}
	if first < 0 {
		t.Fatalf("the comment body did not render:\n%s", strings.Join(card, "\n"))
	}
	if !strings.Contains(card[first+1], "Second line") {
		t.Errorf("row after the first line = %q, want the second line on its own row", card[first+1])
	}
}

// TestNarrowCommentsPaneDoesNotPanic covers a pane too small to frame a card.
// A negative pad inside a draw func takes the app down with it.
func TestNarrowCommentsPaneDoesNotPanic(t *testing.T) {
	app := newCommentsTestApp(t)

	for _, width := range []int{0, 1, 3, 5, 11, 12, 16, 20} {
		// A card wider than the pane is wrapped by the text view, which shows
		// up as rows of uneven width rather than as an over-long row: the
		// harness only ever reads back what fits on screen.
		for i, card := range commentCards(drawComments(t, app, width)) {
			for row, line := range card {
				if got, want := len([]rune(line)), len([]rune(card[0])); got != want {
					t.Errorf("width %d: card %d row %d is %d cells, want %d: %q", width, i, row, got, want, line)
				}
			}
		}
	}
}

// TestCommentCardKeepsAnUnbreakableLine covers what glamour cannot wrap: a bare
// URL, a code block, a table. Clipped to the card, their tails were lost.
func TestCommentCardKeepsAnUnbreakableLine(t *testing.T) {
	app := newDetailsTestApp(t)
	issue := detailsFixture()
	now := time.Now()
	const url = "https://example.com/a/very/long/path/that/cannot/be/broken/on/a/space/at/all/ever"
	issue.Comments = []linearapi.Comment{{
		ID: "comment-1", CreatedAt: now, UpdatedAt: now,
		Author: linearapi.User{ID: "u1", DisplayName: "drew"},
		Body:   "See " + url + " for details.",
	}}
	app.selectedIssue = issue
	app.updateDetailsView()

	card := commentCards(drawComments(t, app, 60))[0]
	body := strings.Join(strings.Fields(strings.ReplaceAll(strings.Join(card, ""), "│", "")), "")
	if !strings.Contains(body, url) {
		t.Errorf("the URL did not survive the card:\n%s", strings.Join(card, "\n"))
	}
	if strings.Contains(strings.Join(card, ""), "…") {
		t.Errorf("the body was clipped rather than wrapped:\n%s", strings.Join(card, "\n"))
	}
}

// TestUnwrappedMarkdownIsUntouchedByHardBreaks covers the renderer the agent
// output modal uses. Glamour joins soft breaks while wrapping, so at width 0,
// where wrapping is off, hard breaks change nothing: the modal reads the same
// before and after, and it needs no renderer of its own.
func TestUnwrappedMarkdownIsUntouchedByHardBreaks(t *testing.T) {
	initMarkdownRenderer(LinearTheme)

	if got := strings.Count(strings.TrimSpace(renderMarkdown("one\ntwo\nthree")), "\n"); got != 2 {
		t.Errorf("unwrapped render kept %d breaks, want the source's 2", got)
	}
	if got := strings.Count(strings.TrimSpace(renderMarkdownAt("one\ntwo\nthree", 40)), "\n"); got != 2 {
		t.Errorf("render at width 40 kept %d breaks, want Linear's hard breaks", got)
	}
}

// TestCommentsEmptyState covers the issue with no comments. It stays unframed:
// refitDetailsComments skips an empty source, so a card laid out to a width
// would never see the pane's real one.
func TestCommentsEmptyState(t *testing.T) {
	app := newDetailsTestApp(t)

	for _, width := range []int{20, 90} {
		lines := drawComments(t, app, width)
		// Normalized because a narrow pane wraps the message across rows.
		drawn := strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
		if !strings.Contains(drawn, "No comments yet.") {
			t.Errorf("width %d drew no empty state:\n%s", width, strings.Join(lines, "\n"))
		}
		if cards := commentCards(lines); len(cards) != 0 {
			t.Errorf("width %d framed the empty state in %d cards", width, len(cards))
		}
	}
}

// TestIsCommentEdited covers the marker that used to sit on every card. Linear
// stamps updatedAt before createdAt on a new comment, so comparing them for
// inequality called a comment edited the moment it was written.
func TestIsCommentEdited(t *testing.T) {
	created := time.Date(2026, 8, 10, 19, 5, 4, 762_000_000, time.UTC)
	tests := []struct {
		name    string
		updated time.Time
		want    bool
	}{
		{name: "stamped before creation", updated: created.Add(-18 * time.Millisecond)},
		{name: "identical", updated: created},
		{name: "a moment after creation", updated: created.Add(200 * time.Millisecond)},
		{name: "edited minutes later", updated: created.Add(4 * time.Minute), want: true},
		{name: "edited hours later", updated: created.Add(3 * time.Hour), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comment := linearapi.Comment{CreatedAt: created, UpdatedAt: tt.updated}
			if got := isCommentEdited(comment); got != tt.want {
				t.Errorf("isCommentEdited(%v) = %v, want %v", tt.updated.Sub(created), got, tt.want)
			}
		})
	}
}

// TestBylineDoesNotCallANewCommentEdited drives the marker through the pane, on
// the timestamps Linear actually returns.
func TestBylineDoesNotCallANewCommentEdited(t *testing.T) {
	app := newDetailsTestApp(t)
	issue := detailsFixture()
	created := time.Now().Add(-3 * time.Hour)
	issue.Comments = []linearapi.Comment{{
		ID: "comment-1", CreatedAt: created, UpdatedAt: created.Add(-18 * time.Millisecond),
		Author: linearapi.User{ID: "u1", DisplayName: "drew"},
		Body:   "Just written.",
	}}
	app.selectedIssue = issue
	app.updateDetailsView()

	if byline := commentCards(drawComments(t, app, 80))[0][1]; strings.Contains(byline, "edited") {
		t.Errorf("byline = %q, want no edited marker on a comment nobody edited", byline)
	}
}

func TestFormatRelativeTime(t *testing.T) {
	tests := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{name: "seconds", ago: 30 * time.Second, want: "now"},
		{name: "minutes", ago: 34 * time.Minute, want: "34m"},
		{name: "hours", ago: 5 * time.Hour, want: "5h"},
		{name: "days", ago: 73 * time.Hour, want: "3d"},
		{name: "years", ago: 800 * 24 * time.Hour, want: "2y"},
		{name: "future", ago: -time.Hour, want: "now"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatRelativeTime(time.Now().Add(-tt.ago)); got != tt.want {
				t.Errorf("formatRelativeTime(%v ago) = %q, want %q", tt.ago, got, tt.want)
			}
		})
	}

	if got := formatRelativeTime(time.Time{}); got != "" {
		t.Errorf("formatRelativeTime(zero) = %q, want an empty string", got)
	}
}
