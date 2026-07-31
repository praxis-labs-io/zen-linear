package tui

import (
	"fmt"

	"github.com/rivo/tview"
)

// The issues and details panes render lazygit-style tabs: one view fills the
// pane and the border title lists every tab, highlighting the active one.
// The [ and ] keys cycle tabs within the focused pane.

// tableForSection returns the table widget backing a section.
func (a *App) tableForSection(section IssuesSection) *tview.Table {
	switch section {
	case IssuesSectionMy:
		return a.myIssuesTable
	case IssuesSectionOther:
		return a.otherIssuesTable
	}
	return nil
}

// jumpToSection makes the given section the visible issues tab and selects a
// row in it.
func (a *App) jumpToSection(section IssuesSection, row int) {
	a.activeIssuesSection = section
	a.updateIssuesColumnLayout()
	table := a.tableForSection(a.activeIssuesSection)
	if table == nil || row < 1 {
		return
	}
	table.Select(row, 0)
	if issue := a.getIssueFromRowForSection(row, a.activeIssuesSection); issue != nil {
		a.onIssueSelected(*issue)
	}
	a.updateFocus()
}

// cycleIssuesSection switches between the My Issues and Other Issues tabs,
// keeping each tab's own selection.
func (a *App) cycleIssuesSection() {
	target := IssuesSectionMy
	rows := a.myIssueRows
	if a.activeIssuesSection == IssuesSectionMy {
		target = IssuesSectionOther
		rows = a.otherIssueRows
	}
	if len(rows) == 0 {
		return
	}
	table := a.tableForSection(target)
	row, _ := table.GetSelection()
	if row < 1 || row > len(rows) {
		row = 1
	}
	a.jumpToSection(target, row)
}

// issuesTabsTitle renders the tab strip for the issues pane border.
func (a *App) issuesTabsTitle(focused bool) string {
	prefix := " "
	if focused {
		prefix = " ▶ "
	}
	if len(a.myIssueRows) == 0 {
		return prefix + fmt.Sprintf("Other Issues (%d) ", len(a.otherIssueRows))
	}
	my := a.tabSegment(fmt.Sprintf("My Issues (%d)", len(a.myIssueRows)), a.activeIssuesSection == IssuesSectionMy, focused)
	other := a.tabSegment(fmt.Sprintf("Other Issues (%d)", len(a.otherIssueRows)), a.activeIssuesSection == IssuesSectionOther, focused)
	return prefix + my + " · " + other + " "
}

// detailsTabsTitle renders the tab strip for the details pane border.
func (a *App) detailsTabsTitle(focused bool) string {
	prefix := " "
	if focused {
		prefix = " ▶ "
	}
	if !a.detailsCommentsVisible {
		return prefix + "Details "
	}
	details := a.tabSegment("Details", !a.focusedDetailsView, focused)
	comments := a.tabSegment("Comments", a.focusedDetailsView, focused)
	return prefix + details + " · " + comments + " "
}

// tabSegment colors one tab label: the active tab follows focus, inactive
// tabs stay muted.
func (a *App) tabSegment(label string, active bool, focused bool) string {
	tag := a.themeTags.SecondaryText
	if active {
		tag = a.themeTags.Foreground
		if focused {
			tag = a.themeTags.Accent
		}
	}
	return tag + label + "[-]"
}

// updateDetailsLayout shows the active details tab at full height.
func (a *App) updateDetailsLayout() {
	if a.detailsView == nil || a.detailsDescriptionView == nil || a.detailsCommentsView == nil {
		return
	}
	a.detailsView.Clear()
	if a.detailsCommentsVisible && a.focusedDetailsView {
		a.detailsView.AddItem(a.detailsCommentsView, 0, 1, true)
	} else {
		a.detailsView.AddItem(a.detailsDescriptionView, 0, 1, true)
	}
}
