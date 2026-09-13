package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
)

type optionScope struct {
	teamID    string
	projectID string
}

func (a *App) navOptionScope() optionScope {
	return optionScope{teamID: a.GetSelectedTeamID()}
}

func (a *App) issueOptionScope(issue linearapi.Issue) optionScope {
	teamID := issue.TeamID
	if teamID == "" {
		teamID = a.GetSelectedTeamID()
	}
	return optionScope{teamID: teamID, projectID: issue.ProjectID}
}

type pickerLoad[T any] struct {
	name   string
	teamID string
	cached []T
	store  func(values []T)
	load   func(ctx context.Context, teamID string) ([]T, error)
	render func(values []T)
	fail   func(error)
}

func showCachedPicker[T any](a *App, p pickerLoad[T]) {
	if a.metadataTeamID == p.teamID && len(p.cached) > 0 {
		p.render(p.cached)
		return
	}
	if p.teamID == "" {
		logger.Warning("tui.app: cannot load %s, no team", p.name)
		if p.fail != nil {
			p.fail(fmt.Errorf("team context is required"))
		}
		return
	}
	teamID := p.teamID
	go func() {
		logger.Debug("tui.app: loading %s team_id=%s", p.name, teamID)
		values, err := p.load(context.Background(), teamID)
		a.QueueUpdateDraw(func() {
			if err != nil {
				logger.ErrorWithErr(err, "tui.app: failed to load %s team_id=%s", p.name, teamID)
				if p.fail != nil {
					p.fail(err)
				}
				return
			}
			if a.metadataTeamID == teamID {
				p.store(values)
			}
			p.render(values)
		})
	}()
}

func (a *App) presentPicker(title, contextLine string, items []PickerItem, onSelect func(item PickerItem)) {
	a.pickerModal.ShowWithContext(title, contextLine, items, onSelect)
}

func (a *App) issueFieldOptions(field issueField, scope optionScope, onLoaded func(items []PickerItem), onFail func(error)) {
	switch field {
	case issueFieldState:
		showCachedPicker(a, pickerLoad[linearapi.WorkflowState]{
			name:   "workflow states",
			teamID: scope.teamID,
			cached: a.workflowStates,
			store:  func(loaded []linearapi.WorkflowState) { a.workflowStates = loaded },
			load:   a.fetchWorkflowStatesFunc,
			fail:   onFail,
			render: func(states []linearapi.WorkflowState) {
				items := make([]PickerItem, 0, len(states))
				for _, state := range states {
					items = append(items, PickerItem{ID: state.ID, Label: state.Name})
				}
				onLoaded(items)
			},
		})
	case issueFieldAssignee:
		showCachedPicker(a, pickerLoad[linearapi.User]{
			name:   "users for picker",
			teamID: scope.teamID,
			cached: a.teamUsers,
			store:  func(loaded []linearapi.User) { a.teamUsers = loaded },
			load:   a.fetchUsersFunc,
			fail:   onFail,
			render: func(users []linearapi.User) {
				items := make([]PickerItem, 0, len(users))
				for _, user := range users {
					name := formatUserDisplayName(user)
					label := name
					if user.IsMe {
						label += " (me)"
					}
					items = append(items, PickerItem{ID: user.ID, Label: label, Name: name})
				}
				onLoaded(items)
			},
		})
	case issueFieldCycle:
		fetchCycles := a.fetchCyclesFunc
		showCachedPicker(a, pickerLoad[linearapi.Cycle]{
			name:   "cycles for picker",
			teamID: scope.teamID,
			cached: a.teamCycles,
			store:  func(loaded []linearapi.Cycle) { a.teamCycles = loaded },
			load: func(ctx context.Context, teamID string) ([]linearapi.Cycle, error) {
				loaded, err := fetchCycles(ctx, teamID)
				if err != nil {
					return nil, err
				}
				sortCyclesForNavigation(loaded)
				return loaded, nil
			},
			fail: onFail,
			render: func(cycles []linearapi.Cycle) {
				items := make([]PickerItem, 0, len(cycles))
				for _, cycle := range cycles {
					items = append(items, PickerItem{ID: cycle.ID, Label: cycleOptionLabel(cycle), Name: cycle.DisplayName()})
				}
				onLoaded(items)
			},
		})
	case issueFieldProject:
		showCachedPicker(a, pickerLoad[linearapi.Project]{
			name:   "projects for picker",
			teamID: scope.teamID,
			cached: a.teamProjects,
			store:  func(loaded []linearapi.Project) { a.teamProjects = loaded },
			load:   a.fetchProjectsFunc,
			fail:   onFail,
			render: func(projects []linearapi.Project) {
				items := make([]PickerItem, 0, len(projects))
				for _, project := range projects {
					items = append(items, PickerItem{ID: project.ID, Label: project.Name})
				}
				onLoaded(items)
			},
		})
	case issueFieldLabels:
		showCachedPicker(a, pickerLoad[linearapi.IssueLabel]{
			name:   "labels for picker",
			teamID: scope.teamID,
			cached: a.teamLabels,
			store:  func(loaded []linearapi.IssueLabel) { a.teamLabels = loaded },
			load:   a.fetchIssueLabelsFunc,
			fail:   onFail,
			render: func(labels []linearapi.IssueLabel) {
				items := make([]PickerItem, 0, len(labels))
				for _, label := range labels {
					items = append(items, PickerItem{ID: label.ID, Label: label.Name})
				}
				onLoaded(items)
			},
		})
	case issueFieldMilestone:
		a.projectMilestoneOptions(scope.projectID, onLoaded, onFail)
	case issueFieldTeam:
		a.teamOptions(onLoaded, onFail)
	case issueFieldPriority:
		items := make([]PickerItem, 0, len(priorityLabels))
		for value, label := range priorityLabels {
			items = append(items, PickerItem{ID: strconv.Itoa(value), Label: label})
		}
		onLoaded(items)
	default:
		logger.Warning("tui.app: no options for field %q", field)
		if onFail != nil {
			onFail(fmt.Errorf("%s cannot be picked", issueFieldNames[field]))
		}
	}
}

