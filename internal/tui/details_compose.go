package tui

import (
	"context"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/rivo/tview"
)

const composeRows = 4

const (
	composePlaceholder = "Leave a comment"
	replyPlaceholder   = "Leave a reply"
)

func (a *App) replyParentID() string {
	return a.composeReplyTo[a.composeDraftIssueID]
}

func (a *App) openReplyBox(parentID string) {
	issueID := a.composeDraftIssueID
	if issueID == "" || parentID == "" || a.detailsReplyArea == nil {
		return
	}
	if a.composeReplyTo == nil {
		a.composeReplyTo = make(map[string]string)
	}
	a.holdReplyDraft()
	a.composeReplyTo[issueID] = parentID
	fillWritingBox(a.detailsReplyArea, a.replyDrafts[parentID])
	a.applyComposePlaceholder()

	a.renderDetailsPage()
	a.detailsFocus, a.focusedCommentID = detailsFocusReply, blockIDReply
	a.updateFocus()
	if index := a.commentSpanIndex(blockIDReply); index >= 0 {
		a.scrollCommentIntoView(a.commentSpans[index])
	}
}

func (a *App) editingCommentID() string {
	return a.composeEditing[a.composeDraftIssueID]
}

func (a *App) openEditBox(commentID, body string) {
	issueID := a.composeDraftIssueID
	if issueID == "" || commentID == "" || a.detailsEditArea == nil {
		return
	}
	if a.composeEditing == nil {
		a.composeEditing = make(map[string]string)
	}
	a.composeEditing[issueID] = commentID
	fillWritingBox(a.detailsEditArea, body)

	a.renderDetailsPage()
	a.detailsFocus, a.focusedCommentID = detailsFocusEdit, commentID
	a.updateFocus()
	if index := a.commentSpanIndex(commentID); index >= 0 {
		a.scrollCommentIntoView(a.commentSpans[index])
	}
}

func (a *App) closeEditBox() {
	commentID := a.editingCommentID()
	if commentID == "" {
		return
	}
	delete(a.composeEditing, a.composeDraftIssueID)
	a.detailsEditArea.SetText("", false)
	a.detailsFocus = detailsFocusCards
	a.focusedCommentID = commentID
	a.renderDetailsPage()
	a.updateFocus()
}

func (a *App) dropEditForMissingComment() {
	editing := a.editingCommentID()
	if editing == "" {
		return
	}
	for _, comment := range a.detailsCommentsSource {
		if comment.ID == editing {
			return
		}
	}
	a.closeEditBox()
}

func (a *App) closeReplyBox() {
	parent := a.replyParentID()
	if parent == "" {
		return
	}
	a.holdReplyDraft()
	delete(a.composeReplyTo, a.composeDraftIssueID)
	a.detailsReplyArea.SetText("", false)
	a.detailsFocus = detailsFocusCards
	a.focusedCommentID = parent
	a.renderDetailsPage()
	a.updateFocus()
}

// A box holding only a quote is not held: the app put it there on one keystroke, and kept it stacked in front of the next quote.
func (a *App) holdReplyDraft() {
	parent := a.replyParentID()
	if parent == "" || a.detailsReplyArea == nil {
		return
	}
	body := a.detailsReplyArea.GetText()
	if strings.TrimSpace(body) == "" || isAllQuoted(body) {
		delete(a.replyDrafts, parent)
		return
	}
	if a.replyDrafts == nil {
		a.replyDrafts = make(map[string]string)
	}
	a.replyDrafts[parent] = body
}

func isAllQuoted(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, ">") {
			return false
		}
	}
	return true
}

func (a *App) applyComposePlaceholder() {
	if a.detailsComposeArea == nil {
		return
	}
	a.detailsComposeArea.SetPlaceholder(composePlaceholder)
	if a.detailsReplyArea != nil {
		a.detailsReplyArea.SetPlaceholder(replyPlaceholder)
	}
}

// tview's TextArea believes it is one row tall until drawn, so text filled with the cursor at the end scrolls all but the last line out of view.
func fillWritingBox(area *tview.TextArea, text string) {
	if area == nil {
		return
	}
	area.SetText(text, true)
	area.SetOffset(0, 0)
}

