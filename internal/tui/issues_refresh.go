package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// issuesRepaintInterval bounds how long fetched-but-unpainted issues stay
// unreachable during pagination. Short enough that the list keeps filling in,
// long enough that a fast multi-page load still paints a handful of times
// rather than once per page.
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
	// The query changed, so anything in flight is already stale. Drop it here
	// rather than a debounce window later when the next fetch starts, or every
	// typing burst holds a known-dead request against the API while the live
	// query queues behind it.
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

// queueIssuesRefresh records a refresh request while a fetch is in progress.
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

// runQueuedIssuesRefresh triggers any queued refresh after a fetch completes.
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

// currentFetchParams describes the issue list as it is scoped right now: the
// rich filters plus whatever the navigation selection narrows to. Callers must
// build it on the UI thread, since it reads state a refresh reassigns.
func (a *App) currentFetchParams(orderBy string) linearapi.FetchIssuesParams {
	params := linearapi.FetchIssuesParams{
		First:   a.config.PageSize,
		OrderBy: orderBy,
	}
	a.applyRichFiltersToParams(&params)

	// Apply team/project/state filter based on navigation selection
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
			// A team-scoped All Issues favorite carries a team and none of the
			// flags above, so it must stay last to avoid shadowing them.
			params.TeamID = a.selectedNavigation.TeamID
		}
		// Workspace-wide "All Issues" reaches here with nothing set, unfiltered
	}
	return params
}

// refreshIssues fetches issues from the API and updates the UI.
func (a *App) refreshIssues() {
	a.refreshIssuesWithFocusChange(true)
}

