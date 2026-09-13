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

type commentSpan struct {
	id    string
	focus detailsFocus
	start int
	end   int
}

// Reads the focus field, not Application.GetFocus: the render runs from a draw, which holds the app lock.
func (a *App) detailsHaveFocus() bool {
	return a.focusedPane == FocusDetails
}

func (a *App) cardsHaveFocus() bool {
	return a.detailsHaveFocus() && a.detailsFocus == detailsFocusCards
}

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

func (a *App) commentByID(id string) (linearapi.Comment, bool) {
	for _, comment := range a.detailsCommentsSource {
		if comment.ID == id {
			return comment, true
		}
	}
	return linearapi.Comment{}, false
}

func (a *App) commentStopIndex() int {
	for i, span := range a.commentSpans {
		if span.focus == a.detailsFocus && (span.focus != detailsFocusCards || span.id == a.focusedCommentID) {
			return i
		}
	}
	return -1
}

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

func (a *App) stepCommentRing(step int) bool {
	if len(a.commentSpans) == 0 {
		return false
	}
	index := a.commentStopIndex()
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

func (a *App) clearCommentFocus() bool {
	if a.focusedCommentID == "" {
		return false
	}
	a.focusedCommentID = ""
	a.renderDetailsPage()
	a.updateStatusBar()
	return true
}

func (a *App) focusCommentAt(index int) {
	if index < 0 || index >= len(a.commentSpans) {
		return
	}
	span := a.commentSpans[index]
	if a.focusedCommentID != span.id || a.detailsFocus != span.focus {
		a.focusedCommentID, a.detailsFocus = span.id, span.focus
		a.renderDetailsPage()
		if index = a.commentStopIndex(); index < 0 {
			return
		}
		span = a.commentSpans[index]
	}
	a.scrollCommentIntoView(span)
	a.updateStatusBar()
}

func (a *App) focusComment(id string) {
	a.detailsFocus = detailsFocusCards
	a.focusCommentAt(a.commentSpanIndex(id))
}

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

type commentPaint struct {
	active bool
	focus  detailsFocus
	id     string
}

func (a *App) commentRing() commentPaint {
	return commentPaint{active: a.detailsHaveFocus(), focus: a.detailsFocus, id: a.focusedCommentID}
}

func (a *App) refreshCommentRing() {
	if a.detailsPageView == nil || len(a.commentSpans) == 0 {
		return
	}
	if a.commentRing() == a.commentPainted {
		return
	}
	a.renderDetailsPage()
}

func (a *App) scrollCommentIntoView(span commentSpan) {
	a.scrollRowsIntoView(span.start, span.end)
}

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

func (a *App) commentSpanVisible(span commentSpan) bool {
	height := viewHeight(a.detailsPageView)
	if height <= 0 {
		return true
	}
	row, _ := a.detailsPageView.GetScrollOffset()
	return span.end >= row && span.start < row+height
}

func (a *App) handleCommentKey(event *tcell.EventKey) bool {
	if !a.detailsHaveFocus() || len(a.commentSpans) == 0 {
		return false
	}
	if event.Key() != tcell.KeyRune {
		return false
	}
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
	draft := quoted
	if held := strings.TrimSpace(a.detailsReplyArea.GetText()); held != "" {
		draft = held + "\n\n" + quoted
	}
	fillWritingBox(a.detailsReplyArea, draft)
}

func (a *App) quoteFocusedComment() {
	comment, ok := a.focusedComment()
	if !ok {
		return
	}
	a.replyToFocusedComment(quoteBody(comment.Body) + "\n\n")
}

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
					if a.replyParentID() == comment.ID {
						a.closeReplyBox()
					}
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

const commentPreviewWidth = 60

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

	held := a.detailsFocus == detailsFocusCards && a.focusedCommentID == commentID
	a.cancelDetailFetch()
	a.detailsCommentsSource = comments
	a.renderDetailsPage()
	if !held {
		return
	}
	if neighbor == "" {
		a.clearCommentFocus()
		return
	}
	a.focusComment(neighbor)
}

func commentAuthorLabel(comment linearapi.Comment) string {
	if name := formatUserDisplayName(comment.Author); name != "" {
		return name
	}
	return "comment"
}