func (a *App) copyText(text string) {
	copyFn := a.copyToClipboardFunc
	if copyFn == nil {
		copyFn = copyToClipboard
	}
	if err := copyFn(text); err != nil {
		a.updateStatusBarWithError(err)
		return
	}
	a.flashSuccess("Copied")
}

const (
	postLabel = "  Post  "
	saveLabel = "  Save  "
)

type detailsFocus int

const (
	detailsFocusCards detailsFocus = iota
	detailsFocusReply
	detailsFocusReplyPost
	detailsFocusText
	detailsFocusPost
	detailsFocusEdit
	detailsFocusEditPost
	detailsFocusField
	detailsFocusDescription
)

func (f detailsFocus) isWriting() bool {
	return f == detailsFocusReply || f == detailsFocusText || f == detailsFocusEdit
}

func postFocusFor(f detailsFocus) (detailsFocus, bool) {
	switch f {
	case detailsFocusText:
		return detailsFocusPost, true
	case detailsFocusReply:
		return detailsFocusReplyPost, true
	case detailsFocusEdit:
		return detailsFocusEditPost, true
	}
	return 0, false
}

func (a *App) buildDetailsPage() {
	a.detailsComposeArea, a.detailsComposePost = a.newWritingBox(detailsFocusText, detailsFocusPost, postLabel)
	a.detailsReplyArea, a.detailsReplyPost = a.newWritingBox(detailsFocusReply, detailsFocusReplyPost, postLabel)
	a.detailsEditArea, a.detailsEditPost = a.newWritingBox(detailsFocusEdit, detailsFocusEditPost, saveLabel)
	a.detailsFieldInput = newThemedInputField(a.theme.Background)
	a.detailsFieldInput.SetFieldWidth(0)
	a.detailsFieldInput.SetFocusFunc(func() { a.claimFieldEditorFocus() })
	a.detailsDescArea = tview.NewTextArea()
	a.detailsDescArea.SetFocusFunc(func() { a.claimDescriptionFocus() })
	a.detailsDescArea.SetChangedFunc(func() { a.refitDescriptionBox() })
	a.detailsDescArea.SetClipboard(func(text string) { a.copyText(text) }, nil)
	a.detailsPageView.SetFocusFunc(func() { a.enterDetailsFocus(detailsFocusCards) })
	a.applyComposeTheme()

	a.detailsPage = newDetailsPage(a.detailsPageView, a.refitDetailsPage, a.recordImages)
	a.detailsPage.SetBackgroundColor(a.theme.Background)
}

func (a *App) newWritingBox(text, post detailsFocus, label string) (*tview.TextArea, *tview.Button) {
	area := tview.NewTextArea()
	button := tview.NewButton(label)
	button.SetSelectedFunc(func() { a.postFrom(text) })

	area.SetFocusFunc(func() { a.enterDetailsFocus(text) })
	button.SetFocusFunc(func() { a.enterDetailsFocus(post) })
	area.SetChangedFunc(func() {
		a.applyPostButtonTheme()
		a.refitWritingBox(text, area)
	})
	area.SetClipboard(func(text string) { a.copyText(text) }, nil)
	return area, button
}

func (a *App) refitWritingBox(focus detailsFocus, area *tview.TextArea) {
	if a.detailsPage == nil || area == nil || !a.detailsHaveFocus() || a.detailsFocus != focus {
		return
	}
	for _, slot := range a.detailsPage.slots {
		if slot.primitive != area {
			continue
		}
		if writingBoxRows(area, slot.width) != slot.height {
			a.renderDetailsPage()
			a.scrollToWritingBox(focus)
		}
		return
	}
}

func (a *App) writingBoxBlockID(focus detailsFocus) string {
	switch focus {
	case detailsFocusReply, detailsFocusReplyPost:
		return blockIDReply
	case detailsFocusEdit, detailsFocusEditPost:
		return a.editingCommentID()
	}
	return blockIDCompose
}

