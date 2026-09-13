package tui

import (
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/praxis-labs-io/zen-linear/internal/session"
)

// UseSession sets where the session is saved on quit and the place to restore for the opening workspace.
func (a *App) UseSession(path string, file session.File) {
	a.sessionPath = path
	if !a.config.SessionRestore {
		return
	}
	if state, ok := file.StateFor(a.activeWorkspaceName); ok {
		a.pendingSession = &state
	}
}

func (a *App) consumePendingSession() *session.State {
	state := a.pendingSession
	a.pendingSession = nil
	return state
}

func (a *App) sessionSnapshot() session.State {
	return session.State{
		Nav:     navSelectionFor(a.selectedNavigation),
		IssueID: a.selectedIssueID(a.activeIssuesSection),
		Filters: sessionFiltersFor(a.richFilters),
		Search:  a.searchQuery,
	}
}

func (a *App) persistSession() {
	if a.sessionPath == "" || !a.config.SessionRestore {
		return
	}
	if a.selectedNavigation == nil {
		return
	}
	if err := session.Record(a.sessionPath, a.activeWorkspaceName, a.sessionSnapshot()); err != nil {
		logger.Warning("tui.session: failed to record session path=%s error=%v", a.sessionPath, err)
	}
}

func (a *App) markSessionWorkspace() {
	if a.sessionPath == "" || !a.config.SessionRestore {
		return
	}
	if err := session.MarkLast(a.sessionPath, a.activeWorkspaceName); err != nil {
		logger.Warning("tui.session: failed to mark workspace path=%s error=%v", a.sessionPath, err)
	}
}

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
		selection.Kind = session.NavAll
	}
	return selection
}

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
