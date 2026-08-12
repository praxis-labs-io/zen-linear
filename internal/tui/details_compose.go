package tui

import (
	"context"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// The compose box is the last card in the Comments tab, under everything
// already said, which is where the comment being written is going to appear. It
// is always on the page: a box that has to be summoned is one nobody knows is
// there. It scrolls with the page rather than sitting over it, because a box
// pinned to the foot of the pane covers the conversation it is about.
//
// The reply box is the same card opened inside a thread, so an answer is
// written where it is going to appear as well.

// composeRows is the writing a box shows at once. It does not grow with what
// you type, so a long draft does not push the thread it answers off the page.
const composeRows = 4

// composePlaceholder is what the empty box says.
const composePlaceholder = "Leave a comment"

// replyParentID is the thread the reply box is open on, empty when no box is
// open. One box at a time per issue: a second one open elsewhere on the page
// would be two answers being written to two different people at once.
func (a *App) replyParentID() string {
	return a.composeReplyTo[a.composeDraftIssueID]
}

// openReplyBox opens the box under a thread and puts the keyboard in it, with
// whatever was last written to that thread and not sent.
func (a *App) openReplyBox(parentID string) {
	issueID := a.composeDraftIssueID
	if issueID == "" || parentID == "" || a.detailsReplyArea == nil {
		return
	}
	if a.composeReplyTo == nil {
		a.composeReplyTo = make(map[string]string)
	}
	// A box already open elsewhere keeps its words against its own thread.
	a.holdReplyDraft()
	a.composeReplyTo[issueID] = parentID
	a.detailsReplyArea.SetText(a.replyDrafts[parentID], true)
	a.applyComposePlaceholder()

	// Rendered before the focus moves: the box has no place on the page, and so
	// no stop in the ring, until the page has been written with it in.
	a.renderDetailsComments()
	a.commentsFocus = commentsFocusReply
	a.updateFocus()
	if index := a.commentSpanIndex(blockIDReply); index >= 0 {
		a.scrollCommentIntoView(a.commentSpans[index])
	}
}

// closeReplyBox takes the box off the page, keeping what was written against
// the thread it was written to. The ring lands on the comment being answered,
// which is where the reader was.
func (a *App) closeReplyBox() {
	parent := a.replyParentID()
	if parent == "" {
		return
	}
	a.holdReplyDraft()
	delete(a.composeReplyTo, a.composeDraftIssueID)
	a.detailsReplyArea.SetText("", false)
	a.commentsFocus = commentsFocusCards
	a.focusedCommentID = parent
	a.renderDetailsComments()
	a.updateFocus()
}

// holdReplyDraft puts what is in the reply box away against the thread it
// answers, so closing the box is not the same as losing the words.
func (a *App) holdReplyDraft() {
	parent := a.replyParentID()
	if parent == "" || a.detailsReplyArea == nil {
		return
	}
	body := a.detailsReplyArea.GetText()
	if strings.TrimSpace(body) == "" {
		delete(a.replyDrafts, parent)
		return
	}
	if a.replyDrafts == nil {
		a.replyDrafts = make(map[string]string)
	}
	a.replyDrafts[parent] = body
}

// applyComposePlaceholder says what each box is for while it is empty: the
// compose card takes a comment, and the reply box names who it is answering.
func (a *App) applyComposePlaceholder() {
	if a.detailsComposeArea == nil {
		return
	}
	a.detailsComposeArea.SetPlaceholder(composePlaceholder)
	if a.detailsReplyArea == nil {
		return
	}
	a.detailsReplyArea.SetPlaceholder("Reply" + replyTo(a.detailsCommentsSource, a.replyParentID()))
}

// replyTo names the author of a comment for the reply box, or nothing at all
// when the page has moved on from the comment being answered.
func replyTo(comments []linearapi.Comment, parentID string) string {
	if parentID == "" {
		return ""
	}
	for _, comment := range comments {
		if comment.ID == parentID {
			if name := formatUserDisplayName(comment.Author); name != "" {
				return " to " + name
			}
			break
		}
	}
	return ""
}

// postLabel is what the button says, padded out either side: a filled surface
// reads as something to press where a bare word reads as a caption.
const postLabel = "  Post  "

// commentsFocus names what inside the Comments tab holds the keyboard. Tab
// steps through the cards, then either box and its button, and leaves the pane
// off either end.
type commentsFocus int

const (
	commentsFocusCards commentsFocus = iota
	commentsFocusReply
	commentsFocusReplyPost
	commentsFocusText
	commentsFocusPost
)

// isWriting reports whether a focus is one of the two writing boxes.
func (f commentsFocus) isWriting() bool {
	return f == commentsFocusReply || f == commentsFocusText
}

// postFocusFor is the button that sends what a box holds, and whether the focus
// named a box at all.
func postFocusFor(f commentsFocus) (commentsFocus, bool) {
	switch f {
	case commentsFocusText:
		return commentsFocusPost, true
	case commentsFocusReply:
		return commentsFocusReplyPost, true
	}
	return 0, false
}

// buildDetailsCommentsPanel wraps the page in one bordered panel, the way the
// Search tab wraps its input and results. The panel owns the border, the tab
// title, and the density padding; the page inside it is borderless.
func (a *App) buildDetailsCommentsPanel() {
	a.detailsComposeArea, a.detailsComposePost = a.newWritingBox(commentsFocusText, commentsFocusPost)
	a.detailsReplyArea, a.detailsReplyPost = a.newWritingBox(commentsFocusReply, commentsFocusReplyPost)
	a.detailsCommentsView.SetFocusFunc(func() { a.enterCommentsFocus(commentsFocusCards) })
	a.applyComposeTheme()

	a.detailsCommentsPage = newCommentsPage(a.detailsCommentsView, a.refitDetailsComments)
	a.detailsCommentsPage.SetBackgroundColor(a.theme.Background)

	a.detailsCommentsPanel = tview.NewFlex().SetDirection(tview.FlexRow)
	a.detailsCommentsPanel.Box = tview.NewBox().SetBackgroundColor(a.theme.Background)
	a.detailsCommentsPanel.
		SetBorder(true).
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(a.theme.Foreground).
		SetBorderColor(a.theme.Border).
		SetBackgroundColor(a.theme.Background)
	padding := a.density.DetailsPadding
	a.detailsCommentsPanel.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)
	// The page is the panel's focus item. With none flagged, Flex.Focus falls
	// through to the panel's own Box, whose InputHandler is nil, and any focus
	// tview delegates on its own leaves the tab dead to the keyboard.
	a.detailsCommentsPanel.AddItem(a.detailsCommentsPage, 0, 1, true)
}