func (a *App) scrollToWritingBox(focus detailsFocus) {
	if index := a.commentSpanIndex(a.writingBoxBlockID(focus)); index >= 0 {
		a.scrollCommentIntoView(a.commentSpans[index])
	}
}

func (a *App) writingBox(focus detailsFocus) (*tview.TextArea, *tview.Button, bool) {
	switch focus {
	case detailsFocusText, detailsFocusPost:
		return a.detailsComposeArea, a.detailsComposePost, true
	case detailsFocusReply, detailsFocusReplyPost:
		return a.detailsReplyArea, a.detailsReplyPost, true
	case detailsFocusEdit, detailsFocusEditPost:
		return a.detailsEditArea, a.detailsEditPost, true
	}
	return nil, nil, false
}

// Restyled in place rather than rebuilt, since a rebuild would drop a draft.
func (a *App) applyComposeTheme() {
	for _, focus := range []detailsFocus{detailsFocusText, detailsFocusReply, detailsFocusEdit} {
		area, _, _ := a.writingBox(focus)
		if area == nil {
			continue
		}
		area.SetTextStyle(tcell.StyleDefault.
			Foreground(a.theme.Foreground).
			Background(a.theme.Background))
		area.SetPlaceholderStyle(tcell.StyleDefault.
			Foreground(a.theme.SecondaryText).
			Background(a.theme.Background))
		area.SetSelectedStyle(tcell.StyleDefault.
			Foreground(a.theme.InverseTextColor()).
			Background(a.theme.Accent))
		area.SetBackgroundColor(a.theme.Background)
	}
	if a.detailsFieldInput != nil {
		a.detailsFieldInput.SetFieldStyle(a.fieldEditorStyle(a.detailsEdit.editing))
		a.detailsFieldInput.SetBackgroundColor(a.theme.Background)
	}
	if a.detailsDescArea != nil {
		a.detailsDescArea.SetTextStyle(tcell.StyleDefault.
			Foreground(a.theme.Foreground).
			Background(a.theme.Background))
		a.detailsDescArea.SetSelectedStyle(tcell.StyleDefault.
			Foreground(a.theme.InverseTextColor()).
			Background(a.theme.Accent))
		a.detailsDescArea.SetBackgroundColor(a.theme.Background)
	}
	a.applyComposePlaceholder()
	a.applyPostButtonTheme()
	if a.detailsPage != nil {
		a.detailsPage.SetBackgroundColor(a.theme.Background)
	}
}

func (a *App) applyPostButtonTheme() {
	for _, focus := range []detailsFocus{detailsFocusText, detailsFocusReply, detailsFocusEdit} {
		area, button, _ := a.writingBox(focus)
		if area == nil || button == nil {
			continue
		}
		label := a.theme.Foreground
		if strings.TrimSpace(area.GetText()) == "" {
			label = a.theme.SecondaryText
		}
		button.SetStyle(tcell.StyleDefault.
			Foreground(label).
			Background(a.theme.SelectionBg))
		button.SetActivatedStyle(tcell.StyleDefault.
			Foreground(a.theme.InverseTextColor()).
			Background(a.theme.Accent))
	}
}

func (a *App) composeBoxOnScreen() bool {
	return a.detailsView != nil && !a.detailsHidden && a.commentSpanIndex(blockIDCompose) >= 0
}

// Key path only: Application.GetFocus blocks on the lock Application.draw holds for the whole frame.
func (a *App) composeBoxActive() bool {
	if a.detailsComposeArea == nil || !a.composeBoxOnScreen() {
		return false
	}
	return a.activeWritingBox() != detailsFocusCards
}

func (a *App) activeWritingBox() detailsFocus {
	focus := a.app.GetFocus()
	for _, target := range []detailsFocus{
		detailsFocusText, detailsFocusPost,
		detailsFocusReply, detailsFocusReplyPost,
		detailsFocusEdit, detailsFocusEditPost,
	} {
		area, button, _ := a.writingBox(target)
		if target.isWriting() && focus == area {
			return target
		}
		if !target.isWriting() && focus == button {
			return target
		}
	}
	return detailsFocusCards
}

