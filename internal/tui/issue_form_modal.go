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

// IssueFormOptions describes one opening of the issue form.
type IssueFormOptions struct {
	TeamID string
	// Parent, ParentID, ProjectID and CycleID seed a create from whatever the
	// navigation tree had selected.
	Parent    *linearapi.IssueRef
	ParentID  string
	ProjectID string
	CycleID   string
}

// issueFormValues is what the fields hold at submit. Estimate and due date stay
// as typed text, so validation reports what the user wrote.
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

// pickerOption is one row of a dropdown: what the user reads, and the id the
// save sends.
type pickerOption struct {
	id    string
	label string
}

// IssueFormModal is the full issue form, blank, for creating one issue. An
// existing issue is edited in the details pane.
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

	parentID string
	// formTitle is the border's base text, kept so a team change can retitle
	// without re-deriving whether this is a sub-issue.
	formTitle string

	team      pickerOption
	state     pickerOption
	assignee  pickerOption
	project   pickerOption
	milestone pickerOption
	cycle     pickerOption
	priority  int

	// saving is true from submit until the write answers. The form stays up
	// for that, so a refusal keeps the typing and the caret where they were.
	saving bool
	// openGen discards an option fetch from an earlier opening of the form.
	// Atomic because the guard has to hold wherever the queued callback runs:
	// the tests stub QueueUpdateDraw to run inline, so the loader reads this
	// on its own goroutine while a close writes it from the test's.
	openGen atomic.Int64
	// milestoneGen discards a milestone fetch whose project has since changed.
	milestoneGen int
}

// NewIssueFormModal builds the issue form.
func NewIssueFormModal(app *App) *IssueFormModal {
	f := &IssueFormModal{app: app}

	f.fm = NewFormModal(app, "New Issue")
	f.fm.SetMaxWidth(92)

	f.parentView = f.fm.AddStatic("")
	f.parentRowIdx = f.fm.RowCount() - 1
	// First, because every field below it is loaded for whatever it holds.
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
	// The team governs the form, so it reads first; the title is what the user
	// opened the form to type.
	f.fm.SetInitialFocus(f.titleField)

	return f
}

// Show opens the form for one create.
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

// reset empties every field and seeds the ids a create takes from whatever the
// navigation tree had selected.
func (f *IssueFormModal) reset(options IssueFormOptions) {
	// Before the title, which names the team the create is going to.
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

// createTitle names the team the new issue will land in, so a form scrolled
// down to its buttons still says where the issue is going.
func (f *IssueFormModal) createTitle(base string) string {
	team := findTeamByID(f.app.navTeams, f.team.id)
	if team == nil {
		return base
	}
	return base + " · " + team.Name
}

// teamSentinel is the row a create offers before a team is chosen. A scope
// with no team of its own opens here: a favorited project spans teams, and
// Linear takes the team as the one field a create cannot do without.
const teamSentinel = "Select a team"

// assignTeam moves the whole form to another team. Every field below it is
// loaded for one team and refused by Linear for any other, so the picks go
// with the team that offered them.
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

// statusSentinel is the row a create offers when nothing can name the state
// Linear would pick: a team with no default, or a fetch that failed.
const statusSentinel = "Team default"

// defaultStateOption is the state a create opens on, which Linear would
// otherwise apply without naming it. Empty where the team has none set.
func defaultStateOption(states []linearapi.WorkflowState) pickerOption {
	for _, state := range states {
		if state.IsDefault {
			return pickerOption{id: state.ID, label: state.Name}
		}
	}
	return pickerOption{}
}

// setPicker rebuilds a dropdown around the value the form holds. A value the
// options cannot show keeps a row of its own, or a slow fetch clears it.
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

// assignProject swaps the milestone list to the new project's. A milestone
// belongs to one project, so the old one would be orphaned.
func (f *IssueFormModal) assignProject(option pickerOption) {
	moved := option.id != f.project.id
	f.project = option
	if !moved {
		return
	}
	f.milestone = pickerOption{}
	f.loadMilestones(option.id)
}

// values reads the fields back into the shape submitCreate sends.
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

// submit validates the form and creates the issue. A rejected form stays open
// with the reason on the status bar.
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

// begin marks a write in flight: the form stays up, says so, and takes no
// second submit until the first one answers.
func (f *IssueFormModal) begin(message string) {
	f.saving = true
	f.fm.SetStatus(message, false)
}

// completion binds a result handler to this opening of the form. A write the
// user escaped out of, or one from a form since reopened on another issue,
// must not close or repaint what is on screen now.
func (f *IssueFormModal) completion() func(error) {
	generation := f.openGen.Load()
	return func(err error) {
		if generation != f.openGen.Load() {
			return
		}
		f.finish(err)
	}
}

// finish closes the form on a successful write, or hands the reason back to
// the user with every field still as they left it.
func (f *IssueFormModal) finish(err error) {
	f.saving = false
	if err != nil {
		f.fail(err)
		return
	}
	f.fm.SetStatus("", false)
	f.Hide()
}

// fail reports inside the modal, where the user is looking.
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

// warmFor returns the cached metadata only when it belongs to the team being
// created in. The cache lags a team switch, and Linear rejects a foreign state.
func warmFor[T any](f *IssueFormModal, cached []T) []T {
	if f.app.metadataTeamID != f.team.id {
		return nil
	}
	return cached
}

// loadIssueFormOptions fills a field from cached team data, or fetches it in
// the background when the cache is cold. A method cannot take a type
// parameter, hence the free function.
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
			// A fetch from an earlier opening must not write into this one.
			if generation != f.openGen.Load() {
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

// loadTeams fills the team picker from the workspace's teams. It is the one
// option list a create does not scope to a team, so it never reloads on a
// team change.
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
		// The sentinel stood for a team nobody had chosen. Once one is on the
		// row, leaving it would be a second row meaning no team.
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
		// The sentinel stood for a state nobody could name. Naming it leaves two
		// rows meaning one thing, so it goes wherever the default resolved.
		sentinel := statusSentinel
		if f.state.id != "" {
			sentinel = ""
		}
		f.setPicker(f.statusField, sentinel, options, f.state, f.assignState)
	}, func() {
		// Reporting the failure as an option would make it selectable, and its
		// empty id would then be saved as the issue's status.
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
			// Read off the member list rather than App.currentUser, so a team
			// that does not list the viewer cannot open assigned to them.
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

// loadLabels fills the label list, keeping whatever is already ticked: the
// options arrive after the form has been told what the issue carries.
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

// loadMilestones fills the milestone picker for one project. A fetch whose
// project has since changed is dropped, so flipping between projects cannot
// leave one project's milestones under another.
func (f *IssueFormModal) loadMilestones(projectID string) {
	f.milestoneGen++
	generation := f.milestoneGen

	if projectID == "" {
		f.setPicker(f.milestoneField, "No milestone", nil, f.milestone, f.assignMilestone)
		return
	}

	// Keep the current value on screen while the fetch runs. A "Loading..."
	// row would be selectable and would save as no milestone.
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

// Hide closes the form, and retires this opening so a write or a fetch still
// in flight cannot write into the next one.
func (f *IssueFormModal) Hide() {
	f.openGen.Add(1)
	f.fm.Hide("issue_form")
}

// Focus returns keyboard focus to the form, for when an overlay closes.
func (f *IssueFormModal) Focus() { f.fm.Focus() }

// HandleKey handles keyboard input for the form.
func (f *IssueFormModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	return f.fm.HandleKey(event)
}

// GetModal returns the modal flex for adding to pages.
func (f *IssueFormModal) GetModal() *tview.Flex { return f.fm.Root() }
