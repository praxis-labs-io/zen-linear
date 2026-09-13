package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
)

const issuesRepaintInterval = 250 * time.Millisecond

func (a *App) searchDebounceDelay() time.Duration {
	if a.config.SearchDebounce > 0 {
		return a.config.SearchDebounce
	}
	return config.DefaultSearchDebounce
}

func (a *App) scheduleSearchDebounce(query string) {
	delay := a.searchDebounceDelay()
	generation := a.searchDebounceGeneration.Add(1)
	a.cancelSearchFetch()

	a.searchDebounceMu.Lock()
	if a.searchDebounceTimer != nil {
		a.searchDebounceTimer.Stop()
	}
	a.searchDebounceTimer = time.AfterFunc(delay, func() {
		if generation != a.searchDebounceGeneration.Load() {
			return
		}
		a.QueueUpdateDraw(func() {
			if generation != a.searchDebounceGeneration.Load() {
				return
			}
			a.performIssueSearch(query)
		})
	})
	a.searchDebounceMu.Unlock()
}

func (a *App) cancelSearchDebounce() {
	a.searchDebounceGeneration.Add(1)

	a.searchDebounceMu.Lock()
	if a.searchDebounceTimer != nil {
		a.searchDebounceTimer.Stop()
		a.searchDebounceTimer = nil
	}
	a.searchDebounceMu.Unlock()
}

func (a *App) queueIssuesRefresh(allowFocusChange bool, issueID ...string) {
	logger.Debug("tui.app: queueing issues refresh issue_id=%v", issueID)
	a.pendingRefresh = true
	a.pendingRefreshAllowFocusChange = allowFocusChange
	a.refreshGeneration.Add(1)
	if len(issueID) > 0 {
		a.pendingRefreshIssueID = issueID[0]
		return
	}
	a.pendingRefreshIssueID = ""
}

func (a *App) runQueuedIssuesRefresh() {
	if !a.pendingRefresh {
		return
	}
	issueID := a.pendingRefreshIssueID
	allowFocusChange := a.pendingRefreshAllowFocusChange
	logger.Debug("tui.app: running queued refresh issue_id=%s", issueID)
	a.pendingRefresh = false
	a.pendingRefreshIssueID = ""
	a.pendingRefreshAllowFocusChange = true
	if issueID != "" {
		a.refreshIssuesWithFocusChange(allowFocusChange, issueID)
		return
	}
	a.refreshIssuesWithFocusChange(allowFocusChange)
}

func (a *App) notifyRefreshCompleted() {
	if a.refreshCompleted != nil {
		a.refreshCompleted()
	}
}

func (a *App) currentFetchParams(orderBy string) linearapi.FetchIssuesParams {
	params := linearapi.FetchIssuesParams{
		First:   a.config.PageSize,
		OrderBy: orderBy,
	}
	a.applyRichFiltersToParams(&params)

	if a.selectedNavigation != nil {
		switch {
		case a.selectedNavigation.CustomViewID != "":
			params.CustomViewID = a.selectedNavigation.CustomViewID
		case a.selectedNavigation.StateType != "":
			params.TeamID = a.selectedNavigation.TeamID
			params.StateType = a.selectedNavigation.StateType
		case a.selectedNavigation.IsStatus:
			params.TeamID = a.selectedNavigation.TeamID
			params.StateID = a.selectedNavigation.StateID
		case a.selectedNavigation.IsCycle:
			params.TeamID = a.selectedNavigation.TeamID
			params.CycleID = a.selectedNavigation.CycleID
		case a.selectedNavigation.IsTeam:
			params.TeamID = a.selectedNavigation.TeamID
		case a.selectedNavigation.IsProject:
			params.TeamID = a.selectedNavigation.TeamID
			params.ProjectID = a.selectedNavigation.ID
		case a.selectedNavigation.TeamID != "":
			params.TeamID = a.selectedNavigation.TeamID
		}
	}
	return params
}

func (a *App) refreshIssues() {
	a.refreshIssuesWithFocusChange(true)
}