func (a *App) releaseStrandedCompose() {
	if a.detailsComposeArea == nil || a.composeBoxOnScreen() {
		return
	}
	if a.activeWritingBox() == detailsFocusCards {
		return
	}
	a.detailsFocus = detailsFocusCards
	a.updateFocus()
}

func (a *App) showWritingBox() {
	focus := a.activeWritingBox()
	if focus == detailsFocusCards {
		return
	}
	a.scrollToWritingBox(focus)
}

func (a *App) postButtonActive() bool {
	switch a.activeWritingBox() {
	case detailsFocusPost, detailsFocusReplyPost:
		return true
	}
	return false
}

// Must never move focus itself, or focusing recurses.
func (a *App) enterDetailsFocus(target detailsFocus) {
	if a.focusedPane == FocusPalette || a.activeModal() != nil {
		return
	}
	if !a.composeBoxOnScreen() {
		return
	}
	if target != detailsFocusCards {
		a.leaveDetailsEdit()
	}
	a.detailsFocus = target
	a.applyPaneBorders()
	a.updateStatusBar()
}

func (a *App) stepDetailsFocus(backward bool) {
	if a.focusedPane != FocusDetails {
		return
	}
	step := 1
	if backward {
		step = -1
	}
	if !a.stepCommentRing(step) {
		return
	}
	a.updateFocus()
}

func (a *App) stepWritingBoxFocus() {
	if !a.detailsHaveFocus() {
		return
	}
	switch a.detailsFocus {
	case detailsFocusText:
		a.detailsFocus = detailsFocusPost
	case detailsFocusPost:
		a.detailsFocus = detailsFocusText
	case detailsFocusReply:
		a.detailsFocus = detailsFocusReplyPost
	case detailsFocusReplyPost:
		a.detailsFocus = detailsFocusReply
	case detailsFocusEdit:
		a.detailsFocus = detailsFocusEditPost
	case detailsFocusEditPost:
		a.detailsFocus = detailsFocusEdit
	default:
		return
	}
	a.updateFocus()
}

func (a *App) openComposeBox() bool {
	issue := a.GetSelectedIssue()
	if a.detailsPage == nil || issue == nil {
		return false
	}
	a.syncComposeDraft(issue.ID)
	a.detailsHidden = false
	a.focusedPane = FocusDetails
	a.detailsFocus = detailsFocusText
	a.focusedCommentID = blockIDCompose
	a.rebuildContentLayout()
	a.updateFocus()
	if !a.composeBoxOnScreen() {
		a.detailsFocus = detailsFocusCards
		a.updateFocus()
		return false
	}
	if index := a.commentSpanIndex(blockIDCompose); index >= 0 {
		a.scrollCommentIntoView(a.commentSpans[index])
	}
	return true
}

func (a *App) leaveComposeBox() {
	a.detailsFocus = detailsFocusCards
	a.updateFocus()
}

// Swallows Ctrl+C rather than returning it, because tview stops the app on a returned Ctrl+C.
func (a *App) handleComposeKey(event *tcell.EventKey) *tcell.EventKey {
	a.showWritingBox()

	switch event.Key() {
	case tcell.KeyCtrlC:
		if area, _, ok := a.writingBox(a.activeWritingBox()); ok && area != nil {
			if text, _, _ := area.GetSelection(); text != "" {
				a.copyText(text)
			}
		}
		return nil
	case tcell.KeyEscape:
		switch box := a.activeWritingBox(); box {
		case detailsFocusReply, detailsFocusReplyPost:
			a.closeReplyBox()
			return nil
		case detailsFocusEdit, detailsFocusEditPost:
			a.closeEditBox()
			return nil
		}
		a.leaveComposeBox()
		return nil
	case tcell.KeyEnter:
		if a.postButtonActive() || event.Modifiers()&tcell.ModCtrl != 0 || event.Modifiers()&tcell.ModMeta != 0 {
			a.postFrom(a.activeWritingBox())
			return nil
		}
	case tcell.KeyTab, tcell.KeyBacktab:
		a.stepWritingBoxFocus()
		return nil
	}
	return event
}

func (a *App) postComment() { a.postFrom(detailsFocusText) }