// newWritingBox builds one box: the writing area and the button that sends it.
// Both are drawn inside a card on the page rather than mounted in a layout, so
// neither carries a frame of its own.
func (a *App) newWritingBox(text, post commentsFocus) (*tview.TextArea, *tview.Button) {
	area := tview.NewTextArea()
	button := tview.NewButton(postLabel)
	button.SetSelectedFunc(func() { a.postFrom(text) })

	// A box takes the keyboard by mouse as well as by key, and a click never
	// goes through updateFocus. Recording it here is what keeps a typed letter
	// out of the command shortcuts.
	area.SetFocusFunc(func() { a.enterCommentsFocus(text) })
	button.SetFocusFunc(func() { a.enterCommentsFocus(post) })
	// The button greys out with nothing to send, so the control does not appear
	// and disappear as you type.
	area.SetChangedFunc(a.applyPostButtonTheme)
	return area, button
}

// writingBox returns the widgets behind a focus, and whether that focus is a
// box at all.
func (a *App) writingBox(focus commentsFocus) (*tview.TextArea, *tview.Button, bool) {
	switch focus {
	case commentsFocusText, commentsFocusPost:
		return a.detailsComposeArea, a.detailsComposePost, true
	case commentsFocusReply, commentsFocusReplyPost:
		return a.detailsReplyArea, a.detailsReplyPost, true
	}
	return nil, nil, false
}

