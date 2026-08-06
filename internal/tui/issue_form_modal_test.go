package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// newIssueFormTestApp seeds the team metadata the form reads, so every option
// list but milestones fills synchronously and no fetch reaches the network.
// UI callbacks are routed to the test goroutine: a background fetch writing
// into the form has to be ordered against the assertions.
func newIssueFormTestApp(t *testing.T) (*App, chan func()) {
	t.Helper()
	app := newUXTestApp(t)
	app.workflowStates = []linearapi.WorkflowState{
		{ID: "state-todo", Name: "Todo"},
		{ID: "state-doing", Name: "In Progress"},
	}
	app.teamProjects = []linearapi.Project{
		{ID: "project-1", Name: "Alpha"},
		{ID: "project-2", Name: "Beta"},
	}
	app.teamLabels = []linearapi.IssueLabel{
		{ID: "label-bug", Name: "Bug"},
		{ID: "label-chore", Name: "Chore"},
	}
	app.fetchMilestonesFunc = func(_ context.Context, projectID string) ([]linearapi.ProjectMilestone, error) {
		return []linearapi.ProjectMilestone{{ID: "milestone-" + projectID, Name: "Milestone " + projectID}}, nil
	}

	pending := make(chan func(), 32)
	app.queueUpdateDraw = func(fn func()) { pending <- fn }
	return app, pending
}

// runNextUpdate runs the next queued UI callback on the test goroutine.
func runNextUpdate(t *testing.T, pending chan func()) {
	t.Helper()
	select {
	case fn := <-pending:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a queued UI update")
	}
}

func editableIssue() linearapi.Issue {
	estimate := 3.0
	due := "2026-09-01"
	return linearapi.Issue{
		ID:          "issue-1",
		Identifier:  "ZNL-7",
		Title:       "Original title",
		Description: "Original body",
		State:       "Todo",
		StateID:     "state-todo",
		Assignee:    "Test User",
		AssigneeID:  "user-1",
		Priority:    3,
		TeamID:      "team-1",
		ProjectID:   "project-1",
		ProjectName: "Alpha",
		Cycle:       &linearapi.CycleRef{ID: "cycle-1", Name: "Test Cycle", Number: 1},
		DueDate:     &due,
		Estimate:    &estimate,
		Labels:      []linearapi.IssueLabel{{ID: "label-bug", Name: "Bug"}},
	}
}

// captureUpdates stubs the mutation seam. The input travels back over a
// channel so the assertion is ordered against the goroutine that sent it.
func captureUpdates(app *App) chan linearapi.UpdateIssueInput {
	updates := make(chan linearapi.UpdateIssueInput, 4)
	app.updateIssueFunc = func(_ context.Context, input linearapi.UpdateIssueInput) (linearapi.Issue, error) {
		updates <- input
		return linearapi.Issue{ID: input.ID, Identifier: "ZNL-7"}, nil
	}
	return updates
}

func recvUpdate(t *testing.T, updates chan linearapi.UpdateIssueInput) linearapi.UpdateIssueInput {
	t.Helper()
	select {
	case input := <-updates:
		return input
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the issue update")
		return linearapi.UpdateIssueInput{}
	}
}

func showEditForm(t *testing.T, app *App, issue linearapi.Issue) *IssueFormModal {
	t.Helper()
	form := app.issueFormModal
	form.Show(IssueFormOptions{Mode: IssueFormEdit, TeamID: issue.TeamID, Issue: &issue})
	if !app.pages.HasPage("issue_form") {
		t.Fatal("Show left no issue_form page behind")
	}
	return form
}

