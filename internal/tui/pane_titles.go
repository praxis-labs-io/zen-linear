package tui

import (
	"fmt"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

func (a *App) tableForSection(section IssuesSection) *tview.Table {
	switch section {
	case IssuesSectionList:
		return a.listIssuesTable
	case IssuesSectionSearch:
		return a.searchResultsTable
	}
	return nil
}

func (a *App) jumpToSection(section IssuesSection, row int) {
	a.activeIssuesSection = section
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
		a.clearSelectedIssue()
		a.updateFocus()
		return
	}
	selectIssueRow(table, rows, row)
	if issue := a.getIssueFromRowForSection(row, section); issue != nil {
		a.onIssueSelected(*issue)
	}
	a.updateFocus()
}

func (a *App) clearSelectedIssue() {
	a.abandonDetailFetch()
	a.issuesMu.Lock()
	a.selectedIssue = nil
	a.issuesMu.Unlock()
	a.updateDetailsView()
}

func (a *App) jumpToParent(parentID string) bool {
	section := a.activeIssuesSection
	row := 0
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
		a.clearNavSearch()
	}
	a.jumpToSection(section, row)
	return true
}

const (
	paneNumberNavigation = 1
	paneNumberIssues     = 2
	paneNumberDetails    = 3
)

func (a *App) paneTitle(number int, label string, focused bool) string {
	tag := a.themeTags.SecondaryText
	if focused {
		tag = a.themeTags.Accent
	}
	return fmt.Sprintf(" %s[%d][-] %s ", tag, number, label)
}

func paneTitleWidth(label string) int {
	return runewidth.StringWidth(label) + 6
}

func (a *App) paneLabel(label string, focused bool) string {
	tag := a.themeTags.SecondaryText
	if focused {
		tag = a.themeTags.Accent
	}
	return tag + label + "[-]"
}

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

func (a *App) issuesPaneTitle(focused bool) string {
	return a.paneLabel(tview.Escape(a.issuesTitleLabel()), focused)
}

func visibleRowCount(rows []IssueRow) int {
	count := 0
	for _, row := range rows {
		if !row.IsSpacer {
			count++
		}
	}
	return count
}
