package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// The Search tab hosts a query input above a flat results list, like Linear's
// own search. Results come from the server-side searchIssues query and live in
// their own model (searchIssues/searchIssueRows), so the My/Other/All tabs are
// never touched by searching.

// buildSearchPanel (re)creates the Search tab widgets. Called from buildLayout
// and again on theme changes: tview bakes the input's inner background at
// construction, so re-theming requires a fresh InputField.
func (a *App) buildSearchPanel() {
	previousQuery := a.searchQuery
	if a.searchInput != nil {
		previousQuery = a.searchInput.GetText()
	}

	a.searchInput = newThemedInputField(a.theme.InputBg)
	a.searchInput.
		SetLabel("/ ").
		SetLabelColor(a.theme.Accent).
		SetFieldWidth(0).
		SetPlaceholder("Search issues...").
		SetFieldStyle(tcell.StyleDefault.Foreground(a.theme.Foreground).Background(a.theme.InputBg)).
		SetPlaceholderStyle(tcell.StyleDefault.Foreground(a.theme.SecondaryText).Background(a.theme.InputBg)).
		SetBackgroundColor(a.theme.Background)
	// Restore the query before installing the change handler so a theme
	// rebuild does not re-fire the search.
	a.searchInput.SetText(previousQuery)
	a.searchInput.SetChangedFunc(func(text string) {
		a.searchQuery = text
		a.scheduleSearchDebounce(text)
	})

	if a.searchResultsTable == nil {
		a.searchResultsTable = a.buildIssuesTable("", IssuesSectionSearch)
		// The panel flex owns the border and tab title; the inner table
		// stays borderless.
		a.searchResultsTable.SetBorder(false)
	}

	a.searchPlaceholder = tview.NewTextView()
	a.searchPlaceholder.SetTextAlign(tview.AlignCenter)
	a.searchPlaceholder.SetTextColor(a.theme.SecondaryText)
	a.searchPlaceholder.SetBackgroundColor(a.theme.Background)

	a.searchBody = tview.NewFlex().SetDirection(tview.FlexRow)
	// Flex sets dontClear and never paints its own background; restore the
	// fill so the layer beneath cannot bleed through.
	a.searchBody.Box = tview.NewBox().SetBackgroundColor(a.theme.Background)

	a.searchPanel = tview.NewFlex().SetDirection(tview.FlexRow)
	a.searchPanel.Box = tview.NewBox().SetBackgroundColor(a.theme.Background)
	a.searchPanel.
		SetBorder(true).
		SetBorderColor(a.theme.Border).
		SetBackgroundColor(a.theme.Background)
	a.searchPanel.
		AddItem(a.searchInput, 1, 0, true).
		AddItem(a.searchBody, 0, 1, false)

	a.updateSearchBody()
}

// updateSearchBody mounts the results table when there are rows, or a
// vertically centered status message otherwise.
func (a *App) updateSearchBody() {
	if a.searchBody == nil {
		return
	}
	a.searchBody.Clear()
	if len(a.searchIssueRows) > 0 {
		a.searchBody.AddItem(a.searchResultsTable, 0, 1, false)
		return
	}
	message := "Search issues"
	color := a.theme.SecondaryText
	lines := 1
	switch {
	case a.searchErr != nil:
		message = fmt.Sprintf("Search failed\n%v", a.searchErr)
		color = a.theme.StatusCanceled
		lines = 2
	case a.searchLoading:
		message = "Searching..."
	case strings.TrimSpace(a.searchQuery) != "":
		message = "No results"
	}
	a.searchPlaceholder.SetText(message)
	a.searchPlaceholder.SetTextColor(color)
	a.searchBody.
		AddItem(nil, 0, 1, false).
		AddItem(a.searchPlaceholder, lines, 0, false).
		AddItem(nil, 0, 1, false)
}