// toggleLabel focuses the multi-select, moves its highlight, and toggles the
// row through the form's own key handling.
func toggleLabel(app *App, form *IssueFormModal, index int) {
	app.app.SetFocus(form.labelsField.list)
	form.labelsField.list.SetCurrentItem(index)
	form.HandleKey(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
}

func TestIssueFormEditSubmitsOnlyChangedFields(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	updates := captureUpdates(app)
	form := showEditForm(t, app, editableIssue())

	form.titleField.SetText("New title")
	form.priorityField.SetCurrentOption(1)
	toggleLabel(app, form, 1)

	form.submit()

	input := recvUpdate(t, updates)
	if input.ID != "issue-1" {
		t.Fatalf("input.ID = %q, want issue-1", input.ID)
	}
	if input.Title == nil || *input.Title != "New title" {
		t.Fatalf("input.Title = %v, want New title", input.Title)
	}
	if input.Priority == nil || *input.Priority != 1 {
		t.Fatalf("input.Priority = %v, want 1", input.Priority)
	}
	if input.LabelIDs == nil || !reflect.DeepEqual(*input.LabelIDs, []string{"label-bug", "label-chore"}) {
		t.Fatalf("input.LabelIDs = %v, want both labels", input.LabelIDs)
	}
	untouched := map[string]interface{}{
		"Description":        input.Description,
		"StateID":            input.StateID,
		"AssigneeID":         input.AssigneeID,
		"ProjectID":          input.ProjectID,
		"ProjectMilestoneID": input.ProjectMilestoneID,
		"CycleID":            input.CycleID,
		"DueDate":            input.DueDate,
		"Estimate":           input.Estimate,
	}
	for name, field := range untouched {
		if !reflect.ValueOf(field).IsNil() {
			t.Fatalf("input.%s = %v, want nil for an untouched field", name, field)
		}
	}
	if input.ClearEstimate {
		t.Fatal("input.ClearEstimate is true for an untouched estimate")
	}
}

func TestIssueFormEditWithNoChangesSendsNoRequest(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	updates := captureUpdates(app)
	form := showEditForm(t, app, editableIssue())

	form.submit()

	if len(updates) != 0 {
		t.Fatalf("update calls = %d, want 0", len(updates))
	}
	if app.pages.HasPage("issue_form") {
		t.Fatal("form is still open after an unchanged submit")
	}
}

func TestIssueFormEditClearsEmptiedFields(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	updates := captureUpdates(app)
	form := showEditForm(t, app, editableIssue())

	form.estimateField.SetText("")
	form.dueDateField.SetText("")
	form.assigneeField.SetCurrentOption(0)
	form.cycleField.SetCurrentOption(0)
	toggleLabel(app, form, 0)

	form.submit()

	input := recvUpdate(t, updates)
	if !input.ClearEstimate || input.Estimate != nil {
		t.Fatalf("estimate cleared as ClearEstimate=%v Estimate=%v, want true and nil", input.ClearEstimate, input.Estimate)
	}
	if input.DueDate == nil || *input.DueDate != "" {
		t.Fatalf("input.DueDate = %v, want an empty string", input.DueDate)
	}
	if input.AssigneeID == nil || *input.AssigneeID != "" {
		t.Fatalf("input.AssigneeID = %v, want an empty string", input.AssigneeID)
	}
	if input.CycleID == nil || *input.CycleID != "" {
		t.Fatalf("input.CycleID = %v, want an empty string", input.CycleID)
	}
	if input.LabelIDs == nil || len(*input.LabelIDs) != 0 {
		t.Fatalf("input.LabelIDs = %v, want an empty slice", input.LabelIDs)
	}
}

func TestIssueFormEditTargetsTheIssueCapturedAtOpen(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	updates := captureUpdates(app)
	form := showEditForm(t, app, editableIssue())

	// A background refresh moves the selection while the form is up.
	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{ID: "issue-2", Identifier: "ZNL-8"}
	app.issuesMu.Unlock()

	form.titleField.SetText("Renamed")
	form.submit()

	if input := recvUpdate(t, updates); input.ID != "issue-1" {
		t.Fatalf("input.ID = %q, want the issue the form opened on", input.ID)
	}
}

func TestIssueFormProjectChangeClearsAndReloadsMilestone(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	fetched := make(chan string, 4)
	app.fetchMilestonesFunc = func(_ context.Context, projectID string) ([]linearapi.ProjectMilestone, error) {
		fetched <- projectID
		return []linearapi.ProjectMilestone{{ID: "milestone-" + projectID, Name: "Milestone " + projectID}}, nil
	}
	updates := captureUpdates(app)

	issue := editableIssue()
	issue.ProjectMilestone = &linearapi.ProjectMilestoneRef{ID: "milestone-project-1", Name: "Milestone project-1"}
	form := showEditForm(t, app, issue)
	if got := <-fetched; got != "project-1" {
		t.Fatalf("first milestone fetch = %q, want project-1", got)
	}

	form.projectField.SetCurrentOption(2) // "No project", "Alpha", "Beta"

	if got := <-fetched; got != "project-2" {
		t.Fatalf("second milestone fetch = %q, want project-2", got)
	}
	if form.project.id != "project-2" {
		t.Fatalf("project = %q, want project-2", form.project.id)
	}

	form.submit()

	input := recvUpdate(t, updates)
	if input.ProjectID == nil || *input.ProjectID != "project-2" {
		t.Fatalf("input.ProjectID = %v, want project-2", input.ProjectID)
	}
	if input.ProjectMilestoneID == nil || *input.ProjectMilestoneID != "" {
		t.Fatalf("input.ProjectMilestoneID = %v, want an empty string", input.ProjectMilestoneID)
	}
}

// TestIssueFormStaleMilestoneLoadIsIgnored covers the flip from one project to
// another and back: the first fetch must not paint its milestones under the
// project the form has moved on to.
func TestIssueFormStaleMilestoneLoadIsIgnored(t *testing.T) {
	app, pending := newIssueFormTestApp(t)
	form := showEditForm(t, app, editableIssue())

	form.loadMilestones("project-2")
	form.loadMilestones("project-3")

	// Three fetches are in flight: the one Show kicked off, then these two.
	// Only the last one may land.
	runNextUpdate(t, pending)
	runNextUpdate(t, pending)
	runNextUpdate(t, pending)

	options := form.milestoneField.options
	for _, option := range options {
		if strings.Contains(option, "project-1") || strings.Contains(option, "project-2") {
			t.Fatalf("milestone options = %v, want only project-3 milestones", options)
		}
	}
	if len(options) != 2 || !strings.Contains(options[1], "project-3") {
		t.Fatalf("milestone options = %v, want the sentinel and project-3", options)
	}
}

func TestIssueFormRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*IssueFormModal)
		wantText string
	}{
		{"empty title", func(f *IssueFormModal) { f.titleField.SetText("") }, "title is required"},
		{"bad estimate", func(f *IssueFormModal) { f.estimateField.SetText("soon") }, "estimate must be numeric"},
		{"bad due date", func(f *IssueFormModal) { f.dueDateField.SetText("next week") }, "date must be YYYY-MM-DD"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, _ := newIssueFormTestApp(t)
			updates := captureUpdates(app)
			form := showEditForm(t, app, editableIssue())

			tc.mutate(form)
			form.submit()

			if len(updates) != 0 {
				t.Fatalf("update calls = %d, want 0", len(updates))
			}
			if !app.pages.HasPage("issue_form") {
				t.Fatal("form closed on a rejected submit")
			}
			if got := app.statusBar.GetText(true); !strings.Contains(got, tc.wantText) {
				t.Fatalf("status bar = %q, want it to mention %q", got, tc.wantText)
			}
		})
	}
}

