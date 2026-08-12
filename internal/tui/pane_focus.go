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
		a.allIssuesTable,
		a.myIssuesTable,
		a.issuesPlaceholder,
		a.searchInput,
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
		// Enter on the description, the same way l enters it.
		a.focusedDetailsView = false
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
	a.focusedPane = panes[next]
	if a.focusedPane == FocusDetails {
		// Enter on the description, the same way the pane numbers do.
		a.focusedDetailsView = false
	}
	a.updateFocus()
}

// updateFocus updates the focus state of all panes.
func (a *App) updateFocus() {
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
	// In responsive modes the focused pane decides what is visible.
	if a.layoutMode != layoutWide {
		a.rebuildContentLayout()
	}
	switch a.focusedPane {
	case FocusNavigation:
		a.app.SetFocus(a.navigationTree)
		a.navigationTree.SetBorderColor(a.theme.BorderFocus)
		a.myIssuesTable.SetBorderColor(a.theme.Border)
		a.allIssuesTable.SetBorderColor(a.theme.Border)
		a.searchPanel.SetBorderColor(a.theme.Border)
		a.setIssuesPlaceholderBorder(a.theme.Border)
		a.detailsDescriptionView.SetBorderColor(a.theme.Border)
		a.detailsCommentsPanel.SetBorderColor(a.theme.Border)
		// Update all pane titles
		a.updateAllPaneTitles()
	case FocusIssues:
		// Focus the visible issues section
		a.myIssuesTable.SetBorderColor(a.theme.Border)
		a.allIssuesTable.SetBorderColor(a.theme.Border)
		a.searchPanel.SetBorderColor(a.theme.Border)
		a.setIssuesPlaceholderBorder(a.theme.Border)
		if a.activeIssuesSection == IssuesSectionSearch {
			a.searchPanel.SetBorderColor(a.theme.BorderFocus)
			if a.searchInputFocused {
				a.app.SetFocus(a.searchInput)
			} else {
				a.app.SetFocus(a.searchResultsTable)
			}
		} else if a.issuesPaneIsEmpty() && a.issuesPlaceholder != nil {
			// The placeholder is what is mounted, so it is what takes the focus
			// border. Highlighting the detached table leaves the pane looking
			// unfocused and sends keys to something off screen.
			a.app.SetFocus(a.issuesPlaceholder)
			a.issuesPlaceholder.SetBorderColor(a.theme.BorderFocus)
		} else if table := a.tableForSection(a.activeIssuesSection); table != nil {
			a.app.SetFocus(table)
			table.SetBorderColor(a.theme.BorderFocus)
		}
		// Update all pane titles
		a.updateAllPaneTitles()
		a.navigationTree.SetBorderColor(a.theme.Border)
		a.detailsDescriptionView.SetBorderColor(a.theme.Border)
		a.detailsCommentsPanel.SetBorderColor(a.theme.Border)
	case FocusDetails:
		// Focus the appropriate sub-view based on state
		if !a.detailsCommentsVisible {
			a.focusedDetailsView = false
		}
		if !a.focusedDetailsView {
			// Every way out of the Comments tab drops the sub-focus, so coming
			// back lands on the cards rather than mid-sentence.
			a.commentsFocus = commentsFocusCards
		}
		a.updateDetailsLayout()
		if a.focusedDetailsView && a.detailsCommentsVisible {
			// The page has two kinds of stop: a card reads, and a box writes
			// with a button that sends.
			if area, button, ok := a.writingBox(a.commentsFocus); ok {
				if a.commentsFocus.isWriting() {
					a.app.SetFocus(area)
				} else {
					a.app.SetFocus(button)
				}
			} else {
				a.app.SetFocus(a.detailsCommentsPage)
			}
			a.detailsDescriptionView.SetBorderColor(a.theme.Border)
			a.detailsCommentsPanel.SetBorderColor(a.theme.BorderFocus)
		} else {
			a.app.SetFocus(a.detailsDescriptionView)
			a.detailsDescriptionView.SetBorderColor(a.theme.BorderFocus)
			a.detailsCommentsPanel.SetBorderColor(a.theme.Border)
		}
		a.navigationTree.SetBorderColor(a.theme.Border)
		a.myIssuesTable.SetBorderColor(a.theme.Border)
		a.allIssuesTable.SetBorderColor(a.theme.Border)
		a.searchPanel.SetBorderColor(a.theme.Border)
		a.setIssuesPlaceholderBorder(a.theme.Border)
		// Update all pane titles
		a.updateAllPaneTitles()
	case FocusPalette:
		a.app.SetFocus(a.paletteInput)
		a.navigationTree.SetBorderColor(a.theme.Border)
		a.myIssuesTable.SetBorderColor(a.theme.Border)
		a.allIssuesTable.SetBorderColor(a.theme.Border)
		a.searchPanel.SetBorderColor(a.theme.Border)
		a.setIssuesPlaceholderBorder(a.theme.Border)
		a.detailsDescriptionView.SetBorderColor(a.theme.Border)
		a.detailsCommentsPanel.SetBorderColor(a.theme.Border)
		// Update all pane titles
		a.updateAllPaneTitles()
	}
	// The Search tab's two halves share one border, so which of them is live
	// has to be said in their own colors, from every branch above.
	a.applySearchFocusStyles()
	// The comment ring's border is in the card text, not on a primitive, so it
	// takes a rewrite rather than a color set.
	a.refreshCommentRing()
	a.updateStatusBar()
}

