package tui

import (
	"context"
	"fmt"
	"strconv"

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

// issueFieldOptions loads the rows a field can be set to. The overlay picker
// and the inline editor share it, so the two cannot offer different options.
func (a *App) issueFieldOptions(field issueField, onLoaded func(items []PickerItem)) {
	switch field {
	case issueFieldState:
		showCachedPicker(a, "workflow states", a.workflowStates,
			func(loaded []linearapi.WorkflowState) { a.workflowStates = loaded },
			a.cache.GetWorkflowStates,
			func(states []linearapi.WorkflowState) {
				items := make([]PickerItem, 0, len(states))
				for _, state := range states {
					items = append(items, PickerItem{ID: state.ID, Label: state.Name})
				}
				onLoaded(items)
			},
		)
	case issueFieldAssignee:
		showCachedPicker(a, "users for picker", a.teamUsers,
			func(loaded []linearapi.User) { a.teamUsers = loaded },
			a.cache.GetUsers,
			func(users []linearapi.User) {
				items := make([]PickerItem, 0, len(users))
				for _, user := range users {
					label := user.Name
					if user.IsMe {
						label += " (me)"
					}
					items = append(items, PickerItem{ID: user.ID, Label: label})
				}
				onLoaded(items)
			},
		)
	case issueFieldCycle:
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
			func(cycles []linearapi.Cycle) {
				items := make([]PickerItem, 0, len(cycles))
				for _, cycle := range cycles {
					items = append(items, PickerItem{ID: cycle.ID, Label: cycleOptionLabel(cycle)})
				}
				onLoaded(items)
			},
		)
	case issueFieldProject:
		showCachedPicker(a, "projects for picker", a.teamProjects,
			func(loaded []linearapi.Project) { a.teamProjects = loaded },
			a.cache.GetProjects,
			func(projects []linearapi.Project) {
				items := make([]PickerItem, 0, len(projects))
				for _, project := range projects {
					items = append(items, PickerItem{ID: project.ID, Label: project.Name})
				}
				onLoaded(items)
			},
		)
	case issueFieldPriority:
		items := make([]PickerItem, 0, len(priorityLabels))
		for value, label := range priorityLabels {
			items = append(items, PickerItem{ID: strconv.Itoa(value), Label: label})
		}
		onLoaded(items)
	default:
		logger.Warning("tui.app: no options for field %q", field)
	}
}

// cycleOptionLabel marks where a cycle sits relative to now, which is how a
// user picks one without knowing its number.
func cycleOptionLabel(cycle linearapi.Cycle) string {
	switch {
	case cycle.IsActive:
		return cycle.DisplayName() + " (active)"
	case cycle.IsNext:
		return cycle.DisplayName() + " (next)"
	case cycle.IsPrevious:
		return cycle.DisplayName() + " (previous)"
	}
	return cycle.DisplayName()
}

// issueFieldValueName is what a save calls the option just picked. Empty when
// the cache cannot name it, which fieldSetMessage answers with "Updated".
func (a *App) issueFieldValueName(field issueField, id string) string {
	switch field {
	case issueFieldState:
		for _, state := range a.workflowStates {
			if state.ID == id {
				return state.Name
			}
		}
	case issueFieldAssignee:
		for _, user := range a.teamUsers {
			if user.ID == id {
				return formatUserDisplayName(user)
			}
		}
	case issueFieldCycle:
		for _, cycle := range a.teamCycles {
			if cycle.ID == id {
				return cycle.DisplayName()
			}
		}
	case issueFieldProject:
		// Not projectNameByID: that answers a filter summary and returns the id
		// itself on a miss, which would put a uuid in the corner.
		for _, project := range a.teamProjects {
			if project.ID == id {
				return project.Name
			}
		}
	}
	return ""
}

// ShowStatusPicker shows a picker for workflow states. contextLine names the
// issue being modified; empty for non-issue uses like filters.
func (a *App) ShowStatusPicker(contextLine string, onSelect func(stateID string)) {
	logger.Debug("tui.app: showing status picker")
	a.issueFieldOptions(issueFieldState, func(items []PickerItem) {
		a.presentPicker("Select Status", contextLine, items, onSelect)
	})
}

// ShowUserPicker shows a picker for team users. contextLine names the issue
// being modified; empty for non-issue uses like filters.
func (a *App) ShowUserPicker(contextLine string, onSelect func(userID string)) {
	logger.Debug("tui.app: showing user picker")
	a.issueFieldOptions(issueFieldAssignee, func(items []PickerItem) {
		a.presentPicker("Select Assignee", contextLine, items, onSelect)
	})
}

// ShowCyclePicker shows a picker for team cycles. contextLine names the
// issue being modified; empty for non-issue uses like filters.
func (a *App) ShowCyclePicker(contextLine string, onSelect func(cycleID string)) {
	logger.Debug("tui.app: showing cycle picker")
	a.issueFieldOptions(issueFieldCycle, func(items []PickerItem) {
		a.presentPicker("Select Cycle", contextLine, items, onSelect)
	})
}

// ShowProjectPicker shows a picker for team projects. contextLine names the
// issue being modified; empty for non-issue uses like filters.
func (a *App) ShowProjectPicker(contextLine string, onSelect func(projectID string)) {
	logger.Debug("tui.app: showing project picker")
	a.issueFieldOptions(issueFieldProject, func(items []PickerItem) {
		a.presentPicker("Select Project", contextLine, items, onSelect)
	})
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
	// Snapshotted on the UI thread: applySettings reassigns linearDeps whole,
	// so reading the seam inside the goroutine races it.
	fetch := a.fetchTeamsFunc
	go func() {
		teams, err := fetch(context.Background())
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
