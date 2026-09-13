package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (a *App) handleMouse(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
	if event == nil {
		return event, action
	}
	switch action {
	case tview.MouseLeftUp, tview.MouseLeftClick, tview.MouseLeftDoubleClick:
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
	if a.focusedPane == FocusPalette || a.activeModal() != nil {
		return event, action
	}
	a.repairLayoutFocus()
	a.releaseFieldEditorOnClick(event.Position())

	pane, ok := a.paneAt(event.Position())
	if !ok {
		a.swallowingClick = true
		return nil, action
	}
	if a.claimPaneFocus(pane) {
		a.swallowingClick = true
		return nil, action
	}
	return event, action
}

// Runs in the mouse capture, never a focus callback: TextView holds its lock while it moves focus, and the render would wedge on it.
func (a *App) releaseFieldEditorOnClick(x, y int) {
	box := a.openEditorBox()
	if box == nil || box.InRect(x, y) {
		return
	}
	a.leaveDetailsEdit()
	a.updateFocus()
}

// tview.Primitive does not carry InRect; Box does.
type editorBox interface {
	InRect(x, y int) bool
}

func (a *App) openEditorBox() editorBox {
	switch a.detailsEdit.editing {
	case "":
	case issueFieldDescription:
		if a.detailsDescArea != nil {
			return a.detailsDescArea
		}
	default:
		if a.detailsFieldInput != nil {
			return a.detailsFieldInput
		}
	}
	return nil
}

// Walks the mounted items: Flex.Clear leaves an unmounted pane's rect where the last draw put it.
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

func (a *App) claimPaneFocus(pane FocusTarget) bool {
	if a.focusedPane == pane {
		return false
	}
	a.leaveDetailsEdit()
	a.focusedPane = pane
	if pane == FocusDetails {
		a.detailsFocus = detailsFocusCards
	}
	before := a.mountedPanes()
	a.updateFocus()
	a.app.ForceDraw()
	return a.mountedPanes() != before
}

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