// refreshIssuesWithFocusChange fetches issues and optionally shifts focus to the
// issues pane. It must be called on the UI thread: everything it reads before
// starting the fetch, isLoading included, belongs to the event loop, and that
// contract is what keeps them free of synchronization.
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
	// Snapshot the chain here: setSortFields reassigns it on the UI thread
	// while this goroutine runs.
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

		// A custom view carries its own display settings; fetch them first
		// so the issue query can use the view's sort. Failures fall back to
		// the configured defaults.
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
				a.finishIssuesLoad(generation, err)
				logger.ErrorWithErr(err, "tui.app: failed to fetch issues")
				a.updateStatusBarWithError(err)
				a.updateIssuesColumnLayout()
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
			logger.Debug("tui.app: fetched issues page=%d count=%d", pageCount, len(page.Issues))
			// Install (or clear) the active view's display settings with
			// the list they belong to.
			a.viewPrefs = prefs
			a.updateIssuesData(page.Issues, targetIssueID)
			a.issuesMu.RLock()
			merge.reset(a.issues)
			a.issuesMu.RUnlock()
			if allowFocus {
				// Ensure focus is on issues table after initial load
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
					a.updateStatusBarWithError(err)
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
				// A superseded refresh must not merge into the list the
				// surviving one paints. The check above ran on the fetching
				// goroutine; the generation can change before this closure does.
				if generation != a.refreshGeneration.Load() {
					return
				}
				if a.accumulateIssues(page.Issues, merge) {
					unpainted = true
				}
				// Repaint on a budget rather than per page. Per page costs
				// roughly fifty times a single rebuild for the same end state;
				// never until the last page leaves fetched issues unreachable
				// for the whole of a slow load.
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
			// A superseded refresh must not paint its partial list. The queued
			// refresh that replaced it runs next and owns the table.
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

// updateIssuesColumnLayout shows the active issues section at full height.
func (a *App) updateIssuesColumnLayout() {
	// Focus lives on the primitive, not the pane, so swapping the table for the
	// placeholder under a focused pane sends keys to something off screen.
	refocus := a.issuesPaneHasFocus()
	a.issuesColumn.Clear()

	// A section about to come on screen may still be holding cells from before
	// the last model change.
	a.flushPendingSectionRender(a.activeIssuesSection)

	// A section with no rows mounts the placeholder, which says what it is
	// waiting on rather than showing column headers over nothing.
	if a.issuesPaneIsEmpty() && a.issuesPlaceholder != nil {
		a.updateIssuesPlaceholder()
		a.issuesColumn.AddItem(a.issuesPlaceholder, 0, 1, false)
	} else {
		a.issuesColumn.AddItem(a.tableForSection(a.activeIssuesSection), 0, 1, false)
	}

	// Update all pane titles to reflect current state
	a.updateAllPaneTitles()

	if refocus {
		a.updateFocus()
	}
}

// updateIssuesData updates the UI with new issues data.
// If issueID is provided, that issue will be selected if found in the list.
func (a *App) updateIssuesData(issues []linearapi.Issue, issueID ...string) {
	a.issuesMu.Lock()
	a.issues = issues
	a.sortIssuesLocally()

	// Determine target issue ID
	var targetIssueID string
	if len(issueID) > 0 && issueID[0] != "" {
		targetIssueID = issueID[0]
	} else if a.selectedIssue != nil {
		targetIssueID = a.selectedIssue.ID
	}
	a.issuesMu.Unlock()

	selectedIssue := a.rebuildIssuesTables(targetIssueID)
	if a.activeIssuesSection == IssuesSectionSearch {
		// A background refresh must not overwrite the search result the
		// user is browsing.
		a.updateStatusBar()
		return
	}
	if selectedIssue != nil {
		// A refresh repoints the list rather than moving the cursor, so there
		// is nothing to debounce: deferring the render leaves the pane on the
		// pre-refresh copy, or empty on a cold start, until the window closes.
		a.selectIssueNow(*selectedIssue)
	} else {
		a.clearSelectedIssue()
	}
	a.updateStatusBar()
}

// rebuildIssuesTables rebuilds issue rows and renders tables, returning the
// selected issue. The returned issue is a copy: the id maps point into a
// snapshot of the list that the next rebuild replaces.
func (a *App) rebuildIssuesTables(targetIssueID string) *linearapi.Issue {
	a.rebuildIssueRowModels()

	a.renderIssueSections(a.sectionSelectionsFor(targetIssueID))
	a.updateIssuesColumnLayout()

	var found *linearapi.Issue
	if targetIssueID != "" {
		found = a.listIDToIssue[targetIssueID]
	}

	// Without a target, fall back to the first issue row of the tab on screen,
	// skipping group headers, which carry no issue. Falling back to All instead
	// would hand the caller an issue from a tab the user is not looking at. The
	// Search tab keeps its own selection.
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

// pageMerge carries dedup state across the pages of one refresh, so merging a
// page stays linear instead of rebuilding a set over the whole accumulated
// slice every time.
type pageMerge struct {
	seen map[string]bool
	// length a.issues had when this merge last wrote it. A different length
	// means something else spliced the list.
	length int
}

// reset seeds the merge from the current list. Callers must hold issuesMu.
func (m *pageMerge) reset(issues []linearapi.Issue) {
	clear(m.seen)
	for i := range issues {
		m.seen[issues[i].ID] = true
	}
	m.length = len(issues)
}

// accumulateIssues merges a fetched page into the issue list without painting.
// Reports whether anything was added, so a run of empty or fully duplicated
// pages costs no repaint.
func (a *App) accumulateIssues(newIssues []linearapi.Issue, merge *pageMerge) bool {
	a.issuesMu.Lock()
	defer a.issuesMu.Unlock()

	// insertIssue splices into a.issues when an edit brings an issue into
	// scope, and it can land mid-refresh. Reconcile rather than trust the set,
	// or the server page carrying that issue appends it a second time.
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
		// Hold the sort invariant across pagination. Repaint paths that read
		// a.issues directly (toggleIssueExpanded, expand_all) would otherwise
		// render fetch order. Sorting was never the expensive part; the
		// regroup and full table repaint were.
		a.sortIssuesLocally()
	}
	merge.length = len(a.issues)
	return added
}

// renderAccumulatedIssues repaints the list from the accumulated issues.
// accumulateIssues owns the sort, so this only rebuilds and paints.
func (a *App) renderAccumulatedIssues() {
	a.issuesMu.RLock()
	previousID := ""
	if a.selectedIssue != nil {
		previousID = a.selectedIssue.ID
	}
	a.issuesMu.RUnlock()

	selectedIssue := a.rebuildIssuesTables(previousID)

	if a.activeIssuesSection == IssuesSectionSearch {
		// A background refresh must not overwrite the search result the user
		// is browsing, and its selection is not in the My/Other models.
		a.updateStatusBar()
		return
	}

	if selectedIssue != nil && selectedIssue.ID == previousID {
		// The selection survived the reorder. Keep the copy onIssueSelected
		// hydrated: the list model carries no comments, relations, or
		// attachments, so replacing it here strips them from the details pane.
		a.updateStatusBar()
		return
	}

	a.issuesMu.Lock()
	a.selectedIssue = selectedIssue
	a.issuesMu.Unlock()
	a.updateDetailsView()
	a.updateStatusBar()
}

// sortIssuesLocally applies the sort chain. The API can only order by one
// timestamp, so every field past the first, and priority and status at any
// position, are resolved here. Callers must hold issuesMu.
func (a *App) sortIssuesLocally() {
	sortIssuesByFields(a.issues, a.effectiveSortFields())
}

// issueContextLine renders "ID · Title" for issue-scoped modals, so every
// form names the issue it modifies the same way.
func (a *App) issueContextLine(issue linearapi.Issue) string {
	title := []rune(strings.TrimSpace(issue.Title))
	const maxTitleRunes = 48
	if len(title) > maxTitleRunes {
		title = append(title[:maxTitleRunes-1], '…')
	}
	return fmt.Sprintf("%s%s[-] %s%s[-]", a.themeTags.Accent, issue.Identifier, a.themeTags.SecondaryText, string(title))
}
