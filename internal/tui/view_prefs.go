package tui

import (
	"strings"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

type viewDisplayPrefs struct {
	groupBy     string
	subgroupBy  string
	hasGrouping bool
	sortField   SortField
	hasSort     bool
}

func mapViewGrouping(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "workflowstate", "status", "state":
		return GroupByStatus, true
	case "priority":
		return GroupByPriority, true
	case "assignee":
		return GroupByAssignee, true
	case "cycle":
		return GroupByCycle, true
	case "project":
		return GroupByProject, true
	case "projectmilestone", "milestone":
		return GroupByMilestone, true
	case "none", "nogrouping":
		return GroupByNone, true
	}
	return "", false
}

func mapViewOrdering(value string) (SortField, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "priority":
		return SortByPriority, true
	case "updatedat", "lastupdated", "updated":
		return SortByUpdatedAt, true
	case "createdat", "lastcreated", "created":
		return SortByCreatedAt, true
	case "workflowstate", "status", "state":
		return SortByStatus, true
	}
	return "", false
}

func resolveViewPrefs(values *linearapi.ViewPreferencesValues) *viewDisplayPrefs {
	if values == nil {
		return nil
	}
	prefs := &viewDisplayPrefs{}
	if groupBy, ok := mapViewGrouping(values.IssueGrouping); ok {
		prefs.hasGrouping = true
		prefs.groupBy = groupBy
		prefs.subgroupBy = GroupByNone
		if subgroupBy, ok := mapViewGrouping(values.IssueSubGrouping); ok && subgroupBy != groupBy {
			prefs.subgroupBy = subgroupBy
		}
	}
	if sortField, ok := mapViewOrdering(values.ViewOrdering); ok {
		prefs.hasSort = true
		prefs.sortField = sortField
	}
	if !prefs.hasGrouping && !prefs.hasSort {
		return nil
	}
	return prefs
}

func (a *App) effectiveGroupBy() string {
	if !a.groupingOverridden && a.viewPrefs != nil && a.viewPrefs.hasGrouping {
		return a.viewPrefs.groupBy
	}
	return a.config.GroupBy
}

func (a *App) effectiveSubgroupBy() string {
	if !a.groupingOverridden && a.viewPrefs != nil && a.viewPrefs.hasGrouping {
		return a.viewPrefs.subgroupBy
	}
	return a.config.SubgroupBy
}

func (a *App) effectiveSortFields() []SortField {
	if !a.sortOverridden && a.viewPrefs != nil && a.viewPrefs.hasSort {
		return []SortField{a.viewPrefs.sortField}
	}
	return a.sortFields
}
