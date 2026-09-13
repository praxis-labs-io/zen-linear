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
	commentCardChrome    = 4
	commentCardMinWidth  = 12
	detailsFallbackWidth = 40
)

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
		a.releaseFieldEditor()
		a.detailsFocus, a.focusedCommentID = detailsFocusCards, ""
		a.detailsEdit = detailsEditState{}
		a.detailsPageView.SetText(strings.Repeat("\n", a.density.DetailsPadding.Top) + a.emptyDetailsMessage())
		a.detailsPage.setSlots(nil)
		a.detailsPage.setImages(nil)
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
	lines = append(lines,
		truncateTagged(a.themeTags.SecondaryText+"Activity[-]", width),
		"")
	for i, block := range blocks {
		if i > 0 && (blocks[i-1].event == nil || block.event == nil) {
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

		if block.event != nil {
			continue
		}

		ring := block.focus
		if block.id == blockIDCompose {
			ring = detailsFocusCards
		}
		span := commentSpan{id: block.id, focus: ring, start: start, end: len(lines) - 1}
		a.commentSpans = append(a.commentSpans, span)
		if button, ok := postFocusFor(ring); ok {
			span.focus = button
			a.commentSpans = append(a.commentSpans, span)
		}
	}
	a.detailsPageView.SetText(strings.Join(lines, "\n") + a.trailingPad())
	a.detailsPage.setSlots(slots)
	a.detailsPage.setImages(header.images)
}

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
		area.SetPlaceholder(a.composePrompt())
	}
	return a.writingCard(width, heading, a.commentBorderTag(block.id), block.focus, area, post)
}

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
	label := buttonWidth(post)
	lines = append(lines,
		cardRow(a.writingHints(focus, max(inner-label-1, 0)), inner, border),
		cardEdge("╰", "╯", width, border))

	const cardInset = commentCardChrome / 2
	return lines, []pageSlot{
		{primitive: area, row: 3, height: rows, column: cardInset, width: max(inner, 0)},
		{primitive: post, row: 3 + rows, height: 1, column: cardInset + max(inner-label, 0), width: min(label, max(inner, 0))},
	}
}

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
	if tview.TaggedStringWidth(tail) >= inner {
		rows++
	}
	return max(composeRows, rows)
}

func buttonWidth(post *tview.Button) int {
	if post == nil {
		return len([]rune(postLabel))
	}
	return len([]rune(post.GetLabel()))
}

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
		done, verb = "esc discard", "save"
	}
	send := "ctrl+enter " + verb
	if a.detailsFocus == button {
		send = "enter " + verb
	}
	return a.themeTags.SecondaryText + cardHintLine(width, send, "tab "+verb+" button", done) + "[-]"
}

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

func cardHintLine(width int, parts ...string) string {
	for len(parts) > 0 {
		if line := strings.Join(parts, " · "); len(line) <= width {
			return line
		}
		parts = parts[:len(parts)-1]
	}
	return ""
}

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

func (a *App) threadBranch(card []string, last bool) []string {
	rail := a.themeTags.Border + "│[-:-:-]  "
	elbow, under := a.themeTags.Border+"├─[-:-:-] ", rail
	if last {
		elbow, under = a.themeTags.Border+"╰─[-:-:-] ", strings.Repeat(" ", commentThreadIndent)
	}

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

func (a *App) threadGapLine(blocks []commentBlock, index int) string {
	if blocks[index].depth == 0 {
		return ""
	}
	return a.themeTags.Border + "│[-:-:-]"
}

func isLastReply(blocks []commentBlock, index int) bool {
	return index == len(blocks)-1 || blocks[index+1].depth < blocks[index].depth
}

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

func (a *App) cardFooter(comment linearapi.Comment, width int, border string) string {
	hints := ""
	if comment.ID != "" && comment.ID == a.focusedCommentID && a.cardsHaveFocus() {
		hints = cardHintLine(width-4, a.commentActionHints(comment)...)
	}
	if hints == "" {
		return cardEdge("╰", "╯", width, border)
	}
	fill := max(0, width-2-len([]rune(hints))-1)
	return border + "╰" + strings.Repeat("─", fill) + "[-:-:-]" +
		a.themeTags.SecondaryText + hints + "[-:-:-]" + border + "─╯[-:-:-]"
}

func (a *App) commentBorderTag(id string) string {
	if id != "" && id == a.focusedCommentID && a.detailsHaveFocus() {
		return a.themeTags.BorderFocus
	}
	return a.themeTags.Border
}

func (a *App) commentPlain(comment linearapi.Comment, width int) []string {
	lines := []string{truncateTagged(a.commentByline(comment), width)}
	for _, line := range commentBodyLines(comment.Body, width) {
		lines = append(lines, wrapTagged(line, width)...)
	}
	return lines
}

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

// Linear stamps updatedAt a few milliseconds before createdAt on a new comment.
const commentEditGrace = time.Second

func isCommentEdited(comment linearapi.Comment) bool {
	return comment.UpdatedAt.Sub(comment.CreatedAt) > commentEditGrace
}

func commentBodyLines(body string, width int) []string {
	rendered := strings.Split(renderMarkdownAt(body, width), "\n")
	lines := make([]string, 0, len(rendered))
	for _, line := range rendered {
		lines = append(lines, tview.TranslateANSI(trimRenderedPadding(line)))
	}
	return lines
}

func wrapTagged(line string, width int) []string {
	if wrapped := tview.WordWrap(line, width); len(wrapped) > 0 {
		return wrapped
	}
	return []string{line}
}

func cardEdge(left, right string, width int, borderTag string) string {
	return borderTag + left + strings.Repeat("─", max(0, width-2)) + right + "[-:-:-]"
}

func cardRow(line string, inner int, borderTag string) string {
	return borderTag + "│[-:-:-] " + fitTagged(line, inner) + " " + borderTag + "│[-:-:-]"
}

func fitTagged(line string, width int) string {
	line = truncateTagged(line, width)
	pad := width - tview.TaggedStringWidth(line)
	return line + "[-:-:-]" + strings.Repeat(" ", max(0, pad))
}

// Glamour styles its trailing padding, wrapping each space in an escape, so a plain TrimRight misses it.
var renderedLinePadding = regexp.MustCompile(`(?:\x1b\[[0-9;]*m|[ \t])+$`)

func trimRenderedPadding(line string) string {
	return renderedLinePadding.ReplaceAllString(line, "")
}

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
