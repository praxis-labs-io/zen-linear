package tui

import (
	"github.com/gdamore/tcell/v2"
)

// bindGlobalKeys sets up global keyboard shortcuts.
func (a *App) bindGlobalKeys() {
	a.app.SetInputCapture(a.handleGlobalKey)
}

// handleGlobalKey is the app's single input capture: modals first, then the
// palette and search input, then global keys, then the focused pane.
func (a *App) handleGlobalKey(event *tcell.EventKey) *tcell.EventKey {
	if modal := a.activeModal(); modal != nil {
		return modal.HandleKey(event)
	}

	// Handle palette first if it's open
	if a.focusedPane == FocusPalette {
		return a.handlePaletteKey(event)
	}

	// The search input owns keys next, so typed letters reach the field
	// instead of firing global or pane shortcuts.
	if a.searchInputActive() {
		return a.handleSearchInputKey(event)
	}

	// Global shortcuts (only when not in palette)
	switch event.Key() {
	case tcell.KeyCtrlC:
		a.app.Stop()
		return nil
	case tcell.KeyTab, tcell.KeyBacktab:
		// Tab moves between panes and nothing else. The Details/Comments
		// tabs inside the details pane belong to that pane's tab keys.
		if a.focusedPane != FocusPalette {
			if event.Key() == tcell.KeyBacktab || event.Modifiers()&tcell.ModShift != 0 {
				a.cyclePanesBackward()
			} else {
				a.cyclePanesForward()
			}
		}
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case a.actionKey("quit", 'q'):
			a.app.Stop()
			return nil
		case a.actionKey("open_palette", ':'):
			a.openPalette()
			return nil
		case a.actionKey("search", '/'):
			a.openSearchTab()
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

// runCommandShortcut fires the palette command bound to the rune, if any.
func (a *App) runCommandShortcut(r rune) bool {
	for _, cmd := range a.paletteCtrl.commands {
		if cmd.ShortcutRune != 0 && cmd.ShortcutRune == r {
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
		a.focusedPane = FocusIssues
		a.updateFocus()
		return nil
	case tcell.KeyRune:
		switch r := event.Rune(); r {
		case 'l':
			a.focusedPane = FocusIssues
			a.updateFocus()
			return nil
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
		case 'j', 'k', 'g', 'G', 'h':
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
		// Esc in the search results returns to the search input.
		if a.activeIssuesSection == IssuesSectionSearch {
			a.focusSearchInput()
			return nil
		}
	case tcell.KeyLeft:
		a.focusedPane = FocusNavigation
		a.updateFocus()
		return nil
	case tcell.KeyRight:
		a.focusedPane = FocusDetails
		a.focusedDetailsView = false // Start with description
		a.updateFocus()
		return nil
	case tcell.KeyRune:
		r := event.Rune()
		// Handle vim-style navigation first
		switch r {
		case 'h':
			a.focusedPane = FocusNavigation
			a.updateFocus()
			return nil
		case 'l':
			a.focusedPane = FocusDetails
			a.focusedDetailsView = false // Start with description
			a.updateFocus()
			return nil
		}
		// { and } cycle the issues tabs, lazygit-style ([ and ] keep their
		// original expand/collapse-all bindings).
		switch r {
		case a.actionKey("tab_prev", '{'):
			a.cycleIssuesSection(-1)
			return nil
		case a.actionKey("tab_next", '}'):
			a.cycleIssuesSection(1)
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

// handleDetailsKey handles keyboard input when details pane is focused.
func (a *App) handleDetailsKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEnter:
		// Enter closes the details pane and returns to the issues list.
		a.focusedPane = FocusIssues
		a.toggleDetailsPane()
		return nil
	case tcell.KeyLeft:
		a.focusedPane = FocusIssues
		a.updateFocus()
		return nil
	case tcell.KeyRune:
		switch r := event.Rune(); r {
		case 'h':
			a.focusedPane = FocusIssues
			a.updateFocus()
			return nil
		case a.actionKey("tab_prev", '{'), a.actionKey("tab_next", '}'):
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
