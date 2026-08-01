package tui

import (
	"strings"

	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

// A Linear custom view saves its own display settings (grouping, subgrouping,
// ordering). When a favorited view is opened, those settings override the
// configured defaults until the user picks another list or overrides them
// manually in-session.

// viewDisplayPrefs holds a custom view's display settings mapped onto the
// app's dimensions. UI-thread only.
type viewDisplayPrefs struct {
	groupBy     string
	subgroupBy  string
	hasGrouping bool
	sortField   SortField
	hasSort     bool
}

// mapViewGrouping maps Linear's issueGrouping strings onto the app's GroupBy
// dimensions. The API types these as plain strings, so unknown values (label,
// team, parent grouping the TUI does not render) report !ok and callers fall
// back to config.
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

// mapViewOrdering maps Linear's viewOrdering strings onto the app's sort
// fields. Manual and due-date orderings have no app equivalent and report
// !ok.
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

// resolveViewPrefs maps a view's raw preference values onto display
// overrides. Returns nil when nothing is applicable, so config stays in
// charge.
func resolveViewPrefs(values *linearapi.ViewPreferencesValues) *viewDisplayPrefs {
	if values == nil {
		return nil
	}
	prefs := &viewDisplayPrefs{}
	if groupBy, ok := mapViewGrouping(values.IssueGrouping); ok {
		prefs.hasGrouping = true
		prefs.groupBy = groupBy
		// A view's subgrouping only applies with its grouping; absent or
		// unmappable means none, not the configured fallback.
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

// effectiveGroupBy returns the grouping dimension in effect: the active
// view's, unless the user overrode grouping this session.
func (a *App) effectiveGroupBy() string {
	if !a.groupingOverridden && a.viewPrefs != nil && a.viewPrefs.hasGrouping {
		return a.viewPrefs.groupBy
	}
	return a.config.GroupBy
}

// effectiveSubgroupBy returns the subgrouping dimension in effect.
func (a *App) effectiveSubgroupBy() string {
	if !a.groupingOverridden && a.viewPrefs != nil && a.viewPrefs.hasGrouping {
		return a.viewPrefs.subgroupBy
	}
	return a.config.SubgroupBy
}

// effectiveSortField returns the sort in effect: the active view's, unless
// the user overrode sorting this session.
func (a *App) effectiveSortField() SortField {
	if !a.sortOverridden && a.viewPrefs != nil && a.viewPrefs.hasSort {
		return a.viewPrefs.sortField
	}
	return a.sortField
}