// Posts to composeDraftIssueID, never the selection, which moves ahead of the debounced syncComposeDraft.
func (a *App) postFrom(from detailsFocus) {
	if from == detailsFocusPost {
		from = detailsFocusText
	}
	if from == detailsFocusReplyPost {
		from = detailsFocusReply
	}
	if from == detailsFocusEdit || from == detailsFocusEditPost {
		a.saveCommentEdit()
		return
	}
	area, _, ok := a.writingBox(from)
	if !ok {
		return
	}
	body := strings.TrimSpace(area.GetText())
	if body == "" {
		return
	}
	issueID := a.composeDraftIssueID
	if issueID == "" {
		a.flashStatus("No issue selected")
		return
	}
	parentID := ""
	if from == detailsFocusReply {
		parentID = a.replyParentID()
	}

	area.SetText("", false)
	if from == detailsFocusReply {
		a.closeReplyBox()
	} else {
		a.leaveComposeBox()
	}
	a.flashStatus("Posting comment...")

	post := a.createCommentFunc
	go func() {
		comment, err := post(context.Background(), linearapi.CreateCommentInput{
			IssueID:  issueID,
			Body:     body,
			ParentID: parentID,
		})
		a.QueueUpdateDraw(func() {
			if err != nil {
				logger.ErrorWithErr(err, "tui.details_compose: create comment failed issue=%s", issueID)
				a.restoreComposeDraft(issueID, body, parentID)
				a.updateStatusBarWithError(err)
				return
			}
			logger.Info("tui.details_compose: comment posted issue=%s", issueID)
			a.appendComment(issueID, comment)
			a.flashSuccess("Comment added")
		})
	}()
}

func (a *App) appendComment(issueID string, comment linearapi.Comment) {
	a.issuesMu.Lock()
	selected := a.selectedIssue
	if selected == nil || selected.ID != issueID {
		a.issuesMu.Unlock()
		return
	}
	selected.Comments = insertCommentInOrder(selected.Comments, comment)
	comments := selected.Comments
	a.issuesMu.Unlock()

	a.detailsCommentsSource = comments
	a.renderDetailsPage()
	a.detailsPageView.ScrollToEnd()
	a.focusComment(comment.ID)
}

// Sends the body untrimmed: leading whitespace is an indented code block to Linear.
func (a *App) saveCommentEdit() {
	commentID := a.editingCommentID()
	if commentID == "" || a.detailsEditArea == nil {
		return
	}
	body := a.detailsEditArea.GetText()
	if strings.TrimSpace(body) == "" {
		return
	}
	issueID := a.composeDraftIssueID
	if current, ok := a.commentByID(commentID); ok && current.Body == body {
		a.closeEditBox()
		return
	}
	if _, out := a.savingComments[commentID]; out {
		a.flashStatus("Already saving this comment")
		return
	}
	if a.savingComments == nil {
		a.savingComments = make(map[string]struct{})
	}
	a.savingComments[commentID] = struct{}{}
	a.flashStatus("Saving comment...")

	update := a.updateCommentFunc
	go func() {
		comment, err := update(context.Background(), linearapi.UpdateCommentInput{
			ID:      commentID,
			Body:    body,
			IssueID: issueID,
		})
		a.QueueUpdateDraw(func() {
			delete(a.savingComments, commentID)
			if err != nil {
				logger.ErrorWithErr(err, "tui.details_compose: update comment failed comment=%s", commentID)
				a.updateStatusBarWithError(err)
				return
			}
			logger.Info("tui.details_compose: comment updated comment=%s", commentID)
			if a.editingCommentID() == commentID {
				a.closeEditBox()
			}
			a.replaceComment(issueID, comment)
			a.flashSuccess("Comment updated")
		})
	}()
}

func (a *App) replaceComment(issueID string, comment linearapi.Comment) {
	a.issuesMu.Lock()
	selected := a.selectedIssue
	if selected == nil || selected.ID != issueID {
		a.issuesMu.Unlock()
		return
	}
	for i, held := range selected.Comments {
		if held.ID == comment.ID {
			selected.Comments[i] = comment
			break
		}
	}
	comments := selected.Comments
	a.issuesMu.Unlock()

	a.cancelDetailFetch()
	a.detailsCommentsSource = comments
	a.renderDetailsPage()
	if a.detailsFocus == detailsFocusCards && a.focusedCommentID == comment.ID {
		a.focusComment(comment.ID)
	}
}

