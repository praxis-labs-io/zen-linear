package tui

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// IssueFormMode selects which of the two jobs the shared issue form is doing.
type IssueFormMode int

const (
	IssueFormCreate IssueFormMode = iota
	IssueFormEdit
)

// IssueFormOptions describes one opening of the issue form.
type IssueFormOptions struct {
	Mode   IssueFormMode
	TeamID string
	// Issue is the issue being edited, captured by the caller at open time.
	Issue *linearapi.Issue
	// Parent, ParentID, ProjectID and CycleID seed a create from whatever the
	// navigation tree had selected.
	Parent    *linearapi.IssueRef
	ParentID  string
	ProjectID string
	CycleID   string
}

// issueFormValues is what the fields hold, in the shape the save diff compares.
// Estimate and due date stay as typed text so validation reports what the user
// wrote rather than what a failed parse left behind.
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

// IssueFormModal is the full issue form. New Issue opens it blank, Edit Issue
// opens it prefilled and saves every changed field in one update.
type IssueFormModal struct {
	app *App
	fm  *FormModal

	parentView     *tview.TextView
	parentRowIdx   int
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

	mode       IssueFormMode
	teamID     string
	issueID    string
	identifier string
	parentID   string

	state     pickerOption
	assignee  pickerOption
	project   pickerOption
	milestone pickerOption
	cycle     pickerOption
	priority  int

	original issueFormValues
	// saving is true from submit until the write answers. The form stays up
	// for that, so a refusal keeps the typing and the caret where they were.
	saving bool
	// openGen discards an option fetch from an earlier opening of the form.
	// Atomic because the option loaders read it from their own goroutines
	// while a close writes it, which the UI thread alone does not order.
	openGen atomic.Int64
	// milestoneGen discards a milestone fetch whose project has since changed.
	milestoneGen int
}

// NewIssueFormModal builds the shared issue form.
func NewIssueFormModal(app *App) *IssueFormModal {
	f := &IssueFormModal{app: app}

	f.fm = NewFormModal(app, "New Issue")
	f.fm.SetMaxWidth(92)

	f.parentView = f.fm.AddStatic("")
	f.parentRowIdx = f.fm.RowCount() - 1
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

	return f
}

// Show opens the form for one create or edit.
func (f *IssueFormModal) Show(options IssueFormOptions) {
	f.openGen.Add(1)
	f.mode = options.Mode
	f.teamID = options.TeamID
	f.parentID = options.ParentID
	f.issueID = ""
	f.identifier = ""

	issue := options.Issue
	if issue != nil {
		// The write targets the issue named here: a background refresh can
		// move the selection while the form is up.
		f.issueID = issue.ID
		f.identifier = issue.Identifier
		if f.teamID == "" {
			f.teamID = issue.TeamID
		}
	}

	f.saving = false
	f.reset(options)
	f.original = f.values()

	logger.Debug("tui.issue_form: showing form mode=%d team_id=%s issue_id=%s", f.mode, f.teamID, f.issueID)
	f.fm.Show("issue_form")

	f.loadStatuses()
	f.loadAssignees()
	f.loadProjects()
	f.loadCycles()
	f.loadLabels()
	f.loadMilestones(f.project.id)
}

