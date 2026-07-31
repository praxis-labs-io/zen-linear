package tui

// layoutMode captures how many panes fit the current terminal width.
type layoutMode int

const (
	layoutWide   layoutMode = iota // three panes
	layoutMedium                   // two panes: details appears only when focused
	layoutNarrow                   // one pane: whichever has focus
)

// layoutModeForWidth picks the layout mode for a terminal width in cells.
func layoutModeForWidth(width int) layoutMode {
	switch {
	case width >= 110:
		return layoutWide
	case width >= 70:
		return layoutMedium
	default:
		return layoutNarrow
	}
}

// rebuildContentLayout re-adds the visible panes to the content flex,
// honoring both manual pane toggles and the responsive layout mode. The
// issues column is the anchor; on narrow terminals only the focused pane
// shows, and focus movement (h/l/Tab) walks between panes.
func (a *App) rebuildContentLayout() {
	if a.contentFlex == nil {
		return
	}

	showNav := !a.navigationHidden
	showDetails := !a.detailsHidden
	showIssues := true
	navWeight := 2
	switch a.layoutMode {
	case layoutMedium:
		if a.focusedPane == FocusDetails {
			showNav = false
		} else {
			showDetails = false
		}
		navWeight = 1
	case layoutNarrow:
		showNav = showNav && a.focusedPane == FocusNavigation
		showDetails = showDetails && a.focusedPane == FocusDetails
		showIssues = !showNav && !showDetails
	}

	a.contentFlex.Clear()
	if showNav {
		a.contentFlex.AddItem(a.navigationTree, 0, navWeight, a.focusedPane == FocusNavigation)
	}
	if showIssues {
		a.contentFlex.AddItem(a.issuesColumn, 0, 5, a.focusedPane == FocusIssues)
	}
	if showDetails {
		a.contentFlex.AddItem(a.detailsView, 0, 3, a.focusedPane == FocusDetails)
	}
}

// watchLayoutWidth re-evaluates the responsive layout before every draw and
// rebuilds the panes when the terminal crosses a breakpoint.
func (a *App) watchLayoutWidth(width int) {
	if mode := layoutModeForWidth(width); mode != a.layoutMode {
		a.layoutMode = mode
		a.rebuildContentLayout()
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
