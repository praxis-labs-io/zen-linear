package tui

import (
	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
	"github.com/zen-linear/zen-linear/internal/session"
)

// UseSession installs the saved session: the path the quit flush writes to,
// and the restore point for the workspace this App opened with. The path is
// kept even when restore is off, so turning the toggle on mid-session still
// records a place to come back to.
func (a *App) UseSession(path string, file session.File) {
	a.sessionPath = path
	if !a.config.SessionRestore {
		return
	}
	if state, ok := file.StateFor(a.activeWorkspaceName); ok {
		a.pendingSession = &state
	}
}

// consumePendingSession returns the restore point and clears it. A settings
// save and a workspace switch both re-run loadInitialData, and re-applying a
// stale session there would lay it over where the user actually is.
func (a *App) consumePendingSession() *session.State {
	state := a.pendingSession
	a.pendingSession = nil
	return state
}

// sessionSnapshot reads the current place. Call it on the UI thread, or after
// the event loop has stopped.
func (a *App) sessionSnapshot() session.State {
	return session.State{
		Nav:     navSelectionFor(a.selectedNavigation),
		IssueID: a.selectedIssueID(a.activeIssuesSection),
		Filters: sessionFiltersFor(a.richFilters),
		Search:  a.searchQuery,
	}
}

// persistSession records the current place for the active workspace. A write
// failure is logged and swallowed: a lost restore point must not change how
// the process exits.
func (a *App) persistSession() {
	if a.sessionPath == "" || !a.config.SessionRestore {
		return
	}
	// A startup that failed before the navigation tree built has no place to
	// record, and writing an empty one would erase the last good place.
	if a.selectedNavigation == nil {
		return
	}
	if err := session.Record(a.sessionPath, a.activeWorkspaceName, a.sessionSnapshot()); err != nil {
		logger.Warning("tui.session: failed to record session path=%s error=%v", a.sessionPath, err)
	}
}

// markSessionWorkspace records the workspace now open, for the window between
// a switch and the next quit.
func (a *App) markSessionWorkspace() {
	if a.sessionPath == "" || !a.config.SessionRestore {
		return
	}
	if err := session.MarkLast(a.sessionPath, a.activeWorkspaceName); err != nil {
		logger.Warning("tui.session: failed to mark workspace path=%s error=%v", a.sessionPath, err)
	}
}

// navSelectionFor maps the live navigation node onto a saved locator. The
// branch order mirrors currentFetchParams, so a restored selection scopes the
// issue list exactly as the live one did.
func navSelectionFor(node *NavigationNode) session.NavSelection {
	if node == nil {
		return session.NavSelection{Kind: session.NavAll}
	}

	selection := session.NavSelection{FavoriteID: node.FavoriteID, TeamID: node.TeamID}
	switch {
	case node.CustomViewID != "":
		selection.Kind = session.NavCustomView
		selection.CustomViewID = node.CustomViewID
	case node.StateType != "":
		selection.Kind = session.NavStateType
		selection.StateType = node.StateType
	case node.IsStatus:
		selection.Kind = session.NavStatus
		selection.StateID = node.StateID
	case node.IsCycle:
		selection.Kind = session.NavCycle
		selection.CycleID = node.CycleID
	case node.IsTeam:
		selection.Kind = session.NavTeam
	case node.IsProject:
		selection.Kind = session.NavProject
		selection.ProjectID = node.ID
	default:
		// A team-scoped All Issues favorite lands here carrying a team and none
		// of the flags above, same as the last case in currentFetchParams.
		selection.Kind = session.NavAll
	}
	return selection
}

// sessionFiltersFor converts the live filters for storage. Only Eq is carried
// on the date and estimate filters because only Eq is ever set.
func sessionFiltersFor(filters IssueFilters) session.Filters {
	return session.Filters{
		AssigneeID:   filters.AssigneeID,
		AssigneeName: filters.AssigneeName,
		LabelIDs:     filters.LabelIDs,
		LabelNames:   filters.LabelNames,
		StateID:      filters.StateID,
		StateName:    filters.StateName,
		ProjectID:    filters.ProjectID,
		ProjectName:  filters.ProjectName,
		CycleID:      filters.CycleID,
		CycleName:    filters.CycleName,
		DueDate:      filters.DueDate.Eq,
		Estimate:     filters.Estimate.Eq,
	}
}

// filtersFromSession converts stored filters back into the live form.
func filtersFromSession(filters session.Filters) IssueFilters {
	live := IssueFilters{
		AssigneeID:   filters.AssigneeID,
		AssigneeName: filters.AssigneeName,
		LabelIDs:     filters.LabelIDs,
		LabelNames:   filters.LabelNames,
		StateID:      filters.StateID,
		StateName:    filters.StateName,
		ProjectID:    filters.ProjectID,
		ProjectName:  filters.ProjectName,
		CycleID:      filters.CycleID,
		CycleName:    filters.CycleName,
	}
	if filters.DueDate != "" {
		live.DueDate = linearapi.DateFilter{Eq: filters.DueDate}
	}
	if filters.Estimate != nil {
		estimate := *filters.Estimate
		live.Estimate = linearapi.NumberFilter{Eq: &estimate}
	}
	return live
}
