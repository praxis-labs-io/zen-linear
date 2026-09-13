package tui

import (
	"cmp"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/rivo/tview"
)

// Taken off the App so fetch goroutines never read fields applySettings reassigns.
type navFetchers struct {
	teams     func(context.Context) ([]linearapi.Team, error)
	favorites func(context.Context) ([]linearapi.Favorite, error)
}

func (a *App) navFetchers() navFetchers {
	return navFetchers{
		teams:     a.fetchTeamsFunc,
		favorites: a.fetchFavoritesFunc,
	}
}

type fetchedNav struct {
	teams       []linearapi.Team
	favorites   []linearapi.Favorite
	favoritesOK bool
	err         error
}

func fetchNavigationData(ctx context.Context, fetchers navFetchers) fetchedNav {
	var result fetchedNav
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		result.teams, result.err = fetchers.teams(ctx)
	}()
	go func() {
		defer wg.Done()
		fetched, err := fetchers.favorites(ctx)
		if err != nil {
			logger.ErrorWithErr(err, "tui.app: failed to load favorites")
			return
		}
		result.favorites = fetched
		result.favoritesOK = true
	}()
	wg.Wait()

	if result.err != nil {
		return fetchedNav{err: result.err}
	}

	logger.Debug("tui.app: loaded teams count=%d favorites_count=%d", len(result.teams), len(result.favorites))
	return result
}

func (a *App) navigationPaneLabel() string {
	if a.activeWorkspaceName == "" {
		return "Navigation"
	}
	return a.activeWorkspaceName
}

func (a *App) buildWaitingNavigationRoot() *tview.TreeNode {
	root := tview.NewTreeNode("").
		SetSelectable(false)

	loadingNode := tview.NewTreeNode(a.navLoadingText()).
		SetColor(a.theme.SecondaryText).
		SetSelectable(false)
	a.navLoadingNode = loadingNode
	root.AddChild(loadingNode)
	a.applyNavSelectionStyle(root)

	return root
}

func (a *App) resetNavigationTree() {
	if a.navigationTree == nil {
		return
	}
	a.navNodeLabels = make(map[*tview.TreeNode]navNodeLabel)
	a.favorites = nil
	a.allIssuesNode = nil
	a.favoritesGroup = nil
	a.teamsGroup = nil
	a.navTeams = nil
	root := a.buildWaitingNavigationRoot()
	a.navigationTree.SetRoot(root)
	a.navigationTree.SetCurrentNode(root)
}

func (a *App) rebuildNavigationTree(teams []linearapi.Team, favorites []linearapi.Favorite) {
	a.navNodeLabels = make(map[*tview.TreeNode]navNodeLabel)
	a.navLoadingNode = nil
	a.favorites = favorites
	a.navTeams = teams
	root := tview.NewTreeNode("").
		SetSelectable(false)

	allIssues := tview.NewTreeNode("All Issues").
		SetReference(&NavigationNode{ID: "all", Text: "All Issues"}).
		SetExpanded(true)
	a.allIssuesNode = allIssues
	a.favoritesGroup = a.buildFavoritesGroup(favorites)
	a.teamsGroup = a.buildTeamsGroup(teams)
	root.SetChildren(a.navRootChildren())

	a.applyNavigationNodeColors(root)
	a.applyNavSelectionStyle(root)
	a.navigationTree.SetRoot(root)
	a.navigationTree.SetCurrentNode(allIssues)
	a.selectedNavigation = &NavigationNode{ID: "all", Text: "All Issues"}
}

func (a *App) buildTeamsGroup(teams []linearapi.Team) *tview.TreeNode {
	if len(teams) == 0 {
		return nil
	}

	group := tview.NewTreeNode("Teams").
		SetColor(a.theme.Accent).
		SetSelectable(false).
		SetExpanded(true)

	for _, team := range teams {
		group.AddChild(tview.NewTreeNode(team.Name).
			SetReference(&NavigationNode{
				ID:     team.ID,
				Text:   team.Name,
				IsTeam: true,
				TeamID: team.ID,
			}).
			SetExpanded(false))
	}
	return group
}

func (a *App) navRootChildren() []*tview.TreeNode {
	children := make([]*tview.TreeNode, 0, 5)
	for _, section := range []*tview.TreeNode{a.allIssuesNode, a.favoritesGroup, a.teamsGroup} {
		if section == nil {
			continue
		}
		if len(children) > 0 {
			children = append(children, navSectionSpacer())
		}
		children = append(children, section)
	}
	return children
}

func navSectionSpacer() *tview.TreeNode {
	return tview.NewTreeNode("").SetSelectable(false)
}

