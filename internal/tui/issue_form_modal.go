package tui

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/rivo/tview"
)

type IssueFormOptions struct {
	TeamID    string
	Parent    *linearapi.IssueRef
	ParentID  string
	ProjectID string
	CycleID   string
}

type issueFormValues struct {
	title       string
	description string
	stateID     string
	assigneeID  string
	projectID   string
	milestoneID string
	cycleID     string
	priority    int
	estimate    string
	dueDate     string
	labelIDs    []string
}

type pickerOption struct {
	id    string
	label string
}

// IssueFormModal creates issues only; an existing issue is edited in the details pane.
type IssueFormModal struct {
	app *App
	fm  *FormModal

	parentView     *tview.TextView
	parentRowIdx   int
	teamField      *FormPicker
	titleField     *tview.InputField
	descField      *tview.TextArea
	statusField    *FormPicker
	assigneeField  *FormPicker
	priorityField  *FormPicker
	projectField   *FormPicker
	milestoneField *FormPicker
	cycleField     *FormPicker
	estimateField  *tview.InputField
	dueDateField   *tview.InputField
	labelsField    *FormMultiSelect

	parentID  string
	formTitle string

	team      pickerOption
	state     pickerOption
	assignee  pickerOption
	project   pickerOption
	milestone pickerOption
	cycle     pickerOption
	priority  int

	saving bool
	// Atomic because tests stub QueueUpdateDraw inline, so the loader reads it off the goroutine a close writes it from.
	openGen      atomic.Int64
	milestoneGen int
}

func NewIssueFormModal(app *App) *IssueFormModal {
	f := &IssueFormModal{app: app}

	f.fm = NewFormModal(app, "New Issue")
	f.fm.SetMaxWidth(92)

	f.parentView = f.fm.AddStatic("")
	f.parentRowIdx = f.fm.RowCount() - 1
	f.teamField = f.fm.AddPicker("Team", []string{"Loading..."}, 0, nil)
	f.titleField = f.fm.AddInput("Title", "")
	f.descField = f.fm.AddTextArea("Description", "", 4)

	f.statusField = f.fm.AddPicker("Status", []string{"Loading..."}, 0, nil)
	f.assigneeField = f.fm.AddPicker("Assignee", []string{"Unassigned"}, 0, nil)
	f.priorityField = f.fm.AddPicker("Priority", priorityLabels, 3, func(_ string, index int) {
		if index >= 0 {
			f.priority = index
		}
	})
	f.fm.EndRow()

	f.projectField = f.fm.AddPicker("Project", []string{"No project"}, 0, nil)
	f.milestoneField = f.fm.AddPicker("Milestone", []string{"No milestone"}, 0, nil)
	f.cycleField = f.fm.AddPicker("Cycle", []string{"No cycle"}, 0, nil)
	f.fm.EndRow()

	var sideFields []*tview.InputField
	f.labelsField, sideFields = f.fm.AddSplitRow("Labels", 5, []string{"Due date", "Estimate"})
	f.dueDateField, f.estimateField = sideFields[0], sideFields[1]
	f.fm.SetPlaceholder(f.dueDateField, "YYYY-MM-DD")
	f.fm.SetPlaceholder(f.estimateField, "points, e.g. 3")

	f.fm.AddButtons(
		FormButton{Label: "Create", OnPress: f.submit},
		FormButton{Label: "Cancel", OnPress: f.Hide},
	)
	f.fm.SetOnSubmit(f.submit)
	f.fm.SetOnCancel(f.Hide)
	f.fm.SetInitialFocus(f.titleField)

	return f
}

func (f *IssueFormModal) Show(options IssueFormOptions) {
	f.openGen.Add(1)
	f.parentID = options.ParentID

	f.saving = false
	f.reset(options)

	logger.Debug("tui.issue_form: showing form team_id=%s parent_id=%s", f.team.id, f.parentID)
	f.fm.Show("issue_form")

	f.loadTeams()
	f.loadStatuses()
	f.loadAssignees()
	f.loadProjects()
	f.loadCycles()
	f.loadLabels()
	f.loadMilestones(f.project.id)
}