func (a *App) refreshIssuesWithFocusChange(allowFocusChange bool, issueID ...string) {
	if a.isLoading {
		a.queueIssuesRefresh(allowFocusChange, issueID...)
		return
	}
	targetIssueID := ""
	if len(issueID) > 0 {
		targetIssueID = issueID[0]
	}
	logger.Debug("tui.app: starting issues refresh target_issue_id=%s", targetIssueID)
	generation := a.refreshGeneration.Add(1)
	a.loadingGeneration = generation
	a.issuesErr = nil
	a.setIssuesLoading(true)

	allowFocus := allowFocusChange
	orderBy := string(a.sortFields[0])
	params := a.currentFetchParams(orderBy)
	sortOverridden := a.sortOverridden
	fetchPage := a.fetchIssuesPage
	if fetchPage == nil {
		fetchPage = a.api.FetchIssuesPage
	}
	fetchPrefs := a.fetchViewPrefsFunc
	if fetchPrefs == nil {
		fetchPrefs = a.api.FetchCustomViewPreferences
	}
	a.setLoadingMessage("Loading...")
	go func() {
		refreshStarted := time.Now()
		ctx := context.Background()

		var prefs *viewDisplayPrefs
		if params.CustomViewID != "" {
			values, prefsErr := fetchPrefs(ctx, params.CustomViewID)
			if prefsErr != nil {
				logger.ErrorWithErr(prefsErr, "tui.app: failed to fetch view preferences view_id=%s", params.CustomViewID)
			} else if values != nil {
				logger.Debug("tui.app: view preferences view_id=%s grouping=%q subgrouping=%q ordering=%q direction=%q", params.CustomViewID, values.IssueGrouping, values.IssueSubGrouping, values.ViewOrdering, values.ViewOrderingDirection)
				prefs = resolveViewPrefs(values)
			}
			if prefs != nil && prefs.hasSort && !sortOverridden {
				params.OrderBy = string(prefs.sortField)
			}
		}

		pageCount := 0
		fetchedCount := 0
		logger.Debug("tui.app: refreshing issues team_id=%s project_id=%s state_id=%s cycle_id=%s assignee_id=%s labels=%d", params.TeamID, params.ProjectID, params.StateID, params.CycleID, params.AssigneeID, len(params.LabelIDs))
		page, err := fetchPage(ctx, params, nil)
		if err != nil {
			a.QueueUpdateDraw(func() {
				logger.ErrorWithErr(err, "tui.app: failed to fetch issues")
				if generation != a.refreshGeneration.Load() {
					a.finishIssuesLoad(generation, nil)
				} else {
					a.finishIssuesLoad(generation, err)
					a.updateStatusBarWithError(err)
					a.updateIssuesColumnLayout()
				}
				a.notifyRefreshCompleted()
				a.runQueuedIssuesRefresh()
			})
			return
		}
		if generation != a.refreshGeneration.Load() {
			a.QueueUpdateDraw(func() {
				a.finishIssuesLoad(generation, nil)
				a.notifyRefreshCompleted()
				a.runQueuedIssuesRefresh()
			})
			return
		}

		pageCount++
		fetchedCount += len(page.Issues)
		merge := &pageMerge{seen: make(map[string]bool, len(page.Issues))}
		lastPaint := time.Now()
		a.QueueUpdateDraw(func() {
			if generation != a.refreshGeneration.Load() {
				return
			}
			logger.Debug("tui.app: fetched issues page=%d count=%d", pageCount, len(page.Issues))
			a.viewPrefs = prefs
			a.updateIssuesData(page.Issues, targetIssueID)
			a.issuesMu.RLock()
			merge.reset(a.issues)
			a.issuesMu.RUnlock()
			if allowFocus {
				a.focusedPane = FocusIssues
				a.updateFocus()
			}
			if page.HasNext {
				a.setLoadingMessage(fmt.Sprintf("Loading more (page %d, fetched %d)...", pageCount, fetchedCount))
			}
		})

		after := page.EndCursor
		unpainted := false
		for page.HasNext {
			if generation != a.refreshGeneration.Load() {
				break
			}
			nextPage, err := fetchPage(ctx, params, after)
			if err != nil {
				a.QueueUpdateDraw(func() {
					logger.ErrorWithErr(err, "tui.app: failed to fetch more issues page=%d", pageCount+1)
					if generation == a.refreshGeneration.Load() {
						a.updateStatusBarWithError(err)
					}
				})
				break
			}
			if generation != a.refreshGeneration.Load() {
				break
			}

			page = nextPage
			after = page.EndCursor
			pageCount++
			fetchedCount += len(page.Issues)
			a.QueueUpdateDraw(func() {
				if generation != a.refreshGeneration.Load() {
					return
				}
				if a.accumulateIssues(page.Issues, merge) {
					unpainted = true
				}
				if unpainted && time.Since(lastPaint) >= issuesRepaintInterval {
					a.renderAccumulatedIssues()
					unpainted = false
					lastPaint = time.Now()
				}
				if page.HasNext {
					a.setLoadingMessage(fmt.Sprintf("Loading more (page %d, fetched %d)...", pageCount, fetchedCount))
				}
			})
		}

		a.QueueUpdateDraw(func() {
			if unpainted && generation == a.refreshGeneration.Load() {
				a.renderAccumulatedIssues()
			}
			a.finishIssuesLoad(generation, nil)
			logger.Debug("tui.app: refresh completed pages=%d total_fetched=%d elapsed=%s", pageCount, fetchedCount, time.Since(refreshStarted))
			a.updateStatusBar()
			a.notifyRefreshCompleted()
			a.runQueuedIssuesRefresh()
		})
	}()
}

