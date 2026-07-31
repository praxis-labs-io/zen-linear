package tui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

// The issues and details panes render lazygit-style tabs: one view fills the
// pane and the border title lists every tab, highlighting the active one.
// The { and } keys cycle tabs within the focused pane.

// tableForSection returns the table widget backing a section.
func (a *App) tableForSection(section IssuesSection) *tview.Table {
	switch section {
	case IssuesSectionMy:
		return a.myIssuesTable
	case IssuesSectionOther:
		return a.otherIssuesTable
	case IssuesSectionAll:
		return a.allIssuesTable
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
	if row <= 1 {
		// Reset any stale scroll offset when landing at the top of a tab.
		table.SetOffset(0, 0)
	}
	if issue := a.getIssueFromRowForSection(row, a.activeIssuesSection); issue != nil {
		a.onIssueSelected(*issue)
	}
	a.updateFocus()
}

// sectionRows returns the row model backing a section.
func (a *App) sectionRows(section IssuesSection) []IssueRow {
	switch section {
	case IssuesSectionMy:
		return a.myIssueRows
	case IssuesSectionOther:
		return a.otherIssueRows
	case IssuesSectionAll:
		return a.issueRows
	}
	return nil
}

// cycleIssuesSection cycles the My, Other, and All tabs in the given
// direction (+1 forward, -1 backward), skipping empty tabs and keeping each
// tab's own selection.
func (a *App) cycleIssuesSection(direction int) {
	order := []IssuesSection{IssuesSectionMy, IssuesSectionOther, IssuesSectionAll}
	current := 0
	for i, section := range order {
		if section == a.activeIssuesSection {
			current = i
			break
		}
	}
	for step := 1; step < len(order); step++ {
		target := order[((current+step*direction)%len(order)+len(order))%len(order)]
		rows := a.sectionRows(target)
		if len(rows) == 0 {
			continue
		}
		table := a.tableForSection(target)
		row, _ := table.GetSelection()
		if row < 1 || row > len(rows) {
			row = 1
		}
		a.jumpToSection(target, row)
		return
	}
}

// issuesTabsTitle renders the tab strip for the issues pane border.
func (a *App) issuesTabsTitle(focused bool) string {
	prefix := " "
	if focused {
		prefix = " ▶ "
	}
	var segments []string
	if len(a.myIssueRows) > 0 {
		segments = append(segments, a.tabSegment(fmt.Sprintf("My (%d)", len(a.myIssueRows)), a.activeIssuesSection == IssuesSectionMy, focused))
	}
	segments = append(segments,
		a.tabSegment(fmt.Sprintf("Other (%d)", len(a.otherIssueRows)), a.activeIssuesSection == IssuesSectionOther, focused),
		a.tabSegment(fmt.Sprintf("All (%d)", len(a.issueRows)), a.activeIssuesSection == IssuesSectionAll, focused))
	return prefix + strings.Join(segments, " · ") + " "
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