// updateAllPaneTitles updates all pane titles with visual indicators for the active pane.
func (a *App) updateAllPaneTitles() {
	// Update Navigation pane title
	isNavFocused := a.focusedPane == FocusNavigation
	a.navigationTree.SetTitle(a.paneTitle(paneNumberNavigation, a.tabSegment("Navigation", true, isNavFocused), isNavFocused))
	a.navigationTree.SetTitleColor(a.theme.Foreground)

	// Update Issues pane tab strip
	isIssuesFocused := a.focusedPane == FocusIssues
	issuesTitle := a.paneTitle(paneNumberIssues, a.issuesTabsTitle(isIssuesFocused), isIssuesFocused)
	a.myIssuesTable.SetTitle(issuesTitle)
	a.myIssuesTable.SetTitleColor(a.theme.Foreground)
	a.allIssuesTable.SetTitle(issuesTitle)
	a.allIssuesTable.SetTitleColor(a.theme.Foreground)
	if a.searchPanel != nil {
		a.searchPanel.SetTitle(issuesTitle)
		a.searchPanel.SetTitleColor(a.theme.Foreground)
	}
	if a.issuesPlaceholder != nil {
		// It stands in for the table, so it wears the same tab strip.
		a.issuesPlaceholder.SetTitle(issuesTitle)
		a.issuesPlaceholder.SetTitleColor(a.theme.Foreground)
	}

	// Update Details pane tab strip
	isDetailsFocused := a.focusedPane == FocusDetails
	if a.detailsDescriptionView != nil {
		detailsTitle := a.paneTitle(paneNumberDetails, a.detailsTabsTitle(isDetailsFocused), isDetailsFocused)
		a.detailsDescriptionView.SetTitle(detailsTitle)
		a.detailsDescriptionView.SetTitleColor(a.theme.Foreground)
		if a.detailsCommentsPanel != nil {
			a.detailsCommentsPanel.SetTitle(detailsTitle)
			a.detailsCommentsPanel.SetTitleColor(a.theme.Foreground)
		}
	}
}

// openPalette opens the command palette overlay.
func (a *App) openPalette() {
	a.paletteCtrl.SetScope(a.paneScope())
	a.paletteCtrl.Reset()
	a.paletteInput.SetText("")
	a.paletteInput.SetLabel("> ")
	a.paletteInput.SetPlaceholder("Type to filter commands...")
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