// reset points every field and every tracked id at this opening's issue, or
// clears them for a create.
func (f *IssueFormModal) reset(options IssueFormOptions) {
	issue := options.Issue

	switch {
	case f.mode == IssueFormEdit:
		f.fm.SetTitle("Edit Issue")
		f.fm.SetButtonLabel(0, "Save")
		f.fm.SetHint("Esc cancel · Tab next · Space toggle · ⏎ open dropdown · ⌃⏎ save")
	case options.Parent != nil:
		f.fm.SetTitle(f.createTitle("New Sub-Issue"))
		f.fm.SetButtonLabel(0, "Create")
		f.fm.SetHint("Esc cancel · Tab next · Space toggle · ⏎ open dropdown · ⌃⏎ create")
	default:
		f.fm.SetTitle(f.createTitle("New Issue"))
		f.fm.SetButtonLabel(0, "Create")
		f.fm.SetHint("Esc cancel · Tab next · Space toggle · ⏎ open dropdown · ⌃⏎ create")
	}

	f.fm.SetRowHidden(f.parentRowIdx, options.Parent == nil)
	if options.Parent != nil {
		f.parentView.SetText(fmt.Sprintf("Parent: %s - %s", options.Parent.Identifier, options.Parent.Title))
	} else {
		f.parentView.SetText("")
	}

	if issue != nil {
		f.fm.SetContext(f.app.issueContextLine(*issue))
		f.titleField.SetText(issue.Title)
		f.descField.SetText(issue.Description, true)
		f.state = pickerOption{id: issue.StateID, label: issue.State}
		f.assignee = pickerOption{id: issue.AssigneeID, label: issue.Assignee}
		f.project = pickerOption{id: issue.ProjectID, label: issue.ProjectName}
		f.milestone = pickerOption{}
		if issue.ProjectMilestone != nil {
			f.milestone = pickerOption{id: issue.ProjectMilestone.ID, label: issue.ProjectMilestone.Name}
		}
		f.cycle = pickerOption{}
		if issue.Cycle != nil {
			f.cycle = pickerOption{id: issue.Cycle.ID, label: issue.Cycle.DisplayName()}
		}
		f.priority = issue.Priority
		f.estimateField.SetText(estimateText(issue.Estimate))
		due := ""
		if issue.DueDate != nil {
			due = *issue.DueDate
		}
		f.dueDateField.SetText(due)
		f.labelsField.SetItems(nil, labelIDs(issue.Labels))
	} else {
		f.fm.SetContext("")
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
	}

	f.priorityField.SetCurrentOption(f.priority)
	f.labelsField.SetPlaceholder("Loading...")
	f.setPicker(f.statusField, f.statusSentinel(), nil, f.state, f.assignState)
	f.setPicker(f.assigneeField, "Unassigned", nil, f.assignee, f.assignAssignee)
	f.setPicker(f.projectField, "No project", nil, f.project, f.assignProject)
	f.setPicker(f.milestoneField, "No milestone", nil, f.milestone, f.assignMilestone)
	f.setPicker(f.cycleField, "No cycle", nil, f.cycle, f.assignCycle)
}

// createTitle names the team the new issue will land in. A create takes its
// team from the navigation selection and offers no way to change it, so the
// border is the only place that says where the issue is going.
func (f *IssueFormModal) createTitle(base string) string {
	team := findTeamByID(f.app.navTeams, f.teamID)
	if team == nil {
		return base
	}
	return base + " · " + team.Name
}

func (f *IssueFormModal) assignState(option pickerOption)     { f.state = option }
func (f *IssueFormModal) assignAssignee(option pickerOption)  { f.assignee = option }
func (f *IssueFormModal) assignMilestone(option pickerOption) { f.milestone = option }
func (f *IssueFormModal) assignCycle(option pickerOption)     { f.cycle = option }

// statusSentinel is the "leave it to Linear" row. An existing issue always has
// a status, so only a create offers one.
func (f *IssueFormModal) statusSentinel() string {
	if f.mode == IssueFormCreate {
		return "Team default"
	}
	return ""
}

// setPicker rebuilds a dropdown around the value the form currently holds.
// SetCurrentOption fires the change callback, so assign runs for the selected
// row and the tracked value and the visible row cannot drift apart. A current
// value the options can't show is kept as its own row: a slow or failed fetch
// must not quietly clear a field the issue has.
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

// values reads the fields back into a comparable snapshot.
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

// submit validates the form and routes to the create or the update path. A
// rejected form stays open with the reason on the status bar.
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

	if f.mode == IssueFormEdit {
		f.submitEdit(values, estimate)
		return
	}
	f.submitCreate(values, estimate)
}

