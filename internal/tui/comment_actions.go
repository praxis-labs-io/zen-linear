package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/rivo/tview"
)

// The page carries a focus ring: the braces step it card by card, through the
// reply box where one is open and on to the compose card at the end, and the
// actions below act on whichever card it is on. The ring is what gives a
// per-comment action something to aim at.

// commentSpan is one stop in the ring and where it landed on the page, in line
// numbers. The ring moves by these and scrolls by them, so nothing has to
// re-measure a card that has already been drawn.
type commentSpan struct {
	// id is the comment's own id, or one of the box names.
	id string
	// focus is what the ring hands the keyboard to on this stop.
	focus detailsFocus
	start int
	end   int
}

// detailsHaveFocus reports whether the page holds the keyboard.
//
// It reads the focus field rather than Application.GetFocus because the render
// path runs from a draw, where taking the app's lock again hangs the process.
func (a *App) detailsHaveFocus() bool {
	return a.focusedPane == FocusDetails
}

// cardsHaveFocus reports whether the ring is on a comment card rather than in
// one of the boxes, which is what the per-comment keys answer from.
func (a *App) cardsHaveFocus() bool {
	return a.detailsHaveFocus() && a.detailsFocus == detailsFocusCards
}

// focusedComment returns the comment the ring is on, and whether there is one
// to act on.
//
// A card scrolled out of the pane is not one: the reader cannot see it, so a
// key that acted on it would answer about something off screen. That is the
// same rule the ring re-anchors by.
func (a *App) focusedComment() (linearapi.Comment, bool) {
	if a.detailsFocus != detailsFocusCards {
		return linearapi.Comment{}, false
	}
	index := a.commentStopIndex()
	if index < 0 || !a.commentSpanVisible(a.commentSpans[index]) {
		return linearapi.Comment{}, false
	}
	return a.commentByID(a.focusedCommentID)
}

// commentByID finds a comment among the ones the page was rendered from.
func (a *App) commentByID(id string) (linearapi.Comment, bool) {
	for _, comment := range a.detailsCommentsSource {
		if comment.ID == id {
			return comment, true
		}
	}
	return linearapi.Comment{}, false
}