func (a *App) applyRichFiltersToParams(params *linearapi.FetchIssuesParams) {
	if params == nil {
		return
	}
	filters := a.richFilters
	if filters.AssigneeID != "" {
		params.AssigneeID = filters.AssigneeID
	}
	if len(filters.LabelIDs) > 0 {
		params.LabelIDs = append([]string(nil), filters.LabelIDs...)
	}
	if filters.StateID != "" {
		params.StateID = filters.StateID
	}
	if filters.ProjectID != "" {
		params.ProjectID = filters.ProjectID
	}
	if filters.CycleID != "" {
		params.CycleID = filters.CycleID
	}
	if !filters.DueDate.Empty() {
		params.DueDate = filters.DueDate
	}
	if !filters.Estimate.Empty() {
		params.Estimate = filters.Estimate
	}
}

func (a *App) updateIssuesColumnLayout() {
	refocus := a.issuesPaneHasFocus()
	a.issuesColumn.Clear()

	a.flushPendingSectionRender(a.activeIssuesSection)

	if a.issuesPaneIsEmpty() && a.issuesPlaceholder != nil {
		a.updateIssuesPlaceholder()
		a.issuesColumn.AddItem(a.issuesPlaceholder, 0, 1, false)
	} else {
		a.issuesColumn.AddItem(a.tableForSection(a.activeIssuesSection), 0, 1, false)
	}

	a.updateAllPaneTitles()
	a.applyNavSearchStyles()

	if refocus {
		a.updateFocus()
	}
}

func (a *App) updateIssuesData(issues []linearapi.Issue, issueID ...string) {
	a.issuesMu.Lock()
	a.issues = issues
	a.sortIssuesLocally()

	var targetIssueID string
	if len(issueID) > 0 && issueID[0] != "" {
		targetIssueID = issueID[0]
	} else if a.selectedIssue != nil {
		targetIssueID = a.selectedIssue.ID
	}
	a.issuesMu.Unlock()

	selectedIssue := a.rebuildIssuesTables(targetIssueID)
	if a.activeIssuesSection == IssuesSectionSearch {
		a.updateStatusBar()
		return
	}
	if selectedIssue != nil {
		a.selectIssueNow(*selectedIssue)
	} else {
		a.clearSelectedIssue()
	}
	a.updateStatusBar()
}

func (a *App) rebuildIssuesTables(targetIssueID string) *linearapi.Issue {
	a.rebuildIssueRowModels()

	a.renderIssueSections(a.sectionSelectionsFor(targetIssueID))
	a.updateIssuesColumnLayout()

	var found *linearapi.Issue
	if targetIssueID != "" {
		found = a.listIDToIssue[targetIssueID]
	}

	if found == nil && a.activeIssuesSection != IssuesSectionSearch {
		rows := a.rowsForSection(a.activeIssuesSection)
		if first := nextIssueRow(rows, 0, 1); first > 0 {
			found = a.issueMapForSection(a.activeIssuesSection)[rows[first-1].IssueID]
		}
	}

	if found == nil {
		return nil
	}
	selected := *found
	return &selected
}

type pageMerge struct {
	seen   map[string]bool
	length int
}

func (m *pageMerge) reset(issues []linearapi.Issue) {
	clear(m.seen)
	for i := range issues {
		m.seen[issues[i].ID] = true
	}
	m.length = len(issues)
}

func (a *App) accumulateIssues(newIssues []linearapi.Issue, merge *pageMerge) bool {
	a.issuesMu.Lock()
	defer a.issuesMu.Unlock()

	if len(a.issues) != merge.length {
		merge.reset(a.issues)
	}

	added := false
	for _, issue := range newIssues {
		if merge.seen[issue.ID] {
			continue
		}
		a.issues = append(a.issues, issue)
		merge.seen[issue.ID] = true
		added = true
	}
	if added {
		a.sortIssuesLocally()
	}
	merge.length = len(a.issues)
	return added
}

func (a *App) renderAccumulatedIssues() {
	a.issuesMu.RLock()
	previousID := ""
	if a.selectedIssue != nil {
		previousID = a.selectedIssue.ID
	}
	a.issuesMu.RUnlock()

	selectedIssue := a.rebuildIssuesTables(previousID)

	if a.activeIssuesSection == IssuesSectionSearch {
		a.updateStatusBar()
		return
	}

	if selectedIssue != nil && selectedIssue.ID == previousID {
		a.updateStatusBar()
		return
	}

	a.issuesMu.Lock()
	a.selectedIssue = selectedIssue
	a.issuesMu.Unlock()
	a.updateDetailsView()
	a.updateStatusBar()
}

// The API orders by one timestamp only, so the rest of the chain is resolved here. Callers must hold issuesMu.
func (a *App) sortIssuesLocally() {
	sortIssuesByFields(a.issues, a.effectiveSortFields())
}

func (a *App) issueContextLine(issue linearapi.Issue) string {
	title := []rune(strings.TrimSpace(issue.Title))
	const maxTitleRunes = 48
	if len(title) > maxTitleRunes {
		title = append(title[:maxTitleRunes-1], '…')
	}
	return fmt.Sprintf("%s%s[-] %s%s[-]", a.themeTags.Accent, issue.Identifier, a.themeTags.SecondaryText, string(title))
}