func (a *App) onTeamExpanded(teamID string, teamNode *tview.TreeNode) {
	if teamChildrenLoaded(teamNode) || teamNode.IsExpanded() {
		setNavFold(teamNode, !teamNode.IsExpanded())
		return
	}

	go func() {
		logger.Debug("tui.app: loading navigation children team_id=%s", teamID)
		ctx := context.Background()

		if err := a.cache.PreloadTeamMetadata(ctx, teamID); err != nil {
			logger.ErrorWithErr(err, "tui.app: team metadata preload incomplete team_id=%s", teamID)
		}
		projects, projectsErr := a.cache.GetProjects(ctx, teamID)
		states, statesErr := a.cache.GetWorkflowStates(ctx, teamID)
		cycles, cyclesErr := a.cache.GetCycles(ctx, teamID)
		if err := cmp.Or(projectsErr, statesErr, cyclesErr); err != nil {
			logger.ErrorWithErr(err, "tui.app: failed to load navigation children team_id=%s", teamID)
			a.app.QueueUpdateDraw(func() {
				a.updateStatusBarWithError(err)
			})
			return
		}
		logger.Debug("tui.app: loaded navigation children team_id=%s projects=%d states=%d cycles=%d", teamID, len(projects), len(states), len(cycles))

		a.app.QueueUpdateDraw(func() {
			if teamChildrenLoaded(teamNode) {
				setNavFold(teamNode, true)
				return
			}
			a.populateTeamNodeChildren(teamNode, teamID, projects, states, cycles)
			setNavFold(teamNode, true)
		})
	}()
}

func (a *App) populateTeamNodeChildren(teamNode *tview.TreeNode, teamID string, projects []linearapi.Project, states []linearapi.WorkflowState, cycles []linearapi.Cycle) {
	logger.Debug("tui.app: building team rows team_id=%s projects=%d states=%d cycles=%d", teamID, len(projects), len(states), len(cycles))
	for _, child := range teamNode.GetChildren() {
		a.forgetNavNodeLabels(child)
	}
	teamNode.SetChildren(nil)
	if nav, ok := teamNode.GetReference().(*NavigationNode); ok {
		nav.ChildrenLoaded = len(states) > 0
	}

	teamNode.AddChild(tview.NewTreeNode("All Issues").
		SetReference(&NavigationNode{
			ID:     fmt.Sprintf("%s-all", teamID),
			Text:   "All Issues",
			TeamID: teamID,
		}))

	if len(cycles) > 0 {
		sortCyclesForNavigation(cycles)
		cyclesGroup := a.newTeamGroupNode(teamID, "Cycles")
		for _, cycle := range cycles {
			label := cycle.DisplayName()
			switch {
			case cycle.IsActive:
				label += " (active)"
			case cycle.IsNext:
				label += " (next)"
			case cycle.IsPrevious:
				label += " (previous)"
			}
			cycleNode := tview.NewTreeNode(label).
				SetReference(&NavigationNode{
					ID:        cycle.ID,
					Text:      label,
					TeamID:    teamID,
					IsCycle:   true,
					CycleID:   cycle.ID,
					CycleName: cycle.DisplayName(),
				})
			cyclesGroup.AddChild(cycleNode)
		}
		teamNode.AddChild(cyclesGroup)
	}
	if len(states) > 0 {
		sort.Slice(states, func(i, j int) bool {
			return states[i].Position < states[j].Position
		})
		statusGroup := a.newTeamGroupNode(teamID, "Status")
		for _, state := range states {
			stateNode := tview.NewTreeNode(state.Name).
				SetReference(&NavigationNode{
					ID:        state.ID,
					Text:      state.Name,
					TeamID:    teamID,
					IsStatus:  true,
					StateID:   state.ID,
					StateName: state.Name,
				})
			statusGroup.AddChild(stateNode)
		}
		teamNode.AddChild(statusGroup)
	}
	if len(projects) > 0 {
		projectsGroup := a.newTeamGroupNode(teamID, "Projects")
		for _, proj := range projects {
			projectsGroup.AddChild(tview.NewTreeNode(proj.Name).
				SetReference(&NavigationNode{
					ID:        proj.ID,
					Text:      proj.Name,
					IsProject: true,
					TeamID:    teamID,
				}))
		}
		teamNode.AddChild(projectsGroup)
	}
	a.applyNavigationNodeColors(teamNode)
	a.applyNavSelectionStyle(teamNode)
}

func teamChildrenLoaded(teamNode *tview.TreeNode) bool {
	nav, ok := teamNode.GetReference().(*NavigationNode)
	return ok && nav.ChildrenLoaded
}

func (a *App) newTeamGroupNode(teamID, name string) *tview.TreeNode {
	return tview.NewTreeNode(name).
		SetExpanded(false).
		SetReference(&NavigationNode{
			ID:      fmt.Sprintf("%s-%s", teamID, strings.ToLower(name)),
			Text:    name,
			TeamID:  teamID,
			IsGroup: true,
		})
}

func sortCyclesForNavigation(cycles []linearapi.Cycle) {
	sort.SliceStable(cycles, func(i, j int) bool {
		leftRank := cycleNavigationRank(cycles[i])
		rightRank := cycleNavigationRank(cycles[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if cycles[i].IsFuture || cycles[i].IsNext {
			return cycles[i].StartsAt.Before(cycles[j].StartsAt)
		}
		return cycles[i].StartsAt.After(cycles[j].StartsAt)
	})
}

func cycleNavigationRank(cycle linearapi.Cycle) int {
	switch {
	case cycle.IsActive:
		return 0
	case cycle.IsNext:
		return 1
	case cycle.IsFuture:
		return 2
	case cycle.IsPrevious:
		return 3
	case cycle.IsPast:
		return 4
	default:
		return 5
	}
}