// commentStopIndex is where the ring is sitting in the page just rendered, or
// -1 when what it names is no longer on it.
func (a *App) commentStopIndex() int {
	for i, span := range a.commentSpans {
		if span.focus == a.detailsFocus && (span.focus != detailsFocusCards || span.id == a.focusedCommentID) {
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
// It answers false off either end, where the ring stays put rather than
// leaving the pane.
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
	// a card they happen to be looking at would take the keys away from what
	// they were writing.
	if index < 0 || (a.detailsFocus == detailsFocusCards && !a.commentSpanVisible(a.commentSpans[index])) {
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
// lit. Nothing is lit until a brace says so, and Escape puts it back that way.
func (a *App) clearCommentFocus() bool {
	if a.focusedCommentID == "" {
		return false
	}
	a.focusedCommentID = ""
	a.renderDetailsPage()
	a.updateStatusBar()
	return true
}

// focusCommentAt puts the ring on the stop at index and brings it into view.
func (a *App) focusCommentAt(index int) {
	if index < 0 || index >= len(a.commentSpans) {
		return
	}
	span := a.commentSpans[index]
	if a.focusedCommentID != span.id || a.detailsFocus != span.focus {
		a.focusedCommentID, a.detailsFocus = span.id, span.focus
		// Re-rendered rather than repainted: the border lives in the text, so
		// the ring only moves when the page is written again. The spans are
		// rebuilt by the same call, so the scroll below reads the new ones.
		a.renderDetailsPage()
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
	a.detailsFocus = detailsFocusCards
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
	focus  detailsFocus
	id     string
}

// commentRing is the ring as it stands now, to be compared against what the
// page is currently showing.
func (a *App) commentRing() commentPaint {
	return commentPaint{active: a.detailsHaveFocus(), focus: a.detailsFocus, id: a.focusedCommentID}
}

// refreshCommentRing repaints the page when the ring has moved or has gained or
// lost the keyboard. The border and the hints live in the card text, so a focus
// change shows nothing until the page is written again, and rewriting it on
// every focus change would redraw a hundred cards to change none of them.
func (a *App) refreshCommentRing() {
	// Guarded on the page, not on the comments: an issue nobody has written on
	// still draws the compose card, and that card takes the focus border and
	// its hints like any other.
	if a.detailsPageView == nil || len(a.commentSpans) == 0 {
		return
	}
	if a.commentRing() == a.commentPainted {
		return
	}
	a.renderDetailsPage()
}

// scrollCommentIntoView scrolls the stack the least it can to show a card,
// showing the top of one taller than the pane.
func (a *App) scrollCommentIntoView(span commentSpan) {
	a.scrollRowsIntoView(span.start, span.end)
}

// scrollRowsIntoView scrolls the page the least it can to show a run of rows,
// showing the top of a run taller than the pane.
func (a *App) scrollRowsIntoView(start, end int) {
	height := viewHeight(a.detailsPageView)
	if height <= 0 {
		return
	}
	row, column := a.detailsPageView.GetScrollOffset()
	switch {
	case start < row:
		row = start
	case end >= row+height:
		row = min(start, end-height+1)
	default:
		return
	}
	a.detailsPageView.ScrollTo(max(0, row), column)
}

// commentSpanVisible reports whether any row of a card is on screen.
func (a *App) commentSpanVisible(span commentSpan) bool {
	height := viewHeight(a.detailsPageView)
	if height <= 0 {
		// Before the first draw nothing can be off screen, and treating the
		// ring as stranded there would re-anchor it on every key.
		return true
	}
	row, _ := a.detailsPageView.GetScrollOffset()
	return span.end >= row && span.start < row+height
}

// handleCommentKey answers the keys the focused card owns, returning whether it
// took the event. It runs ahead of the pane's command shortcuts, which is what
// lets r mean reply here and refresh everywhere else.
func (a *App) handleCommentKey(event *tcell.EventKey) bool {
	if !a.detailsHaveFocus() || len(a.commentSpans) == 0 {
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
	case a.actionKey("comment_edit", 'e'):
		a.editFocusedComment()
	case a.actionKey("comment_delete", 'd'):
		a.deleteFocusedComment()
	default:
		return false
	}
	return true
}

// editFocusedComment turns the focused card into a box holding what it says.
//
// Linear refuses to rewrite somebody else's comment, and the card offers no
// hint for the key, so a press here is a reader trying it anyway rather than
// one about to succeed.
func (a *App) editFocusedComment() {
	comment, ok := a.focusedComment()
	if !ok {
		return
	}
	if !comment.Author.IsMe {
		a.flashStatus("You can only edit your own comments")
		return
	}
	a.openEditBox(comment.ID, comment.Body)
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
	if quoted == "" {
		return
	}
	// Under what is already written, not over it: the words in the box came
	// first, and the quote is being added to them.
	draft := quoted
	// Trimmed before joining, because the quote above it already ends in the
	// blank line the cursor sits on. Joined untrimmed, a second quote lands
	// three lines below the first instead of one.
	if held := strings.TrimSpace(a.detailsReplyArea.GetText()); held != "" {
		draft = held + "\n\n" + quoted
	}
	fillWritingBox(a.detailsReplyArea, draft)
}

// quoteFocusedComment starts a reply with the comment quoted above it, the way
// a mail client does. The body goes in as markdown, which is what Linear reads
// back on the other end.
func (a *App) quoteFocusedComment() {
	comment, ok := a.focusedComment()
	if !ok {
		return
	}
	// Two newlines: one closes the quote, and one is the blank line the reply
	// gets written on.
	a.replyToFocusedComment(quoteBody(comment.Body) + "\n\n")
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

// deleteFocusedComment takes the focused card off the page, behind the same
// confirmation every other destructive action goes through.
//
// Linear keeps the replies under a deleted comment, and the page draws a reply
// whose parent it does not have as a root, so a thread outlives its root rather
// than going with it.
func (a *App) deleteFocusedComment() {
	comment, ok := a.focusedComment()
	if !ok {
		return
	}
	if !comment.Author.IsMe {
		a.flashStatus("You can only delete your own comments")
		return
	}
	issueID := a.composeDraftIssueID
	if issueID == "" {
		a.flashStatus("No issue selected")
		return
	}
	if _, sent := a.deletingComments[comment.ID]; sent {
		a.flashStatus("Already deleting this comment")
		return
	}

	del := a.deleteCommentFunc
	a.confirmationModal.Show(
		"Delete Comment",
		fmt.Sprintf("Delete this comment?\n\n%s", tview.Escape(commentPreview(comment.Body))),
		"Delete",
		func() {
			if a.deletingComments == nil {
				a.deletingComments = make(map[string]struct{})
			}
			a.deletingComments[comment.ID] = struct{}{}
			a.flashStatus("Deleting comment...")
			go func() {
				err := del(context.Background(), comment.ID)
				a.QueueUpdateDraw(func() {
					delete(a.deletingComments, comment.ID)
					if err != nil {
						logger.ErrorWithErr(err, "tui.comment_actions: delete comment failed comment=%s", comment.ID)
						a.updateStatusBarWithError(err)
						return
					}
					logger.Info("tui.comment_actions: comment deleted comment=%s", comment.ID)
					// A box open on this thread is answering a card that is
					// about to leave the page. Closed here rather than left to
					// go missing: the render places it by its parent, and with
					// the parent gone it would simply stop being drawn.
					if a.replyParentID() == comment.ID {
						a.closeReplyBox()
					}
					// The card stayed actionable while this was out, so a box
					// can have been opened on it. Left open, the render would
					// pull its slot away from a widget still holding the keys.
					if a.editingCommentID() == comment.ID {
						a.closeEditBox()
					}
					a.removeComment(issueID, comment.ID)
					a.flashSuccess("Comment deleted")
				})
			}()
		},
	)
}

// commentPreviewWidth is how much of a body the prompt shows. Enough to tell
// one comment from another, short enough that the modal stays a modal.
const commentPreviewWidth = 60

// commentPreview is the opening line of a body, cut to fit a prompt.
func commentPreview(body string) string {
	line := strings.TrimSpace(body)
	if at := strings.IndexByte(line, '\n'); at >= 0 {
		line = strings.TrimSpace(line[:at])
	}
	if runes := []rune(line); len(runes) > commentPreviewWidth {
		return string(runes[:commentPreviewWidth-1]) + "…"
	}
	return line
}

// removeComment takes a deleted comment off the page and lands the ring on the
// card before it, or on the first one left when it was the first.
//
// A fetch already out was answering about a page that still had this comment on
// it, so it is invalidated here rather than left to put the card back. The
// cancel comes after the issue check: a delete that lands once the user has
// moved on has no business killing the fetch filling in the issue they moved to.
func (a *App) removeComment(issueID, commentID string) {
	a.issuesMu.Lock()
	selected := a.selectedIssue
	if selected == nil || selected.ID != issueID {
		a.issuesMu.Unlock()
		return
	}
	at := -1
	for i, comment := range selected.Comments {
		if comment.ID == commentID {
			at = i
			break
		}
	}
	if at < 0 {
		a.issuesMu.Unlock()
		return
	}
	selected.Comments = append(selected.Comments[:at], selected.Comments[at+1:]...)
	comments := selected.Comments
	neighbor := ""
	switch {
	case at > 0:
		neighbor = comments[at-1].ID
	case len(comments) > 0:
		neighbor = comments[0].ID
	}
	a.issuesMu.Unlock()

	// Read before the render, which is what takes the card off the page.
	held := a.detailsFocus == detailsFocusCards && a.focusedCommentID == commentID
	a.cancelDetailFetch()
	a.detailsCommentsSource = comments
	a.renderDetailsPage()
	// The ring moves only when the card that left was the one holding it. A
	// reader who stepped away, or started writing, keeps where they are.
	if !held {
		return
	}
	// Rendered before the ring is aimed: the spans still hold the card that
	// just left, and a stop looked up in those would be the deleted one.
	if neighbor == "" {
		a.clearCommentFocus()
		return
	}
	a.focusComment(neighbor)
}

// commentAuthorLabel names a comment for a status line, falling back to the
// comment itself where the author did not come back with a name.
func commentAuthorLabel(comment linearapi.Comment) string {
	if name := formatUserDisplayName(comment.Author); name != "" {
		return name
	}
	return "comment"
}
