package tui

import (
	"context"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// The compose box is the last thing in the Comments tab, under everything
// already said, which is where the comment being written is going to appear. It
// is always on the page: a box that has to be summoned is one nobody knows is
// there.

// composeRows is the writing the box shows at once. It does not grow with what
// you type, so the card stack above keeps a fixed share of the pane.
const composeRows = 4

// composeBoxRows is what the box costs the tab: the writing, the button row
// under it, and the frame around both.
const composeBoxRows = composeRows + 3

// composePlaceholder is what the empty box says.
const composePlaceholder = "Leave a comment"

// postLabel is what the button says, padded out either side: a filled surface
// reads as something to press where a bare word reads as a caption.
const postLabel = "  Post  "

// commentsFocus names what inside the Comments tab holds the keyboard. Tab
// steps through them in this order and leaves the pane off either end.
type commentsFocus int

const (
	commentsFocusCards commentsFocus = iota
	commentsFocusText
	commentsFocusPost
)

// commentsFocusOrder is the ring Tab walks.
var commentsFocusOrder = []commentsFocus{commentsFocusCards, commentsFocusText, commentsFocusPost}

// buildDetailsCommentsPanel wraps the card stack and the compose box in one
// bordered panel, the way the Search tab wraps its input and results. The panel
// owns the border, the tab title, and the density padding; both children go
// borderless inside it.
func (a *App) buildDetailsCommentsPanel() {
	a.detailsComposeArea = tview.NewTextArea()
	a.detailsComposePost = tview.NewButton(postLabel)
	a.detailsComposePost.SetSelectedFunc(a.postComment)
	a.applyComposeTheme()

	// The box takes the keyboard by mouse as well as by key, and a click never
	// goes through updateFocus. Recording it here is what keeps a typed letter
	// out of the command shortcuts.
	a.detailsComposeArea.SetFocusFunc(func() { a.enterCommentsFocus(commentsFocusText) })
	a.detailsComposePost.SetFocusFunc(func() { a.enterCommentsFocus(commentsFocusPost) })
	a.detailsCommentsView.SetFocusFunc(func() { a.enterCommentsFocus(commentsFocusCards) })
	// The button greys out with nothing to send, so the control does not appear
	// and disappear as you type.
	a.detailsComposeArea.SetChangedFunc(a.applyPostButtonTheme)

	buttonRow := tview.NewFlex()
	buttonRow.Box = tview.NewBox().SetBackgroundColor(a.theme.Background)
	buttonRow.
		AddItem(nil, 0, 1, false).
		AddItem(a.detailsComposePost, len([]rune(postLabel)), 0, false)

	a.detailsComposeBox = tview.NewFlex().SetDirection(tview.FlexRow)
	// Flex sets dontClear and never paints its own background; restore the
	// fill so the layer beneath cannot bleed through.
	a.detailsComposeBox.Box = tview.NewBox().SetBackgroundColor(a.theme.Background)
	a.detailsComposeBox.SetDrawFunc(a.composeDrawFunc)
	a.detailsComposeBox.
		AddItem(a.detailsComposeArea, 0, 1, true).
		AddItem(buttonRow, 1, 0, false)

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
	a.detailsCommentsPanel.
		AddItem(a.detailsCommentsView, 0, 1, false).
		// The cards are separated by a blank line; the box gets the same gap.
		AddItem(nil, 1, 0, false).
		AddItem(a.detailsComposeBox, composeBoxRows, 0, false)
}

// applyComposeTheme restyles the text area in place. TextArea bakes its styles
// from the tview globals at construction but exposes setters for all of them,
// so the box is restyled rather than rebuilt: a rebuild would drop a draft.
func (a *App) applyComposeTheme() {
	if a.detailsComposeArea == nil {
		return
	}
	a.detailsComposeArea.SetTextStyle(tcell.StyleDefault.
		Foreground(a.theme.Foreground).
		Background(a.theme.Background))
	a.detailsComposeArea.SetPlaceholderStyle(tcell.StyleDefault.
		Foreground(a.theme.SecondaryText).
		Background(a.theme.Background))
	a.detailsComposeArea.SetSelectedStyle(tcell.StyleDefault.
		Foreground(a.theme.InverseTextColor()).
		Background(a.theme.Accent))
	a.detailsComposeArea.SetPlaceholder(composePlaceholder)
	a.detailsComposeArea.SetBackgroundColor(a.theme.Background)
	a.applyPostButtonTheme()
}

// applyPostButtonTheme colors the button for what it can do. It is a filled
// surface in every state, because it is a button in every state: an empty
// buffer greys the label without taking the surface away.
func (a *App) applyPostButtonTheme() {
	if a.detailsComposePost == nil {
		return
	}
	label := a.theme.Foreground
	if strings.TrimSpace(a.detailsComposeArea.GetText()) == "" {
		label = a.theme.SecondaryText
	}
	a.detailsComposePost.SetStyle(tcell.StyleDefault.
		Foreground(label).
		Background(a.theme.SelectionBg))
	a.detailsComposePost.SetActivatedStyle(tcell.StyleDefault.
		Foreground(a.theme.InverseTextColor()).
		Background(a.theme.Accent))
}

// composeDrawFunc frames the box and hands the text area the room inside it.
//
// The frame is painted rune by rune rather than set on the primitive because
// tview.Borders is a global that the rounded_borders setting squares off, and
// the comment cards above are rounded either way. The box is one of them.
func (a *App) composeDrawFunc(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
	measure, gutter := readingMeasure(width)
	if measure < commentCardMinWidth || height < 3 {
		return x, y, 0, 0
	}

	// The ring is read off the field, never off live focus: this runs inside
	// Application.draw, which holds the app's write lock, and reading focus
	// there takes that same lock again and hangs the process on its first
	// frame — no draws, no keys, not even Ctrl+C.
	color := a.theme.Border
	if a.commentsFocus != commentsFocusCards {
		color = a.theme.BorderFocus
	}
	style := tcell.StyleDefault.Foreground(color).Background(a.theme.Background)
	drawRoundedFrame(screen, x+gutter, y, measure, height, style)

	// Inset by the card's own chrome, so the caret starts in the column the
	// card bodies above it start in.
	inset := commentCardChrome / 2
	return x + gutter + inset, y + 1, measure - commentCardChrome, height - 2
}

// drawRoundedFrame paints a rounded box of width by height cells at x, y.
func drawRoundedFrame(screen tcell.Screen, x, y, width, height int, style tcell.Style) {
	right, bottom := x+width-1, y+height-1
	for column := x + 1; column < right; column++ {
		screen.SetContent(column, y, '─', nil, style)
		screen.SetContent(column, bottom, '─', nil, style)
	}
	for row := y + 1; row < bottom; row++ {
		screen.SetContent(x, row, '│', nil, style)
		screen.SetContent(right, row, '│', nil, style)
	}
	screen.SetContent(x, y, '╭', nil, style)
	screen.SetContent(right, y, '╮', nil, style)
	screen.SetContent(x, bottom, '╰', nil, style)
	screen.SetContent(right, bottom, '╯', nil, style)
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
	focus := a.app.GetFocus()
	return focus == a.detailsComposeArea || focus == a.detailsComposePost
}

// releaseStrandedCompose takes the keyboard back off a box that is no longer on
// screen. It runs before every key, so a layout change that unmounted the tab
// under the cursor costs one keystroke rather than the session.
func (a *App) releaseStrandedCompose() {
	if a.detailsComposeArea == nil || a.composeBoxOnScreen() {
		return
	}
	if focus := a.app.GetFocus(); focus != a.detailsComposeArea && focus != a.detailsComposePost {
		return
	}
	a.commentsFocus = commentsFocusCards
	a.updateFocus()
}

// postButtonActive reports whether the button is the thing with the keyboard,
// which is where Enter posts.
func (a *App) postButtonActive() bool {
	return a.detailsComposePost != nil && a.app.GetFocus() == a.detailsComposePost
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

// stepCommentsFocus walks the Comments tab's own focus ring, reporting whether
// it moved. Off either end it reports false and the caller cycles panes, so Tab
// still leaves the pane once there is nothing left inside it to reach.
func (a *App) stepCommentsFocus(backward bool) bool {
	if a.focusedPane != FocusDetails || !a.detailsCommentsVisible || !a.focusedDetailsView {
		return false
	}
	step := 1
	if backward {
		step = -1
	}
	current := 0
	for i, target := range commentsFocusOrder {
		if target == a.commentsFocus {
			current = i
			break
		}
	}
	next := current + step
	if next < 0 || next >= len(commentsFocusOrder) {
		return false
	}
	a.commentsFocus = commentsFocusOrder[next]
	a.updateFocus()
	return true
}

// openComposeBox shows the Comments tab and puts the keyboard in the box,
// reporting whether it got there. It sets the fields itself rather than calling
// focusPane, which enters the details pane on the description.
func (a *App) openComposeBox() bool {
	if a.detailsCommentsPanel == nil || a.GetSelectedIssue() == nil {
		return false
	}
	a.detailsHidden = false
	a.focusedPane = FocusDetails
	a.focusedDetailsView = true
	a.commentsFocus = commentsFocusText
	a.rebuildContentLayout()
	a.updateFocus()
	// Never leave the keyboard in a box the layout did not put on screen.
	if !a.composeBoxOnScreen() {
		a.commentsFocus = commentsFocusCards
		a.updateFocus()
		return false
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
		a.leaveComposeBox()
		return nil
	case tcell.KeyEnter:
		// Enter posts from the button and nowhere else. In the text it is a
		// newline, which is the only thing it can be, so the chord is the way
		// to send without leaving the words.
		if a.postButtonActive() || event.Modifiers()&tcell.ModCtrl != 0 || event.Modifiers()&tcell.ModMeta != 0 {
			a.postComment()
			return nil
		}
	case tcell.KeyTab, tcell.KeyBacktab:
		backward := event.Key() == tcell.KeyBacktab || event.Modifiers()&tcell.ModShift != 0
		if a.stepCommentsFocus(backward) {
			return nil
		}
		a.commentsFocus = commentsFocusCards
		if backward {
			a.cyclePanesBackward()
		} else {
			a.cyclePanesForward()
		}
		return nil
	}
	return event
}

// postComment sends what is in the box and empties it. The words are held here
// until the API answers, so a failed post can put them back.
func (a *App) postComment() {
	body := strings.TrimSpace(a.detailsComposeArea.GetText())
	if body == "" {
		return
	}
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	issueID := issue.ID

	a.detailsComposeArea.SetText("", false)
	// The keyboard goes with the comment. The box is empty and no longer taking
	// words, so a focus ring left on it says the keys are somewhere they are
	// not. A failed post takes it back.
	a.leaveComposeBox()
	a.flashStatus("Posting comment...")

	post := a.createCommentFunc
	go func() {
		comment, err := post(context.Background(), linearapi.CreateCommentInput{IssueID: issueID, Body: body})
		a.QueueUpdateDraw(func() {
			if err != nil {
				logger.ErrorWithErr(err, "tui.details_compose: create comment failed issue=%s", issueID)
				a.updateStatusBarWithError(err)
				a.restoreComposeDraft(issueID, body)
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
	selected.Comments = append(selected.Comments, comment)
	comments := selected.Comments
	a.issuesMu.Unlock()

	a.detailsCommentsSource = comments
	a.renderDetailsComments()
	a.detailsCommentsView.ScrollToEnd()
	// The tab strip carries the count.
	a.updateAllPaneTitles()
}

// restoreComposeDraft puts a failed comment back where it was written.
//
// It appends when something is already there. A post is answered for long after
// the box emptied, and by then the writer may be part way into the next
// comment; overwriting would destroy that one to rescue this one, which is the
// same loss in the other direction. Two comments run together can be cut apart.
func (a *App) restoreComposeDraft(issueID, body string) {
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
	a.composeDraftIssueID = ""
	if a.detailsComposeArea != nil {
		a.detailsComposeArea.SetText("", false)
	}
}
