package tui

import (
	"context"
	"strings"

	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// issueColumns returns the configured issue list columns, or the default
// Linear-style layout.
func (a *App) issueColumns() []string {
	if len(a.config.Columns) == 0 {
		return DefaultIssueColumns
	}
	return a.config.Columns
}

// buildIssueRowsFor builds table rows for an issue list, honoring the
// grouping dimensions in effect (the active view's, else config).
func (a *App) buildIssueRowsFor(issues []linearapi.Issue) ([]IssueRow, map[string]*linearapi.Issue) {
	groupBy := a.effectiveGroupBy()
	if groupBy == GroupByNone {
		return BuildIssueRows(issues, a.expandedState)
	}
	return BuildGroupedIssueRows(issues, a.expandedState, groupBy, a.effectiveSubgroupBy(), a.collapsedGroups)
}

// toggleGroupCollapse collapses or expands a group header and keeps the
// header selected after the rebuild.
func (a *App) toggleGroupCollapse(section IssuesSection, header IssueRow) {
	if header.HeaderKey == "" {
		return
	}
	if a.collapsedGroups == nil {
		a.collapsedGroups = make(map[string]bool)
	}
	a.collapsedGroups[header.HeaderKey] = !a.collapsedGroups[header.HeaderKey]

	a.issuesMu.RLock()
	targetIssueID := ""
	if a.selectedIssue != nil {
		targetIssueID = a.selectedIssue.ID
	}
	a.issuesMu.RUnlock()
	a.applyRebuiltSelection(targetIssueID, a.rebuildIssuesTables(targetIssueID))

	// Re-select the toggled header so repeated presses toggle in place.
	table := a.tableForSection(section)
	if table == nil {
		return
	}
	for index, row := range a.rowsForSection(section) {
		if row.IsHeader && row.HeaderKey == header.HeaderKey {
			table.Select(index+1, 0)
			break
		}
	}
}

// regroupIssues rebuilds the tables after a grouping change, keeping the
// current selection.
func (a *App) regroupIssues(message string) {
	a.issuesMu.RLock()
	targetIssueID := ""
	if a.selectedIssue != nil {
		targetIssueID = a.selectedIssue.ID
	}
	a.issuesMu.RUnlock()

	a.applyRebuiltSelection(targetIssueID, a.rebuildIssuesTables(targetIssueID))
	a.flashStatus(message)
}

// applyRebuiltSelection installs the selection rebuildIssuesTables resolved.
// The list model carries no comments, relations, or attachments, so the same
// issue keeps the hydrated copy and a different one fetches its own.
func (a *App) applyRebuiltSelection(previousID string, selected *linearapi.Issue) {
	switch {
	case selected == nil:
		a.clearSelectedIssue()
	case selected.ID == previousID:
		a.setSelectedIssue(*selected)
		a.updateDetailsView()
	default:
		a.selectIssueNow(*selected)
	}
}

// showSortByPicker selects the list ordering. Every row is a whole ordering,
// so one keystroke settles it.
func (a *App) showSortByPicker() {
	a.pickerModal.Show("Sort Issues By", sortOrderingPickerItems(a.configuredSortFields), func(item PickerItem) {
		a.setSortFields(parseSortFields(strings.Split(item.ID, ",")))
	})
}

// groupDimensionPickerItems lists the grouping dimensions for the pickers.
func groupDimensionPickerItems() []PickerItem {
	return []PickerItem{
		{ID: GroupByNone, Label: "None"},
		{ID: GroupByStatus, Label: "Status"},
		{ID: GroupByPriority, Label: "Priority"},
		{ID: GroupByAssignee, Label: "Assignee"},
		{ID: GroupByCycle, Label: "Cycle"},
		{ID: GroupByProject, Label: "Project"},
		{ID: GroupByMilestone, Label: "Milestone"},
	}
}

// showGroupByPicker selects the primary grouping dimension.
func (a *App) showGroupByPicker() {
	a.pickerModal.Show("Group Issues By", groupDimensionPickerItems(), func(item PickerItem) {
		// A manual grouping choice outranks the active view's for the
		// session.
		a.groupingOverridden = true
		a.config.GroupBy = item.ID
		if a.config.SubgroupBy == item.ID {
			a.config.SubgroupBy = GroupByNone
		}
		if item.ID == GroupByNone {
			a.regroupIssues("Grouping off")
		} else {
			a.regroupIssues("Grouped by " + item.Label)
		}
	})
}

// showSubgroupByPicker selects the secondary grouping dimension.
func (a *App) showSubgroupByPicker() {
	if a.config.GroupBy == GroupByNone {
		a.flashStatus("Set a grouping first (Group issues by…)")
		return
	}
	items := make([]PickerItem, 0, 4)
	for _, item := range groupDimensionPickerItems() {
		if item.ID != a.config.GroupBy {
			items = append(items, item)
		}
	}
	a.pickerModal.Show("Subgroup Issues By", items, func(item PickerItem) {
		a.groupingOverridden = true
		a.config.SubgroupBy = item.ID
		if item.ID == GroupByNone {
			a.regroupIssues("Subgrouping off")
		} else {
			a.regroupIssues("Subgrouped by " + item.Label)
		}
	})
}

// toggleIssueExpanded toggles the expand/collapse state of a parent issue.
func (a *App) toggleIssueExpanded(issueID string) {
	// All holds every fetched issue, whichever tab the press came from.
	issue, ok := a.listIDToIssue[issueID]
	if !ok || issue == nil {
		logger.Debug("tui.app: issue not found for toggle issue_id=%s", issueID)
		return
	}

	// Only toggle if this issue has children
	if len(issue.Children) == 0 {
		return
	}

	wasExpanded := a.expandedState[issueID]
	logger.Debug("tui.app: toggling issue expanded issue=%s was_expanded=%v", issue.Identifier, wasExpanded)

	ToggleExpanded(a.expandedState, issueID)

	a.rebuildIssueRowModels()
	a.renderIssueSections(a.sectionSelectionsFor(issueID))
	a.updateIssuesColumnLayout()
}

// onNavigationSelected handles when a navigation item is selected. An
// optional issueID asks the refresh to land on that issue, which is how a
// restored session reopens the row the user left on.
func (a *App) onNavigationSelected(node *NavigationNode, issueID ...string) {
	logger.Debug("tui.app: navigation selected node_id=%s node_text=%s is_team=%v is_project=%v is_cycle=%v is_issue=%v", node.ID, node.Text, node.IsTeam, node.IsProject, node.IsCycle, node.IsIssue)

	// Picking a list is asking to see it, and a live query is holding the pane
	// that would show it. The refresh below mounts the list; there is nothing
	// to select in it yet. A restore is the exception: it picks the saved node
	// and the saved query together, and the query is what the user left up.
	if a.searchQuery != "" && !a.restoringSession {
		a.clearNavSearch()
		a.activeIssuesSection = IssuesSectionList
	}

	// A new list starts fresh: its own view settings apply again until the
	// user overrides them.
	a.groupingOverridden = false
	a.sortOverridden = false

	// Picking a list is asking to see it, so the zoom covering it gives way.
	// A favorited issue is the opposite request, so the zoom survives that one
	// and the issue lands in the pane already open to read it. The refresh
	// below does not move focus, so releasing has to carry focus itself: it
	// can close the details pane, and whoever is in it cannot stay there.
	if a.detailsZoomed && !node.IsIssue {
		a.releaseDetailsZoom()
		a.rebuildContentLayout()
		a.updateFocus()
	}

	// A favorited issue is not a filter of its own: scope to its team and ask
	// the refresh to land on the issue via the target-issue plumbing.
	if node.IsIssue {
		a.selectedNavigation = &NavigationNode{
			ID:     node.TeamID,
			Text:   node.Text,
			TeamID: node.TeamID,
			IsTeam: true,
		}
		if node.TeamID != "" {
			go a.preloadTeamMetadataFunc(node.TeamID)
		}
		a.refreshIssuesWithFocusChange(false, node.IssueID)
		return
	}

	a.selectedNavigation = node

	// Update selected team metadata for commands and create-issue defaults.
	if node.TeamID != "" {
		go a.preloadTeamMetadataFunc(node.TeamID)
	}

	a.refreshIssuesWithFocusChange(false, issueID...)
}

// preloadTeamMetadata warms team-scoped metadata caches for commands and create-issue defaults.
func (a *App) preloadTeamMetadata(teamID string) {
	logger.Debug("tui.app: preloading team metadata team_id=%s", teamID)
	ctx := context.Background()
	_ = a.cache.PreloadTeamMetadata(ctx, teamID)

	users, _ := a.cache.GetUsers(ctx, teamID)
	projects, _ := a.cache.GetProjects(ctx, teamID)
	states, _ := a.cache.GetWorkflowStates(ctx, teamID)
	cycles, _ := a.cache.GetCycles(ctx, teamID)
	labels, _ := a.cache.GetIssueLabels(ctx, teamID)

	logger.Debug("tui.app: loaded team metadata team_id=%s users_count=%d projects_count=%d states_count=%d cycles_count=%d", teamID, len(users), len(projects), len(states), len(cycles))
	a.app.QueueUpdateDraw(func() {
		a.teamUsers = users
		a.teamProjects = projects
		a.workflowStates = states
		a.teamCycles = cycles
		a.teamLabels = labels
		a.metadataTeamID = teamID
	})
}

// setSortFields sets the sort chain and refreshes issues. A manual sort
// choice outranks the active view's ordering for the session, and follows the
// grouping pickers in updating the in-memory config so a later settings save
// records the choice instead of reverting it.
func (a *App) setSortFields(fields []SortField) {
	if len(fields) == 0 {
		return
	}
	logger.Debug("tui.app: setting sort chain fields=%s", sortChainLabel(fields))
	a.sortFields = fields
	a.sortOverridden = true
	a.config.SortBy = sortConfigNames(fields)

	// Reorder what is already loaded before the refresh, so the list matches
	// the status bar even when the fetch fails.
	a.issuesMu.Lock()
	a.sortIssuesLocally()
	a.issuesMu.Unlock()
	a.regroupIssues("")

	a.refreshIssues()
}
