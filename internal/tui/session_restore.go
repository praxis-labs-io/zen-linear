package tui

import (
	"context"

	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
	"github.com/zen-linear/zen-linear/internal/session"
)

// teamChildren holds a team's lazily loaded navigation children, fetched so a
// restored project, status, or cycle node exists to select.
type teamChildren struct {
	projects []linearapi.Project
	states   []linearapi.WorkflowState
	cycles   []linearapi.Cycle
	loaded   bool
}

// applySessionNavigation reopens the saved place: filters, tab, navigation
// selection, focused issue, and search query. It must run off the UI
// goroutine; UI mutations are queued and the lazy child fetches would block
// the event loop. Reports whether it took ownership of the startup refresh.
//
// Anything it cannot resolve returns false without a status flash, unlike
// applyDefaultNavigation: a session record is machine state, and warning on
// every launch after a project is deleted is noise the user cannot act on.
func (a *App) applySessionNavigation(ctx context.Context, teams []linearapi.Team, favorites []linearapi.Favorite) bool {
	state := a.consumePendingSession()
	if state == nil {
		return false
	}

	nav := state.Nav
	if !isKnownNavKind(nav.Kind) {
		// A newer build wrote a kind this one cannot scope a list to, and a
		// near miss is worse than the configured default.
		logger.Debug("tui.session: unsupported saved navigation kind=%s", nav.Kind)
		return false
	}

	if nav.FavoriteID != "" {
		if !hasFavoriteNode(favoriteNavigationNodes(favorites), nav.FavoriteID) {
			logger.Debug("tui.session: saved favorite is gone favorite_id=%s", nav.FavoriteID)
			return false
		}
		a.queueUpdateDraw(func() {
			a.restoreSessionFavorite(*state)
		})
		return true
	}

	// Custom views and predefined views only exist in the tree as favorites,
	// so without one there is no node left to select.
	if nav.Kind == session.NavCustomView || nav.Kind == session.NavStateType {
		return false
	}

	if nav.Kind == session.NavAll && nav.TeamID == "" {
		a.queueUpdateDraw(func() {
			a.restoreSessionAllIssues(*state)
		})
		return true
	}

	if findTeamByID(teams, nav.TeamID) == nil {
		logger.Debug("tui.session: saved team is gone team_id=%s", nav.TeamID)
		return false
	}

	children := teamChildren{loaded: true}
	if navKindNeedsTeamChildren(nav.Kind) {
		children = a.fetchTeamChildren(ctx, nav.TeamID)
		if !children.loaded || !children.contain(nav) {
			logger.Debug("tui.session: saved navigation is gone kind=%s team_id=%s", nav.Kind, nav.TeamID)
			return false
		}
	}

	a.queueUpdateDraw(func() {
		a.restoreSessionTeamNode(*state, children)
	})
	return true
}

// restoreSessionAllIssues reopens the workspace-wide All Issues list, which
// the tree already has selected, so only the surrounding state is restored.
func (a *App) restoreSessionAllIssues(state session.State) {
	a.beginSessionRestore(state)
	current := a.navigationTree.GetCurrentNode()
	if current == nil {
		a.refreshIssuesWithFocusChange(false, state.IssueID)
		return
	}
	a.selectSessionNode(current, state)
}

// restoreSessionFavorite reopens a list saved from the Favorites section.
func (a *App) restoreSessionFavorite(state session.State) {
	a.beginSessionRestore(state)
	target := a.findFavoriteTreeNode(state.Nav.FavoriteID)
	if target == nil {
		a.refreshIssuesWithFocusChange(false)
		return
	}
	a.selectSessionNode(target, state)
}

// restoreSessionTeamNode reopens a team node or one of its descendants,
// populating the team's children first when they have not been built yet.
func (a *App) restoreSessionTeamNode(state session.State, children teamChildren) {
	a.beginSessionRestore(state)

	teamNode := a.findTeamTreeNode(state.Nav.TeamID)
	if teamNode == nil {
		a.refreshIssuesWithFocusChange(false)
		return
	}
	// Leave children unpopulated on fetch errors so expanding the team retries.
	if children.loaded && len(teamNode.GetChildren()) == 0 {
		a.populateTeamNodeChildren(teamNode, state.Nav.TeamID, children.projects, children.states, children.cycles)
	}
	if len(teamNode.GetChildren()) > 0 {
		teamNode.SetExpanded(true)
	}

	target := teamNode
	if navKindNeedsTeamChildren(state.Nav.Kind) {
		descendant := findTeamDescendant(teamNode, func(nav *NavigationNode) bool {
			return navMatchesSelection(nav, state.Nav)
		})
		if descendant == nil {
			a.refreshIssuesWithFocusChange(false)
			return
		}
		target = descendant
	}
	a.selectSessionNode(target, state)
}

// beginSessionRestore reinstates the state a refresh reads before it starts.
// The filters have to land first: currentFetchParams reads them the moment
// the refresh begins, so setting them afterwards is a page too late.
func (a *App) beginSessionRestore(state session.State) {
	a.richFilters = filtersFromSession(state.Filters)
	a.activeIssuesSection = sectionFromSession(state.Section)
	if a.activeIssuesSection == IssuesSectionSearch {
		// Focus stays on the navigation pane; this only decides where a Tab
		// into the issues pane lands.
		a.searchInputFocused = true
	}
	a.updateIssuesColumnLayout()
	if state.Search != "" && a.searchInput != nil {
		// The input's change handler schedules the debounced search, so the
		// results load without a fetch call here.
		a.searchInput.SetText(state.Search)
	}
}

