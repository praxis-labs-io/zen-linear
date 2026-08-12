package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// The page carries a focus ring: Tab steps it card by card, through the reply
// box where one is open and on to the compose card at the end, and the actions
// below act on whichever card it is on. The ring is what gives a per-comment
// action something to aim at.

// commentSpan is one stop in the ring and where it landed on the page, in line
// numbers. The ring moves by these and scrolls by them, so nothing has to
// re-measure a card that has already been drawn.
type commentSpan struct {
	// id is the comment's own id, or one of the box names.
	id string
	// focus is what the ring hands the keyboard to on this stop.
	focus commentsFocus
	start int
	end   int
}

// commentsHaveFocus reports whether the page holds the keyboard.
//
// It reads the focus fields rather than Application.GetFocus because the render
// path runs from a draw func, where taking the app's lock again hangs the
// process.
func (a *App) commentsHaveFocus() bool {
	return a.focusedPane == FocusDetails && a.detailsCommentsVisible && a.focusedDetailsView
}

// cardsHaveFocus reports whether the ring is on a comment card rather than in
// one of the boxes, which is what the per-comment keys answer from.
func (a *App) cardsHaveFocus() bool {
	return a.commentsHaveFocus() && a.commentsFocus == commentsFocusCards
}

// focusedComment returns the comment the ring is on, and whether there is one
// to act on.
//
// A card scrolled out of the pane is not one: the reader cannot see it, so a
// key that acted on it would answer about something off screen. That is the
// same rule the ring re-anchors by.
func (a *App) focusedComment() (linearapi.Comment, bool) {
	if a.commentsFocus != commentsFocusCards {
		return linearapi.Comment{}, false
	}
	index := a.commentStopIndex()
	if index < 0 || !a.commentSpanVisible(a.commentSpans[index]) {
		return linearapi.Comment{}, false
	}
	for _, comment := range a.detailsCommentsSource {
		if comment.ID == a.focusedCommentID {
			return comment, true
		}
	}
	return linearapi.Comment{}, false
}

// commentStopIndex is where the ring is sitting in the page just rendered, or
// -1 when what it names is no longer on it.
func (a *App) commentStopIndex() int {
	for i, span := range a.commentSpans {
		if span.focus == a.commentsFocus && (span.focus != commentsFocusCards || span.id == a.focusedCommentID) {
			return i
		}
	}
	return -1
}

// commentSpanIndex finds a block on the page by id, -1 when it is not on it.
func (a *App) commentSpanIndex(id string) int {
	if id == "" {
		return -1
	}
	for i, span := range a.commentSpans {
		if span.id == id {
			return i
		}
	}
	return -1
}

// stepCommentRing moves the ring one stop and reports whether it took the step.
// It answers false off either end, where Tab stays put rather than leaving the
// pane.
//
// A ring with no stop, or one that has been scrolled off screen, anchors to
// what is on screen instead of stepping. A reader who scrolled away has moved
// on, and hauling them back to the card they left is the one thing the ring
// must not do.
func (a *App) stepCommentRing(step int) bool {
	if len(a.commentSpans) == 0 {
		return false
	}
	index := a.commentStopIndex()
	// Only a card is given up for being off screen. A box holding the keyboard
	// is where the reader is whatever the scroll says, and stepping off it to
	// a card they happen to be looking at would take Tab away from the button
	// that sends what they just wrote.
	if index < 0 || (a.commentsFocus == commentsFocusCards && !a.commentSpanVisible(a.commentSpans[index])) {
		a.focusCommentAt(a.anchorComment(step))
		return true
	}
	next := index + step
	if next < 0 || next >= len(a.commentSpans) {
		return false
	}
	a.focusCommentAt(next)
	return true
}

// clearCommentFocus takes the ring off every card, reporting whether one was
// lit. Nothing is lit until Tab says so, and Escape puts it back that way.
func (a *App) clearCommentFocus() bool {
	if a.focusedCommentID == "" {
		return false
	}
	a.focusedCommentID = ""
	a.renderDetailsComments()
	a.updateStatusBar()
	return true
}

// focusCommentAt puts the ring on the stop at index and brings it into view.
func (a *App) focusCommentAt(index int) {
	if index < 0 || index >= len(a.commentSpans) {
		return
	}
	span := a.commentSpans[index]
	if a.focusedCommentID != span.id || a.commentsFocus != span.focus {
		a.focusedCommentID, a.commentsFocus = span.id, span.focus
		// Re-rendered rather than repainted: the border lives in the text, so
		// the ring only moves when the page is written again. The spans are
		// rebuilt by the same call, so the scroll below reads the new ones.
		a.renderDetailsComments()
		if index = a.commentStopIndex(); index < 0 {
			return
		}
		span = a.commentSpans[index]
	}
	a.scrollCommentIntoView(span)
	a.updateStatusBar()
}

// focusComment puts the ring on a comment by id, for the paths that know the
// comment rather than its place: a posted reply, a restored selection.
func (a *App) focusComment(id string) {
	a.commentsFocus = commentsFocusCards
	a.focusCommentAt(a.commentSpanIndex(id))
}

// anchorComment is where the ring lands when it has no stop to move from: the
// first block on screen going forward, the last going back. Any part of a card
// showing counts, so one taller than the pane is the card it lands on rather
// than a card it skips.
func (a *App) anchorComment(step int) int {
	if step < 0 {
		for i := len(a.commentSpans) - 1; i >= 0; i-- {
			if a.commentSpanVisible(a.commentSpans[i]) {
				return i
			}
		}
		return len(a.commentSpans) - 1
	}
	for i, span := range a.commentSpans {
		if a.commentSpanVisible(span) {
			return i
		}
	}
	return 0
}

