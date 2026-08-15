package tui

import (
	"context"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// The compose box is the last card on the details page, under everything
// already said, which is where the comment being written is going to appear. It
// is always on the page: a box that has to be summoned is one nobody knows is
// there. It scrolls with the page rather than sitting over it, because a box
// pinned to the foot of the pane covers the conversation it is about.
//
// The reply box is the same card opened inside a thread, so an answer is
// written where it is going to appear as well.

// composeRows is how tall an empty box is. A box grows past it with what is
// written in it, so nothing being typed is hidden behind a scroll inside a
// frame; see writingBoxRows.
const composeRows = 4

// composePlaceholder and replyPlaceholder are what each empty box says.
const (
	composePlaceholder = "Leave a comment"
	replyPlaceholder   = "Leave a reply"
)

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
	fillWritingBox(a.detailsReplyArea, a.replyDrafts[parentID])
	a.applyComposePlaceholder()

	// Rendered before the focus moves: the box has no place on the page, and so
	// no stop in the ring, until the page has been written with it in.
	a.renderDetailsPage()
	// The ring is a pair, the stop and what it names. Moving one and not the
	// other left the card the reader came from lit while the keyboard was in
	// the box, and the box itself unlit.
	a.commentsFocus, a.focusedCommentID = commentsFocusReply, blockIDReply
	a.updateFocus()
	if index := a.commentSpanIndex(blockIDReply); index >= 0 {
		a.scrollCommentIntoView(a.commentSpans[index])
	}
}

// editingCommentID is the comment the edit box is open on, empty when none is.
// One at a time per issue, for the reason the reply box gives.
func (a *App) editingCommentID() string {
	return a.composeEditing[a.composeDraftIssueID]
}

// openEditBox turns a card into a box holding what the comment says, in the
// place the card was and inside its thread.
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

	// Rendered before the focus moves, for the reason openReplyBox gives: the
	// box has no stop in the ring until the page has been written with it in.
	a.renderDetailsPage()
	a.commentsFocus, a.focusedCommentID = commentsFocusEdit, commentID
	a.updateFocus()
	if index := a.commentSpanIndex(commentID); index >= 0 {
		a.scrollCommentIntoView(a.commentSpans[index])
	}
}

// closeEditBox puts the card back and drops what was in the box. The ring lands
// on the comment that was being rewritten, which is where the reader was.
func (a *App) closeEditBox() {
	commentID := a.editingCommentID()
	if commentID == "" {
		return
	}
	delete(a.composeEditing, a.composeDraftIssueID)
	a.detailsEditArea.SetText("", false)
	a.commentsFocus = commentsFocusCards
	a.focusedCommentID = commentID
	a.renderDetailsPage()
	a.updateFocus()
}

// dropEditForMissingComment closes an edit box whose comment the latest fetch
// no longer carries, deleted upstream or otherwise. Left open, the box is drawn
// nowhere while it still holds the keyboard, and the compose card on the page
// is enough for composeBoxOnScreen to keep routing keys into it.
//
// Nothing holds a rewrite by design, so there is nothing here to keep.
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
	a.renderDetailsPage()
	a.updateFocus()
}

// holdReplyDraft puts what is in the reply box away against the thread it
// answers, so closing the box is not the same as losing the words.
//
// A box holding nothing but a quote holds nothing the reader wrote: the app put
// it there on one keystroke and can put it back on the same one. Kept, it
// followed them to the next comment they quoted and stacked up in front of it.
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

// isAllQuoted reports whether every line with anything on it is quoted, which
// is a box the reader has not written in yet.
func isAllQuoted(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, ">") {
			return false
		}
	}
	return true
}

// applyComposePlaceholder says what each box is for while it is empty. Neither
// names who is being answered: the reply box is drawn inside the thread it
// answers, and the card above it is the answer to that already.
func (a *App) applyComposePlaceholder() {
	if a.detailsComposeArea == nil {
		return
	}
	a.detailsComposeArea.SetPlaceholder(composePlaceholder)
	if a.detailsReplyArea != nil {
		a.detailsReplyArea.SetPlaceholder(replyPlaceholder)
	}
}

// fillWritingBox puts text in a box and shows it from the top.
//
// A box that has not been drawn yet believes it is one row tall: tview's
// TextArea starts that way so its measurements work before the first frame.
// Filling it with the cursor at the end scrolls every line above the last one
// out of view, so a quote reply opened on a box that had never been drawn read
// as empty while holding every word of the quote.
func fillWritingBox(area *tview.TextArea, text string) {
	if area == nil {
		return
	}
	area.SetText(text, true)
	area.SetOffset(0, 0)
}

