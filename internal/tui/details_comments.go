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
	a.commentPainted = a.commentRing()

	width := a.detailsCommentsFittedWidth
	if width <= 0 {
		width = commentCardFallbackWidth
	}

	blocks := a.commentBlocks()
	var lines []string
	var slots []pageSlot
	if len(a.detailsCommentsSource) == 0 {
		// The empty state is a line above the compose card, not instead of the
		// page: an issue nobody has written on is the one most likely to be
		// written on, and a page with no card to write in leaves the keyboard
		// in a box that was never drawn.
		lines = append(lines, wrapTagged(fmt.Sprintf("%sNo comments yet.[-]", a.themeTags.SecondaryText), width)...)
		lines = append(lines, "")
	}
	for i, block := range blocks {
		if i > 0 {
			// The rail runs through the gap above a reply, which is what joins
			// it to the card it answers rather than leaving it floating.
			lines = append(lines, a.threadGapLine(blocks, i))
		}
		inset := block.depth * commentThreadIndent
		start := len(lines)

		card, boxes := a.blockCard(block, width-inset)
		if block.depth == 0 {
			lines = append(lines, card...)
		} else {
			lines = append(lines, a.threadBranch(card, isLastReply(blocks, i))...)
		}
		for _, box := range boxes {
			box.row += start
			box.column += inset
			slots = append(slots, box)
		}

		span := commentSpan{id: block.id, focus: block.focus, start: start, end: len(lines) - 1}
		a.commentSpans = append(a.commentSpans, span)
		// A box is two stops in the ring, the writing and the button, sharing
		// the card they are drawn in.
		if button, ok := postFocusFor(block.focus); ok {
			span.focus = button
			a.commentSpans = append(a.commentSpans, span)
		}
	}
	a.detailsCommentsView.SetText(strings.Join(lines, "\n") + a.trailingPad())
	if a.detailsCommentsPage != nil {
		a.detailsCommentsPage.setSlots(slots)
	}
}

// blockCard renders one block of the page and reports the widgets that go in
// the holes it left: a comment's card has none, a box's card has its writing
// area and its button.
func (a *App) blockCard(block commentBlock, width int) ([]string, []pageSlot) {
	if block.focus == commentsFocusCards {
		return a.commentCard(block.comment, width), nil
	}
	area, post := a.detailsComposeArea, a.detailsComposePost
	heading := "write a comment"
	if block.focus == commentsFocusReply {
		area, post = a.detailsReplyArea, a.detailsReplyPost
		heading = "write a reply"
	}
	return a.writingCard(width, heading, a.commentBorderTag(block.id), block.focus, area, post)
}

// writingCard frames a box the way a comment is framed, so what is being
// written sits among what has been said rather than beside it. The interior
// rows are left blank for the text area drawn over them.
func (a *App) writingCard(width int, heading, border string, focus commentsFocus, area *tview.TextArea, post *tview.Button) ([]string, []pageSlot) {
	inner := width - commentCardChrome
	lines := []string{
		cardEdge("╭", "╮", width, border),
		cardRow(a.writingByline(heading), inner, border),
		cardEdge("├", "┤", width, border),
	}
	for i := 0; i < composeRows; i++ {
		lines = append(lines, cardRow("", inner, border))
	}
	// The hint rides beside the button rather than in the border below it: it
	// names what sends the words, and it belongs on the row with the control
	// that does the sending.
	label := len([]rune(postLabel))
	lines = append(lines,
		cardRow(a.writingHints(focus, max(inner-label-1, 0)), inner, border),
		cardEdge("╰", "╯", width, border))

	// The chrome is a border cell and a pad cell, so the writing starts two in.
	const cardInset = commentCardChrome / 2
	return lines, []pageSlot{
		{primitive: area, row: 3, height: composeRows, column: cardInset, width: max(inner, 0)},
		{primitive: post, row: 3 + composeRows, height: 1, column: cardInset + max(inner-label, 0), width: min(label, max(inner, 0))},
	}
}