func (f *IssueFormModal) reset(options IssueFormOptions) {
	f.team = pickerOption{id: options.TeamID}

	f.formTitle = "New Issue"
	if options.Parent != nil {
		f.formTitle = "New Sub-Issue"
	}
	f.fm.SetTitle(f.createTitle(f.formTitle))
	f.fm.SetHint("Esc cancel · Tab next · Space toggle · ⏎ open dropdown · ⌃⏎ create")

	f.fm.SetRowHidden(f.parentRowIdx, options.Parent == nil)
	if options.Parent != nil {
		f.parentView.SetText(fmt.Sprintf("Parent: %s - %s", options.Parent.Identifier, options.Parent.Title))
	} else {
		f.parentView.SetText("")
	}

	f.titleField.SetText("")
	f.descField.SetText("", true)
	f.state = pickerOption{}
	f.assignee = pickerOption{}
	f.project = pickerOption{id: options.ProjectID}
	f.milestone = pickerOption{}
	f.cycle = pickerOption{id: options.CycleID}
	f.priority = 0
	f.estimateField.SetText("")
	f.dueDateField.SetText("")
	f.labelsField.SetItems(nil, nil)

	f.priorityField.SetCurrentOption(f.priority)
	f.labelsField.SetPlaceholder("Loading...")
	f.setPicker(f.teamField, teamSentinel, nil, f.team, f.assignTeam)
	f.setPicker(f.statusField, statusSentinel, nil, f.state, f.assignState)
	f.setPicker(f.assigneeField, "Unassigned", nil, f.assignee, f.assignAssignee)
	f.setPicker(f.projectField, "No project", nil, f.project, f.assignProject)
	f.setPicker(f.milestoneField, "No milestone", nil, f.milestone, f.assignMilestone)
	f.setPicker(f.cycleField, "No cycle", nil, f.cycle, f.assignCycle)
}

func (f *IssueFormModal) createTitle(base string) string {
	team := findTeamByID(f.app.navTeams, f.team.id)
	if team == nil {
		return base
	}
	return base + " · " + team.Name
}

// Linear refuses a create without a team, and a favorited project spans teams, so a create can open with none chosen.
const teamSentinel = "Select a team"

func (f *IssueFormModal) assignTeam(option pickerOption) {
	if option.id == f.team.id {
		return
	}
	f.team = option
	f.fm.SetTitle(f.createTitle(f.formTitle))

	f.state = pickerOption{}
	f.assignee = pickerOption{}
	f.project = pickerOption{}
	f.milestone = pickerOption{}
	f.cycle = pickerOption{}
	f.setPicker(f.statusField, statusSentinel, nil, f.state, f.assignState)
	f.setPicker(f.assigneeField, "Unassigned", nil, f.assignee, f.assignAssignee)
	f.setPicker(f.projectField, "No project", nil, f.project, f.assignProject)
	f.setPicker(f.cycleField, "No cycle", nil, f.cycle, f.assignCycle)
	f.labelsField.SetPlaceholder("Loading...")
	f.labelsField.SetItems(nil, nil)

	f.loadStatuses()
	f.loadAssignees()
	f.loadProjects()
	f.loadCycles()
	f.loadLabels()
	f.loadMilestones("")
}

func (f *IssueFormModal) assignState(option pickerOption)     { f.state = option }
func (f *IssueFormModal) assignAssignee(option pickerOption)  { f.assignee = option }
func (f *IssueFormModal) assignMilestone(option pickerOption) { f.milestone = option }
func (f *IssueFormModal) assignCycle(option pickerOption)     { f.cycle = option }

const statusSentinel = "Team default"

