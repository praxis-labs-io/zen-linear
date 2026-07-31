package tui

// rebuildContentLayout re-adds the visible panes to the content flex. The
// issues column is always present; navigation and details can be hidden.
func (a *App) rebuildContentLayout() {
	if a.contentFlex == nil {
		return
	}
	a.contentFlex.Clear()
	if !a.navigationHidden {
		a.contentFlex.AddItem(a.navigationTree, 0, 2, a.focusedPane == FocusNavigation)
	}
	a.contentFlex.AddItem(a.issuesColumn, 0, 5, a.focusedPane == FocusIssues)
	if !a.detailsHidden {
		a.contentFlex.AddItem(a.detailsView, 0, 3, a.focusedPane == FocusDetails)
	}
}

// toggleNavigationPane shows or hides the navigation pane.
func (a *App) toggleNavigationPane() {
	a.navigationHidden = !a.navigationHidden
	a.rebuildContentLayout()
	a.updateFocus()
	if a.navigationHidden {
		a.flashStatus("Navigation pane hidden")
	} else {
		a.flashStatus("Navigation pane shown")
	}
}

// toggleDetailsPane shows or hides the details pane.
func (a *App) toggleDetailsPane() {
	a.detailsHidden = !a.detailsHidden
	a.rebuildContentLayout()
	a.updateFocus()
	if a.detailsHidden {
		a.flashStatus("Details pane hidden")
	} else {
		a.flashStatus("Details pane shown")
	}
}
