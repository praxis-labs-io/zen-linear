package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

const (
	// commentCardChrome is what a card spends on itself: a border cell and a
	// pad cell either side of the text.
	commentCardChrome = 4
	// commentCardMinWidth is where the frame stops earning its share of the
	// line and the card falls back to plain text.
	commentCardMinWidth = 12
	// commentCardFallbackWidth stands in before the first draw fixes the
	// measure. The draw func's refit re-renders at the real one.
	commentCardFallbackWidth = 40
)

// renderDetailsComments writes the comments tab as a stack of cards at the
// width of the pane's last draw, threads nested under their parent. It records
// where each card landed, which is what the ring moves and scrolls by.
func (a *App) renderDetailsComments() {
	a.commentSpans = nil
	a.commentRingPainted = a.commentsHaveFocus()
	if len(a.detailsCommentsSource) == 0 {
		// Unframed, because refitDetailsComments skips an empty source: an
		// empty state laid out to a width would never see the real one.
		a.detailsCommentsView.SetText(fmt.Sprintf("%sNo comments yet.[-]", a.themeTags.SecondaryText))
		return
	}

	width := a.detailsCommentsFittedWidth
	if width <= 0 {
		width = commentCardFallbackWidth
	}

	rows := buildCommentRows(a.detailsCommentsSource)
	var lines []string
	for i, row := range rows {
		if i > 0 {
			// The rail runs through the gap above a reply, which is what joins
			// it to the card it answers rather than leaving it floating.
			lines = append(lines, a.threadGapLine(rows, i))
		}
		inset := row.Depth * commentThreadIndent
		start := len(lines)
		card := a.commentCard(row.Comment, width-inset)
		if row.Depth == 0 {
			lines = append(lines, card...)
		} else {
			lines = append(lines, a.threadBranch(card, isLastReply(rows, i))...)
		}
		a.commentSpans = append(a.commentSpans, commentSpan{
			id:    row.Comment.ID,
			start: start,
			end:   len(lines) - 1,
		})
	}
	a.detailsCommentsView.SetText(strings.Join(lines, "\n"))
}

// threadBranch hangs a reply off the card above it, drawing the rail down its
// gutter. The last reply closes the run so the rail stops rather than trailing
// into whatever comes next.
func (a *App) threadBranch(card []string, last bool) []string {
	rail := a.themeTags.Border + "│[-:-:-]  "
	elbow, under := a.themeTags.Border+"├─[-:-:-] ", rail
	if last {
		elbow, under = a.themeTags.Border+"╰─[-:-:-] ", strings.Repeat(" ", commentThreadIndent)
	}

	// The elbow meets the byline rather than the top border, which is where a
	// reader's eye goes and where the card's own name sits. A card too narrow
	// to frame starts on that byline, so the elbow moves up with it.
	elbowRow := min(1, len(card)-1)

	out := make([]string, 0, len(card))
	for i, line := range card {
		switch {
		case i == elbowRow:
			out = append(out, elbow+line)
		case i < elbowRow:
			out = append(out, rail+line)
		default:
			out = append(out, under+line)
		}
	}
	return out
}

// threadGapLine is the blank row above the card at index, carrying the rail
// when a reply hangs below it.
func (a *App) threadGapLine(rows []commentRow, index int) string {
	if rows[index].Depth == 0 {
		return ""
	}
	return a.themeTags.Border + "│[-:-:-]"
}

// isLastReply reports whether the reply at index closes its thread.
func isLastReply(rows []commentRow, index int) bool {
	return index == len(rows)-1 || rows[index+1].Depth < rows[index].Depth
}

// commentCard frames one comment: a byline, a rule under it, and the body
// inside a rounded box.
func (a *App) commentCard(comment linearapi.Comment, width int) []string {
	if width < commentCardMinWidth {
		return a.commentPlain(comment, width)
	}

	border := a.commentBorderTag(comment.ID)
	inner := width - commentCardChrome

	lines := []string{
		cardEdge("╭", "╮", width, border),
		cardRow(a.commentByline(comment), inner, border),
		cardEdge("├", "┤", width, border),
	}
	for _, line := range commentBodyLines(comment.Body, inner) {
		for _, row := range wrapTagged(line, inner) {
			lines = append(lines, cardRow(row, inner, border))
		}
	}
	return append(lines, cardEdge("╰", "╯", width, border))
}

// commentBorderTag colors one card's frame: the ring's card in the focus color
// while the stack holds the keyboard, and the card a reply is aimed at in the
// accent, which is what keeps the target on screen once the box's placeholder
// has been typed over.
func (a *App) commentBorderTag(id string) string {
	switch {
	case id == a.focusedCommentID && a.commentsHaveFocus():
		return a.themeTags.BorderFocus
	case id != "" && id == a.replyParentID():
		return a.themeTags.Accent
	default:
		return a.themeTags.Border
	}
}