// selectSessionNode moves the tree cursor to the restored node and opens its
// list on the saved issue.
func (a *App) selectSessionNode(target *tview.TreeNode, state session.State) {
	nav, ok := target.GetReference().(*NavigationNode)
	if !ok {
		a.refreshIssuesWithFocusChange(false)
		return
	}
	a.navigationTree.SetCurrentNode(target)
	a.onNavigationSelected(nav, state.IssueID)
}

// fetchTeamChildren loads the projects, states, and cycles a team node needs
// before one of its descendants can be selected.
func (a *App) fetchTeamChildren(ctx context.Context, teamID string) teamChildren {
	projects, projectsErr := a.fetchProjectsFunc(ctx, teamID)
	states, statesErr := a.fetchWorkflowStatesFunc(ctx, teamID)
	cycles, cyclesErr := a.fetchCyclesFunc(ctx, teamID)
	if projectsErr != nil || statesErr != nil || cyclesErr != nil {
		logger.Warning("tui.session: failed to load team children team_id=%s projects_err=%v states_err=%v cycles_err=%v", teamID, projectsErr, statesErr, cyclesErr)
		return teamChildren{}
	}
	return teamChildren{projects: projects, states: states, cycles: cycles, loaded: true}
}

// contain reports whether the fetched children still hold the saved node.
func (c teamChildren) contain(nav session.NavSelection) bool {
	switch nav.Kind {
	case session.NavProject:
		for _, project := range c.projects {
			if project.ID == nav.ProjectID {
				return true
			}
		}
	case session.NavStatus:
		for _, state := range c.states {
			if state.ID == nav.StateID {
				return true
			}
		}
	case session.NavCycle:
		for _, cycle := range c.cycles {
			if cycle.ID == nav.CycleID {
				return true
			}
		}
	}
	return false
}

// isKnownNavKind reports whether this build can reopen a saved kind.
func isKnownNavKind(kind session.NavKind) bool {
	switch kind {
	case session.NavAll, session.NavTeam, session.NavProject, session.NavStatus,
		session.NavCycle, session.NavCustomView, session.NavStateType:
		return true
	default:
		return false
	}
}

// navKindNeedsTeamChildren reports whether a saved kind lives under a team's
// lazily built children rather than on the team node itself.
func navKindNeedsTeamChildren(kind session.NavKind) bool {
	switch kind {
	case session.NavProject, session.NavStatus, session.NavCycle:
		return true
	default:
		return false
	}
}

// navMatchesSelection reports whether a tree node is the one a saved
// selection points at.
func navMatchesSelection(nav *NavigationNode, selection session.NavSelection) bool {
	switch selection.Kind {
	case session.NavProject:
		return nav.IsProject && nav.ID == selection.ProjectID
	case session.NavStatus:
		return nav.IsStatus && nav.StateID == selection.StateID
	case session.NavCycle:
		return nav.IsCycle && nav.CycleID == selection.CycleID
	default:
		return false
	}
}

// findTeamDescendant returns the first node under a team matching the
// predicate. Status and cycle nodes are grandchildren: the groups holding
// them are not selectable and carry no id of their own, so a search of the
// team's direct children alone would never reach them.
func findTeamDescendant(teamNode *tview.TreeNode, match func(*NavigationNode) bool) *tview.TreeNode {
	for _, child := range teamNode.GetChildren() {
		if nav, ok := child.GetReference().(*NavigationNode); ok && match(nav) {
			return child
		}
		for _, grandchild := range child.GetChildren() {
			if nav, ok := grandchild.GetReference().(*NavigationNode); ok && match(nav) {
				return grandchild
			}
		}
	}
	return nil
}

// findFavoriteTreeNode returns the tree node built from a favorite, recursing
// into favorite folders.
func (a *App) findFavoriteTreeNode(favoriteID string) *tview.TreeNode {
	if a.favoritesGroup == nil {
		return nil
	}
	return findFavoriteNode(a.favoritesGroup, favoriteID)
}

// findFavoriteNode walks a favorites subtree for a node carrying the id.
func findFavoriteNode(parent *tview.TreeNode, favoriteID string) *tview.TreeNode {
	for _, child := range parent.GetChildren() {
		if nav, ok := child.GetReference().(*NavigationNode); ok && nav.FavoriteID == favoriteID && !nav.IsFolder {
			return child
		}
		if found := findFavoriteNode(child, favoriteID); found != nil {
			return found
		}
	}
	return nil
}

// hasFavoriteNode reports whether the favorites still hold a displayable node
// with the id, folders excluded: a folder opens no list.
func hasFavoriteNode(nodes []*NavigationNode, favoriteID string) bool {
	for _, node := range nodes {
		if node.FavoriteID == favoriteID && !node.IsFolder {
			return true
		}
		if hasFavoriteNode(node.Children, favoriteID) {
			return true
		}
	}
	return false
}

// findTeamByID returns the team with the id, or nil when it is gone.
func findTeamByID(teams []linearapi.Team, teamID string) *linearapi.Team {
	if teamID == "" {
		return nil
	}
	for i := range teams {
		if teams[i].ID == teamID {
			return &teams[i]
		}
	}
	return nil
}
