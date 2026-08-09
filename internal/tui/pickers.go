package tui

import (
	"context"
	"fmt"

	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// loadPickerData fetches picker data for the selected team in the background and
// hands it to onLoaded on the UI thread, where the caller caches it. A method
// cannot take a type parameter, hence the free function.
func loadPickerData[T any](
	a *App,
	resourceName string,
	load func(ctx context.Context, teamID string) ([]T, error),
	onLoaded func(values []T),
) {
	teamID := a.GetSelectedTeamID()
	if teamID == "" {
		logger.Warning("tui.app: cannot show %s picker, no team selected", resourceName)
		return
	}
	go func() {
		logger.Debug("tui.app: loading %s team_id=%s", resourceName, teamID)
		values, err := load(context.Background(), teamID)
		if err != nil {
			logger.ErrorWithErr(err, "tui.app: failed to load %s team_id=%s", resourceName, teamID)
			a.QueueUpdateDraw(func() {
				a.updateStatusBarWithError(err)
			})
			return
		}
		logger.Debug("tui.app: loaded %s team_id=%s", resourceName, teamID)
		a.QueueUpdateDraw(func() {
			onLoaded(values)
		})
	}()
}

// showCachedPicker renders cached picker data immediately, or loads it for the
// selected team first — caching it via store — then renders. A method cannot
// take a type parameter, hence the free function.
func showCachedPicker[T any](
	a *App,
	resourceName string,
	cached []T,
	store func(values []T),
	load func(ctx context.Context, teamID string) ([]T, error),
	render func(values []T),
) {
	if len(cached) == 0 {
		loadPickerData(a, resourceName, load, func(loaded []T) {
			store(loaded)
			render(loaded)
		})
		return
	}
	render(cached)
}

// presentPicker opens the shared picker modal over items, forwarding the chosen
// item's ID to onSelect.
func (a *App) presentPicker(title, contextLine string, items []PickerItem, onSelect func(id string)) {
	a.pickerModal.ShowWithContext(title, contextLine, items, func(item PickerItem) {
		onSelect(item.ID)
	})
}

// ShowStatusPicker shows a picker for workflow states. contextLine names the
// issue being modified; empty for non-issue uses like filters.
func (a *App) ShowStatusPicker(contextLine string, onSelect func(stateID string)) {
	logger.Debug("tui.app: showing status picker")
	showCachedPicker(a, "workflow states", a.workflowStates,
		func(loaded []linearapi.WorkflowState) { a.workflowStates = loaded },
		a.cache.GetWorkflowStates,
		func(states []linearapi.WorkflowState) { a.showStatusPickerWithStates(states, contextLine, onSelect) },
	)
}

func (a *App) showStatusPickerWithStates(states []linearapi.WorkflowState, contextLine string, onSelect func(stateID string)) {
	items := make([]PickerItem, 0, len(states))
	for _, state := range states {
		items = append(items, PickerItem{
			ID:    state.ID,
			Label: state.Name,
		})
	}

	a.presentPicker("Select Status", contextLine, items, onSelect)
}

// ShowUserPicker shows a picker for team users. contextLine names the issue
// being modified; empty for non-issue uses like filters.
func (a *App) ShowUserPicker(contextLine string, onSelect func(userID string)) {
	logger.Debug("tui.app: showing user picker")
	showCachedPicker(a, "users for picker", a.teamUsers,
		func(loaded []linearapi.User) { a.teamUsers = loaded },
		a.cache.GetUsers,
		func(users []linearapi.User) { a.showUserPickerWithUsers(users, contextLine, onSelect) },
	)
}

func (a *App) showUserPickerWithUsers(users []linearapi.User, contextLine string, onSelect func(userID string)) {
	items := make([]PickerItem, 0, len(users))
	for _, user := range users {
		label := user.Name
		if user.IsMe {
			label += " (me)"
		}
		items = append(items, PickerItem{
			ID:    user.ID,
			Label: label,
		})
	}

	a.presentPicker("Select Assignee", contextLine, items, onSelect)
}

// ShowCyclePicker shows a picker for team cycles. contextLine names the
// issue being modified; empty for non-issue uses like filters.
func (a *App) ShowCyclePicker(contextLine string, onSelect func(cycleID string)) {
	logger.Debug("tui.app: showing cycle picker")
	showCachedPicker(a, "cycles for picker", a.teamCycles,
		func(loaded []linearapi.Cycle) { a.teamCycles = loaded },
		func(ctx context.Context, teamID string) ([]linearapi.Cycle, error) {
			loaded, err := a.cache.GetCycles(ctx, teamID)
			if err != nil {
				return nil, err
			}
			sortCyclesForNavigation(loaded)
			return loaded, nil
		},
		func(cycles []linearapi.Cycle) { a.showCyclePickerWithCycles(cycles, contextLine, onSelect) },
	)
}

func (a *App) showCyclePickerWithCycles(cycles []linearapi.Cycle, contextLine string, onSelect func(cycleID string)) {
	items := make([]PickerItem, 0, len(cycles))
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
		items = append(items, PickerItem{
			ID:    cycle.ID,
			Label: label,
		})
	}

	a.presentPicker("Select Cycle", contextLine, items, onSelect)
}