// copyText puts text on the system clipboard through the seam the commands use,
// so a test can read what a box copied.
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

// postLabel and saveLabel are what the buttons say, padded out either side: a
// filled surface reads as something to press where a bare word reads as a
// caption. An edit says Save because it is not adding anything to the page.
const (
	postLabel = "  Post  "
	saveLabel = "  Save  "
)

// commentsFocus names what on the details page holds the keyboard. The braces
// step the cards and the boxes; Tab moves between a box and its button.
type commentsFocus int

const (
	commentsFocusCards commentsFocus = iota
	commentsFocusReply
	commentsFocusReplyPost
	commentsFocusText
	commentsFocusPost
	commentsFocusEdit
	commentsFocusEditPost
)

// isWriting reports whether a focus is one of the writing boxes.
func (f commentsFocus) isWriting() bool {
	return f == commentsFocusReply || f == commentsFocusText || f == commentsFocusEdit
}

// postFocusFor is the button that sends what a box holds, and whether the focus
// named a box at all.
func postFocusFor(f commentsFocus) (commentsFocus, bool) {
	switch f {
	case commentsFocusText:
		return commentsFocusPost, true
	case commentsFocusReply:
		return commentsFocusReplyPost, true
	case commentsFocusEdit:
		return commentsFocusEditPost, true
	}
	return 0, false
}

// buildDetailsPage builds the page and the three boxes drawn in it. The panel
// around it owns the border, the title and the density padding; the page is
// borderless.
func (a *App) buildDetailsPage() {
	a.detailsComposeArea, a.detailsComposePost = a.newWritingBox(commentsFocusText, commentsFocusPost, postLabel)
	a.detailsReplyArea, a.detailsReplyPost = a.newWritingBox(commentsFocusReply, commentsFocusReplyPost, postLabel)
	a.detailsEditArea, a.detailsEditPost = a.newWritingBox(commentsFocusEdit, commentsFocusEditPost, saveLabel)
	a.detailsPageView.SetFocusFunc(func() { a.enterCommentsFocus(commentsFocusCards) })
	a.applyComposeTheme()

	a.detailsPage = newDetailsPage(a.detailsPageView, a.refitDetailsPage)
	a.detailsPage.SetBackgroundColor(a.theme.Background)
}

// newWritingBox builds one box: the writing area and the button that sends it.
// Both are drawn inside a card on the page rather than mounted in a layout, so
// neither carries a frame of its own.
func (a *App) newWritingBox(text, post commentsFocus, label string) (*tview.TextArea, *tview.Button) {
	area := tview.NewTextArea()
	button := tview.NewButton(label)
	button.SetSelectedFunc(func() { a.postFrom(text) })

	// A box takes the keyboard by mouse as well as by key, and a click never
	// goes through updateFocus. Recording it here is what keeps a typed letter
	// out of the command shortcuts.
	area.SetFocusFunc(func() { a.enterCommentsFocus(text) })
	button.SetFocusFunc(func() { a.enterCommentsFocus(post) })
	// The button greys out with nothing to send, so the control does not appear
	// and disappear as you type, and the box grows to hold what is typed into
	// it.
	area.SetChangedFunc(func() {
		a.applyPostButtonTheme()
		a.refitWritingBox(text, area)
	})
	// Copy reaches the system clipboard rather than a buffer inside the widget,
	// which is what tview gives a box with no clipboard of its own: text copied
	// out of a comment could not be pasted anywhere else. Paste is left to the
	// terminal, which sends what it holds as a paste event.
	area.SetClipboard(func(text string) { a.copyText(text) }, nil)
	return area, button
}

// refitWritingBox redraws the page when what has been typed no longer fits the
// rows the box was drawn with, which is how a box grows and shrinks with its
// own text.
//
// Only for the box holding the keyboard, and only on the keystroke that changes
// the count. The page re-renders the whole issue, which is not work to do on
// every letter, and every programmatic fill happens while the focus is
// somewhere else — including the one inside updateDetailsView, which would
// otherwise render a page the caller is halfway through rebuilding.
func (a *App) refitWritingBox(focus commentsFocus, area *tview.TextArea) {
	if a.detailsPage == nil || area == nil || !a.detailsHaveFocus() || a.commentsFocus != focus {
		return
	}
	for _, slot := range a.detailsPage.slots {
		if slot.primitive != area {
			continue
		}
		if writingBoxRows(area, slot.width) != slot.height {
			a.renderDetailsPage()
			// The page scrolled to the box before the key that grew it landed,
			// so without this the row just gained sits below the fold until
			// the next key scrolls again.
			a.scrollToWritingBox(focus)
		}
		return
	}
}