// applyComposeTheme restyles the boxes in place. TextArea bakes its styles from
// the tview globals at construction but exposes setters for all of them, so a
// box is restyled rather than rebuilt: a rebuild would drop a draft.
func (a *App) applyComposeTheme() {
	for _, focus := range []commentsFocus{commentsFocusText, commentsFocusReply} {
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
	a.applyComposePlaceholder()
	a.applyPostButtonTheme()
	if a.detailsCommentsPage != nil {
		a.detailsCommentsPage.SetBackgroundColor(a.theme.Background)
	}
}

// applyPostButtonTheme colors each button for what it can do. It is a filled
// surface in every state, because it is a button in every state: an empty
// buffer greys the label without taking the surface away.
func (a *App) applyPostButtonTheme() {
	for _, focus := range []commentsFocus{commentsFocusText, commentsFocusReply} {
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

// composeBoxOnScreen reports whether the box is mounted and showing.
//
// Focus outlives the layout that put it there: clearing the selection unmounts
// the Comments tab without moving the keyboard off the box. Letting the box
// take keys from there locks the app — every key goes to a text area nobody can
// see, and there is nothing on screen to say so.
func (a *App) composeBoxOnScreen() bool {
	if a.detailsView == nil || a.detailsCommentsPanel == nil ||
		a.detailsHidden || !a.detailsCommentsVisible {
		return false
	}
	for i := 0; i < a.detailsView.GetItemCount(); i++ {
		if a.detailsView.GetItem(i) == a.detailsCommentsPanel {
			return true
		}
	}
	return false
}

// composeBoxActive reports whether a key belongs to the compose box.
//
// It reads live focus rather than the sub-focus field, because a mouse click
// puts the keyboard in the box without going through updateFocus. Tested on the
// field alone, a click left every letter the reader typed firing a command
// shortcut instead of landing in the comment.
//
// Call it from the key path only. Application.GetFocus takes the app's lock,
// which Application.draw holds for the whole frame, so anything reachable from
// a draw func reads a.commentsFocus instead.
func (a *App) composeBoxActive() bool {
	if a.detailsComposeArea == nil || !a.composeBoxOnScreen() {
		return false
	}
	return a.activeWritingBox() != commentsFocusCards
}

// activeWritingBox names the box the keyboard is actually in, or the cards when
// it is in neither. Key path only, for the reason above.
func (a *App) activeWritingBox() commentsFocus {
	focus := a.app.GetFocus()
	for _, target := range []commentsFocus{commentsFocusText, commentsFocusPost, commentsFocusReply, commentsFocusReplyPost} {
		area, button, _ := a.writingBox(target)
		if target.isWriting() && focus == area {
			return target
		}
		if !target.isWriting() && focus == button {
			return target
		}
	}
	return commentsFocusCards
}

// releaseStrandedCompose takes the keyboard back off a box that is no longer on
// screen. It runs before every key, so a layout change that unmounted the tab
// under the cursor costs one keystroke rather than the session.
func (a *App) releaseStrandedCompose() {
	if a.detailsComposeArea == nil || a.composeBoxOnScreen() {
		return
	}
	if a.activeWritingBox() == commentsFocusCards {
		return
	}
	a.commentsFocus = commentsFocusCards
	a.updateFocus()
}

// postButtonActive reports whether a button is the thing with the keyboard,
// which is where Enter posts.
func (a *App) postButtonActive() bool {
	switch a.activeWritingBox() {
	case commentsFocusPost, commentsFocusReplyPost:
		return true
	}
	return false
}

// enterCommentsFocus records that something in the Comments tab took the
// keyboard, for the paths that take it without calling updateFocus: a mouse
// click, and tview handing focus down to a child. It repaints the cues but must
// never move focus itself, or focusing would recurse.
func (a *App) enterCommentsFocus(target commentsFocus) {
	// tview delegates focus down the tree on its own during layout rebuilds and
	// page adds. Acting on one of those would claim the pane for a tab that is
	// not even mounted.
	if !a.composeBoxOnScreen() {
		return
	}
	a.focusedPane = FocusDetails
	a.focusedDetailsView = true
	a.commentsFocus = target
	if a.detailsCommentsPanel != nil {
		a.detailsCommentsPanel.SetBorderColor(a.theme.BorderFocus)
	}
	a.updateAllPaneTitles()
	a.updateStatusBar()
}

// stepCommentsFocus walks the Comments tab's own focus ring: every comment card
// in turn, the reply box where one is open on the thread, and the compose card
// that ends the page, each box followed by its button. The ring does not wrap
// and Tab does not leave the pane, so off either end the focus stays where it
// is.
//
// The ring is the page: the stops are recorded by the render, in the order the
// cards were written, so what Tab does follows what the reader can see.
func (a *App) stepCommentsFocus(backward bool) {
	if a.focusedPane != FocusDetails || !a.detailsCommentsVisible || !a.focusedDetailsView {
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

// openComposeBox shows the Comments tab and puts the keyboard in the box,
// reporting whether it got there. It sets the fields itself rather than calling
// focusPane, which enters the details pane on the description.
func (a *App) openComposeBox() bool {
	issue := a.GetSelectedIssue()
	if a.detailsCommentsPanel == nil || issue == nil {
		return false
	}
	// The selection moves at once but the draft sync rides the detail debounce,
	// so opening inside that window would show the previous issue's words and
	// swap them out mid-sentence when the debounce fired.
	a.syncComposeDraft(issue.ID)
	a.detailsHidden = false
	a.focusedPane = FocusDetails
	a.focusedDetailsView = true
	a.commentsFocus = commentsFocusText
	a.focusedCommentID = blockIDCompose
	a.rebuildContentLayout()
	a.updateFocus()
	// Never leave the keyboard in a box the layout did not put on screen.
	if !a.composeBoxOnScreen() {
		a.commentsFocus = commentsFocusCards
		a.updateFocus()
		return false
	}
	// The card is at the end of the page, so on a long thread it is below the
	// fold when the key is pressed.
	if index := a.commentSpanIndex(blockIDCompose); index >= 0 {
		a.scrollCommentIntoView(a.commentSpans[index])
	}
	return true
}

// leaveComposeBox hands the keyboard back to the card stack, keeping every
// word. The box stays where it is: it is part of the tab, not something
// summoned.
func (a *App) leaveComposeBox() {
	a.commentsFocus = commentsFocusCards
	a.updateFocus()
}

// handleComposeKey routes keys while the box has the keyboard. Anything not
// answered here falls through to the text area, so Enter is a newline and
// letters type instead of firing global or pane shortcuts.
func (a *App) handleComposeKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlC:
		a.app.Stop()
		return nil
	case tcell.KeyEscape:
		// The reply box closes on its way out: it is open because a card was
		// being answered, and a box left open under a thread nobody is writing
		// in is a card of empty rows in the middle of the conversation. The
		// words are kept against the comment, so reopening finds them.
		if box := a.activeWritingBox(); box == commentsFocusReply || box == commentsFocusReplyPost {
			a.closeReplyBox()
			return nil
		}
		a.leaveComposeBox()
		return nil
	case tcell.KeyEnter:
		// Enter posts from the button and nowhere else. In the text it is a
		// newline, which is the only thing it can be, so the chord is the way
		// to send without leaving the words.
		if a.postButtonActive() || event.Modifiers()&tcell.ModCtrl != 0 || event.Modifiers()&tcell.ModMeta != 0 {
			a.postFrom(a.activeWritingBox())
			return nil
		}
	case tcell.KeyTab, tcell.KeyBacktab:
		a.stepCommentsFocus(event.Key() == tcell.KeyBacktab || event.Modifiers()&tcell.ModShift != 0)
		return nil
	}
	return event
}

// postComment sends what is in the compose box, for the paths that name no box
// of their own.
func (a *App) postComment() { a.postFrom(commentsFocusText) }

// postFrom sends what is in one of the boxes and empties it. The words are held
// here until the API answers, so a failed post can put them back.
//
// The reply box posts under the comment it was opened on; the compose box posts
// at top level. Which box the keys were in is the whole of that difference,
// which is why the caller names it rather than the aim being read off a field
// that both boxes would share.
func (a *App) postFrom(from commentsFocus) {
	if from == commentsFocusPost {
		from = commentsFocusText
	}
	if from == commentsFocusReplyPost {
		from = commentsFocusReply
	}
	area, _, ok := a.writingBox(from)
	if !ok {
		return
	}
	body := strings.TrimSpace(area.GetText())
	if body == "" {
		return
	}
	// The draft's own issue, not whatever is selected this instant. A selection
	// move writes selectedIssue immediately but defers syncComposeDraft behind
	// the detail debounce, so reading the selection here can send one issue's
	// words to another — the swap the draft map exists to prevent.
	issueID := a.composeDraftIssueID
	if issueID == "" {
		a.flashStatus("No issue selected")
		return
	}
	parentID := ""
	if from == commentsFocusReply {
		parentID = a.replyParentID()
	}

	area.SetText("", false)
	if from == commentsFocusReply {
		a.closeReplyBox()
	} else {
		// The keyboard goes with the comment. The box is empty and no longer
		// taking words, so a focus ring left on it says the keys are somewhere
		// they are not. A failed post takes it back.
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
				// Restore first: it moves focus, and the status bar rebuilt on
				// the way paints statusMessage, which is still the posting
				// flash. Clearing that is what leaves the error on screen.
				a.restoreComposeDraft(issueID, body, parentID)
				a.statusMessage = ""
				a.updateStatusBarWithError(err)
				return
			}
			logger.Info("tui.details_compose: comment posted issue=%s", issueID)
			a.appendComment(issueID, comment)
			a.flashStatus("Comment added")
		})
	}()
}