// ShowProjectPicker shows a picker for team projects. contextLine names the
// issue being modified; empty for non-issue uses like filters.
func (a *App) ShowProjectPicker(contextLine string, onSelect func(projectID string)) {
	logger.Debug("tui.app: showing project picker")
	showCachedPicker(a, "projects for picker", a.teamProjects,
		func(loaded []linearapi.Project) { a.teamProjects = loaded },
		a.cache.GetProjects,
		func(projects []linearapi.Project) { a.showProjectPickerWithProjects(projects, contextLine, onSelect) },
	)
}

func (a *App) showProjectPickerWithProjects(projects []linearapi.Project, contextLine string, onSelect func(projectID string)) {
	items := make([]PickerItem, 0, len(projects))
	for _, project := range projects {
		items = append(items, PickerItem{
			ID:    project.ID,
			Label: project.Name,
		})
	}

	a.presentPicker("Select Project", contextLine, items, onSelect)
}

// ShowTeamPicker shows a picker for the workspace's teams. The navigation tree
// is built from that same list, so this only fetches in the window before the
// tree has painted. contextLine names the issue being moved.
func (a *App) ShowTeamPicker(contextLine string, onSelect func(teamID string)) {
	logger.Debug("tui.app: showing team picker")
	if len(a.navTeams) > 0 {
		a.showTeamPickerWithTeams(a.navTeams, contextLine, onSelect)
		return
	}
	go func() {
		teams, err := a.fetchTeamsFunc(context.Background())
		a.QueueUpdateDraw(func() {
			if err != nil {
				logger.ErrorWithErr(err, "tui.app: failed to load teams for picker")
				a.updateStatusBarWithError(err)
				return
			}
			a.showTeamPickerWithTeams(teams, contextLine, onSelect)
		})
	}()
}

func (a *App) showTeamPickerWithTeams(teams []linearapi.Team, contextLine string, onSelect func(teamID string)) {
	if len(teams) == 0 {
		logger.Warning("tui.app: no teams available for picker")
		a.updateStatusBarWithError(fmt.Errorf("no teams available"))
		return
	}
	items := make([]PickerItem, 0, len(teams))
	for _, team := range teams {
		items = append(items, PickerItem{
			ID:    team.ID,
			Label: fmt.Sprintf("%s (%s)", team.Name, team.Key),
		})
	}

	a.presentPicker("Select Team", contextLine, items, onSelect)
}

// ShowParentIssuePicker shows a picker for selecting a parent issue.
// It lists all top-level issues (issues without a parent) from the current
// list. contextLine names the issue being reparented.
func (a *App) ShowParentIssuePicker(contextLine string, onSelect func(parentID string)) {
	// Filter to only show issues that could be parents (no parent themselves)
	a.issuesMu.RLock()
	issues := a.issues
	selectedIssue := a.selectedIssue
	a.issuesMu.RUnlock()
	excludedIDs := excludedParentCandidateIDs(selectedIssue, issues)
	items := make([]PickerItem, 0)
	for _, issue := range issues {
		if issue.Parent == nil && !excludedIDs[issue.ID] {
			items = append(items, PickerItem{
				ID:    issue.ID,
				Label: issue.Identifier + " - " + issue.Title,
			})
		}
	}

	if len(items) == 0 {
		logger.Warning("tui.app: no parent issues available for picker")
		a.updateStatusBarWithError(fmt.Errorf("no parent issues available"))
		return
	}
	logger.Debug("tui.app: parent issue picker items count=%d", len(items))

	a.presentPicker("Select Parent Issue", contextLine, items, onSelect)
}

func excludedParentCandidateIDs(selected *linearapi.Issue, issues []linearapi.Issue) map[string]bool {
	excluded := make(map[string]bool)
	if selected == nil {
		return excluded
	}
	excluded[selected.ID] = true
	byID := make(map[string]linearapi.Issue, len(issues))
	for _, issue := range issues {
		byID[issue.ID] = issue
	}
	var visit func(issue linearapi.Issue)
	visit = func(issue linearapi.Issue) {
		for _, child := range issue.Children {
			if excluded[child.ID] {
				continue
			}
			excluded[child.ID] = true
			if fullChild, ok := byID[child.ID]; ok {
				visit(fullChild)
			}
		}
	}
	visit(*selected)
	return excluded
}
