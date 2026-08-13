package tui

import (
	"github.com/gdamore/tcell/v2"
)

// bindGlobalKeys sets up global keyboard shortcuts.
func (a *App) bindGlobalKeys() {
	a.app.SetInputCapture(a.handleGlobalKey)
}

// handleGlobalKey is the app's single input capture: modals first, then the
// palette and the query box, then global keys, then the focused pane.
func (a *App) handleGlobalKey(event *tcell.EventKey) *tcell.EventKey {
	if modal := a.activeModal(); modal != nil {
		return modal.HandleKey(event)
	}

	// A layout change can unmount the Comments tab while the compose box still
	// holds the keyboard. Recover before routing, or every key from then on
	// disappears into a box that is not on the screen.
	a.releaseStrandedCompose()

	// Handle palette first if it's open
	if a.focusedPane == FocusPalette {
		return a.handlePaletteKey(event)
	}

	// The nav pane's query box owns keys next, so typed letters reach the field
	// instead of firing global or pane shortcuts.
	if a.navSearchActive() {
		return a.handleNavSearchKey(event)
	}

	// The compose box owns keys for the same reason: a comment is prose, and q
	// in the middle of one is a letter, not a quit.
	if a.composeBoxActive() {
		return a.handleComposeKey(event)
	}

	// Global shortcuts (only when not in palette)
	switch event.Key() {
	case tcell.KeyCtrlC:
		a.app.Stop()
		return nil
	case tcell.KeyTab, tcell.KeyBacktab:
		// Tab walks a pane's own controls and nothing else. Panes move on h/l
		// and the pane numbers, so Tab is swallowed rather than handed to
		// tview, whose focus delegation would land it on an arbitrary
		// primitive. The palette never reaches here; it returned above.
		backward := event.Key() == tcell.KeyBacktab || event.Modifiers()&tcell.ModShift != 0
		if a.focusedPane == FocusNavigation {
			// Two controls under one border, so either direction is the other
			// one. Tab out of the query box is handled with its own keys.
			a.focusNavSearch()
			return nil
		}
		a.stepCommentsFocus(backward)
		return nil
	case tcell.KeyRune:
		// A command bound by id beats the action holding that rune by default.
		// Out of scope for this pane it does not run, and the action answers.
		if r := event.Rune(); a.commandBoundTo(r) && a.runCommandShortcut(r) {
			return nil
		}
		switch event.Rune() {
		case a.actionKey("quit", 'q'):
			a.app.Stop()
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

	// Pane-specific shortcuts
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

// paneScope returns the command scope the focused pane answers for. The
// palette borrows the scope of the pane it was opened from, which is the pane
// its commands will act on.
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

// runCommandShortcut fires the palette command bound to the rune, if any. A
// command out of scope for the focused pane never fires: a key that acts on the
// selected issue has no business answering from the navigation tree.
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

// handleNavigationKey handles keyboard input when navigation pane is focused.
func (a *App) handleNavigationKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyRight:
		a.stepPane(1)
		return nil
	case tcell.KeyUp:
		if a.navigationTreeIsAtTop() {
			a.focusNavSearch()
			return nil
		}
	case tcell.KeyRune:
		switch r := event.Rune(); r {
		case 'l':
			a.stepPane(1)
			return nil
		case 'k':
			// Off the top of the tree is the query box above it, matching the
			// Down that came out of it.
			if a.navigationTreeIsAtTop() {
				a.focusNavSearch()
				return nil
			}
		case a.actionKey("favorite_move_up", 'K'):
			if a.moveFavorite(a.currentNavigationNode(), -1) {
				return nil
			}
		case a.actionKey("favorite_move_down", 'J'):
			if a.moveFavorite(a.currentNavigationNode(), 1) {
				return nil
			}
		case a.actionKey("favorite_nest", 'L'):
			if a.nestFavorite(a.currentNavigationNode(), false) {
				return nil
			}
		case a.actionKey("favorite_unnest", 'H'):
			if a.nestFavorite(a.currentNavigationNode(), true) {
				return nil
			}
		case 'j', 'g', 'G', 'h':
			// Tree movement keys stay with the tree.
		default:
			// Command shortcuts work from the navigation pane too.
			if a.runCommandShortcut(r) {
				return nil
			}
		}
	}
	return event
}

// handleIssuesKey handles keyboard input when issues pane is focused.
func (a *App) handleIssuesKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		// Esc in the search results returns to the query box that produced
		// them, over in the navigation pane.
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
		// Handle vim-style navigation first
		switch r {
		case 'h':
			a.stepPane(-1)
			return nil
		case 'l':
			a.stepPane(1)
			return nil
		}
		// Handle command shortcuts (plain letters) - skip navigation keys
		if r != 'j' && r != 'k' { // j/k are handled by table for up/down
			if a.runCommandShortcut(r) {
				return nil
			}
		}
	}
	return event
}

// leaveDetailsForIssues moves focus back to the issues list, releasing the zoom
// that was covering it.
func (a *App) leaveDetailsForIssues() {
	a.releaseDetailsZoom()
	a.focusedPane = FocusIssues
	a.rebuildContentLayout()
	a.updateFocus()
}

// handleDetailsKey handles keyboard input when details pane is focused.
func (a *App) handleDetailsKey(event *tcell.EventKey) *tcell.EventKey {
	// The focused card answers first. That is what lets r reply here and
	// refresh everywhere else; a command the user bound to r by id still beats
	// both, one branch up in handleGlobalKey.
	if a.handleCommentKey(event) {
		return nil
	}
	switch event.Key() {
	case tcell.KeyEnter, tcell.KeyEscape:
		// Escape lets go of a card before it does anything larger, the way it
		// drops a reply's aim before leaving the compose box.
		if event.Key() == tcell.KeyEscape && a.commentsHaveFocus() && a.clearCommentFocus() {
			return nil
		}
		// Zoomed, the way back is the issues list itself; unzoomed, Enter
		// closes the pane to get there. Escape only has the first meaning.
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
		case a.actionKey("tab_prev", '['), a.actionKey("tab_next", ']'):
			// Cycle the Details/Comments tabs, lazygit-style.
			if a.detailsCommentsVisible {
				a.focusedDetailsView = !a.focusedDetailsView
				a.updateFocus()
			}
			return nil
		case 'j', 'k', 'g', 'G':
			// Scrolling keys stay with the text view.
		default:
			// Command shortcuts work from the details pane too.
			if a.runCommandShortcut(r) {
				return nil
			}
		}
	}
	return event
}

// handlePaletteKey handles keyboard input when palette is open.
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