// Two posts can be in flight at once and answer out of order.
func insertCommentInOrder(comments []linearapi.Comment, comment linearapi.Comment) []linearapi.Comment {
	at := len(comments)
	for i, held := range comments {
		if held.CreatedAt.After(comment.CreatedAt) {
			at = i
			break
		}
	}
	comments = append(comments, linearapi.Comment{})
	copy(comments[at+1:], comments[at:])
	comments[at] = comment
	return comments
}

// Appends to what is already in the box, since overwriting would destroy the next comment to rescue this one.
func (a *App) restoreComposeDraft(issueID, body, parentID string) {
	if parentID != "" {
		a.restoreReplyDraft(issueID, body, parentID)
		return
	}

	if a.composeDraftIssueID != issueID {
		a.setComposeDraft(issueID, joinDrafts(body, a.composeDrafts[issueID]))
		return
	}

	fillWritingBox(a.detailsComposeArea, joinDrafts(body, a.detailsComposeArea.GetText()))
	if a.detailsHaveFocus() && a.composeBoxOnScreen() {
		a.detailsFocus = detailsFocusText
		a.updateFocus()
	}
}

func (a *App) restoreReplyDraft(issueID, body, parentID string) {
	held := a.replyDrafts[parentID]
	if a.composeDraftIssueID == issueID && a.replyParentID() == parentID {
		held = a.detailsReplyArea.GetText()
	}
	if a.replyDrafts == nil {
		a.replyDrafts = make(map[string]string)
	}
	a.replyDrafts[parentID] = joinDrafts(body, held)

	if a.composeDraftIssueID != issueID || !a.detailsHaveFocus() {
		return
	}
	a.openReplyBox(parentID)
}

func joinDrafts(body, held string) string {
	if strings.TrimSpace(held) == "" {
		return body
	}
	return body + "\n\n" + held
}

func (a *App) syncComposeDraft(issueID string) {
	if a.detailsComposeArea == nil || a.composeDraftIssueID == issueID {
		return
	}
	delete(a.composeEditing, a.composeDraftIssueID)
	if a.detailsEditArea != nil {
		a.detailsEditArea.SetText("", false)
	}
	a.holdReplyDraft()
	a.setComposeDraft(a.composeDraftIssueID, a.detailsComposeArea.GetText())
	a.composeDraftIssueID = issueID
	fillWritingBox(a.detailsComposeArea, a.composeDrafts[issueID])
	fillWritingBox(a.detailsReplyArea, a.replyDrafts[a.replyParentID()])
	strandedReply := a.replyParentID() == "" &&
		(a.detailsFocus == detailsFocusReply || a.detailsFocus == detailsFocusReplyPost)
	strandedEdit := a.detailsFocus == detailsFocusEdit || a.detailsFocus == detailsFocusEditPost
	if strandedReply || strandedEdit {
		a.detailsFocus = detailsFocusCards
		a.focusedCommentID = ""
		if a.focusedPane == FocusDetails {
			a.updateFocus()
		}
	}
	a.applyComposePlaceholder()
}

func (a *App) setComposeDraft(issueID, body string) {
	if issueID == "" {
		return
	}
	if strings.TrimSpace(body) == "" {
		delete(a.composeDrafts, issueID)
		return
	}
	if a.composeDrafts == nil {
		a.composeDrafts = make(map[string]string)
	}
	a.composeDrafts[issueID] = body
}

func (a *App) clearComposeDrafts() {
	a.composeDrafts = nil
	a.composeReplyTo = nil
	a.replyDrafts = nil
	a.composeEditing = nil
	a.composeDraftIssueID = ""
	if a.detailsComposeArea != nil {
		a.detailsComposeArea.SetText("", false)
		a.detailsReplyArea.SetText("", false)
		a.detailsEditArea.SetText("", false)
		a.applyComposePlaceholder()
	}
}