// commentPlain drops the frame on a pane too narrow to hold one, where the box
// would take a third of every line. The body goes out unfitted for the text
// view to wrap, since there is no border here for a long line to break.
func (a *App) commentPlain(comment linearapi.Comment, width int) []string {
	lines := []string{truncateTagged(a.commentByline(comment), width)}
	return append(lines, commentBodyLines(comment.Body, width)...)
}

// commentByline is the line above a comment: who, what they did, and when.
func (a *App) commentByline(comment linearapi.Comment) string {
	parts := make([]string, 0, 4)
	if author := formatUserDisplayName(comment.Author); author != "" {
		if comment.Author.IsMe {
			author += " (me)"
		}
		parts = append(parts, a.themeTags.AssigneeText+author+a.themeTags.SecondaryText)
	}
	parts = append(parts, "said")
	if at := formatRelativeTime(comment.CreatedAt); at != "" {
		parts = append(parts, at)
	}
	if isCommentEdited(comment) {
		parts = append(parts, "edited")
	}
	return a.themeTags.SecondaryText + strings.Join(parts, " · ") + "[-]"
}

// commentEditGrace is the gap below which an update is the comment's own
// creation rather than an edit. Linear stamps updatedAt a few milliseconds
// before createdAt on a new comment, so inequality marks every one of them.
const commentEditGrace = time.Second

func isCommentEdited(comment linearapi.Comment) bool {
	return comment.UpdatedAt.Sub(comment.CreatedAt) > commentEditGrace
}

// commentBodyLines renders a comment body to card-width rows, already in tview
// tags so the card can measure them.
func commentBodyLines(body string, width int) []string {
	rendered := strings.Split(renderMarkdownAt(body, width), "\n")
	lines := make([]string, 0, len(rendered))
	for _, line := range rendered {
		lines = append(lines, tview.TranslateANSI(trimRenderedPadding(line)))
	}
	return lines
}

// wrapTagged breaks a line that overruns the card. Glamour wraps prose to the
// measure but cannot break a bare URL and does not wrap code blocks or tables,
// and a card that clipped those would lose the tail for good.
func wrapTagged(line string, width int) []string {
	if wrapped := tview.WordWrap(line, width); len(wrapped) > 0 {
		return wrapped
	}
	return []string{line}
}

// cardEdge draws one of the card's horizontal runs, corners included.
func cardEdge(left, right string, width int, borderTag string) string {
	return borderTag + left + strings.Repeat("─", max(0, width-2)) + right + "[-:-:-]"
}

// cardRow frames one line of card content between the box's sides.
func cardRow(line string, inner int, borderTag string) string {
	return borderTag + "│[-:-:-] " + fitTagged(line, inner) + " " + borderTag + "│[-:-:-]"
}

// fitTagged clips a tagged line to width and pads it back out, so the border
// after it lands in the same column on every row. The full reset is what keeps
// a bold or a background glamour left open off the padding and the border.
func fitTagged(line string, width int) string {
	line = truncateTagged(line, width)
	// Re-measured rather than assumed: truncateTagged breaks on a word, so
	// what comes back is rarely the full width.
	pad := width - tview.TaggedStringWidth(line)
	return line + "[-:-:-]" + strings.Repeat(" ", max(0, pad))
}

// renderedLinePadding matches glamour's trailing line padding, which is styled
// rather than plain: an escape sequence around each space, so a TrimRight on
// the text misses it. Matched before translation, where an escape byte is
// unambiguous and a bracket from a comment cannot be mistaken for one.
var renderedLinePadding = regexp.MustCompile(`(?:\x1b\[[0-9;]*m|[ \t])+$`)

// trimRenderedPadding strips the padding glamour lays out to its wrap width.
// The card owns its own, and the line has to be measured without it.
func trimRenderedPadding(line string) string {
	return renderedLinePadding.ReplaceAllString(line, "")
}

// formatRelativeTime renders an age compactly: now, 34m, 5h, 3d, 2y. A future
// timestamp reads as now rather than as a negative number.
func formatRelativeTime(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	since := time.Since(at)
	switch {
	case since < time.Minute:
		return "now"
	case since < time.Hour:
		return fmt.Sprintf("%dm", int(since.Minutes()))
	case since < 24*time.Hour:
		return fmt.Sprintf("%dh", int(since.Hours()))
	case since < 365*24*time.Hour:
		return fmt.Sprintf("%dd", int(since.Hours()/24))
	default:
		return fmt.Sprintf("%dy", int(since.Hours()/24/365))
	}
}
