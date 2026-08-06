package tui

import (
	"context"

	"github.com/zen-linear/zen-linear/internal/cache"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// GetAPI returns the Linear API client (used by commands).
func (a *App) GetAPI() *linearapi.Client {
	return a.api
}

// GetCache returns the team cache (used by commands).
func (a *App) GetCache() *cache.TeamCache {
	return a.cache
}

// GetSelectedIssue returns the currently selected issue.
func (a *App) GetSelectedIssue() *linearapi.Issue {
	a.issuesMu.RLock()
	defer a.issuesMu.RUnlock()
	return a.selectedIssue
}

// GetSelectedTeamID returns the currently selected team ID, if any.
func (a *App) GetSelectedTeamID() string {
	if a.selectedNavigation != nil && a.selectedNavigation.TeamID != "" {
		return a.selectedNavigation.TeamID
	}
	// If we have a selected issue, use its team
	a.issuesMu.RLock()
	selectedIssue := a.selectedIssue
	a.issuesMu.RUnlock()
	if selectedIssue != nil {
		return selectedIssue.TeamID
	}
	return ""
}

// GetCurrentUser returns the current authenticated user.
func (a *App) GetCurrentUser() *linearapi.User {
	return a.currentUser
}

// GetTeamUsers returns the users for the currently selected team.
func (a *App) GetTeamUsers() []linearapi.User {
	return a.teamUsers
}

// FetchTeamUsers fetches users for a specific team from the API. It only
// returns them: the caller runs off the UI thread, and a.teamUsers belongs to
// the event loop.
func (a *App) FetchTeamUsers(teamID string) ([]linearapi.User, error) {
	return a.fetchUsersFunc(context.Background(), teamID)
}

// GetTeamCycles returns the cycles for the currently selected team.
func (a *App) GetTeamCycles() []linearapi.Cycle {
	return a.teamCycles
}

// FetchTeamCycles fetches cycles for a specific team from the API, in
// navigation order. Like FetchTeamUsers it only returns them.
func (a *App) FetchTeamCycles(teamID string) ([]linearapi.Cycle, error) {
	cycles, err := a.fetchCyclesFunc(context.Background(), teamID)
	if err != nil {
		return nil, err
	}
	sortCyclesForNavigation(cycles)
	return cycles, nil
}

// QueueUpdateDraw queues a UI update function to be run in the main thread.
func (a *App) QueueUpdateDraw(f func()) {
	if a.queueUpdateDraw != nil {
		// Serialize UI updates when test overrides queueUpdateDraw to execute immediately
		a.uiUpdateMu.Lock()
		defer a.uiUpdateMu.Unlock()
		a.queueUpdateDraw(f)
		return
	}
	a.app.QueueUpdateDraw(f)
}
