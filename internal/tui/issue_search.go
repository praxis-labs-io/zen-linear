package tui

import (
	"context"
	"strings"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

func (a *App) performIssueSearch(query string) {
	query = strings.TrimSpace(query)
	a.cancelSearchFetch()
	generation := a.searchFetchGeneration.Load()
	if query == "" {
		a.clearSearchResults()
		a.jumpToSection(IssuesSectionList, 0)
		return
	}
	a.setSearchLoading(true)
	a.searchErr = nil
	a.activeIssuesSection = IssuesSectionSearch
	a.updateIssuesColumnLayout()

	fetchPage := a.fetchIssuesPage
	if fetchPage == nil {
		fetchPage = a.api.FetchIssuesPage
	}
	params := linearapi.FetchIssuesParams{
		Search: query,
		First:  a.config.PageSize,
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.searchFetchCancel = cancel
	go func() {
		defer cancel()
		page, err := fetchPage(ctx, params, nil)
		a.QueueUpdateDraw(func() {
			if generation != a.searchFetchGeneration.Load() {
				return
			}
			a.setSearchLoading(false)
			if err != nil {
				a.searchIssues = nil
				a.searchIssueRows = nil
				a.searchIDToIssue = make(map[string]*linearapi.Issue)
				a.searchErr = err
				a.updateIssuesColumnLayout()
				return
			}
			a.searchErr = nil
			a.searchIssues = page.Issues
			a.searchIssueRows, a.searchIDToIssue = buildFlatSearchRows(a.searchIssues)
			selectedID := ""
			if len(a.searchIssueRows) > 0 {
				selectedID = a.searchIssueRows[0].IssueID
			}
			if restored := a.pendingSearchIssueID; restored != "" {
				a.pendingSearchIssueID = ""
				if _, ok := a.searchIDToIssue[restored]; ok {
					selectedID = restored
				}
			}
			renderIssuesTableModel(a.searchResultsTable, a.searchIssueRows, a.searchIDToIssue, selectedID, a.theme, a.issueColumns())
			a.updateIssuesColumnLayout()
			if issue := a.searchIDToIssue[selectedID]; issue != nil {
				a.onIssueSelected(*issue)
			}
		})
	}()
}

// The generation bump is what keeps a canceled request from reading as "Search failed" on every keystroke.
func (a *App) cancelSearchFetch() {
	a.searchFetchGeneration.Add(1)
	if a.searchFetchCancel != nil {
		a.searchFetchCancel()
		a.searchFetchCancel = nil
	}
}

func (a *App) clearSearchResults() {
	a.cancelSearchFetch()
	a.searchIssues = nil
	a.searchIssueRows = nil
	a.searchIDToIssue = make(map[string]*linearapi.Issue)
	a.pendingSearchIssueID = ""
	a.setSearchLoading(false)
	a.searchErr = nil
}
