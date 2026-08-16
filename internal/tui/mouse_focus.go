package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// handleMouse is the app's single mouse capture. tview runs it before handing
// the event to the primitives, so a click sets the pane here and then lands on
// whatever it hit, which is what keeps focusedPane and the keyboard agreeing.
//
// It answers a left press and nothing else. A move would drag the pane around
// under the pointer, a wheel over an unfocused pane conventionally scrolls
// without focusing, and while a text area holds a drag the event goes to that
// area whatever the coordinates say.
func (a *App) handleMouse(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
	// tview reuses one event across the actions it fires for a single report and
	// hands back whatever a capture returned, so a swallowed one arrives here as
	// nil on the next action.
	if event == nil {
		return event, action
	}
	switch action {
	case tview.MouseLeftUp, tview.MouseLeftClick, tview.MouseLeftDoubleClick:
		// A press and its release are separate reports, so the release arrives
		// live however the press went. Swallowing the press alone would drop the
		// half that only focuses and keep the half that selects.
		if a.swallowingClick {
			a.swallowingClick = action == tview.MouseLeftUp
			return nil, action
		}
		return event, action
	case tview.MouseLeftDown:
	default:
		return event, action
	}
	a.swallowingClick = false
	// An overlay owns the keys however focus is delegated underneath it, the
	// same guard claimNavFocus keeps. The repair belongs behind it: it moves the
	// keyboard, and a modal is holding it.
	if a.focusedPane == FocusPalette || a.activeModal() != nil {
		return event, action
	}
	a.repairLayoutFocus()

	pane, ok := a.paneAt(event.Position())
	if !ok {
		// The status row is a text view, and tview would give it the keyboard on
		// a click. Nothing down there is a target, so the click stops here rather
		// than stranding the keys off the panes.
		a.swallowingClick = true
		return nil, action
	}
	if a.claimPaneFocus(pane) {
		// The claim reflowed the panes. Their rects are a frame behind, so
		// forwarding hands the click to whatever moved in under the pointer. A
		// click that rearranges the layout only rearranges it.
		a.swallowingClick = true
		return nil, action
	}
	return event, action
}

// paneAt names the pane covering a screen cell.
//
// It walks what the content flex mounts rather than testing the three panes'
// rects: Flex.Clear leaves an unmounted pane's rect where the last draw put it,
// and in the responsive layouts that stale rect sits under a pane the user can
// actually see.
func (a *App) paneAt(x, y int) (FocusTarget, bool) {
	if a.contentFlex == nil {
		return FocusNavigation, false
	}
	for i := 0; i < a.contentFlex.GetItemCount(); i++ {
		item := a.contentFlex.GetItem(i)
		pane, ok := a.paneOf(item)
		if !ok {
			continue
		}
		left, top, width, height := item.GetRect()
		if x >= left && x < left+width && y >= top && y < top+height {
			return pane, true
		}
	}
	return FocusNavigation, false
}

// paneOf names the pane a mounted primitive is.
func (a *App) paneOf(item tview.Primitive) (FocusTarget, bool) {
	switch item {
	case tview.Primitive(a.navigationPanel):
		return FocusNavigation, true
	case tview.Primitive(a.issuesColumn):
		return FocusIssues, true
	case tview.Primitive(a.detailsView):
		return FocusDetails, true
	}
	return FocusNavigation, false
}

// claimPaneFocus moves the keyboard and the borders to a clicked pane. It
// reports whether the move rearranged which panes are mounted, which is the
// caller's cue to swallow the press.
func (a *App) claimPaneFocus(pane FocusTarget) bool {
	// Clicking the pane you are already in is how a comment card and a writing
	// box are reached, and updateFocus would take the keyboard off both.
	if a.focusedPane == pane {
		return false
	}
	// A click into another pane is leaving this one, and the field cursor goes
	// with it.
	a.leaveDetailsEdit()
	a.focusedPane = pane
	if pane == FocusDetails {
		// Enter on the cards, the same way l and the pane numbers do. A click
		// that landed in a writing box moves on from here when it is delivered.
		a.detailsFocus = detailsFocusCards
	}
	before := a.mountedPanes()
	a.updateFocus()
	// Nothing else redraws: tview paints after a mouse event only when a
	// primitive consumed it, and a press on a pane's border consumes nothing.
	a.app.ForceDraw()
	return a.mountedPanes() != before
}

// mountedPanes fingerprints what the content flex is showing, as a bit per pane.
func (a *App) mountedPanes() int {
	if a.contentFlex == nil {
		return 0
	}
	mounted := 0
	for i := 0; i < a.contentFlex.GetItemCount(); i++ {
		if pane, ok := a.paneOf(a.contentFlex.GetItem(i)); ok {
			mounted |= 1 << pane
		}
	}
	return mounted
}