// commentPaint is the ring as one render drew it: whether the page held the
// keyboard, and which stop was lit.
type commentPaint struct {
	active bool
	focus  commentsFocus
	id     string
}

// commentRing is the ring as it stands now, to be compared against what the
// page is currently showing.
func (a *App) commentRing() commentPaint {
	return commentPaint{active: a.commentsHaveFocus(), focus: a.commentsFocus, id: a.focusedCommentID}
}

// refreshCommentRing repaints the page when the ring has moved or has gained or
// lost the keyboard. The border and the hints live in the card text, so a focus
// change shows nothing until the page is written again, and rewriting it on
// every focus change would redraw a hundred cards to change none of them.
func (a *App) refreshCommentRing() {
	// Guarded on the page, not on the comments: an issue nobody has written on
	// still draws the compose card, and that card takes the focus border and
	// its hints like any other.
	if a.detailsCommentsView == nil || len(a.commentSpans) == 0 {
		return
	}
	if a.commentRing() == a.commentPainted {
		return
	}
	a.renderDetailsComments()
}

// scrollCommentIntoView scrolls the stack the least it can to show a card,
// showing the top of one taller than the pane.
func (a *App) scrollCommentIntoView(span commentSpan) {
	height := viewHeight(a.detailsCommentsView)
	if height <= 0 {
		return
	}
	row, column := a.detailsCommentsView.GetScrollOffset()
	switch {
	case span.start < row:
		row = span.start
	case span.end >= row+height:
		row = min(span.start, span.end-height+1)
	default:
		return
	}
	a.detailsCommentsView.ScrollTo(max(0, row), column)
}

// commentSpanVisible reports whether any row of a card is on screen.
func (a *App) commentSpanVisible(span commentSpan) bool {
	height := viewHeight(a.detailsCommentsView)
	if height <= 0 {
		// Before the first draw nothing can be off screen, and treating the
		// ring as stranded there would re-anchor it on every key.
		return true
	}
	row, _ := a.detailsCommentsView.GetScrollOffset()
	return span.end >= row && span.start < row+height
}

// handleCommentKey answers the keys the focused card owns, returning whether it
// took the event. It runs ahead of the pane's command shortcuts, which is what
// lets r mean reply here and refresh everywhere else.
func (a *App) handleCommentKey(event *tcell.EventKey) bool {
	if !a.commentsHaveFocus() || len(a.commentSpans) == 0 {
		return false
	}
	if event.Key() != tcell.KeyRune {
		return false
	}
	// With no card lit the pane is what it was before the ring: j and k scroll,
	// and r, y and o are the issue's own keys.
	if _, ok := a.focusedComment(); !ok {
		return false
	}
	switch event.Rune() {
	case a.actionKey("comment_reply", 'r'):
		a.replyToFocusedComment("")
	case a.actionKey("comment_quote", 'Q'):
		a.quoteFocusedComment()
	case a.actionKey("comment_copy_link", 'y'):
		a.copyFocusedCommentLink()
	case a.actionKey("comment_open", 'o'):
		a.openFocusedComment()
	default:
		return false
	}
	return true
}

// replyToFocusedComment opens a box at the end of the focused card's thread and
// hands it the keyboard. quoted is put in the box first when there is any.
//
// The box goes in the thread rather than at the foot of the page: an answer
// written under the conversation it answers keeps the two on screen together,
// which is the whole reason the thread is drawn as a thread.
func (a *App) replyToFocusedComment(quoted string) {
	comment, ok := a.focusedComment()
	if !ok {
		return
	}
	if a.composeDraftIssueID == "" {
		a.flashStatus("No issue selected")
		return
	}
	a.openReplyBox(threadRootID(a.detailsCommentsSource, comment.ID))
	if quoted != "" {
		fillWritingBox(a.detailsReplyArea, joinDrafts(quoted, a.detailsReplyArea.GetText()))
	}
}

// quoteFocusedComment starts a reply with the comment quoted above it, the way
// a mail client does. The body goes in as markdown, which is what Linear reads
// back on the other end.
func (a *App) quoteFocusedComment() {
	comment, ok := a.focusedComment()
	if !ok {
		return
	}
	a.replyToFocusedComment(quoteBody(comment.Body) + "\n")
}

// quoteBody marks every line of a body as quoted, blank lines included: a gap
// left unmarked ends the quote and drops the rest of it back into the reply.
func quoteBody(body string) string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight("> "+line, " ")
	}
	return strings.Join(lines, "\n")
}

func (a *App) copyFocusedCommentLink() {
	comment, ok := a.focusedComment()
	if !ok {
		return
	}
	copyFn := a.copyToClipboardFunc
	if copyFn == nil {
		copyFn = copyToClipboard
	}
	a.runIssueValueAction(comment.URL, "No link for this comment",
		copyFn, fmt.Sprintf("Copied comment link: %s", commentAuthorLabel(comment)))
}

func (a *App) openFocusedComment() {
	comment, ok := a.focusedComment()
	if !ok {
		return
	}
	openFn := a.openURLFunc
	if openFn == nil {
		openFn = openURL
	}
	a.runIssueValueAction(comment.URL, "No link for this comment",
		openFn, fmt.Sprintf("Opened comment by %s", commentAuthorLabel(comment)))
}

// commentAuthorLabel names a comment for a status line, falling back to the
// comment itself where the author did not come back with a name.
func commentAuthorLabel(comment linearapi.Comment) string {
	if name := formatUserDisplayName(comment.Author); name != "" {
		return name
	}
	return "comment"
}