// performIssueSearch runs a workspace-wide, first-page-only search. Called on
// the UI thread by the debounce callback.
func (a *App) performIssueSearch(query string) {
	query = strings.TrimSpace(query)
	generation := a.searchFetchGeneration.Add(1)
	// The generation counter already discards a superseded result. Canceling
	// stops the request too, so a fast typist is not leaving one query per
	// debounce window running against the API.
	a.cancelSearchFetch()
	if query == "" {
		a.clearSearchResults()
		a.updateSearchBody()
		a.updateAllPaneTitles()
		return
	}
	a.searchLoading = true
	a.searchErr = nil
	a.updateSearchBody()

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
				a.updateSearchBody()
				return
			}
			a.searchErr = nil
			a.searchIssues = page.Issues
			a.searchIssueRows, a.searchIDToIssue = buildFlatSearchRows(a.searchIssues)
			selectedID := ""
			if len(a.searchIssueRows) > 0 {
				selectedID = a.searchIssueRows[0].IssueID
			}
			renderIssuesTableModel(a.searchResultsTable, a.searchIssueRows, a.searchIDToIssue, selectedID, a.theme, a.issueColumns())
			a.updateSearchBody()
			a.updateAllPaneTitles()
		})
	}()
}

// cancelSearchFetch stops the in-flight search request, if any. UI thread only,
// like the rest of the search path.
func (a *App) cancelSearchFetch() {
	if a.searchFetchCancel != nil {
		a.searchFetchCancel()
		a.searchFetchCancel = nil
	}
}

// clearSearchResults drops all search results and invalidates in-flight
// fetches.
func (a *App) clearSearchResults() {
	a.searchFetchGeneration.Add(1)
	a.cancelSearchFetch()
	a.searchIssues = nil
	a.searchIssueRows = nil
	a.searchIDToIssue = make(map[string]*linearapi.Issue)
	a.searchLoading = false
	a.searchErr = nil
}

// openSearchTab shows the Search tab and focuses its input.
func (a *App) openSearchTab() {
	if a.activeIssuesSection != IssuesSectionSearch {
		a.searchReturnSection = a.activeIssuesSection
	}
	a.activeIssuesSection = IssuesSectionSearch
	a.focusedPane = FocusIssues
	a.searchInputFocused = true
	a.updateIssuesColumnLayout()
	a.updateFocus()
}

// focusSearchInput moves sub-focus from the results list back to the input.
func (a *App) focusSearchInput() {
	a.searchInputFocused = true
	a.updateFocus()
}

// searchInputActive reports whether typed keys belong to the search input.
func (a *App) searchInputActive() bool {
	return a.focusedPane == FocusIssues &&
		a.activeIssuesSection == IssuesSectionSearch &&
		a.searchInputFocused
}

// handleSearchInputKey routes keys while the search input has focus. Anything
// not handled here falls through to the InputField, so plain letters type
// instead of firing global shortcuts.
func (a *App) handleSearchInputKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlC:
		a.app.Stop()
		return nil
	case tcell.KeyEscape:
		if a.searchInput.GetText() != "" {
			// First Esc clears the query (the change handler clears the
			// results); a second Esc leaves the tab.
			a.searchInput.SetText("")
			return nil
		}
		a.searchInputFocused = false
		a.jumpToSection(a.searchReturnSection, 1)
		return nil
	case tcell.KeyEnter, tcell.KeyDown:
		if len(a.searchIssueRows) == 0 {
			return nil
		}
		a.searchInputFocused = false
		row, _ := a.searchResultsTable.GetSelection()
		if row < 1 || row > len(a.searchIssueRows) {
			row = 1
		}
		a.searchResultsTable.Select(row, 0)
		if issue := a.getIssueFromRowForSection(row, IssuesSectionSearch); issue != nil {
			a.onIssueSelected(*issue)
		}
		a.updateFocus()
		return nil
	case tcell.KeyTab, tcell.KeyBacktab:
		a.searchInputFocused = false
		if event.Key() == tcell.KeyBacktab || event.Modifiers()&tcell.ModShift != 0 {
			a.cyclePanesBackward()
		} else {
			a.cyclePanesForward()
		}
		return nil
	case tcell.KeyRune:
		// Tab-cycling keys leave the Search tab instead of typing into the
		// query.
		switch event.Rune() {
		case a.actionKey("tab_prev", '{'):
			a.searchInputFocused = false
			a.cycleIssuesSection(-1)
			return nil
		case a.actionKey("tab_next", '}'):
			a.searchInputFocused = false
			a.cycleIssuesSection(1)
			return nil
		}
	}
	return event
}
