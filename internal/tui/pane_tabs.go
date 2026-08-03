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
	rows := a.rowsForSection(a.activeIssuesSection)
	if table == nil || row < 1 || len(rows) == 0 {
		// An empty tab is still a tab: mount it, focus it, select nothing.
		a.updateFocus()
		return
	}
	// Reset any stale scroll offset when landing at the top of a tab, so
	// leading group headers stay visible.
	selectIssueRow(table, rows, row)
	if issue := a.getIssueFromRowForSection(row, a.activeIssuesSection); issue != nil {
		a.onIssueSelected(*issue)
	}
	a.updateFocus()
}

// jumpToParent selects an issue's parent in the tab on screen, falling back to
// All, which holds every fetched issue. Reports whether the parent was found.
func (a *App) jumpToParent(parentID string) bool {
	section := a.activeIssuesSection
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
// direction (+1 forward, -1 backward), keeping each tab's own selection. The
// three tabs are fixed, so an empty one is reachable and shows itself empty
// rather than being skipped over.
func (a *App) cycleIssuesSection(direction int) {
	order := []IssuesSection{IssuesSectionAll, IssuesSectionMy, IssuesSectionSearch}
	current := 0
	for i, section := range order {
		if section == a.activeIssuesSection {
			current = i
			break
		}
	}
	target := order[((current+direction)%len(order)+len(order))%len(order)]
	rows := a.rowsForSection(target)
	row := 1
	if table := a.tableForSection(target); table != nil {
		if selected, _ := table.GetSelection(); selected >= 1 && selected <= len(rows) {
			row = selected
		}
	}
	a.jumpToSection(target, row)
}

// issuesTabsTitle renders the tab strip for the issues pane border.
func (a *App) issuesTabsTitle(focused bool) string {
	prefix := " "
	if focused {
		prefix = " ▶ "
	}
	shown := a.activeIssuesSection
	segments := []string{
		a.tabSegment(fmt.Sprintf("All (%d)", tabRowCount(a.allIssueRows)), shown == IssuesSectionAll, focused),
	}
	segments = append(segments, a.tabSegment(fmt.Sprintf("My (%d)", tabRowCount(a.myIssueRows)), shown == IssuesSectionMy, focused))
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