func TestIssueFormCreateSendsEveryFieldInOneInput(t *testing.T) {
	app, pending := newIssueFormTestApp(t)
	created := make(chan linearapi.CreateIssueInput, 2)
	app.createIssueFunc = func(_ context.Context, input linearapi.CreateIssueInput) (linearapi.Issue, error) {
		created <- input
		return linearapi.Issue{ID: "issue-9", Identifier: "ZNL-9"}, nil
	}
	form := app.issueFormModal

	form.Show(IssueFormOptions{
		Mode:     IssueFormCreate,
		TeamID:   "team-1",
		ParentID: "parent-1",
		Parent:   &linearapi.IssueRef{ID: "parent-1", Identifier: "ZNL-1", Title: "Parent issue"},
	})

	form.titleField.SetText("Fresh issue")
	form.descField.SetText("Body", true)
	form.statusField.SetCurrentOption(2) // "Team default", "Todo", "In Progress"
	form.assigneeField.SetCurrentOption(1)
	form.priorityField.SetCurrentOption(2)
	form.projectField.SetCurrentOption(1) // "No project", "Alpha", "Beta"
	runNextUpdate(t, pending)             // the milestone fetch the project change kicked off
	form.milestoneField.SetCurrentOption(1)
	form.cycleField.SetCurrentOption(1)
	form.estimateField.SetText("5")
	form.dueDateField.SetText("2026-12-24")
	toggleLabel(app, form, 0)

	form.submit()

	var input linearapi.CreateIssueInput
	select {
	case input = <-created:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the create")
	}

	estimate := 5.0
	want := linearapi.CreateIssueInput{
		TeamID:             "team-1",
		Title:              "Fresh issue",
		Description:        "Body",
		ProjectID:          "project-1",
		ProjectMilestoneID: "milestone-project-1",
		StateID:            "state-doing",
		CycleID:            "cycle-1",
		AssigneeID:         "user-1",
		Priority:           2,
		ParentID:           "parent-1",
		LabelIDs:           []string{"label-bug"},
		DueDate:            "2026-12-24",
		Estimate:           &estimate,
	}
	if !reflect.DeepEqual(input, want) {
		t.Fatalf("create input = %+v, want %+v", input, want)
	}
}

