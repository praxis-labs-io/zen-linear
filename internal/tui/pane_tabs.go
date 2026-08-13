package tui

import (
	"fmt"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// The details pane renders lazygit-style tabs: one view fills the pane and the
// border title lists every tab, highlighting the active one. The [ and ] keys
// cycle them; { and } belong to the pane toggles. Both pairs are remappable, so
// read them through actionKey. The issues pane has no tabs: it shows the list
// the tree picked, or search results.

// tableForSection returns the table widget backing a section.
func (a *App) tableForSection(section IssuesSection) *tview.Table {
	switch section {
	case IssuesSectionList:
		return a.listIssuesTable
	case IssuesSectionSearch:
		return a.searchResultsTable
	}
	return nil
}

// jumpToSection mounts the given section in the issues pane and selects a row
// in it. Pass row 0 to keep whatever selection the section already has,
// including one a deferred render just made.
func (a *App) jumpToSection(section IssuesSection, row int) {
	a.activeIssuesSection = section
	// Flushes any render this section was owed, which may set its selection.
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
		// An empty section still mounts and takes focus, but drops the
		// selection, or every command keeps acting on the one we just left.
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

// jumpToParent selects an issue's parent in whatever is on screen, falling back
// to the navigation list, which holds every fetched issue. Reports whether the
// parent was found.
func (a *App) jumpToParent(parentID string) bool {
	section := a.activeIssuesSection
	row := 0
	// Search results are a flat list, so a parent there is not a tree move.
	if section != IssuesSectionSearch {
		row = a.getRowForIssueInSection(parentID, section)
	}
	if row < 1 {
		section = IssuesSectionList
		row = a.getRowForIssueInSection(parentID, section)
	}
	if row < 1 {
		return false
	}
	if section == IssuesSectionList {
		// Leaving the results for the list is leaving the search. A query left
		// in the box would describe a pane showing something else, and the
		// session would save both.
		a.clearNavSearch()
	}
	a.jumpToSection(section, row)
	return true
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

// paneTitle wraps a pane's label with its number, which goes accent colored
// while the pane holds focus.
func (a *App) paneTitle(number int, label string, focused bool) string {
	tag := a.themeTags.SecondaryText
	if focused {
		tag = a.themeTags.Accent
	}
	return fmt.Sprintf(" %s[%d][-] %s ", tag, number, label)
}

// paneTitleWidth measures a title as drawn, off the untagged label.
func paneTitleWidth(label string) int {
	// " [N] " plus the label plus the trailing space.
	return runewidth.StringWidth(label) + 6
}

// paneLabel colors the name of a pane that has one thing to name, so the label
// dims and lights with the number beside it. tabSegment is for the panes that
// still carry tabs, where the middle shade means "the active one of several".
func (a *App) paneLabel(label string, focused bool) string {
	tag := a.themeTags.SecondaryText
	if focused {
		tag = a.themeTags.Accent
	}
	return tag + label + "[-]"
}

// issuesTitleLabel names what the issues pane is showing, untagged. The context
// line shares the same border row and measures against this to know how much of
// it is already spoken for, so the text has one source and both callers use it.
func (a *App) issuesTitleLabel() string {
	if a.activeIssuesSection == IssuesSectionSearch {
		if count := visibleRowCount(a.searchIssueRows); count > 0 {
			return fmt.Sprintf("Search (%d)", count)
		}
		return "Search"
	}
	scope := a.issuesScopeLabel()
	if scope == "" {
		return "Issues"
	}
	return fmt.Sprintf("%s (%d)", scope, visibleRowCount(a.listIssueRows))
}

// issuesPaneTitle colors the issues pane label. Project and cycle names are
// Linear's and this title is built from color tags, so one called [red] would
// be read as one instead of printed.
func (a *App) issuesPaneTitle(focused bool) string {
	return a.paneLabel(tview.Escape(a.issuesTitleLabel()), focused)
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

// visibleRowCount counts rows excluding gap spacers, so a title's number stays
// unchanged by group spacing.
func visibleRowCount(rows []IssueRow) int {
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
	if a.detailsView == nil || a.detailsDescriptionView == nil || a.detailsCommentsPanel == nil {
		return
	}
	a.detailsView.Clear()
	if a.detailsCommentsVisible && a.focusedDetailsView {
		a.detailsView.AddItem(a.detailsCommentsPanel, 0, 1, true)
	} else {
		a.detailsView.AddItem(a.detailsDescriptionView, 0, 1, true)
	}
}
