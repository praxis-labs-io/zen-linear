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
// row in it. Pass row 0 to keep whatever selection the tab already has,
// including one a deferred render just made.
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
	// Flushes any render this tab was owed, which may set its selection.
	a.updateIssuesColumnLayout()
	table := a.tableForSection(section)
	rows := a.rowsForSection(section)

	if table != nil && len(rows) > 0 {
		if row < 1 || row > len(rows) {
			row, _ = table.GetSelection()
		}
		if row < 1 || row > len(rows) || rows[row-1].IsSpacer {
			row = nextIssueRow(rows, 0, 1)
		}
	} else {
		row = 0
	}

	if row < 1 {
		// An empty tab is still a tab: mount it and focus it, but drop the
		// selection, or every command keeps acting on the tab we just left.
		a.clearSelectedIssue()
		a.updateFocus()
		return
	}
	// Reset any stale scroll offset when landing at the top of a tab, so
	// leading group headers stay visible.
	selectIssueRow(table, rows, row)
	if issue := a.getIssueFromRowForSection(row, section); issue != nil {
		a.onIssueSelected(*issue)
	}
	a.updateFocus()
}

// clearSelectedIssue drops the selection and empties the details pane.
func (a *App) clearSelectedIssue() {
	a.abandonDetailFetch()
	a.issuesMu.Lock()
	a.selectedIssue = nil
	a.issuesMu.Unlock()
	a.updateDetailsView()
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
	a.jumpToSection(order[((current+direction)%len(order)+len(order))%len(order)], 0)
}

// Pane numbers shown in the titles and typed to focus a pane. They are fixed
// to the pane, not to what is on screen, so a hidden pane keeps its number.
const (
	paneNumberNavigation = 1
	paneNumberIssues     = 2
	paneNumberDetails    = 3
)

// tabSeparator sits between tab labels in a pane title.
const tabSeparator = " - "

// paneTitle wraps a pane's tab strip with its number.
func (a *App) paneTitle(number int, tabs string) string {
	return fmt.Sprintf(" %s[%d][-] %s ", a.themeTags.SecondaryText, number, tabs)
}

// issuesTabsTitle renders the tab strip for the issues pane border.
func (a *App) issuesTabsTitle(focused bool) string {
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
	return strings.Join(segments, tabSeparator)
}

// detailsTabsTitle renders the tab strip for the details pane border.
func (a *App) detailsTabsTitle(focused bool) string {
	if !a.detailsCommentsVisible {
		return a.tabSegment("Details", true, focused)
	}
	details := a.tabSegment("Details", !a.focusedDetailsView, focused)
	comments := a.tabSegment(fmt.Sprintf("Comments (%d)", a.selectedIssueCommentCount()), a.focusedDetailsView, focused)
	return details + tabSeparator + comments
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