func defaultStateOption(states []linearapi.WorkflowState) pickerOption {
	for _, state := range states {
		if state.IsDefault {
			return pickerOption{id: state.ID, label: state.Name}
		}
	}
	return pickerOption{}
}

func (f *IssueFormModal) setPicker(dd *FormPicker, sentinel string, options []pickerOption, current pickerOption, assign func(pickerOption)) {
	rows := make([]pickerOption, 0, len(options)+2)
	if sentinel != "" {
		rows = append(rows, pickerOption{label: sentinel})
	}
	rows = append(rows, options...)

	selected := -1
	for i, row := range rows {
		if current.id != "" && row.id == current.id {
			selected = i
			break
		}
	}
	if selected < 0 && current.id != "" {
		label := current.label
		if label == "" {
			label = "(current)"
		}
		rows = append([]pickerOption{{id: current.id, label: label}}, rows...)
		selected = 0
	}
	if len(rows) == 0 {
		rows = append(rows, pickerOption{label: "None available"})
	}
	if selected < 0 {
		selected = 0
	}

	labels := make([]string, len(rows))
	for i, row := range rows {
		labels[i] = row.label
	}
	f.fm.SetPickerOptions(dd, labels, func(_ string, index int) {
		if index < 0 || index >= len(rows) {
			return
		}
		assign(rows[index])
	})
	dd.SetCurrentOption(selected)
}

func (f *IssueFormModal) assignProject(option pickerOption) {
	moved := option.id != f.project.id
	f.project = option
	if !moved {
		return
	}
	f.milestone = pickerOption{}
	f.loadMilestones(option.id)
}

func (f *IssueFormModal) values() issueFormValues {
	return issueFormValues{
		title:       strings.TrimSpace(f.titleField.GetText()),
		description: f.descField.GetText(),
		stateID:     f.state.id,
		assigneeID:  f.assignee.id,
		projectID:   f.project.id,
		milestoneID: f.milestone.id,
		cycleID:     f.cycle.id,
		priority:    f.priority,
		estimate:    strings.TrimSpace(f.estimateField.GetText()),
		dueDate:     strings.TrimSpace(f.dueDateField.GetText()),
		labelIDs:    f.labelsField.SelectedIDs(),
	}
}

func (f *IssueFormModal) submit() {
	if f.saving {
		return
	}
	values := f.values()
	if values.title == "" {
		f.fail(fmt.Errorf("title is required"))
		return
	}
	var estimate *float64
	if values.estimate != "" {
		parsed, err := parseEstimateInput(values.estimate)
		if err != nil {
			f.fail(err)
			return
		}
		estimate = &parsed
	}
	if values.dueDate != "" {
		if err := validateLinearDate(values.dueDate); err != nil {
			f.fail(err)
			return
		}
	}
	f.fm.SetStatus("", false)
	f.submitCreate(values, estimate)
}

func (f *IssueFormModal) begin(message string) {
	f.saving = true
	f.fm.SetStatus(message, false)
}

func (f *IssueFormModal) completion() func(error) {
	generation := f.openGen.Load()
	return func(err error) {
		if generation != f.openGen.Load() {
			return
		}
		f.finish(err)
	}
}

func (f *IssueFormModal) finish(err error) {
	f.saving = false
	if err != nil {
		f.fail(err)
		return
	}
	f.fm.SetStatus("", false)
	f.Hide()
}

func (f *IssueFormModal) fail(err error) {
	f.fm.SetStatus(err.Error(), true)
}

func (f *IssueFormModal) submitCreate(values issueFormValues, estimate *float64) {
	if f.team.id == "" {
		f.fail(fmt.Errorf("please select a team first"))
		return
	}
	input := linearapi.CreateIssueInput{
		TeamID:             f.team.id,
		Title:              values.title,
		Description:        values.description,
		ProjectID:          values.projectID,
		ProjectMilestoneID: values.milestoneID,
		StateID:            values.stateID,
		CycleID:            values.cycleID,
		AssigneeID:         values.assigneeID,
		Priority:           values.priority,
		ParentID:           f.parentID,
		LabelIDs:           values.labelIDs,
		DueDate:            values.dueDate,
		Estimate:           estimate,
	}

	f.begin("Creating...")
	f.app.createIssueFromForm(input, f.completion())
}

