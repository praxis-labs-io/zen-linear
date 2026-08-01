package tui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

// The issues and details panes render lazygit-style tabs: one view fills the
// pane and the border title lists every tab, highlighting the active one.
// The { and } keys cycle tabs within the focused pane.

// effectiveIssuesSection returns the section to display: the active one, or
// Other Issues while the My tab has no rows. The active section itself is
// left untouched so the My default re-applies once data arrives.
func (a *App) effectiveIssuesSection() IssuesSection {
	if a.activeIssuesSection == IssuesSectionMy && len(a.myIssueRows) == 0 {
		return IssuesSectionOther
	}
	return a.activeIssuesSection
}

// tableForSection returns the table widget backing a section.
func (a *App) tableForSection(section IssuesSection) *tview.Table {
	switch section {
	case IssuesSectionMy:
		return a.myIssuesTable
	case IssuesSectionOther:
		return a.otherIssuesTable
	case IssuesSectionAll:
		return a.allIssuesTable
	case IssuesSectionSearch:
		return a.searchResultsTable
	}
	return nil
}

// jumpToSection makes the given section the visible issues tab and selects a
// row in it.
func (a *App) jumpToSection(section IssuesSection, row int) {
	if section == IssuesSectionSearch {
		// Entering the Search tab always lands on its input; Down/Enter
		// reach the results.
		a.activeIssuesSection = section
		a.searchInputFocused = true
		a.updateIssuesColumnLayout()
		a.updateFocus()
		return
	}
	a.activeIssuesSection = section
	a.updateIssuesColumnLayout()
	table := a.tableForSection(a.activeIssuesSection)
	if table == nil || row < 1 {
		return
	}
	// Reset any stale scroll offset when landing at the top of a tab, so
	// leading group headers stay visible.
	selectIssueRow(table, a.rowsForSection(a.activeIssuesSection), row)
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
	case IssuesSectionSearch:
		return a.searchIssueRows
	}
	return nil
}

// cycleIssuesSection cycles the My, Other, and All tabs in the given
// direction (+1 forward, -1 backward), skipping empty tabs and keeping each
// tab's own selection.
func (a *App) cycleIssuesSection(direction int) {
	order := []IssuesSection{IssuesSectionMy, IssuesSectionOther, IssuesSectionAll, IssuesSectionSearch}
	current := 0
	for i, section := range order {
		if section == a.effectiveIssuesSection() {
			current = i
			break
		}
	}
	for step := 1; step < len(order); step++ {
		target := order[((current+step*direction)%len(order)+len(order))%len(order)]
		rows := a.sectionRows(target)
		// The Search tab stays reachable while empty: its body is the input
		// plus a placeholder, not rows.
		if len(rows) == 0 && target != IssuesSectionSearch {
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
	shown := a.effectiveIssuesSection()
	var segments []string
	if len(a.myIssueRows) > 0 {
		segments = append(segments, a.tabSegment(fmt.Sprintf("My (%d)", len(a.myIssueRows)), shown == IssuesSectionMy, focused))
	}
	segments = append(segments,
		a.tabSegment(fmt.Sprintf("Other (%d)", len(a.otherIssueRows)), shown == IssuesSectionOther, focused),
		a.tabSegment(fmt.Sprintf("All (%d)", len(a.issueRows)), shown == IssuesSectionAll, focused))
	searchLabel := "Search"
	if len(a.searchIssueRows) > 0 {
		searchLabel = fmt.Sprintf("Search (%d)", len(a.searchIssueRows))
	}
	segments = append(segments, a.tabSegment(searchLabel, shown == IssuesSectionSearch, focused))
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
	comments := a.tabSegment(fmt.Sprintf("Comments (%d)", a.selectedIssueCommentCount()), a.focusedDetailsView, focused)
	return prefix + details + " · " + comments + " "
}

// selectedIssueCommentCount returns how many comments the selected issue has.
func (a *App) selectedIssueCommentCount() int {
	a.issuesMu.RLock()
	defer a.issuesMu.RUnlock()
	if a.selectedIssue == nil {
		return 0
	}
	return len(a.selectedIssue.Comments)
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
