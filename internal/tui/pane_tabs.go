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
// All while the My tab has no rows. The active section itself is left
// untouched so My re-applies once data arrives.
func (a *App) effectiveIssuesSection() IssuesSection {
	if a.activeIssuesSection == IssuesSectionMy && len(a.myIssueRows) == 0 {
		return IssuesSectionAll
	}
	return a.activeIssuesSection
}

// tableForSection returns the table widget backing a section.
func (a *App) tableForSection(section IssuesSection) *tview.Table {
	switch section {
	case IssuesSectionAll:
		return a.allIssuesTable
	case IssuesSectionMy:
		return a.myIssuesTable
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

// jumpToParent selects an issue's parent in the tab on screen, falling back to
// All, which holds every fetched issue. Reports whether the parent was found.
func (a *App) jumpToParent(parentID string) bool {
	section := a.effectiveIssuesSection()
	row := 0
	// Search results are a flat list, so a parent there is not a tree move.
	if section != IssuesSectionSearch {
		row = a.getRowForIssueInSection(parentID, section)
	}
	if row < 1 {
		section = IssuesSectionAll
		row = a.getRowForIssueInSection(parentID, section)
	}
	if row < 1 {
		return false
	}
	a.jumpToSection(section, row)
	return true
}

// cycleIssuesSection cycles the All, My, and Search tabs in the given
// direction (+1 forward, -1 backward), skipping empty tabs and keeping each
// tab's own selection.
func (a *App) cycleIssuesSection(direction int) {
	order := []IssuesSection{IssuesSectionAll, IssuesSectionMy, IssuesSectionSearch}
	current := 0
	for i, section := range order {
		if section == a.effectiveIssuesSection() {
			current = i
			break
		}
	}
	for step := 1; step < len(order); step++ {
		target := order[((current+step*direction)%len(order)+len(order))%len(order)]
		rows := a.rowsForSection(target)
		// The Search tab stays reachable while empty: its body is the input
		// plus a placeholder, not rows.
		if len(rows) == 0 && target != IssuesSectionSearch {
			continue
		}
		table := a.tableForSection(target)
		if table == nil {
			continue
		}
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
	segments := []string{
		a.tabSegment(fmt.Sprintf("All (%d)", tabRowCount(a.allIssueRows)), shown == IssuesSectionAll, focused),
	}
	if len(a.myIssueRows) > 0 {
		segments = append(segments, a.tabSegment(fmt.Sprintf("My (%d)", tabRowCount(a.myIssueRows)), shown == IssuesSectionMy, focused))
	}
	searchLabel := "Search"
	if len(a.searchIssueRows) > 0 {
		searchLabel = fmt.Sprintf("Search (%d)", tabRowCount(a.searchIssueRows))
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

// tabRowCount counts a tab's rows excluding gap spacers, so the tab strip
// numbers stay unchanged by group spacing.
func tabRowCount(rows []IssueRow) int {
	count := 0
	for _, row := range rows {
		if !row.IsSpacer {
			count++
		}
	}
	return count
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