// writingHints names what a box answers to, for as long as the keys are going
// to it. A box nobody is writing in says nothing: the keys named there would be
// the reader's, and they are not.
func (a *App) writingHints(focus commentsFocus, width int) string {
	button, _ := postFocusFor(focus)
	if !a.commentsHaveFocus() || (a.commentsFocus != focus && a.commentsFocus != button) {
		return ""
	}
	done := "esc done"
	if focus == commentsFocusReply {
		done = "esc close"
	}
	post := "ctrl+enter post"
	if a.commentsFocus == button {
		post = "enter post"
	}
	return a.themeTags.SecondaryText + cardHintLine(width, post, "tab post button", done) + "[-]"
}

// cardHintLine joins hints for a card, dropping them from the right until what
// is left fits. A hint that overran would push the border out of true.
func cardHintLine(width int, parts ...string) string {
	for len(parts) > 0 {
		if line := strings.Join(parts, " · "); len(line) <= width {
			return line
		}
		parts = parts[:len(parts)-1]
	}
	return ""
}

// writingByline heads a box with who is writing and what they are writing,
// matching the byline a comment card carries.
func (a *App) writingByline(what string) string {
	parts := make([]string, 0, 2)
	if user := a.GetCurrentUser(); user != nil {
		if name := formatUserDisplayName(*user); name != "" {
			parts = append(parts, a.themeTags.AssigneeText+name+" (me)"+a.themeTags.SecondaryText)
		}
	}
	parts = append(parts, what)
	return a.themeTags.SecondaryText + strings.Join(parts, " · ") + "[-]"
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

// threadGapLine is the blank row above the block at index, carrying the rail
// when a reply hangs below it.
func (a *App) threadGapLine(blocks []commentBlock, index int) string {
	if blocks[index].depth == 0 {
		return ""
	}
	return a.themeTags.Border + "│[-:-:-]"
}

// isLastReply reports whether the block at index closes its thread. An open
// reply box is the last block of the thread it answers, so the rail's corner
// lands on the box and the comment above it keeps its elbow.
func isLastReply(blocks []commentBlock, index int) bool {
	return index == len(blocks)-1 || blocks[index+1].depth < blocks[index].depth
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
	return append(lines, a.cardFooter(comment.ID, width, border))
}

// cardFooter closes a card, carrying the keys it answers to when the ring is on
// it. The hints ride in the border rather than above it: they are chrome, and a
// row of them inside the card would be a line of the comment's own space spent
// on the reader's keys.
func (a *App) cardFooter(id string, width int, border string) string {
	hints := ""
	if id != "" && id == a.focusedCommentID && a.cardsHaveFocus() {
		// One cell in from each corner, and one more so the run to the right of
		// the hints is a border rather than a gap.
		hints = cardHintLine(width-4, a.commentActionHints()...)
	}
	if hints == "" {
		return cardEdge("╰", "╯", width, border)
	}
	fill := max(0, width-2-len([]rune(hints))-1)
	return border + "╰" + strings.Repeat("─", fill) + "[-:-:-]" +
		a.themeTags.SecondaryText + hints + "[-:-:-]" + border + "─╯[-:-:-]"
}

// commentBorderTag colors one card's frame: the ring's own card in the focus
// color while the page holds the keyboard, and every other card in the border.
//
// One lit border at a time. The card being answered took the accent for a
// while, from when the box sat at the foot of the page and nothing else said
// what it was for; the box is drawn inside the thread now, which says it
// better than a second color on a second card.
func (a *App) commentBorderTag(id string) string {
	if id != "" && id == a.focusedCommentID && a.commentsHaveFocus() {
		return a.themeTags.BorderFocus
	}
	return a.themeTags.Border
}

// commentPlain drops the frame on a pane too narrow to hold one, where the box
// would take a third of every line.
//
// It wraps its own body rather than leaving that to the text view. A line the
// view wraps is one page line drawn as two screen rows, and every slot and span
// below it is then a row out: the boxes paint over the wrong cards and Tab
// lands on them.
func (a *App) commentPlain(comment linearapi.Comment, width int) []string {
	lines := []string{truncateTagged(a.commentByline(comment), width)}
	for _, line := range commentBodyLines(comment.Body, width) {
		lines = append(lines, wrapTagged(line, width)...)
	}
	return lines
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
