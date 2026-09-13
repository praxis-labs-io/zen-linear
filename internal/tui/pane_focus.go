package tui

import "github.com/rivo/tview"

func (a *App) issuesPaneHasFocus() bool {
	focus := a.app.GetFocus()
	if focus == nil {
		return false
	}
	for _, candidate := range []tview.Primitive{
		a.listIssuesTable,
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

func (a *App) focusPane(pane FocusTarget) {
	a.leaveDetailsEdit()
	switch pane {
	case FocusNavigation:
		a.navigationHidden = false
		if a.layoutMode != layoutWide {
			a.releaseDetailsZoom()
		}
	case FocusDetails:
		a.detailsHidden = false
		a.detailsFocus = detailsFocusCards
	case FocusIssues:
		a.releaseDetailsZoom()
	case FocusPalette:
	}
	a.focusedPane = pane
	a.rebuildContentLayout()
	a.updateFocus()
}

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
		a.detailsFocus = detailsFocusCards
	}
	a.updateFocus()
}

func (a *App) resolveFocusedPane() {
	if (a.focusedPane == FocusNavigation && a.navigationHidden) ||
		(a.focusedPane == FocusDetails && a.detailsHidden) {
		a.focusedPane = FocusIssues
	}
	if a.detailsZoomed && !a.detailsHidden &&
		(a.focusedPane == FocusIssues ||
			(a.focusedPane == FocusNavigation && a.layoutMode != layoutWide)) {
		a.focusedPane = FocusDetails
	}
}

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

func (a *App) updateFocus() {
	a.resolveFocusedPane()
	if a.layoutMode != layoutWide {
		a.rebuildContentLayout()
	}
	switch a.focusedPane {
	case FocusNavigation:
		if a.navSearchFocused {
			a.app.SetFocus(a.navSearchInput)
		} else {
			a.app.SetFocus(a.navigationTree)
		}
	case FocusIssues:
		if a.issuesPaneIsEmpty() && a.issuesPlaceholder != nil {
			a.app.SetFocus(a.issuesPlaceholder)
		} else if table := a.tableForSection(a.activeIssuesSection); table != nil {
			a.app.SetFocus(table)
		}
	case FocusDetails:
		if a.detailsFocus == detailsFocusDescription && a.detailsEdit.editing == issueFieldDescription {
			a.app.SetFocus(a.detailsDescArea)
		} else if a.detailsFocus == detailsFocusField && a.detailsEdit.editing != "" {
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
	a.applyPaneBorders()
	a.applyNavSearchStyles()
	a.refreshCommentRing()
	a.updateStatusBar()
}

func (a *App) updateAllPaneTitles() {
	isNavFocused := a.focusedPane == FocusNavigation
	navLabel := tview.Escape(a.navigationPaneLabel())
	a.navigationPanel.SetTitle(a.paneTitle(paneNumberNavigation, a.paneLabel(navLabel, isNavFocused), isNavFocused))
	a.navigationPanel.SetTitleColor(a.theme.Foreground)

	isIssuesFocused := a.focusedPane == FocusIssues
	issuesTitle := a.paneTitle(paneNumberIssues, a.issuesPaneTitle(isIssuesFocused), isIssuesFocused)
	a.listIssuesTable.SetTitle(issuesTitle)
	a.listIssuesTable.SetTitleColor(a.theme.Foreground)
	a.searchResultsTable.SetTitle(issuesTitle)
	a.searchResultsTable.SetTitleColor(a.theme.Foreground)
	if a.issuesPlaceholder != nil {
		a.issuesPlaceholder.SetTitle(issuesTitle)
		a.issuesPlaceholder.SetTitleColor(a.theme.Foreground)
	}

	isDetailsFocused := a.focusedPane == FocusDetails
	if a.detailsView != nil {
		a.detailsView.SetTitle(a.paneTitle(paneNumberDetails, a.paneLabel("Details", isDetailsFocused), isDetailsFocused))
		a.detailsView.SetTitleColor(a.theme.Foreground)
	}
}

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

func (a *App) paletteOpen() bool {
	for _, name := range a.pages.GetPageNames(true) {
		if name == "palette" {
			return true
		}
	}
	return false
}

func (a *App) closePalette() {
	a.pages.HidePage("palette")
	a.focusedPane = a.palettePreviousPane
	a.updateFocus()
}