// writingBoxBlockID names the block a box is drawn in, which is what the page
// scrolls by. An edit box stands where a card did and keeps the comment's id.
func (a *App) writingBoxBlockID(focus commentsFocus) string {
	switch focus {
	case commentsFocusReply, commentsFocusReplyPost:
		return blockIDReply
	case commentsFocusEdit, commentsFocusEditPost:
		return a.editingCommentID()
	}
	return blockIDCompose
}

// scrollToWritingBox brings a box's card back onto the page, and does nothing
// when it is already there.
func (a *App) scrollToWritingBox(focus commentsFocus) {
	if index := a.commentSpanIndex(a.writingBoxBlockID(focus)); index >= 0 {
		a.scrollCommentIntoView(a.commentSpans[index])
	}
}

// writingBox returns the widgets behind a focus, and whether that focus is a
// box at all.
func (a *App) writingBox(focus commentsFocus) (*tview.TextArea, *tview.Button, bool) {
	switch focus {
	case commentsFocusText, commentsFocusPost:
		return a.detailsComposeArea, a.detailsComposePost, true
	case commentsFocusReply, commentsFocusReplyPost:
		return a.detailsReplyArea, a.detailsReplyPost, true
	case commentsFocusEdit, commentsFocusEditPost:
		return a.detailsEditArea, a.detailsEditPost, true
	}
	return nil, nil, false
}