func warmFor[T any](f *IssueFormModal, cached []T) []T {
	if f.app.metadataTeamID != f.team.id {
		return nil
	}
	return cached
}

func loadIssueFormOptions[T any](
	f *IssueFormModal,
	cached []T,
	scopeID string,
	fetch func(string) ([]T, error),
	populate func([]T),
	onFailure func(),
) {
	if len(cached) > 0 {
		populate(cached)
		return
	}
	if scopeID == "" {
		populate(nil)
		return
	}
	generation := f.openGen.Load()
	go func() {
		loaded, err := fetch(scopeID)
		f.app.QueueUpdateDraw(func() {
			if generation != f.openGen.Load() {
				return
			}
			if scopeID != f.team.id {
				return
			}
			if err != nil {
				logger.ErrorWithErr(err, "tui.issue_form: option fetch failed scope_id=%s", scopeID)
				onFailure()
				return
			}
			populate(loaded)
		})
	}()
}

func (f *IssueFormModal) loadTeams() {
	generation := f.openGen.Load()
	f.app.teamOptions(func(items []PickerItem) {
		if generation != f.openGen.Load() {
			return
		}
		options := make([]pickerOption, 0, len(items))
		for _, item := range items {
			options = append(options, pickerOption{id: item.ID, label: item.Label})
			if item.ID == f.team.id {
				f.team.label = item.Label
			}
		}
		sentinel := teamSentinel
		if f.team.id != "" {
			sentinel = ""
		}
		f.setPicker(f.teamField, sentinel, options, f.team, f.assignTeam)
	}, func(err error) {
		if generation != f.openGen.Load() {
			return
		}
		logger.ErrorWithErr(err, "tui.issue_form: team fetch failed")
		f.setPicker(f.teamField, teamSentinel, nil, f.team, f.assignTeam)
		f.fm.SetStatus("Could not load teams", true)
	})
}

func (f *IssueFormModal) loadStatuses() {
	fetch := func(teamID string) ([]linearapi.WorkflowState, error) {
		return f.app.fetchWorkflowStatesFunc(context.Background(), teamID)
	}
	loadIssueFormOptions(f, warmFor(f, f.app.workflowStates), f.team.id, fetch, func(states []linearapi.WorkflowState) {
		options := make([]pickerOption, 0, len(states))
		for _, state := range states {
			options = append(options, pickerOption{id: state.ID, label: state.Name})
		}
		if f.state.id == "" {
			f.state = defaultStateOption(states)
		}
		sentinel := statusSentinel
		if f.state.id != "" {
			sentinel = ""
		}
		f.setPicker(f.statusField, sentinel, options, f.state, f.assignState)
	}, func() {
		f.setPicker(f.statusField, statusSentinel, nil, f.state, f.assignState)
		f.fm.SetStatus("Could not load statuses", true)
	})
}

func (f *IssueFormModal) loadAssignees() {
	loadIssueFormOptions(f, warmFor(f, f.app.GetTeamUsers()), f.team.id, f.app.FetchTeamUsers, func(users []linearapi.User) {
		options := make([]pickerOption, 0, len(users))
		for _, user := range users {
			label := user.Name
			if user.IsMe {
				label = fmt.Sprintf("%s (me)", user.Name)
			}
			options = append(options, pickerOption{id: user.ID, label: label})
			if user.IsMe && f.assignee.id == "" {
				f.assignee = options[len(options)-1]
			}
		}
		f.setPicker(f.assigneeField, "Unassigned", options, f.assignee, f.assignAssignee)
	}, func() {
		f.setPicker(f.assigneeField, "Unassigned", nil, f.assignee, f.assignAssignee)
	})
}