// appendComment puts a posted comment at the end of the thread it belongs to.
// Comments arrive oldest first, so a new one goes last. The tab is only
// re-rendered when the selection has not moved on while the write was out.
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
	a.renderDetailsComments()
	a.detailsCommentsView.ScrollToEnd()
	// The ring follows the comment that was just written, so the reply it is
	// under is the card the next key acts on.
	a.focusComment(comment.ID)
	// The tab strip carries the count.
	a.updateAllPaneTitles()
}

// insertCommentInOrder places a comment by its timestamp. Two posts can be in
// flight at once and answer out of order, and the card stack reads oldest
// first, so arrival order is not the order to render in.
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

// restoreComposeDraft puts a failed comment back where it was written.
//
// It appends when something is already there. A post is answered for long after
// the box emptied, and by then the writer may be part way into the next
// comment; overwriting would destroy that one to rescue this one, which is the
// same loss in the other direction. Two comments run together can be cut apart.
//
// A failed reply goes back to the thread it was written to, not to the compose
// card: posted at top level on the second try it would answer nobody.
func (a *App) restoreComposeDraft(issueID, body, parentID string) {
	if parentID != "" {
		a.restoreReplyDraft(issueID, body, parentID)
		return
	}

	// The selection moved while the write was out, so the box on screen is
	// about something else. The words go back to their own issue and are
	// waiting there the next time it is opened.
	if a.composeDraftIssueID != issueID {
		a.setComposeDraft(issueID, joinDrafts(body, a.composeDrafts[issueID]))
		return
	}

	a.detailsComposeArea.SetText(joinDrafts(body, a.detailsComposeArea.GetText()), true)
	// The keyboard comes back only where the box is on screen. A reader who
	// moved to the description keeps the tab they chose.
	if a.focusedPane == FocusDetails && a.detailsCommentsVisible && a.focusedDetailsView {
		a.commentsFocus = commentsFocusText
		a.updateFocus()
	}
}