func (f *IssueFormModal) submitEdit(values issueFormValues, estimate *float64) {
	input, changed := buildIssueUpdate(f.issueID, f.original, values, estimate)
	if !changed {
		f.Hide()
		f.app.flashStatus("No changes")
		return
	}
	f.begin("Saving...")
	f.app.runIssueUpdateWithResult(input, fmt.Sprintf("Updated %s", f.identifier), f.completion())
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
	if f.teamID == "" {
		f.fail(fmt.Errorf("please select a team first"))
		return
	}
	input := linearapi.CreateIssueInput{
		TeamID:             f.teamID,
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

// buildIssueUpdate turns the difference between the form as opened and the
// form as submitted into one update. A nil field means no change, an empty
// string clears. estimate is the submitted text already parsed, nil when the
// field is empty.
func buildIssueUpdate(issueID string, original, current issueFormValues, estimate *float64) (linearapi.UpdateIssueInput, bool) {
	input := linearapi.UpdateIssueInput{ID: issueID}
	changed := false

	if current.title != original.title {
		input.Title = &current.title
		changed = true
	}
	if current.description != original.description {
		input.Description = &current.description
		changed = true
	}
	if current.stateID != original.stateID {
		input.StateID = &current.stateID
		changed = true
	}
	if current.assigneeID != original.assigneeID {
		input.AssigneeID = &current.assigneeID
		changed = true
	}
	if current.projectID != original.projectID {
		input.ProjectID = &current.projectID
		changed = true
	}
	if current.milestoneID != original.milestoneID {
		input.ProjectMilestoneID = &current.milestoneID
		changed = true
	}
	if current.cycleID != original.cycleID {
		input.CycleID = &current.cycleID
		changed = true
	}
	if current.priority != original.priority {
		input.Priority = &current.priority
		changed = true
	}
	if current.dueDate != original.dueDate {
		input.DueDate = &current.dueDate
		changed = true
	}
	if current.estimate != original.estimate {
		input.ClearEstimate = estimate == nil
		input.Estimate = estimate
		changed = true
	}
	if !reflect.DeepEqual(current.labelIDs, original.labelIDs) {
		labels := current.labelIDs
		input.LabelIDs = &labels
		changed = true
	}

	return input, changed
}

// warmFor returns the App's cached metadata only when it belongs to the team
// this form is working on. The caches follow the navigation tree, and editing
// an issue from another team must not offer that team's ids: Linear rejects a
// foreign state or label, and accepts a foreign project.
func warmFor[T any](f *IssueFormModal, cached []T) []T {
	if f.app.metadataTeamID != f.teamID {
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

func (f *IssueFormModal) loadStatuses() {
	fetch := func(teamID string) ([]linearapi.WorkflowState, error) {
		return f.app.fetchWorkflowStatesFunc(context.Background(), teamID)
	}
	loadIssueFormOptions(f, warmFor(f, f.app.workflowStates), f.teamID, fetch, func(states []linearapi.WorkflowState) {
		options := make([]pickerOption, 0, len(states))
		for _, state := range states {
			options = append(options, pickerOption{id: state.ID, label: state.Name})
		}
		f.setPicker(f.statusField, f.statusSentinel(), options, f.state, f.assignState)
	}, func() {
		// Reporting the failure as an option would make it selectable, and its
		// empty id would then be saved as the issue's status.
		f.setPicker(f.statusField, f.statusSentinel(), nil, f.state, f.assignState)
		f.fm.SetStatus("Could not load statuses", true)
	})
}

func (f *IssueFormModal) loadAssignees() {
	loadIssueFormOptions(f, warmFor(f, f.app.GetTeamUsers()), f.teamID, f.app.FetchTeamUsers, func(users []linearapi.User) {
		options := make([]pickerOption, 0, len(users))
		for _, user := range users {
			label := user.Name
			if user.IsMe {
				label = fmt.Sprintf("%s (me)", user.Name)
			}
			options = append(options, pickerOption{id: user.ID, label: label})
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
	loadIssueFormOptions(f, warmFor(f, f.app.teamProjects), f.teamID, fetch, func(projects []linearapi.Project) {
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
	loadIssueFormOptions(f, warmFor(f, f.app.GetTeamCycles()), f.teamID, f.app.FetchTeamCycles, func(cycles []linearapi.Cycle) {
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
	loadIssueFormOptions(f, warmFor(f, f.app.teamLabels), f.teamID, fetch, func(labels []linearapi.IssueLabel) {
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

// estimateText renders an estimate for the form. formatEstimate is for the
// table and prints "-" for nothing, which the form would then try to save.
func estimateText(estimate *float64) string {
	if estimate == nil {
		return ""
	}
	return strconv.FormatFloat(*estimate, 'f', -1, 64)
}

// labelIDs returns an issue's label ids, sorted to match what the multi-select
// hands back.
func labelIDs(labels []linearapi.IssueLabel) []string {
	ids := make([]string, 0, len(labels))
	for _, label := range labels {
		ids = append(ids, label.ID)
	}
	sort.Strings(ids)
	return ids
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
