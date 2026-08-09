package tui

// layoutMode captures how many panes fit the current terminal width.
type layoutMode int

const (
	layoutWide   layoutMode = iota // three panes
	layoutMedium                   // two panes: details appears only when focused
	layoutNarrow                   // one pane: whichever has focus
)

// Flex weights for the content split, as shares of the visible panes' total.
// The nav pane takes a different share in each of the three arrangements it
// appears in, so it has a weight per case; the other two panes keep theirs.
// The issues weight is 15 because that is the smallest number leaving every
// nav weight a whole one. Change a share by working out the fraction it should
// be of the panes on screen, not by rescaling these.
const (
	navWeight            = 5 // nav and issues, with details toggled off
	navWeightWithDetails = 6 // all three panes
	navWeightMedium      = 3 // the two-pane responsive layout
	navWeightZoomed      = 3 // the zoomed details view, with the nav kept as a spine
	issuesWeight         = 15
	detailsWeight        = 10
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
	if a.detailsZoomed && showDetails {
		// The zoom drops the issues list. The nav tree is the spine you keep
		// your place on, so it survives on a wide terminal; below that
		// breakpoint it does not fit beside the reading measure, and the
		// reading is the point.
		a.contentFlex.Clear()
		if showNav && a.layoutMode == layoutWide {
			a.contentFlex.AddItem(a.navigationTree, 0, navWeightZoomed, a.focusedPane == FocusNavigation)
		}
		a.contentFlex.AddItem(a.detailsView, 0, detailsWeight, a.focusedPane == FocusDetails)
		return
	}
	switch a.layoutMode {
	case layoutWide:
		if showDetails {
			nav = navWeightWithDetails
		}
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
	// Hiding a zoomed pane would leave the content area holding nothing.
	a.detailsZoomed = a.detailsZoomed && !a.detailsHidden
	a.rebuildContentLayout()
	a.updateFocus()
	if a.detailsHidden {
		a.flashStatus("Details pane hidden")
	} else {
		a.flashStatus("Details pane shown")
	}
}

// toggleDetailsZoom widens the details pane over the issues list, for reading a
// whole issue rather than glancing at one.
func (a *App) toggleDetailsZoom() {
	if !a.detailsZoomed && a.GetSelectedIssue() == nil {
		a.flashStatus("No issue selected")
		return
	}
	a.detailsZoomed = !a.detailsZoomed
	a.detailsHidden = false
	// The zoom is a way of looking at the details pane, not a way of leaving
	// it, so unzooming keeps you there. Esc, h, Left, and 2 are the keys that
	// mean the list.
	a.focusedPane = FocusDetails
	a.rebuildContentLayout()
	a.updateFocus()
	if a.detailsZoomed {
		a.flashStatus("Details zoomed")
	} else {
		a.flashStatus("Details unzoomed")
	}
}
