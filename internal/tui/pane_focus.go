package tui

import "github.com/rivo/tview"

// issuesPaneHasFocus reports whether the keyboard is on something the issues
// column mounts. It reads live focus rather than focusedPane: a modal takes the
// keys without moving the pane the user goes back to.
func (a *App) issuesPaneHasFocus() bool {
	focus := a.app.GetFocus()
	if focus == nil {
		return false
	}
	for _, candidate := range []tview.Primitive{
		a.listIssuesTable,
		// The placeholder is a Flex, so a click lands on its text rather than on
		// it, and a refresh that read only the Flex dropped the keyboard.
		a.issuesPlaceholder,
		a.issuesPlaceholderText,
		a.searchResultsTable,
	} {
		if focus == candidate {
			return true
		}
	}
	return false
}

// visiblePanes lists the panes h and l can reach, in screen order. A hidden
// pane is not one of them: stepping onto it would land focus somewhere
// updateFocus has to bounce back, which reads as the key getting stuck.
func (a *App) visiblePanes() []FocusTarget {
	panes := make([]FocusTarget, 0, 3)
	if !a.navigationHidden && (!a.detailsZoomed || a.layoutMode == layoutWide) {
		panes = append(panes, FocusNavigation)
	}
	if !a.detailsZoomed {
		panes = append(panes, FocusIssues)
	}
	if !a.detailsHidden {
		panes = append(panes, FocusDetails)
	}
	return panes
}

// focusPane moves focus straight to a pane by its number, revealing it first
// when it has been toggled off. The numbers are fixed to the pane, so typing
// one is also how a hidden pane is summoned back.
func (a *App) focusPane(pane FocusTarget) {
	// Edit mode is the details pane's, and this is how a pane is left.
	a.leaveDetailsEdit()
	switch pane {
	case FocusNavigation:
		a.navigationHidden = false
		// Below the wide breakpoint the zoom leaves no room for the tree, so
		// asking for it by number releases the zoom the way 2 does. Wide, the
		// tree is already there and the zoom can stay.
		if a.layoutMode != layoutWide {
			a.releaseDetailsZoom()
		}
	case FocusDetails:
		a.detailsHidden = false
		// Enter on the cards, the same way l enters the pane.
		a.detailsFocus = detailsFocusCards
	case FocusIssues:
		// The issues list is not on screen while zoomed, so asking for it by
		// number is also how the zoom is released.
		a.releaseDetailsZoom()
	case FocusPalette:
	}
	a.focusedPane = pane
	a.rebuildContentLayout()
	a.updateFocus()
}

// stepPane moves focus one pane left (-1) or right (+1) without wrapping, so
// h and l walk what is on screen rather than naming a fixed neighbor. The
// zoom takes the issues column away, and a pane that names it lands focus
// somewhere the user cannot see.
func (a *App) stepPane(direction int) {
	panes := a.visiblePanes()
	current := -1
	for i, pane := range panes {
		if pane == a.focusedPane {
			current = i
			break
		}
	}
	next := current + direction
	if current < 0 || next < 0 || next >= len(panes) {
		return
	}
	a.leaveDetailsEdit()
	a.focusedPane = panes[next]
	if a.focusedPane == FocusDetails {
		// Enter on the cards, never mid-sentence in a box, the same way the pane
		// numbers do. The reset lives here rather than in updateFocus, which
		// openComposeBox calls straight after putting the keyboard in the box.
		a.detailsFocus = detailsFocusCards
	}
	a.updateFocus()
}

// resolveFocusedPane moves focusedPane off a pane the layout is not showing.
// It reads and writes App fields only, taking no tview lock, which is what lets
// the before-draw hook call it where updateFocus would deadlock.
func (a *App) resolveFocusedPane() {
	// Hidden panes cannot take focus; fall back to the issues column.
	if (a.focusedPane == FocusNavigation && a.navigationHidden) ||
		(a.focusedPane == FocusDetails && a.detailsHidden) {
		a.focusedPane = FocusIssues
	}
	// The zoom drops the issues column, and drops the nav tree too once the
	// terminal is too narrow for both. Either way the details pane is what is
	// left to hold focus.
	if a.detailsZoomed && !a.detailsHidden &&
		(a.focusedPane == FocusIssues ||
			(a.focusedPane == FocusNavigation && a.layoutMode != layoutWide)) {
		a.focusedPane = FocusDetails
	}
}

// applyPaneBorders colors every pane's border and retitles them for whichever
// pane focusedPane names. It sets primitive state and never moves focus, so the
// before-draw hook can reach it.
func (a *App) applyPaneBorders() {
	a.navigationPanel.SetBorderColor(a.theme.Border)
	a.listIssuesTable.SetBorderColor(a.theme.Border)
	a.searchResultsTable.SetBorderColor(a.theme.Border)
	a.setIssuesPlaceholderBorder(a.theme.Border)
	a.detailsView.SetBorderColor(a.theme.Border)
	switch a.focusedPane {
	case FocusNavigation:
		a.navigationPanel.SetBorderColor(a.theme.BorderFocus)
	case FocusIssues:
		// Whatever the column mounts is what takes the focus border. Lighting
		// the detached table leaves the pane looking unfocused.
		if a.issuesPaneIsEmpty() && a.issuesPlaceholder != nil {
			a.setIssuesPlaceholderBorder(a.theme.BorderFocus)
		} else if table := a.tableForSection(a.activeIssuesSection); table != nil {
			table.SetBorderColor(a.theme.BorderFocus)
		}
	case FocusDetails:
		a.detailsView.SetBorderColor(a.theme.BorderFocus)
	case FocusPalette:
	}
	a.updateAllPaneTitles()
}