// restoreReplyDraft puts a failed reply back under the comment it answers,
// reopening the box there when that thread is still the one on screen.
func (a *App) restoreReplyDraft(issueID, body, parentID string) {
	held := a.replyDrafts[parentID]
	if a.composeDraftIssueID == issueID && a.replyParentID() == parentID {
		held = a.detailsReplyArea.GetText()
	}
	if a.replyDrafts == nil {
		a.replyDrafts = make(map[string]string)
	}
	a.replyDrafts[parentID] = joinDrafts(body, held)

	if a.composeDraftIssueID != issueID || !a.commentsHaveFocus() {
		return
	}
	a.openReplyBox(parentID)
}

// joinDrafts puts a rescued comment above one already in hand.
func joinDrafts(body, held string) string {
	if strings.TrimSpace(held) == "" {
		return body
	}
	return body + "\n\n" + held
}

// syncComposeDraft moves the box from one issue's draft to another's. Called
// whenever the details pane changes issue; an empty id is the empty pane.
func (a *App) syncComposeDraft(issueID string) {
	if a.detailsComposeArea == nil || a.composeDraftIssueID == issueID {
		return
	}
	a.setComposeDraft(a.composeDraftIssueID, a.detailsComposeArea.GetText())
	a.composeDraftIssueID = issueID
	a.detailsComposeArea.SetText(a.composeDrafts[issueID], true)
	// The aim is held per issue beside the draft, so nothing has to be moved
	// here; the placeholder just has to read the new issue's.
	a.applyComposePlaceholder()
}

// setComposeDraft holds a draft against its issue, forgetting an empty one so
// the map does not grow a key per issue the reader has looked at.
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

// clearComposeDrafts drops every held draft, for a workspace the issues no
// longer belong to.
func (a *App) clearComposeDrafts() {
	a.composeDrafts = nil
	a.composeReplyTo = nil
	a.replyDrafts = nil
	a.composeDraftIssueID = ""
	if a.detailsComposeArea != nil {
		a.detailsComposeArea.SetText("", false)
		a.detailsReplyArea.SetText("", false)
		a.applyComposePlaceholder()
	}
}