// applyComposeTheme restyles the boxes in place. TextArea bakes its styles from
// the tview globals at construction but exposes setters for all of them, so a
// box is restyled rather than rebuilt: a rebuild would drop a draft.
func (a *App) applyComposeTheme() {
	for _, focus := range []commentsFocus{commentsFocusText, commentsFocusReply, commentsFocusEdit} {
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
	if a.detailsPage != nil {
		a.detailsPage.SetBackgroundColor(a.theme.Background)
	}
}

// applyPostButtonTheme colors each button for what it can do. It is a filled
// surface in every state, because it is a button in every state: an empty
// buffer greys the label without taking the surface away.
func (a *App) applyPostButtonTheme() {
	for _, focus := range []commentsFocus{commentsFocusText, commentsFocusReply, commentsFocusEdit} {
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

// composeBoxOnScreen reports whether the box was drawn and is showing.
//
// Focus outlives the render that put it there: clearing the selection takes the
// card off the page without moving the keyboard off the box. Letting the box
// take keys from there locks the app — every key goes to a text area nobody can
// see, and there is nothing on screen to say so.
//
// It asks the page rather than a field of its own, so the answer is what was
// actually drawn. Draw-safe: the spans are a slice, read under no lock.
func (a *App) composeBoxOnScreen() bool {
	return a.detailsView != nil && !a.detailsHidden && a.commentSpanIndex(blockIDCompose) >= 0
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
	for _, target := range []commentsFocus{
		commentsFocusText, commentsFocusPost,
		commentsFocusReply, commentsFocusReplyPost,
		commentsFocusEdit, commentsFocusEditPost,
	} {
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

// showWritingBox scrolls the card of the box holding the keyboard back onto the
// page, and does nothing when it is already there.
func (a *App) showWritingBox() {
	focus := a.activeWritingBox()
	if focus == commentsFocusCards {
		return
	}
	a.scrollToWritingBox(focus)
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

// enterCommentsFocus records which stop on the page took the keyboard, for the
// paths that take it without calling updateFocus: a mouse click, and tview
// handing focus down to a child. It repaints the cues but must never move focus
// itself, or focusing would recurse.
//
// It records the stop and nothing else. handleMouse owns focusedPane and sets
// it before the click is delivered, so claiming the pane here as well let
// anything that focuses these widgets claim it too. Same rule, same reason, as
// claimNavFocus.
func (a *App) enterCommentsFocus(target commentsFocus) {
	// An overlay owns the keys however focus is delegated underneath it.
	if a.focusedPane == FocusPalette || a.activeModal() != nil {
		return
	}
	// tview delegates focus down the tree on its own during layout rebuilds and
	// page adds. Acting on one of those would move the ring for a page holding
	// nothing to act on.
	if !a.composeBoxOnScreen() {
		return
	}
	// A box and the field cursor are two rings on one page. The box was clicked
	// into, so the cursor is the one that gives way.
	if target != commentsFocusCards {
		a.leaveDetailsEdit()
	}
	a.commentsFocus = target
	// The border follows focusedPane rather than being lit outright, or a
	// delegation walk would light this pane while another one holds the keys.
	a.applyPaneBorders()
	a.updateStatusBar()
}

// stepCommentsFocus walks the page's focus ring: every comment card in turn,
// the reply box where one is open on the thread, and the compose card that ends
// the page, each box followed by its button. The ring does not wrap and does
// not leave the pane, so off either end the focus stays where it is.
//
// The ring is the page: the stops are recorded by the render, in the order the
// cards were written, so what the braces do follows what the reader can see.
func (a *App) stepCommentsFocus(backward bool) {
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

// stepWritingBoxFocus moves between the box holding the keyboard and the button
// that sends it, which is the whole of what Tab does in this pane. A two-stop
// walk reads the same in both directions, so there is no direction to pass.
func (a *App) stepWritingBoxFocus() {
	// Scoped to the pane that owns the boxes. commentsFocus outlives the pane
	// being left — nothing resets it on the way out — so an unscoped Tab in the
	// issues list would step a box nobody is looking at.
	if !a.detailsHaveFocus() {
		return
	}
	switch a.commentsFocus {
	case commentsFocusText:
		a.commentsFocus = commentsFocusPost
	case commentsFocusPost:
		a.commentsFocus = commentsFocusText
	case commentsFocusReply:
		a.commentsFocus = commentsFocusReplyPost
	case commentsFocusReplyPost:
		a.commentsFocus = commentsFocusReply
	case commentsFocusEdit:
		a.commentsFocus = commentsFocusEditPost
	case commentsFocusEditPost:
		a.commentsFocus = commentsFocusEdit
	default:
		return
	}
	a.updateFocus()
}

// openComposeBox puts the keyboard in the compose box, reporting whether it got
// there. It sets the fields itself rather than calling focusPane, which enters
// the details pane on the cards.
func (a *App) openComposeBox() bool {
	issue := a.GetSelectedIssue()
	if a.detailsPage == nil || issue == nil {
		return false
	}
	// The selection moves at once but the draft sync rides the detail debounce,
	// so opening inside that window would show the previous issue's words and
	// swap them out mid-sentence when the debounce fired.
	a.syncComposeDraft(issue.ID)
	a.detailsHidden = false
	a.focusedPane = FocusDetails
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
	// Typing brings the box back to where it can be seen. Scrolled off the
	// page it still holds the keyboard, and words going into something off
	// screen are words the writer cannot read back.
	a.showWritingBox()

	switch event.Key() {
	case tcell.KeyCtrlC:
		// Copy, never quit. Ctrl+C is what a reader reaches for to copy the
		// words they just selected, and quitting on it costs them the draft
		// and the session. The event is swallowed either way: handed back,
		// tview stops the app on it itself.
		if area, _, ok := a.writingBox(a.activeWritingBox()); ok && area != nil {
			if text, _, _ := area.GetSelection(); text != "" {
				a.copyText(text)
			}
		}
		return nil
	case tcell.KeyEscape:
		// The reply box closes on its way out: it is open because a card was
		// being answered, and a box left open under a thread nobody is writing
		// in is a card of empty rows in the middle of the conversation. The
		// words are kept against the comment, so reopening finds them.
		switch box := a.activeWritingBox(); box {
		case commentsFocusReply, commentsFocusReplyPost:
			a.closeReplyBox()
			return nil
		case commentsFocusEdit, commentsFocusEditPost:
			// The edit is dropped rather than held. A box that always opens on
			// the comment as it stands cannot show a half-edit from last week
			// in place of what the comment actually says.
			a.closeEditBox()
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
		a.stepWritingBoxFocus()
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
	// The edit box writes over a comment rather than adding one, so the chord
	// and the button reach a different call from the same two controls.
	if from == commentsFocusEdit || from == commentsFocusEditPost {
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
				// flash. The error drops that flash, which is what leaves the
				// failure on screen.
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

// appendComment puts a posted comment at the end of the thread it belongs to.
// Comments arrive oldest first, so a new one goes last. The page is only
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
	a.renderDetailsPage()
	a.detailsPageView.ScrollToEnd()
	// The ring follows the comment that was just written, so the reply it is
	// under is the card the next key acts on.
	a.focusComment(comment.ID)
}

// saveCommentEdit sends what is in the edit box and puts the card back on the
// answer.
//
// The box stays open until Linear answers. A rewrite has nowhere to be held —
// there is no edit draft, by design — so the only place to keep one that failed
// to send is where it was written.
func (a *App) saveCommentEdit() {
	commentID := a.editingCommentID()
	if commentID == "" || a.detailsEditArea == nil {
		return
	}
	// Sent as written. Leading whitespace is an indented code block to Linear,
	// so trimming it would rewrite a comment the user only looked at.
	body := a.detailsEditArea.GetText()
	if strings.TrimSpace(body) == "" {
		return
	}
	issueID := a.composeDraftIssueID
	if current, ok := a.commentByID(commentID); ok && current.Body == body {
		// Sending it would light the "edited" byline on a comment nobody edited.
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
			// The box may have moved on while this was in flight. Closing it
			// then would throw away words written against another comment.
			if a.editingCommentID() == commentID {
				a.closeEditBox()
			}
			a.replaceComment(issueID, comment)
			a.flashSuccess("Comment updated")
		})
	}()
}

// replaceComment puts a rewritten comment back where it was. CreatedAt does not
// move, so neither does the card; the updatedAt that came back is what lights
// the "edited" byline.
//
// A fetch already out was answering about the old body, so it is invalidated
// here rather than left to land on top of this. The cancel comes after the
// issue check: a rewrite that lands once the user has moved on has no business
// killing the fetch that is filling in the issue they moved to.
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
	// The reader may have moved while this was out: onto another card, or into
	// a box. The ring follows the rewritten card only when it was already on
	// it, which is where closeEditBox left it a moment ago.
	if a.commentsFocus == commentsFocusCards && a.focusedCommentID == comment.ID {
		a.focusComment(comment.ID)
	}
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

	fillWritingBox(a.detailsComposeArea, joinDrafts(body, a.detailsComposeArea.GetText()))
	// The keyboard comes back only where the box is on screen. A reader who
	// moved to another pane stays in it.
	if a.detailsHaveFocus() && a.composeBoxOnScreen() {
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

	if a.composeDraftIssueID != issueID || !a.detailsHaveFocus() {
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
	// Leaving the issue drops an open edit the way Esc does. Nothing holds a
	// rewrite, and one widget over a changing selection would carry it to
	// whichever comment the next page draws a box for. Dropped here, while the
	// fields still say which issue it belonged to.
	delete(a.composeEditing, a.composeDraftIssueID)
	if a.detailsEditArea != nil {
		a.detailsEditArea.SetText("", false)
	}
	// Both boxes are one widget over a changing selection, so both have to be
	// put away and reloaded. Held against its own thread first, while the
	// fields still say which issue that was.
	a.holdReplyDraft()
	a.setComposeDraft(a.composeDraftIssueID, a.detailsComposeArea.GetText())
	a.composeDraftIssueID = issueID
	fillWritingBox(a.detailsComposeArea, a.composeDrafts[issueID])
	fillWritingBox(a.detailsReplyArea, a.replyDrafts[a.replyParentID()])
	// The new issue has no box open where the old one did. The fields alone are
	// not enough: tview's focus is still on the reply area, so composeBoxActive
	// would keep routing every keystroke into a box this page never drew, which
	// is the lockup with no way out that releaseStrandedCompose cannot see —
	// the panel is mounted, so it reads as on screen.
	strandedReply := a.replyParentID() == "" &&
		(a.commentsFocus == commentsFocusReply || a.commentsFocus == commentsFocusReplyPost)
	strandedEdit := a.commentsFocus == commentsFocusEdit || a.commentsFocus == commentsFocusEditPost
	if strandedReply || strandedEdit {
		a.commentsFocus = commentsFocusCards
		a.focusedCommentID = ""
		if a.focusedPane == FocusDetails {
			a.updateFocus()
		}
	}
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
	a.composeEditing = nil
	a.composeDraftIssueID = ""
	if a.detailsComposeArea != nil {
		a.detailsComposeArea.SetText("", false)
		a.detailsReplyArea.SetText("", false)
		a.detailsEditArea.SetText("", false)
		a.applyComposePlaceholder()
	}
}
