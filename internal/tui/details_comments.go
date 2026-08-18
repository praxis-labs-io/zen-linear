package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

const (
	// commentCardChrome is what a card spends on itself: a border cell and a
	// pad cell either side of the text.
	commentCardChrome = 4
	// commentCardMinWidth is where the frame stops earning its share of the
	// line and the card falls back to plain text.
	commentCardMinWidth = 12
	// detailsFallbackWidth stands in before the first draw fixes the measure.
	// That draw's refit re-renders at the real one.
	detailsFallbackWidth = 40
)

// renderDetailsPage writes the whole details page at the width of the pane's
// last draw: the issue above the rule, its description, then the comment cards
// with threads nested under their parent. It records where each card landed,
// which is what the ring moves and scrolls by.
//
// The header goes into the same line slice the cards are counted in, so every
// span and slot below it is already offset by however many rows the issue took.
// A count kept apart from the lines would have to be recomputed on every refit,
// since the header's height moves with the width.
func (a *App) renderDetailsPage() {
	if a.detailsPageView == nil {
		return
	}
	a.commentSpans = nil
	a.commentPainted = a.commentRing()

	a.detailsFieldSpans = nil
	a.detailsChooserSpan = noChooserSpan
	a.detailsEditorSpan = noEditorSpan
	if a.detailsHeaderRows == nil {
		// No issue: nothing to write on, nothing to write in. The ring is put
		// back where it starts, or it names a card that is no longer drawn.
		a.releaseFieldEditor()
		a.detailsFocus, a.focusedCommentID = detailsFocusCards, ""
		// The field cursor goes with it: there is no header left to point into.
		a.detailsEdit = detailsEditState{}
		// The pane's top padding is text now, so the message carries its own.
		a.detailsPageView.SetText(strings.Repeat("\n", a.density.DetailsPadding.Top) + a.emptyDetailsMessage())
		a.detailsPage.setSlots(nil)
		return
	}

	width := a.detailsMeasureWidth()
	blocks := a.commentBlocks()
	var slots []pageSlot
	header := a.detailsHeaderBlock(width)
	lines := header.lines
	a.detailsFieldSpans = header.fields
	a.detailsChooserSpan = header.chooser
	a.detailsEditorSpan = header.editor
	slots = append(slots, header.slots...)
	lines = append(lines, a.detailsSeam(width)...)
	// The label heads the section the way Description: heads the one above it.
	// It carries no count: the feed holds the issue's own creation, so there is
	// no empty state for a number to report, and a count over comments and
	// events together could not say which it meant.
	lines = append(lines,
		truncateTagged(a.themeTags.SecondaryText+"Activity[-]", width),
		"")
	for i, block := range blocks {
		// A run of events reads as a group without the blank rows between them.
		if i > 0 && (blocks[i-1].event == nil || block.event == nil) {
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

		// An activity line is not a stop. Recording no span is the whole of
		// that: the ring walks spans, and the keys a card answers to are looked
		// up from the span the ring is on.
		if block.event != nil {
			continue
		}

		// The compose card is on every page rather than summoned, so the ring
		// stops on it the way it stops on a comment and the add_comment key is
		// what opens it. A reply or edit box was asked for, so the ring arms it.
		ring := block.focus
		if block.id == blockIDCompose {
			ring = detailsFocusCards
		}
		span := commentSpan{id: block.id, focus: ring, start: start, end: len(lines) - 1}
		a.commentSpans = append(a.commentSpans, span)
		// A box is two stops in the ring, the writing and the button, sharing
		// the card they are drawn in.
		if button, ok := postFocusFor(ring); ok {
			span.focus = button
			a.commentSpans = append(a.commentSpans, span)
		}
	}
	a.detailsPageView.SetText(strings.Join(lines, "\n") + a.trailingPad())
	a.detailsPage.setSlots(slots)
}

// blockCard renders one block of the page and reports the widgets that go in
// the holes it left: a comment's card has none, a box's card has its writing
// area and its button.
func (a *App) blockCard(block commentBlock, width int) ([]string, []pageSlot) {
	if block.event != nil {
		return []string{a.activityLine(*block.event, width)}, nil
	}
	if block.focus == detailsFocusCards {
		return a.commentCard(block.comment, width), nil
	}
	area, post := a.detailsComposeArea, a.detailsComposePost
	heading := "write a comment"
	switch block.focus {
	case detailsFocusReply:
		area, post = a.detailsReplyArea, a.detailsReplyPost
		heading = "write a reply"
	case detailsFocusEdit:
		area, post = a.detailsEditArea, a.detailsEditPost
		heading = "edit this comment"
	case detailsFocusText:
		// Shut, the card invites a comment it will not take a letter of, so it
		// says the key that opens it instead.
		area.SetPlaceholder(a.composePrompt())
	}
	return a.writingCard(width, heading, a.commentBorderTag(block.id), block.focus, area, post)
}

// writingCard frames a box the way a comment is framed, so what is being
// written sits among what has been said rather than beside it. The interior
// rows are left blank for the text area drawn over them, as many as what is in
// the box needs.
func (a *App) writingCard(width int, heading, border string, focus detailsFocus, area *tview.TextArea, post *tview.Button) ([]string, []pageSlot) {
	if width < commentCardMinWidth {
		return a.writingPlain(width, heading, focus, area, post)
	}
	inner := width - commentCardChrome
	rows := writingBoxRows(area, inner)
	lines := []string{
		cardEdge("╭", "╮", width, border),
		cardRow(a.writingByline(heading), inner, border),
		cardEdge("├", "┤", width, border),
	}
	for i := 0; i < rows; i++ {
		lines = append(lines, cardRow("", inner, border))
	}
	// The hint rides beside the button rather than in the border below it: it
	// names what sends the words, and it belongs on the row with the control
	// that does the sending.
	label := buttonWidth(post)
	lines = append(lines,
		cardRow(a.writingHints(focus, max(inner-label-1, 0)), inner, border),
		cardEdge("╰", "╯", width, border))

	// The chrome is a border cell and a pad cell, so the writing starts two in.
	const cardInset = commentCardChrome / 2
	return lines, []pageSlot{
		{primitive: area, row: 3, height: rows, column: cardInset, width: max(inner, 0)},
		{primitive: post, row: 3 + rows, height: 1, column: cardInset + max(inner-label, 0), width: min(label, max(inner, 0))},
	}
}

// writingPlain drops the frame on a pane too narrow to hold one, the way a
// comment does. The box keeps its rows and its button; only the border goes,
// because two border cells and two pad cells out of a pane this size leave
// nothing to write in.
func (a *App) writingPlain(width int, heading string, focus detailsFocus, area *tview.TextArea, post *tview.Button) ([]string, []pageSlot) {
	rows := writingBoxRows(area, width)
	lines := []string{truncateTagged(a.writingByline(heading), width)}
	for i := 0; i < rows; i++ {
		lines = append(lines, "")
	}
	label := min(buttonWidth(post), max(width, 0))
	lines = append(lines, truncateTagged(a.writingHints(focus, max(width-label-1, 0)), width))

	return lines, []pageSlot{
		{primitive: area, row: 1, height: rows, column: 0, width: max(width, 0)},
		{primitive: post, row: 1 + rows, height: 1, column: max(width-label, 0), width: label},
	}
}

// writingBoxRows is how many rows a box needs for what is in it, never fewer
// than composeRows: an empty box is still something to write in.
//
// A box grows with its own text rather than scrolling inside a fixed frame, so
// what has been written is on the page beside what it answers. That is what
// keeps the edit box the size of the card it replaced: it opens holding the
// comment, so it opens at the comment's height.
//
// The text is escaped before it is measured. A TextArea prints what it holds,
// and WordWrap reads a color tag, so an unescaped "[docs](url)" measures four
// cells short of what is drawn and the box comes up a row too small.
func writingBoxRows(area *tview.TextArea, inner int) int {
	if area == nil || inner <= 0 {
		return composeRows
	}
	rows, tail := 0, ""
	for _, line := range strings.Split(area.GetText(), "\n") {
		wrapped := tview.WordWrap(tview.Escape(line), inner)
		rows += max(1, len(wrapped))
		if len(wrapped) > 0 {
			tail = wrapped[len(wrapped)-1]
		} else {
			tail = ""
		}
	}
	// A cursor at the end of a line that exactly fills the measure sits on the
	// row after it, and a TextArea scrolls itself to keep it (findCursor). One
	// row spare is what keeps the top of the box from going.
	if tview.TaggedStringWidth(tail) >= inner {
		rows++
	}
	return max(composeRows, rows)
}

// buttonWidth is how many cells the button's label draws in. It is read off the
// button rather than a constant because the boxes do not all say Post, and a
// width taken from the wrong word puts the slot out of true with the frame.
func buttonWidth(post *tview.Button) int {
	if post == nil {
		return len([]rune(postLabel))
	}
	return len([]rune(post.GetLabel()))
}

// writingHints names what a box answers to, for as long as the keys are going
// to it. A box nobody is writing in says nothing: the keys named there would be
// the reader's, and they are not.
func (a *App) writingHints(focus detailsFocus, width int) string {
	if !a.detailsHaveFocus() {
		return ""
	}
	button, _ := postFocusFor(focus)
	if a.detailsFocus != focus && a.detailsFocus != button {
		return a.closedComposeHint(focus, width)
	}
	done, verb := "esc done", "post"
	switch focus {
	case detailsFocusReply:
		done = "esc close"
	case detailsFocusEdit:
		// Not "close": nothing is kept, and the key that drops a rewrite should
		// say so before it is pressed.
		done, verb = "esc discard", "save"
	}
	send := "ctrl+enter " + verb
	if a.detailsFocus == button {
		send = "enter " + verb
	}
	return a.themeTags.SecondaryText + cardHintLine(width, send, "tab "+verb+" button", done) + "[-]"
}

// closedComposeHint names the key that opens the compose card, for the stop the
// ring makes on it while it is shut. The other two boxes exist only while they
// are open, so this is the one card the ring reaches with nothing to type into.
func (a *App) closedComposeHint(focus detailsFocus, width int) string {
	if focus != detailsFocusText || a.detailsFocus != detailsFocusCards || a.focusedCommentID != blockIDCompose {
		return ""
	}
	key, ok := a.commandShortcutLabel("add_comment")
	if !ok {
		return ""
	}
	return a.themeTags.SecondaryText + cardHintLine(width, key+" write") + "[-]"
}

// composePrompt is what the empty compose box says: what to write once it is
// open, and the key that opens it for as long as it is not.
func (a *App) composePrompt() string {
	if a.detailsFocus == detailsFocusText || a.detailsFocus == detailsFocusPost {
		return composePlaceholder
	}
	key, ok := a.commandShortcutLabel("add_comment")
	if !ok {
		return composePlaceholder
	}
	return fmt.Sprintf("Press %s to leave a comment", key)
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
	return append(lines, a.cardFooter(comment, width, border))
}

// cardFooter closes a card, carrying the keys it answers to when the ring is on
// it. The hints ride in the border rather than above it: they are chrome, and a
// row of them inside the card would be a line of the comment's own space spent
// on the reader's keys.
func (a *App) cardFooter(comment linearapi.Comment, width int, border string) string {
	hints := ""
	if comment.ID != "" && comment.ID == a.focusedCommentID && a.cardsHaveFocus() {
		// One cell in from each corner, and one more so the run to the right of
		// the hints is a border rather than a gap.
		hints = cardHintLine(width-4, a.commentActionHints(comment)...)
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
	if id != "" && id == a.focusedCommentID && a.detailsHaveFocus() {
		return a.themeTags.BorderFocus
	}
	return a.themeTags.Border
}

// commentPlain drops the frame on a pane too narrow to hold one, where the box
// would take a third of every line.
//
// It wraps its own body rather than leaving that to the text view. A line the
// view wraps is one page line drawn as two screen rows, and every slot and span
// below it is then a row out: the boxes paint over the wrong cards and the
// ring lands on them.
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
