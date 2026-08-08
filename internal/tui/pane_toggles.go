package tui

// layoutMode captures how many panes fit the current terminal width.
type layoutMode int

const (
	layoutWide   layoutMode = iota // three panes
	layoutMedium                   // two panes: details appears only when focused
	layoutNarrow                   // one pane: whichever has focus
)

// Flex weights for the content split. The three-pane layout is 1:3:2; the
// numbers are scaled by five so navWeightMedium can hold the nav pane at the
// same share of a two-pane split, where details is gone.
const (
	navWeight       = 5
	issuesWeight    = 15
	detailsWeight   = 10
	navWeightMedium = 3
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
	nav := navWeight
	switch a.layoutMode {
	case layoutMedium:
		if a.focusedPane == FocusDetails {
			showNav = false
		} else {
			showDetails = false
		}
		nav = navWeightMedium
	case layoutNarrow:
		showNav = showNav && a.focusedPane == FocusNavigation
		showDetails = showDetails && a.focusedPane == FocusDetails
		showIssues = !showNav && !showDetails
	}

	a.contentFlex.Clear()
	if showNav {
		a.contentFlex.AddItem(a.navigationTree, 0, nav, a.focusedPane == FocusNavigation)
	}
	if showIssues {
		a.contentFlex.AddItem(a.issuesColumn, 0, issuesWeight, a.focusedPane == FocusIssues)
	}
	if showDetails {
		a.contentFlex.AddItem(a.detailsView, 0, detailsWeight, a.focusedPane == FocusDetails)
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
