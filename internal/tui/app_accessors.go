package tui

import (
	"context"

	"github.com/praxis-labs-io/zen-linear/internal/cache"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

func (a *App) GetAPI() *linearapi.Client {
	return a.api
}

func (a *App) GetCache() *cache.TeamCache {
	return a.cache
}

func (a *App) GetSelectedIssue() *linearapi.Issue {
	a.issuesMu.RLock()
	defer a.issuesMu.RUnlock()
	return a.selectedIssue
}

func (a *App) GetSelectedTeamID() string {
	if a.selectedNavigation != nil && a.selectedNavigation.TeamID != "" {
		return a.selectedNavigation.TeamID
	}
	a.issuesMu.RLock()
	selectedIssue := a.selectedIssue
	a.issuesMu.RUnlock()
	if selectedIssue != nil {
		return selectedIssue.TeamID
	}
	return ""
}

func (a *App) GetCurrentUser() *linearapi.User {
	return a.currentUser
}

func (a *App) GetTeamUsers() []linearapi.User {
	return a.teamUsers
}

// FetchTeamUsers fetches a team's users without caching them, so it is safe off the UI thread.
func (a *App) FetchTeamUsers(teamID string) ([]linearapi.User, error) {
	return a.fetchUsersFunc(context.Background(), teamID)
}

func (a *App) GetTeamCycles() []linearapi.Cycle {
	return a.teamCycles
}

func (a *App) FetchTeamCycles(teamID string) ([]linearapi.Cycle, error) {
	cycles, err := a.fetchCyclesFunc(context.Background(), teamID)
	if err != nil {
		return nil, err
	}
	sortCyclesForNavigation(cycles)
	return cycles, nil
}

func (a *App) QueueUpdateDraw(f func()) {
	if a.queueUpdateDraw != nil {
		a.uiUpdateMu.Lock()
		defer a.uiUpdateMu.Unlock()
		a.queueUpdateDraw(f)
		return
	}
	a.app.QueueUpdateDraw(f)
}
