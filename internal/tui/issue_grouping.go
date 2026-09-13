package tui

import (
	"context"
	"strings"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
)

func (a *App) issueColumns() []string {
	if len(a.config.Columns) == 0 {
		return DefaultIssueColumns
	}
	return a.config.Columns
}

func (a *App) buildIssueRowsFor(issues []linearapi.Issue) ([]IssueRow, map[string]*linearapi.Issue) {
	groupBy := a.effectiveGroupBy()
	if groupBy == GroupByNone {
		return BuildIssueRows(issues, a.expandedState)
	}
	return BuildGroupedIssueRows(issues, a.expandedState, groupBy, a.effectiveSubgroupBy(), a.collapsedGroups)
}

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

// The list model carries no comments, relations, or attachments, so the same issue keeps its hydrated copy.
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

func (a *App) showSortByPicker() {
	a.pickerModal.Show("Sort Issues By", sortOrderingPickerItems(a.configuredSortFields), func(item PickerItem) {
		a.setSortFields(parseSortFields(strings.Split(item.ID, ",")))
	})
}

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

func (a *App) showGroupByPicker() {
	a.pickerModal.Show("Group Issues By", groupDimensionPickerItems(), func(item PickerItem) {
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

func (a *App) toggleIssueExpanded(issueID string) {
	issue, ok := a.listIDToIssue[issueID]
	if !ok || issue == nil {
		logger.Debug("tui.app: issue not found for toggle issue_id=%s", issueID)
		return
	}

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

func (a *App) onNavigationSelected(node *NavigationNode, issueID ...string) {
	logger.Debug("tui.app: navigation selected node_id=%s node_text=%s is_team=%v is_project=%v is_cycle=%v is_issue=%v", node.ID, node.Text, node.IsTeam, node.IsProject, node.IsCycle, node.IsIssue)

	if a.searchQuery != "" && !a.restoringSession {
		a.clearNavSearch()
		a.jumpToSection(IssuesSectionList, 0)
	}

	a.groupingOverridden = false
	a.sortOverridden = false

	if a.detailsZoomed && !node.IsIssue {
		a.releaseDetailsZoom()
		a.rebuildContentLayout()
		a.updateFocus()
	}

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

	if node.TeamID != "" {
		go a.preloadTeamMetadataFunc(node.TeamID)
	}

	a.refreshIssuesWithFocusChange(false, issueID...)
}

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

// Also writes the in-memory config, so a later settings save records the choice instead of reverting it.
func (a *App) setSortFields(fields []SortField) {
	if len(fields) == 0 {
		return
	}
	logger.Debug("tui.app: setting sort chain fields=%s", sortChainLabel(fields))
	a.sortFields = fields
	a.sortOverridden = true
	a.config.SortBy = sortConfigNames(fields)

	a.issuesMu.Lock()
	a.sortIssuesLocally()
	a.issuesMu.Unlock()
	a.regroupIssues("")

	a.refreshIssues()
}