func (f *IssueFormModal) loadProjects() {
	fetch := func(teamID string) ([]linearapi.Project, error) {
		return f.app.fetchProjectsFunc(context.Background(), teamID)
	}
	loadIssueFormOptions(f, warmFor(f, f.app.teamProjects), f.team.id, fetch, func(projects []linearapi.Project) {
		options := make([]pickerOption, 0, len(projects))
		for _, project := range projects {
			options = append(options, pickerOption{id: project.ID, label: project.Name})
		}
		f.setPicker(f.projectField, "No project", options, f.project, f.assignProject)
	}, func() {
		f.setPicker(f.projectField, "No project", nil, f.project, f.assignProject)
	})
}

func (f *IssueFormModal) loadCycles() {
	loadIssueFormOptions(f, warmFor(f, f.app.GetTeamCycles()), f.team.id, f.app.FetchTeamCycles, func(cycles []linearapi.Cycle) {
		options := make([]pickerOption, 0, len(cycles))
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
			options = append(options, pickerOption{id: cycle.ID, label: label})
		}
		f.setPicker(f.cycleField, "No cycle", options, f.cycle, f.assignCycle)
	}, func() {
		f.setPicker(f.cycleField, "No cycle", nil, f.cycle, f.assignCycle)
	})
}

func (f *IssueFormModal) loadLabels() {
	selected := f.labelsField.SelectedIDs()
	fetch := func(teamID string) ([]linearapi.IssueLabel, error) {
		return f.app.fetchIssueLabelsFunc(context.Background(), teamID)
	}
	loadIssueFormOptions(f, warmFor(f, f.app.teamLabels), f.team.id, fetch, func(labels []linearapi.IssueLabel) {
		items := make([]MultiSelectItem, 0, len(labels))
		for _, label := range labels {
			items = append(items, MultiSelectItem{ID: label.ID, Label: label.Name})
		}
		f.labelsField.SetPlaceholder("No labels")
		f.labelsField.SetItems(items, selected)
	}, func() {
		f.labelsField.SetPlaceholder("(Failed to load labels)")
		f.labelsField.SetItems(nil, selected)
	})
}

func (f *IssueFormModal) loadMilestones(projectID string) {
	f.milestoneGen++
	generation := f.milestoneGen

	if projectID == "" {
		f.setPicker(f.milestoneField, "No milestone", nil, f.milestone, f.assignMilestone)
		return
	}

	f.setPicker(f.milestoneField, "No milestone", nil, f.milestone, f.assignMilestone)
	go func() {
		milestones, err := f.app.fetchMilestonesFunc(context.Background(), projectID)
		f.app.QueueUpdateDraw(func() {
			if generation != f.milestoneGen {
				return
			}
			if err != nil {
				logger.ErrorWithErr(err, "tui.issue_form: milestone fetch failed project_id=%s", projectID)
				f.setPicker(f.milestoneField, "No milestone", nil, f.milestone, f.assignMilestone)
				return
			}
			options := make([]pickerOption, 0, len(milestones))
			for _, milestone := range milestones {
				options = append(options, pickerOption{id: milestone.ID, label: milestone.Name})
			}
			f.setPicker(f.milestoneField, "No milestone", options, f.milestone, f.assignMilestone)
		})
	}()
}

// Hide closes the form and discards any write or fetch still in flight for this opening.
func (f *IssueFormModal) Hide() {
	f.openGen.Add(1)
	f.fm.Hide("issue_form")
}

func (f *IssueFormModal) Focus() { f.fm.Focus() }

func (f *IssueFormModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	return f.fm.HandleKey(event)
}

func (f *IssueFormModal) GetModal() *tview.Flex { return f.fm.Root() }