func TestIssueFormCreateResetsFieldsAndShowsParent(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	form := showEditForm(t, app, editableIssue())
	form.Hide()

	form.Show(IssueFormOptions{
		Mode:   IssueFormCreate,
		TeamID: "team-1",
		Parent: &linearapi.IssueRef{ID: "parent-1", Identifier: "ZNL-1", Title: "Parent issue"},
	})

	if app.app.GetFocus() != form.titleField {
		t.Fatal("Show did not focus the title field")
	}
	if form.titleField.GetText() != "" || form.descField.GetText() != "" {
		t.Fatalf("fields carried over the edited issue: %q / %q", form.titleField.GetText(), form.descField.GetText())
	}
	if form.estimateField.GetText() != "" || form.dueDateField.GetText() != "" {
		t.Fatalf("estimate/due date carried over: %q / %q", form.estimateField.GetText(), form.dueDateField.GetText())
	}
	if len(form.labelsField.SelectedIDs()) != 0 {
		t.Fatalf("labels carried over: %v", form.labelsField.SelectedIDs())
	}
	if form.fm.title != "New Sub-Issue" {
		t.Fatalf("modal title = %q, want New Sub-Issue", form.fm.title)
	}
	if got := form.parentView.GetText(true); !strings.Contains(got, "Parent: ZNL-1 - Parent issue") {
		t.Fatalf("parent line = %q, want the parent identifier and title", got)
	}
}

func TestIssueFormEditPrefillsEveryField(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	form := showEditForm(t, app, editableIssue())

	if form.titleField.GetText() != "Original title" {
		t.Fatalf("title = %q", form.titleField.GetText())
	}
	if form.descField.GetText() != "Original body" {
		t.Fatalf("description = %q", form.descField.GetText())
	}
	if form.estimateField.GetText() != "3" {
		t.Fatalf("estimate = %q, want 3", form.estimateField.GetText())
	}
	if form.dueDateField.GetText() != "2026-09-01" {
		t.Fatalf("due date = %q", form.dueDateField.GetText())
	}
	if form.state.id != "state-todo" || form.assignee.id != "user-1" || form.project.id != "project-1" || form.cycle.id != "cycle-1" {
		t.Fatalf("pickers = %+v %+v %+v %+v", form.state, form.assignee, form.project, form.cycle)
	}
	if !reflect.DeepEqual(form.labelsField.SelectedIDs(), []string{"label-bug"}) {
		t.Fatalf("labels = %v, want label-bug", form.labelsField.SelectedIDs())
	}
	if form.fm.title != "Edit Issue" {
		t.Fatalf("modal title = %q, want Edit Issue", form.fm.title)
	}
}

// TestIssueFormKeepsAStatusTheOptionsCannotShow guards the failure that would
// clear a field on a cold cache: an empty status list must not turn the
// issue's status into no status.
func TestIssueFormKeepsAStatusTheOptionsCannotShow(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	app.workflowStates = nil
	app.fetchWorkflowStatesFunc = func(context.Context, string) ([]linearapi.WorkflowState, error) {
		return nil, nil
	}
	updates := captureUpdates(app)
	form := showEditForm(t, app, editableIssue())

	if form.state.id != "state-todo" {
		t.Fatalf("state = %+v, want the issue's own state kept", form.state)
	}
	form.titleField.SetText("Renamed")
	form.submit()

	if input := recvUpdate(t, updates); input.StateID != nil {
		t.Fatalf("input.StateID = %v, want nil", input.StateID)
	}
}

func TestIssueFormEscapeClosesTheOpenMenuBeforeModal(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	form := app.issueFormModal
	form.Show(IssueFormOptions{Mode: IssueFormCreate, TeamID: "team-1"})
	app.app.SetFocus(form.assigneeField.view)
	form.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if !form.assigneeField.IsOpen() {
		t.Fatal("Enter did not open the assignee menu")
	}

	form.HandleKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	if form.assigneeField.IsOpen() {
		t.Fatal("assignee menu is still open after Escape")
	}
	if !app.pages.HasPage("issue_form") {
		t.Fatal("issue form closed; Escape should close only the open menu")
	}
}
