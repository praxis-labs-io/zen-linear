package tui

import (
	"cmp"
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/rivo/tview"
)

// navFetchers is the pair the navigation tree is built from, taken off the App
// so the background goroutines never read fields applySettings reassigns on the
// UI thread.
type navFetchers struct {
	teams     func(context.Context) ([]linearapi.Team, error)
	favorites func(context.Context) ([]linearapi.Favorite, error)
}

// navFetchers snapshots the fetch seams. UI thread only.
func (a *App) navFetchers() navFetchers {
	return navFetchers{
		teams:     a.fetchTeamsFunc,
		favorites: a.fetchFavoritesFunc,
	}
}

// fetchedNav is one navigation fetch's result. favoritesOK rides along because
// a favorites failure is not fatal to rendering but does mean the tree is
// incomplete, which is the difference between a copy worth caching and one that
// would drop the user's Favorites section on the next launch.
type fetchedNav struct {
	teams       []linearapi.Team
	favorites   []linearapi.Favorite
	favoritesOK bool
	err         error
}

// fetchNavigationData fetches the teams and favorites the navigation tree is
// built from. A favorites failure is not fatal: the tree renders without them.
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

// navigationPaneLabel names the navigation pane for the workspace on screen.
// The pane border carries the workspace, so the tree needs no root row of its
// own; the tree hides the root (`SetTopLevel(1)`) and it stays unlabelled.
func (a *App) navigationPaneLabel() string {
	if a.activeWorkspaceName == "" {
		return "Navigation"
	}
	return a.activeWorkspaceName
}

// buildWaitingNavigationRoot returns a root holding nothing but the waiting
// node, for a tree that has no data to show yet.
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

// resetNavigationTree puts the sidebar back to waiting. A workspace switch
// otherwise keeps painting the teams and favorites of the workspace it left,
// and selecting one of those scopes a fetch to an id the new key cannot
// resolve.
func (a *App) resetNavigationTree() {
	if a.navigationTree == nil {
		return
	}
	a.navNodeLabels = make(map[*tview.TreeNode]navNodeLabel)
	a.favorites = nil
	a.favoritesGroup = nil
	a.navTeams = nil
	root := a.buildWaitingNavigationRoot()
	a.navigationTree.SetRoot(root)
	a.navigationTree.SetCurrentNode(root)
}

// rebuildNavigationTree rebuilds the navigation tree with real data.
func (a *App) rebuildNavigationTree(teams []linearapi.Team, favorites []linearapi.Favorite) {
	a.navNodeLabels = make(map[*tview.TreeNode]navNodeLabel)
	a.navLoadingNode = nil
	a.favorites = favorites
	// Held for the disk cache, which a favorites change rewrites without a
	// teams fetch of its own.
	a.navTeams = teams
	root := tview.NewTreeNode("").
		SetSelectable(false)

	// Add "All Issues" at the top
	allIssues := tview.NewTreeNode("All Issues").
		SetColor(a.theme.Foreground).
		SetReference(&NavigationNode{ID: "all", Text: "All Issues"}).
		SetExpanded(true)
	root.AddChild(allIssues)

	a.appendFavoritesSection(root, favorites)

	// Add teams
	for _, team := range teams {
		teamNode := tview.NewTreeNode(team.Name).
			SetColor(a.theme.Foreground).
			SetReference(&NavigationNode{
				ID:     team.ID,
				Text:   team.Name,
				IsTeam: true,
				TeamID: team.ID,
			}).
			SetExpanded(false)

		// Note: Team selection is handled by the tree's SetSelectedFunc in buildNavigationTree()
		// Do NOT set SetSelectedFunc here as it causes duplicate callbacks

		root.AddChild(teamNode)
	}

	a.applyNavSelectionStyle(root)
	a.navigationTree.SetRoot(root)
	a.navigationTree.SetCurrentNode(allIssues)
	a.selectedNavigation = &NavigationNode{ID: "all", Text: "All Issues"}
}

// onTeamExpanded loads projects for a team when it's expanded.
func (a *App) onTeamExpanded(teamID string, teamNode *tview.TreeNode) {
	// If already has children (projects loaded), just toggle expand
	if len(teamNode.GetChildren()) > 0 {
		teamNode.SetExpanded(!teamNode.IsExpanded())
		return
	}

	// Load projects, workflow states, and cycles asynchronously.
	go func() {
		logger.Debug("tui.app: loading navigation children team_id=%s", teamID)
		ctx := context.Background()

		// Warm all five team caches rather than the three this needs.
		// Selecting a team preloads the same set, and duplicating a subset here
		// meant every first click issued each request twice. A preload error is
		// not fatal: it reports the first of five failures, and users and labels
		// are for the pickers, not the tree.
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
			// Double-check children haven't been added by another goroutine
			if len(teamNode.GetChildren()) > 0 {
				teamNode.SetExpanded(true)
				return
			}
			a.populateTeamNodeChildren(teamNode, teamID, projects, states, cycles)
			teamNode.SetExpanded(true)
		})
	}()
}

// populateTeamNodeChildren renders cycle, status, and project child nodes under a team node.
func (a *App) populateTeamNodeChildren(teamNode *tview.TreeNode, teamID string, projects []linearapi.Project, states []linearapi.WorkflowState, cycles []linearapi.Cycle) {
	if len(cycles) > 0 {
		sortCyclesForNavigation(cycles)
		cyclesGroup := tview.NewTreeNode("Cycles").
			SetColor(a.theme.SecondaryText).
			SetSelectable(false).
			SetReference(&NavigationNode{
				ID:      fmt.Sprintf("%s-cycles", teamID),
				Text:    "Cycles",
				TeamID:  teamID,
				IsCycle: true,
			})
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
				SetColor(a.theme.SecondaryText).
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
		statusGroup := tview.NewTreeNode("Status").
			SetColor(a.theme.SecondaryText).
			SetSelectable(false).
			SetReference(&NavigationNode{
				ID:       fmt.Sprintf("%s-status", teamID),
				Text:     "Status",
				TeamID:   teamID,
				IsStatus: true,
			})
		for _, state := range states {
			stateNode := tview.NewTreeNode(state.Name).
				SetColor(a.theme.SecondaryText).
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
	for _, proj := range projects {
		projNode := tview.NewTreeNode(proj.Name).
			SetColor(a.theme.SecondaryText).
			SetReference(&NavigationNode{
				ID:        proj.ID,
				Text:      proj.Name,
				IsProject: true,
				TeamID:    teamID,
			})
		teamNode.AddChild(projNode)
	}
	a.applyNavSelectionStyle(teamNode)
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
