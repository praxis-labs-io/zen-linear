package tui

import (
	"context"
	"time"

	"github.com/zen-linear/zen-linear/internal/cache"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// linearDeps bundles the Linear API client, its metadata cache, and every
// function the UI calls through them. NewApp and applySettings both build it
// with newLinearDeps, so the two wiring lists cannot drift apart — that drift
// is what once let a settings save rebuild the client without the OAuth bearer
// scheme and 401 refresh, silently downgrading an OAuth session.
type linearDeps struct {
	api   *linearapi.Client
	cache *cache.TeamCache

	fetchIssuesPage         func(context.Context, linearapi.FetchIssuesParams, *string) (linearapi.IssuePage, error)
	fetchIssueByID          func(context.Context, string) (linearapi.Issue, error)
	fetchViewPrefsFunc      func(context.Context, string) (*linearapi.ViewPreferencesValues, error)
	updateIssueFunc         func(context.Context, linearapi.UpdateIssueInput) (linearapi.Issue, error)
	createIssueFunc         func(context.Context, linearapi.CreateIssueInput) (linearapi.Issue, error)
	createIssueRelationFunc func(context.Context, linearapi.CreateIssueRelationInput) (linearapi.IssueRelation, error)
	deleteIssueRelationFunc func(context.Context, string) error
	subscribeIssueFunc      func(context.Context, string) (linearapi.Issue, error)
	unsubscribeIssueFunc    func(context.Context, string) (linearapi.Issue, error)
	fetchProjectsFunc       func(context.Context, string) ([]linearapi.Project, error)
	fetchWorkflowStatesFunc func(context.Context, string) ([]linearapi.WorkflowState, error)
	fetchIssueLabelsFunc    func(context.Context, string) ([]linearapi.IssueLabel, error)
	fetchMilestonesFunc     func(context.Context, string) ([]linearapi.ProjectMilestone, error)
	fetchCyclesFunc         func(context.Context, string) ([]linearapi.Cycle, error)
	fetchUsersFunc          func(context.Context, string) ([]linearapi.User, error)
	fetchCurrentUserFunc    func(context.Context) (linearapi.User, error)
	fetchTeamsFunc          func(context.Context) ([]linearapi.Team, error)
	fetchFavoritesFunc      func(context.Context) ([]linearapi.Favorite, error)
	createFavoriteFunc      func(context.Context, linearapi.FavoriteTarget) (linearapi.Favorite, error)
	deleteFavoriteFunc      func(context.Context, string) error
	updateFavoriteSortFunc  func(context.Context, string, float64) error
	moveFavoriteFunc        func(context.Context, string, string, float64) error
}

// newLinearDeps builds the API client for cfg and wires every dependency
// derived from it and its cache.
func newLinearDeps(cfg linearapi.ClientConfig, cacheTTL time.Duration) linearDeps {
	api := linearapi.NewClient(cfg)
	teamCache := cache.NewTeamCache(api, cacheTTL)
	return linearDeps{
		api:                     api,
		cache:                   teamCache,
		fetchIssuesPage:         api.FetchIssuesPage,
		fetchIssueByID:          api.FetchIssueByID,
		fetchViewPrefsFunc:      api.FetchCustomViewPreferences,
		updateIssueFunc:         api.UpdateIssue,
		createIssueFunc:         api.CreateIssue,
		createIssueRelationFunc: api.CreateIssueRelation,
		deleteIssueRelationFunc: api.DeleteIssueRelation,
		subscribeIssueFunc:      api.SubscribeToIssue,
		unsubscribeIssueFunc:    api.UnsubscribeFromIssue,
		fetchProjectsFunc:       teamCache.GetProjects,
		fetchWorkflowStatesFunc: teamCache.GetWorkflowStates,
		fetchIssueLabelsFunc:    teamCache.GetIssueLabels,
		fetchMilestonesFunc:     teamCache.GetProjectMilestones,
		fetchCyclesFunc:         teamCache.GetCycles,
		fetchUsersFunc:          teamCache.GetUsers,
		fetchCurrentUserFunc:    teamCache.GetCurrentUser,
		fetchTeamsFunc:          teamCache.GetTeams,
		fetchFavoritesFunc:      api.ListFavorites,
		createFavoriteFunc:      api.CreateFavorite,
		deleteFavoriteFunc:      api.DeleteFavorite,
		updateFavoriteSortFunc:  api.UpdateFavoriteSortOrder,
		moveFavoriteFunc:        api.MoveFavorite,
	}
}
