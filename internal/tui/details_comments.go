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
// width of the pane's last draw.
func (a *App) renderDetailsComments() {
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

	var lines []string
	for i, comment := range a.detailsCommentsSource {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, a.commentCard(comment, width)...)
	}
	a.detailsCommentsView.SetText(strings.Join(lines, "\n"))
}

// commentCard frames one comment: a byline, a rule under it, and the body
// inside a rounded box.
func (a *App) commentCard(comment linearapi.Comment, width int) []string {
	if width < commentCardMinWidth {
		return a.commentPlain(comment, width)
	}

	border := a.themeTags.Border
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
	if !comment.UpdatedAt.Equal(comment.CreatedAt) {
		parts = append(parts, "edited")
	}
	return a.themeTags.SecondaryText + strings.Join(parts, " · ") + "[-]"
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
// after it lands in the same column on every row. The reset closes whatever
// glamour left open: a bare [-] would clear the foreground and leave a bold or
// a background to bleed onto the padding and the border.
func fitTagged(line string, width int) string {
	line = truncateTagged(line, width)
	// Re-measured rather than assumed: truncateTagged breaks on a word, so
	// what comes back is rarely the full width.
	pad := width - tview.TaggedStringWidth(line)
	return line + "[-:-:-]" + strings.Repeat(" ", max(0, pad))
}

// renderedLinePadding matches glamour's trailing line padding, which is styled
// rather than plain: an escape sequence around each space, so it survives a
// TrimRight on the text. Matched on the ANSI side, where an escape byte is
// unambiguous and a bracket from someone's comment cannot be mistaken for one.
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
