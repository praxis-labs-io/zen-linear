package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// The card stack carries a focus ring: one card at a time wears the focus
// border, j and k step it, and the actions below act on whichever card it is
// on. The ring is what gives a per-comment action something to aim at.

// commentSpan is where one card landed in the rendered stack, in line numbers.
// The ring moves by these and scrolls by them, so nothing has to re-measure a
// card that has already been drawn.
type commentSpan struct {
	id    string
	start int
	end   int
}

// commentsHaveFocus reports whether the card stack holds the keyboard.
//
// It reads the focus fields rather than Application.GetFocus because the render
// path runs from a draw func, where taking the app's lock again hangs the
// process.
func (a *App) commentsHaveFocus() bool {
	return a.focusedPane == FocusDetails && a.detailsCommentsVisible &&
		a.focusedDetailsView && a.commentsFocus == commentsFocusCards
}

// focusedComment returns the comment the ring is on, and whether there is one
// to act on.
//
// A card scrolled out of the pane is not one: the reader cannot see it, so a
// key that acted on it would answer about something off screen. That is the
// same rule the ring re-anchors by.
func (a *App) focusedComment() (linearapi.Comment, bool) {
	index := a.commentSpanIndex(a.focusedCommentID)
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

// commentSpanIndex finds a comment in the drawn stack, -1 when it is not in it.
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

// stepCommentRing moves the ring one card and reports whether it took the step.
// It answers false off either end, which is what hands Tab on to the compose
// box below the cards.
//
// A ring with no card, or one that has been scrolled off screen, anchors to
// what is on screen instead of stepping. A reader who scrolled away has moved
// on, and hauling them back to the card they left is the one thing the ring
// must not do.
func (a *App) stepCommentRing(step int) bool {
	if len(a.commentSpans) == 0 {
		return false
	}
	index := a.commentSpanIndex(a.focusedCommentID)
	if index < 0 || !a.commentSpanVisible(a.commentSpans[index]) {
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

// focusCommentAt puts the ring on the card at index and brings it into view.
func (a *App) focusCommentAt(index int) {
	if index < 0 || index >= len(a.commentSpans) {
		return
	}
	span := a.commentSpans[index]
	if a.focusedCommentID != span.id {
		a.focusedCommentID = span.id
		// Re-rendered rather than repainted: the border lives in the text, so
		// the ring only moves when the stack is written again. The spans are
		// rebuilt by the same call, so the scroll below reads the new ones.
		a.renderDetailsComments()
		index = a.commentSpanIndex(span.id)
		if index < 0 {
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
	a.focusCommentAt(a.commentSpanIndex(id))
}

// refreshCommentRing repaints the stack when the ring has gained or lost the
// keyboard. The border lives in the card text, so a focus change shows nothing
// until the stack is written again, and rewriting it on every focus change
// would redraw a hundred cards to change none of them.
func (a *App) refreshCommentRing() {
	if a.detailsCommentsView == nil || len(a.detailsCommentsSource) == 0 {
		return
	}
	if a.commentsHaveFocus() == a.commentRingPainted {
		return
	}
	a.renderDetailsComments()
}

// anchorComment is where the ring lands when it has no card to move from: the
// first card on screen going forward, the last going back. Any part of a card
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

// replyToFocusedComment aims the compose box at the focused card's thread and
// hands it the keyboard. quoted is put in the box first when there is any.
func (a *App) replyToFocusedComment(quoted string) {
	comment, ok := a.focusedComment()
	if !ok {
		return
	}
	issueID := a.composeDraftIssueID
	if issueID == "" {
		a.flashStatus("No issue selected")
		return
	}
	a.setReplyTarget(issueID, threadRootID(a.detailsCommentsSource, comment.ID))
	if quoted != "" {
		a.detailsComposeArea.SetText(joinDrafts(quoted, a.detailsComposeArea.GetText()), true)
	}
	a.commentsFocus = commentsFocusText
	a.updateFocus()
	// The target's card wears the accent border from here, and the ring's card
	// gives up the focus one.
	a.renderDetailsComments()
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
