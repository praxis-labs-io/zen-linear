package tui

import (
	"context"
	"sync"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/praxis-labs-io/zen-linear/internal/session"
	"github.com/rivo/tview"
)

type teamChildren struct {
	projects []linearapi.Project
	states   []linearapi.WorkflowState
	cycles   []linearapi.Cycle
	loaded   bool
}

// Taken off the App so restore goroutines never read fields applySettings reassigns on the UI thread.
type teamChildFetchers struct {
	projects func(context.Context, string) ([]linearapi.Project, error)
	states   func(context.Context, string) ([]linearapi.WorkflowState, error)
	cycles   func(context.Context, string) ([]linearapi.Cycle, error)
}

func (a *App) teamChildFetchers() teamChildFetchers {
	return teamChildFetchers{
		projects: a.fetchProjectsFunc,
		states:   a.fetchWorkflowStatesFunc,
		cycles:   a.fetchCyclesFunc,
	}
}

// Must run off the UI goroutine: the lazy child fetches would block the event loop.
func (a *App) applySessionNavigation(ctx context.Context, state *session.State, teams []linearapi.Team, favorites []linearapi.Favorite, fetchers teamChildFetchers) bool {
	if state == nil {
		return false
	}

	nav := state.Nav
	if !isKnownNavKind(nav.Kind) {
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

	var children teamChildren
	if navKindNeedsTeamChildren(nav.Kind) {
		children = fetchTeamChildren(ctx, fetchers, nav.TeamID)
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

func (a *App) restoreSessionAllIssues(state session.State) {
	a.beginSessionRestore(state)
	defer func() { a.restoringSession = false }()
	current := a.navigationTree.GetCurrentNode()
	if current == nil {
		a.refreshIssuesWithFocusChange(false, state.IssueID)
		return
	}
	a.selectSessionNode(current, state)
}

func (a *App) restoreSessionFavorite(state session.State) {
	a.beginSessionRestore(state)
	defer func() { a.restoringSession = false }()
	target := a.findFavoriteTreeNode(state.Nav.FavoriteID)
	if target == nil {
		a.refreshIssuesWithFocusChange(false)
		return
	}
	a.selectSessionNode(target, state)
}

func (a *App) restoreSessionTeamNode(state session.State, children teamChildren) {
	a.beginSessionRestore(state)
	defer func() { a.restoringSession = false }()

	teamNode := a.findTeamTreeNode(state.Nav.TeamID)
	if teamNode == nil {
		a.refreshIssuesWithFocusChange(false)
		return
	}
	if children.loaded && !teamChildrenLoaded(teamNode) {
		a.populateTeamNodeChildren(teamNode, state.Nav.TeamID, children.projects, children.states, children.cycles)
	}
	if teamChildrenLoaded(teamNode) {
		setNavFold(teamNode, true)
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
		revealNavNode(teamNode, descendant)
	}
	a.selectSessionNode(target, state)
}

func (a *App) beginSessionRestore(state session.State) {
	a.richFilters = filtersFromSession(state.Filters)
	a.restoringSession = true
	a.restoreSessionSearch(state)
}

func (a *App) restoreSessionSearch(state session.State) {
	if state.Search == "" || a.navSearchInput == nil {
		return
	}
	a.pendingSearchIssueID = state.IssueID
	a.navSearchInput.SetText(state.Search)
}

func (a *App) selectSessionNode(target *tview.TreeNode, state session.State) {
	nav, ok := target.GetReference().(*NavigationNode)
	if !ok {
		a.refreshIssuesWithFocusChange(false)
		return
	}
	a.navigationTree.SetCurrentNode(target)
	a.onNavigationSelected(nav, state.IssueID)
}

func fetchTeamChildren(ctx context.Context, fetchers teamChildFetchers, teamID string) teamChildren {
	var (
		projects    []linearapi.Project
		states      []linearapi.WorkflowState
		cycles      []linearapi.Cycle
		projectsErr error
		statesErr   error
		cyclesErr   error
		wg          sync.WaitGroup
	)

	wg.Add(3)
	go func() {
		defer wg.Done()
		projects, projectsErr = fetchers.projects(ctx, teamID)
	}()
	go func() {
		defer wg.Done()
		states, statesErr = fetchers.states(ctx, teamID)
	}()
	go func() {
		defer wg.Done()
		cycles, cyclesErr = fetchers.cycles(ctx, teamID)
	}()
	wg.Wait()

	if projectsErr != nil || statesErr != nil || cyclesErr != nil {
		logger.Warning("tui.session: failed to load team children team_id=%s projects_err=%v states_err=%v cycles_err=%v", teamID, projectsErr, statesErr, cyclesErr)
		return teamChildren{}
	}
	return teamChildren{projects: projects, states: states, cycles: cycles, loaded: true}
}

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

func isKnownNavKind(kind session.NavKind) bool {
	switch kind {
	case session.NavAll, session.NavTeam, session.NavProject, session.NavStatus,
		session.NavCycle, session.NavCustomView, session.NavStateType:
		return true
	default:
		return false
	}
}

func navKindNeedsTeamChildren(kind session.NavKind) bool {
	switch kind {
	case session.NavProject, session.NavStatus, session.NavCycle:
		return true
	default:
		return false
	}
}

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

func (a *App) findFavoriteTreeNode(favoriteID string) *tview.TreeNode {
	if a.favoritesGroup == nil {
		return nil
	}
	return findFavoriteNode(a.favoritesGroup, favoriteID)
}

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
