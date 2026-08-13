package tui

import (
	"context"
	"strings"

	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// Search results come from the server-side searchIssues query and live in their
// own model (searchIssues/searchIssueRows), so a background refresh of the
// navigation list never touches what the user is browsing. The query box that
// drives this lives in the navigation pane; see nav_search.go.

// performIssueSearch runs a workspace-wide, first-page-only search and mounts
// the result in the issues pane. It owns activeIssuesSection: an empty query
// puts the list back, a real one shows the results, so the pane and the query
// box can never disagree. Called on the UI thread by the debounce callback.
func (a *App) performIssueSearch(query string) {
	query = strings.TrimSpace(query)
	// Read the generation after the cancel, which bumps it: a number taken
	// before would be stale by the time this fetch answers, and this fetch's
	// own result would be thrown away.
	a.cancelSearchFetch()
	generation := a.searchFetchGeneration.Load()
	if query == "" {
		a.clearSearchResults()
		a.jumpToSection(IssuesSectionList, 0)
		return
	}
	a.searchLoading = true
	a.searchErr = nil
	a.activeIssuesSection = IssuesSectionSearch
	a.updateIssuesColumnLayout()

	fetchPage := a.fetchIssuesPage
	if fetchPage == nil {
		fetchPage = a.api.FetchIssuesPage
	}
	// Workspace-wide relevance search: no navigation or rich filters, no
	// OrderBy, first page only.
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
			a.searchLoading = false
			if err != nil {
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
			// A restored session lands on the issue it was left on, when the
			// results still hold it. One shot: later searches start at the top.
			if restored := a.pendingSearchIssueID; restored != "" {
				a.pendingSearchIssueID = ""
				if _, ok := a.searchIDToIssue[restored]; ok {
					selectedID = restored
				}
			}
			renderIssuesTableModel(a.searchResultsTable, a.searchIssueRows, a.searchIDToIssue, selectedID, a.theme, a.issueColumns())
			a.updateIssuesColumnLayout()
		})
	}()
}

// cancelSearchFetch stops the in-flight search request, if any, and invalidates
// whatever it was going to deliver. The bump is the load-bearing half: a
// canceled request answers with context.Canceled, and every keystroke cancels
// one, so without it the pane reads "Search failed" between every two letters
// the user types. UI thread only, like the rest of the search path.
func (a *App) cancelSearchFetch() {
	a.searchFetchGeneration.Add(1)
	if a.searchFetchCancel != nil {
		a.searchFetchCancel()
		a.searchFetchCancel = nil
	}
}

// clearSearchResults drops all search results and invalidates in-flight
// fetches.
func (a *App) clearSearchResults() {
	a.cancelSearchFetch()
	a.searchIssues = nil
	a.searchIssueRows = nil
	a.searchIDToIssue = make(map[string]*linearapi.Issue)
	a.searchLoading = false
	a.searchErr = nil
}