func (a *App) projectMilestoneOptions(projectID string, onLoaded func(items []PickerItem), onFail func(error)) {
	if strings.TrimSpace(projectID) == "" {
		if onFail != nil {
			onFail(fmt.Errorf("issue must have a project"))
		}
		return
	}
	fetch := a.fetchMilestonesFunc
	go func() {
		milestones, err := fetch(context.Background(), projectID)
		a.QueueUpdateDraw(func() {
			if err != nil {
				logger.ErrorWithErr(err, "tui.app: failed to load milestones project_id=%s", projectID)
				if onFail != nil {
					onFail(err)
				}
				return
			}
			items := make([]PickerItem, 0, len(milestones))
			for _, milestone := range milestones {
				items = append(items, PickerItem{
					ID:    milestone.ID,
					Label: milestoneOptionLabel(milestone),
					Name:  milestone.Name,
				})
			}
			onLoaded(items)
		})
	}()
}

func milestoneOptionLabel(milestone linearapi.ProjectMilestone) string {
	if milestone.TargetDate != nil && *milestone.TargetDate != "" {
		return milestone.Name + " (" + *milestone.TargetDate + ")"
	}
	return milestone.Name
}

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

var fieldPickerTitles = map[issueField]string{
	issueFieldState:     "Select Status",
	issueFieldAssignee:  "Select Assignee",
	issueFieldCycle:     "Select Cycle",
	issueFieldProject:   "Select Project",
	issueFieldMilestone: "Select Milestone",
	issueFieldPriority:  "Set Priority",
}

func (a *App) ShowFieldPicker(field issueField, scope optionScope, contextLine string, onSelect func(item PickerItem)) {
	logger.Debug("tui.app: showing %s picker", field)
	a.issueFieldOptions(field, scope, func(items []PickerItem) {
		if len(items) == 0 {
			a.flashStatus("No " + issueFieldNames[field] + " available")
			return
		}
		a.presentPicker(fieldPickerTitles[field], contextLine, items, onSelect)
	}, func(err error) {
		a.updateStatusBarWithError(err)
	})
}

func (a *App) teamOptions(onLoaded func(items []PickerItem), onFail func(error)) {
	if len(a.navTeams) > 0 {
		onLoaded(teamPickerItems(a.navTeams))
		return
	}
	fetch := a.fetchTeamsFunc
	go func() {
		teams, err := fetch(context.Background())
		a.QueueUpdateDraw(func() {
			if err != nil {
				logger.ErrorWithErr(err, "tui.app: failed to load teams for picker")
				if onFail != nil {
					onFail(err)
				}
				return
			}
			onLoaded(teamPickerItems(teams))
		})
	}()
}

func teamPickerItems(teams []linearapi.Team) []PickerItem {
	items := make([]PickerItem, 0, len(teams))
	for _, team := range teams {
		items = append(items, PickerItem{
			ID:    team.ID,
			Label: fmt.Sprintf("%s (%s)", team.Name, team.Key),
			Name:  team.Name,
		})
	}
	return items
}

func (a *App) ShowTeamPicker(contextLine string, onSelect func(item PickerItem)) {
	logger.Debug("tui.app: showing team picker")
	a.teamOptions(func(items []PickerItem) {
		if len(items) == 0 {
			logger.Warning("tui.app: no teams available for picker")
			a.updateStatusBarWithError(fmt.Errorf("no teams available"))
			return
		}
		a.presentPicker("Select Team", contextLine, items, onSelect)
	}, a.updateStatusBarWithError)
}

func (a *App) ShowParentIssuePicker(contextLine string, onSelect func(item PickerItem)) {
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
