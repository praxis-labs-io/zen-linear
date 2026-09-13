package tui

type layoutMode int

const (
	layoutWide layoutMode = iota
	layoutMedium
	layoutNarrow
)

const (
	navWeight            = 5
	navWeightWithDetails = 6
	navWeightZoomed      = 3
	issuesWeight         = 15
	detailsWeight        = 10
)

// Fixed rather than a share, which ran down to 11 columns; 21 matches the three-pane share at the wide breakpoint.
const navWidthMedium = 21

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

func (a *App) rebuildContentLayout() {
	if a.contentFlex == nil {
		return
	}

	showNav := !a.navigationHidden
	showDetails := !a.detailsHidden
	showIssues := true
	nav := navWeight
	navFixed := 0
	if a.detailsZoomed && showDetails {
		a.contentFlex.Clear()
		if showNav && a.layoutMode == layoutWide {
			a.contentFlex.AddItem(a.navigationPanel, 0, navWeightZoomed, a.focusedPane == FocusNavigation)
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
		navFixed = navWidthMedium
	case layoutNarrow:
		showNav = showNav && a.focusedPane == FocusNavigation
		showDetails = showDetails && a.focusedPane == FocusDetails
		showIssues = !showNav && !showDetails
	}

	a.contentFlex.Clear()
	if showNav {
		a.contentFlex.AddItem(a.navigationPanel, navFixed, nav, a.focusedPane == FocusNavigation)
	}
	if showIssues {
		a.contentFlex.AddItem(a.issuesColumn, 0, issuesWeight, a.focusedPane == FocusIssues)
	}
	if showDetails {
		a.contentFlex.AddItem(a.detailsView, 0, detailsWeight, a.focusedPane == FocusDetails)
	}
}

// Runs inside Application.draw under the app lock, so it must never call SetFocus or GetFocus.
func (a *App) watchLayoutWidth(width int) {
	mode := layoutModeForWidth(width)
	if mode == a.layoutMode {
		return
	}
	a.layoutMode = mode
	if a.detailsZoomed && a.focusedPane != FocusDetails {
		a.resolveFocusedPane()
		a.layoutFocusStale = true
	}
	a.rebuildContentLayout()
	a.applyPaneBorders()
	a.applyNavSearchStyles()
	a.updateStatusBar()
}

func (a *App) repairLayoutFocus() {
	if !a.layoutFocusStale {
		return
	}
	a.layoutFocusStale = false
	a.updateFocus()
}

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

func (a *App) toggleDetailsPane() {
	if a.detailsZoomed {
		return
	}
	a.detailsHidden = !a.detailsHidden
	a.rebuildContentLayout()
	a.updateFocus()
	if a.detailsHidden {
		a.flashStatus("Details pane hidden")
	} else {
		a.flashStatus("Details pane shown")
	}
}

func (a *App) releaseDetailsZoom() {
	if !a.detailsZoomed {
		return
	}
	a.detailsZoomed = false
	a.detailsHidden = a.zoomPreviousHidden
}

func (a *App) toggleDetailsZoom() {
	if !a.detailsZoomed && a.GetSelectedIssue() == nil {
		a.flashStatus("No issue selected")
		return
	}
	if a.detailsZoomed {
		a.releaseDetailsZoom()
		a.focusedPane = a.zoomPreviousPane
	} else {
		a.zoomPreviousPane = a.focusedPane
		a.zoomPreviousHidden = a.detailsHidden
		a.detailsZoomed = true
		a.detailsHidden = false
		a.focusedPane = FocusDetails
	}
	a.rebuildContentLayout()
	a.updateFocus()
	if a.detailsZoomed {
		a.flashStatus("Details zoomed")
	} else {
		a.flashStatus("Details unzoomed")
	}
}