// updateFocus updates the focus state of all panes.
func (a *App) updateFocus() {
	a.resolveFocusedPane()
	// In responsive modes the focused pane decides what is visible.
	if a.layoutMode != layoutWide {
		a.rebuildContentLayout()
	}
	switch a.focusedPane {
	case FocusNavigation:
		// The pane holds two controls under one border: the query box and the
		// tree. navSearchFocused says which of them the keyboard is on.
		if a.navSearchFocused {
			a.app.SetFocus(a.navSearchInput)
		} else {
			a.app.SetFocus(a.navigationTree)
		}
	case FocusIssues:
		if a.issuesPaneIsEmpty() && a.issuesPlaceholder != nil {
			// Focus the mounted primitive, or the keys go to something off screen.
			a.app.SetFocus(a.issuesPlaceholder)
		} else if table := a.tableForSection(a.activeIssuesSection); table != nil {
			a.app.SetFocus(table)
		}
	case FocusDetails:
		// The page has three kinds of stop: a card reads, a box writes with a
		// button that sends, and a field is typed into.
		if a.detailsFocus == detailsFocusField && a.detailsEdit.editing != "" {
			a.app.SetFocus(a.detailsFieldInput)
		} else if area, button, ok := a.writingBox(a.detailsFocus); ok {
			if a.detailsFocus.isWriting() {
				a.app.SetFocus(area)
			} else {
				a.app.SetFocus(button)
			}
		} else {
			a.app.SetFocus(a.detailsPage)
		}
	case FocusPalette:
		a.app.SetFocus(a.paletteInput)
	}
	// After the focus moves, not before: SetFocus runs the focus callbacks, and
	// those repaint cues of their own.
	a.applyPaneBorders()
	// The nav pane's two controls share one border, so which of them is live
	// has to be said in the query box's own colors.
	a.applyNavSearchStyles()
	// The comment ring's border is in the card text, not on a primitive, so it
	// takes a rewrite rather than a color set.
	a.refreshCommentRing()
	a.updateStatusBar()
}

// updateAllPaneTitles updates all pane titles with visual indicators for the active pane.
func (a *App) updateAllPaneTitles() {
	// Update Navigation pane title
	isNavFocused := a.focusedPane == FocusNavigation
	// The workspace name is the user's, and the title is built from color tags:
	// a workspace called [red] would be read as one instead of printed.
	navLabel := tview.Escape(a.navigationPaneLabel())
	a.navigationPanel.SetTitle(a.paneTitle(paneNumberNavigation, a.paneLabel(navLabel, isNavFocused), isNavFocused))
	a.navigationPanel.SetTitleColor(a.theme.Foreground)

	// Update Issues pane title
	isIssuesFocused := a.focusedPane == FocusIssues
	issuesTitle := a.paneTitle(paneNumberIssues, a.issuesPaneTitle(isIssuesFocused), isIssuesFocused)
	a.listIssuesTable.SetTitle(issuesTitle)
	a.listIssuesTable.SetTitleColor(a.theme.Foreground)
	a.searchResultsTable.SetTitle(issuesTitle)
	a.searchResultsTable.SetTitleColor(a.theme.Foreground)
	if a.issuesPlaceholder != nil {
		// It stands in for the table, so it wears the same title.
		a.issuesPlaceholder.SetTitle(issuesTitle)
		a.issuesPlaceholder.SetTitleColor(a.theme.Foreground)
	}

	// Update Details pane title
	isDetailsFocused := a.focusedPane == FocusDetails
	if a.detailsView != nil {
		a.detailsView.SetTitle(a.paneTitle(paneNumberDetails, a.paneLabel("Details", isDetailsFocused), isDetailsFocused))
		a.detailsView.SetTitleColor(a.theme.Foreground)
	}
}

// openPalette opens the command palette overlay.
func (a *App) openPalette() {
	a.paletteCtrl.SetScope(a.paneScope())
	a.paletteCtrl.Reset()
	a.paletteInput.SetText("")
	a.updatePaletteList()
	a.pages.ShowPage("palette")
	a.pages.SendToFront("palette")
	if a.focusedPane != FocusPalette {
		a.palettePreviousPane = a.focusedPane
	}
	a.focusedPane = FocusPalette
	a.updateFocus()
}

// closePalette closes the command palette overlay.
func (a *App) closePalette() {
	a.pages.HidePage("palette")
	// Restore focus to the pane the palette was opened from.
	a.focusedPane = a.palettePreviousPane
	a.updateFocus()
}
