package tui

import (
	"github.com/gdamore/tcell/v2"
)

func (a *App) bindGlobalKeys() {
	a.app.SetInputCapture(a.handleGlobalKey)
	a.app.SetMouseCapture(a.handleMouse)
}

func (a *App) handleGlobalKey(event *tcell.EventKey) *tcell.EventKey {
	if modal := a.activeModal(); modal != nil {
		a.repairModalFocus()
		return modal.HandleKey(event)
	}

	a.releaseStrandedCompose()
	a.repairLayoutFocus()
	if a.detailsEdit.on && !a.detailsHaveFocus() {
		a.leaveDetailsEdit()
	}

	if a.focusedPane == FocusPalette {
		return a.handlePaletteKey(event)
	}

	if a.navSearchActive() {
		return a.handleNavSearchKey(event)
	}

	if a.composeBoxActive() {
		return a.handleComposeKey(event)
	}

	if a.detailsEdit.on {
		return a.handleDetailsEditKey(event)
	}

	switch event.Key() {
	case tcell.KeyCtrlC:
		a.quit()
		return nil
	case tcell.KeyTab, tcell.KeyBacktab:
		if a.focusedPane == FocusNavigation {
			a.focusNavSearch()
			return nil
		}
		a.stepWritingBoxFocus()
		return nil
	case tcell.KeyRune:
		if r := event.Rune(); a.commandBoundTo(r) && a.runCommandShortcut(r) {
			return nil
		}
		switch event.Rune() {
		case a.actionKey("quit", 'q'):
			a.quit()
			return nil
		case a.actionKey("open_palette", ':'):
			a.openPalette()
			return nil
		case a.actionKey("search", '/'):
			a.focusNavSearch()
			return nil
		case a.actionKey("focus_navigation", '1'):
			a.focusPane(FocusNavigation)
			return nil
		case a.actionKey("focus_issues", '2'):
			a.focusPane(FocusIssues)
			return nil
		case a.actionKey("focus_details", '3'):
			a.focusPane(FocusDetails)
			return nil
		}
	}

	switch a.focusedPane {
	case FocusNavigation:
		return a.handleNavigationKey(event)
	case FocusIssues:
		return a.handleIssuesKey(event)
	case FocusDetails:
		return a.handleDetailsKey(event)
	}

	return event
}

func (a *App) paneScope() CommandScope {
	pane := a.focusedPane
	if pane == FocusPalette {
		pane = a.palettePreviousPane
	}
	switch pane {
	case FocusNavigation:
		return ScopeNavigation
	case FocusIssues, FocusDetails:
		return ScopeIssue
	}
	return ScopeGlobal
}

func (a *App) runCommandShortcut(r rune) bool {
	scope := a.paneScope()
	for _, cmd := range a.paletteCtrl.commands {
		if cmd.ShortcutRune != 0 && cmd.ShortcutRune == r && cmd.appliesIn(scope) {
			cmd.Run(a)
			return true
		}
	}
	return false
}

func (a *App) handleNavigationKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyRight:
		a.stepPane(1)
		return nil
	case tcell.KeyRune:
		switch r := event.Rune(); r {
		case 'l':
			a.stepPane(1)
			return nil
		case a.actionKey("favorite_move_up", 'K'):
			a.moveFavorite(a.currentNavigationNode(), -1)
			return nil
		case a.actionKey("favorite_move_down", 'J'):
			a.moveFavorite(a.currentNavigationNode(), 1)
			return nil
		case 'j', 'k', 'g', 'G', 'h':
		default:
			if a.runCommandShortcut(r) {
				return nil
			}
		}
	}
	return event
}

func (a *App) handleIssuesKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		if a.activeIssuesSection == IssuesSectionSearch {
			a.focusNavSearch()
			return nil
		}
	case tcell.KeyLeft:
		a.stepPane(-1)
		return nil
	case tcell.KeyRight:
		a.stepPane(1)
		return nil
	case tcell.KeyRune:
		r := event.Rune()
		switch r {
		case 'h':
			a.stepPane(-1)
			return nil
		case 'l':
			a.stepPane(1)
			return nil
		}
		if r != 'j' && r != 'k' {
			if a.runCommandShortcut(r) {
				return nil
			}
		}
	}
	return event
}

func (a *App) leaveDetailsForIssues() {
	a.releaseDetailsZoom()
	a.focusedPane = FocusIssues
	a.rebuildContentLayout()
	a.updateFocus()
}

func (a *App) handleDetailsKey(event *tcell.EventKey) *tcell.EventKey {
	if a.handleCommentKey(event) {
		return nil
	}
	switch event.Key() {
	case tcell.KeyEnter, tcell.KeyEscape:
		if event.Key() == tcell.KeyEscape && a.detailsHaveFocus() && a.clearCommentFocus() {
			return nil
		}
		if a.detailsZoomed {
			a.leaveDetailsForIssues()
			return nil
		}
		if event.Key() == tcell.KeyEscape {
			return event
		}
		a.focusedPane = FocusIssues
		a.toggleDetailsPane()
		return nil
	case tcell.KeyLeft:
		a.stepPane(-1)
		return nil
	case tcell.KeyCtrlD:
		a.scrollDetailsHalfPage(1)
		return nil
	case tcell.KeyCtrlU:
		a.scrollDetailsHalfPage(-1)
		return nil
	case tcell.KeyRune:
		switch r := event.Rune(); r {
		case 'h':
			a.stepPane(-1)
			return nil
		case a.actionKey("comment_prev", '{'):
			a.stepDetailsFocus(true)
			return nil
		case a.actionKey("comment_next", '}'):
			a.stepDetailsFocus(false)
			return nil
		case 'j', 'k', 'g', 'G':
		default:
			if a.runCommandShortcut(r) {
				return nil
			}
		}
	}
	return event
}

func (a *App) handlePaletteKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		a.closePalette()
		return nil
	case tcell.KeyEnter:
		if cmd, ok := a.paletteCtrl.Selected(); ok {
			a.closePalette()
			cmd.Run(a)
			return nil
		}
		return nil
	case tcell.KeyUp:
		a.paletteCtrl.MoveCursorUp()
		a.updatePaletteList()
		return nil
	case tcell.KeyDown:
		a.paletteCtrl.MoveCursorDown()
		a.updatePaletteList()
		return nil
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		query := a.paletteCtrl.Query()
		if len(query) > 0 {
			a.paletteCtrl.SetQuery(query[:len(query)-1])
			a.paletteInput.SetText(a.paletteCtrl.Query())
			a.updatePaletteList()
		}
		return nil
	case tcell.KeyRune:
		query := a.paletteCtrl.Query() + string(event.Rune())
		a.paletteCtrl.SetQuery(query)
		a.paletteInput.SetText(query)
		a.updatePaletteList()
		return nil
	}
	return event
}
