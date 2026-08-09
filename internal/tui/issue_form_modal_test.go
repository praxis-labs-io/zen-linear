package tui

import (
	"context"
	"errors"
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
	app.metadataTeamID = "team-1"

	app.fetchMilestonesFunc = func(_ context.Context, projectID string) ([]linearapi.ProjectMilestone, error) {
		return []linearapi.ProjectMilestone{{ID: "milestone-" + projectID, Name: "Milestone " + projectID}}, nil
	}
	// Every fetch is stubbed, including the ones the seeded caches usually
	// answer: a form opened for another team goes to these, and a test must
	// never reach the network.
	app.fetchWorkflowStatesFunc = func(_ context.Context, teamID string) ([]linearapi.WorkflowState, error) {
		return []linearapi.WorkflowState{{ID: "state-" + teamID, Name: "Todo " + teamID}}, nil
	}
	app.fetchProjectsFunc = func(_ context.Context, teamID string) ([]linearapi.Project, error) {
		return []linearapi.Project{{ID: "project-" + teamID, Name: "Project " + teamID}}, nil
	}
	app.fetchUsersFunc = func(_ context.Context, teamID string) ([]linearapi.User, error) {
		return []linearapi.User{{ID: "user-" + teamID, Name: "User " + teamID}}, nil
	}
	app.fetchCyclesFunc = func(_ context.Context, teamID string) ([]linearapi.Cycle, error) {
		return []linearapi.Cycle{{ID: "cycle-" + teamID, Name: "Cycle " + teamID, Number: 1}}, nil
	}
	app.fetchIssueLabelsFunc = func(_ context.Context, teamID string) ([]linearapi.IssueLabel, error) {
		return []linearapi.IssueLabel{{ID: "label-" + teamID, Name: "Label " + teamID}}, nil
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

// runUpdatesUntil drains queued UI callbacks until the condition holds. A
// background option fetch can queue ahead of the callback under test.
func runUpdatesUntil(t *testing.T, pending chan func(), cond func() bool) {
	t.Helper()
	for i := 0; i < 16; i++ {
		if cond() {
			return
		}
		runNextUpdate(t, pending)
	}
	if !cond() {
		t.Fatal("condition never held after draining queued UI updates")
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
			if got := form.fm.hintView.GetText(true); !strings.Contains(got, tc.wantText) {
				t.Fatalf("modal status line = %q, want it to mention %q", got, tc.wantText)
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

// TestIssueFormCreateFailureKeepsTheFormAndTheTyping is the data-loss guard:
// a refused create used to close the form and take the description with it.
func TestIssueFormCreateFailureKeepsTheFormAndTheTyping(t *testing.T) {
	app, pending := newIssueFormTestApp(t)
	attempted := make(chan linearapi.CreateIssueInput, 2)
	app.createIssueFunc = func(_ context.Context, input linearapi.CreateIssueInput) (linearapi.Issue, error) {
		attempted <- input
		return linearapi.Issue{}, errors.New("labelIds for incorrect team")
	}
	form := app.issueFormModal
	form.Show(IssueFormOptions{Mode: IssueFormCreate, TeamID: "team-1"})

	form.titleField.SetText("Audit modals")
	form.descField.SetText("A long body worth keeping", true)
	form.estimateField.SetText("5")
	form.dueDateField.SetText("2026-12-24")
	form.assigneeField.SetCurrentOption(1)
	toggleLabel(app, form, 0)

	form.submit()

	select {
	case <-attempted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the create")
	}
	runUpdatesUntil(t, pending, func() bool { return !form.saving })

	if !app.pages.HasPage("issue_form") {
		t.Fatal("the form closed on a refused create")
	}
	if got := form.fm.hintView.GetText(true); !strings.Contains(got, "labelIds for incorrect team") {
		t.Fatalf("modal status line = %q, want the refusal", got)
	}
	if got := form.titleField.GetText(); got != "Audit modals" {
		t.Fatalf("title = %q, want it kept", got)
	}
	if got := form.descField.GetText(); got != "A long body worth keeping" {
		t.Fatalf("description = %q, want it kept", got)
	}
	if form.estimateField.GetText() != "5" || form.dueDateField.GetText() != "2026-12-24" {
		t.Fatalf("estimate/due date = %q/%q, want them kept", form.estimateField.GetText(), form.dueDateField.GetText())
	}
	if form.assignee.id != "user-1" {
		t.Fatalf("assignee = %+v, want it kept", form.assignee)
	}
	if got := form.labelsField.SelectedIDs(); len(got) != 1 || got[0] != "label-bug" {
		t.Fatalf("labels = %v, want them kept", got)
	}
}

// TestIssueFormEditRetryAfterAFailureStillDiffsAgainstTheIssue: the retry has
// to measure against the server's version, not against what was resubmitted.
func TestIssueFormEditRetryAfterAFailureStillDiffsAgainstTheIssue(t *testing.T) {
	app, pending := newIssueFormTestApp(t)
	updates := make(chan linearapi.UpdateIssueInput, 4)
	results := make(chan error, 4)
	results <- errors.New("rejected")
	results <- nil
	app.updateIssueFunc = func(_ context.Context, input linearapi.UpdateIssueInput) (linearapi.Issue, error) {
		updates <- input
		if err := <-results; err != nil {
			return linearapi.Issue{}, err
		}
		return linearapi.Issue{ID: input.ID, Identifier: "ZNL-7"}, nil
	}
	form := showEditForm(t, app, editableIssue())

	form.titleField.SetText("Renamed")
	form.submit()

	if first := recvUpdate(t, updates); first.Title == nil || *first.Title != "Renamed" {
		t.Fatalf("first attempt title = %v, want Renamed", first.Title)
	}
	runUpdatesUntil(t, pending, func() bool { return !form.saving })
	if !app.pages.HasPage("issue_form") {
		t.Fatal("the form closed on a refused update")
	}
	if got := form.fm.hintView.GetText(true); !strings.Contains(got, "rejected") {
		t.Fatalf("modal status line = %q, want the refusal", got)
	}
	if got := form.titleField.GetText(); got != "Renamed" {
		t.Fatalf("title = %q, want the edit kept", got)
	}

	form.submit()

	retry := recvUpdate(t, updates)
	if retry.Title == nil || *retry.Title != "Renamed" {
		t.Fatalf("retry title = %v, want it still sent as a change", retry.Title)
	}
	if retry.Description != nil || retry.LabelIDs != nil {
		t.Fatalf("retry sent untouched fields: description=%v labels=%v", retry.Description, retry.LabelIDs)
	}
	runUpdatesUntil(t, pending, func() bool { return !app.pages.HasPage("issue_form") })
}

// TestIssueFormIgnoresASecondSubmitWhileSaving keeps a slow write from being
// fired twice.
func TestIssueFormIgnoresASecondSubmitWhileSaving(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	updates := captureUpdates(app)
	form := showEditForm(t, app, editableIssue())

	form.titleField.SetText("Renamed")
	form.submit()
	form.submit()

	recvUpdate(t, updates)
	select {
	case extra := <-updates:
		t.Fatalf("a second submit went out while saving: %+v", extra)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestIssueFormEditFetchesForTheIssuesOwnTeam guards a cross-team edit: the
// App's warm metadata belongs to whatever the navigation tree has selected,
// and offering it for another team's issue writes ids Linear refuses.
func TestIssueFormEditFetchesForTheIssuesOwnTeam(t *testing.T) {
	app, pending := newIssueFormTestApp(t)
	states := make(chan string, 4)
	app.fetchWorkflowStatesFunc = func(_ context.Context, teamID string) ([]linearapi.WorkflowState, error) {
		states <- teamID
		return []linearapi.WorkflowState{{ID: "state-other", Name: "Other Todo"}}, nil
	}
	labels := make(chan string, 4)
	app.fetchIssueLabelsFunc = func(_ context.Context, teamID string) ([]linearapi.IssueLabel, error) {
		labels <- teamID
		return []linearapi.IssueLabel{{ID: "label-other", Name: "Other"}}, nil
	}

	issue := editableIssue()
	issue.TeamID = "team-2" // the nav tree warmed team-1
	issue.StateID = "state-other"
	issue.State = "Other Todo"
	issue.Labels = nil
	form := showEditForm(t, app, issue)

	if got := <-states; got != "team-2" {
		t.Fatalf("statuses fetched for %q, want the issue's own team", got)
	}
	if got := <-labels; got != "team-2" {
		t.Fatalf("labels fetched for %q, want the issue's own team", got)
	}
	runUpdatesUntil(t, pending, func() bool { return len(form.statusField.options) > 0 })

	for _, option := range form.statusField.options {
		if option == "Todo" || option == "In Progress" {
			t.Fatalf("status options = %v, want team-2's, not the cached team-1 ones", form.statusField.options)
		}
	}
}

// TestIssueFormStaleSaveDoesNotCloseAReopenedForm covers escaping out of a
// slow save and reopening: the first write must not tear down the second form.
func TestIssueFormStaleSaveDoesNotCloseAReopenedForm(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	form := showEditForm(t, app, editableIssue())

	// The result handler the first save would carry.
	stale := form.completion()

	form.Hide()
	second := editableIssue()
	second.ID = "issue-2"
	second.Identifier = "ZNL-8"
	form.Show(IssueFormOptions{Mode: IssueFormEdit, TeamID: second.TeamID, Issue: &second})
	form.titleField.SetText("Half typed")

	stale(nil)
	if !app.pages.HasPage("issue_form") {
		t.Fatal("the earlier save closed the form that replaced it")
	}
	if got := form.titleField.GetText(); got != "Half typed" {
		t.Fatalf("title = %q, want the second form's typing untouched", got)
	}

	stale(errors.New("rejected"))
	if got := form.fm.hintView.GetText(true); strings.Contains(got, "rejected") {
		t.Fatalf("modal status line = %q, want no error from the earlier save", got)
	}
}

// TestIssueFormCreateTitleNamesTheTeam pins the team into the border. A create
// takes its team from the navigation selection and has no picker for it, so
// the title is the only thing telling the user where the issue lands.
func TestIssueFormCreateTitleNamesTheTeam(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	app.navTeams = []linearapi.Team{
		{ID: "team-1", Key: "LIN", Name: "Linear"},
		{ID: "team-2", Key: "DES", Name: "Design"},
	}
	form := app.issueFormModal

	form.Show(IssueFormOptions{Mode: IssueFormCreate, TeamID: "team-2"})
	if form.fm.title != "New Issue · Design" {
		t.Fatalf("modal title = %q, want it to name the Design team", form.fm.title)
	}
	form.Hide()

	form.Show(IssueFormOptions{
		Mode:   IssueFormCreate,
		TeamID: "team-1",
		Parent: &linearapi.IssueRef{ID: "parent-1", Identifier: "LIN-1", Title: "Parent issue"},
	})
	if form.fm.title != "New Sub-Issue · Linear" {
		t.Fatalf("sub-issue title = %q, want it to name the Linear team", form.fm.title)
	}
	form.Hide()

	// An edit is already identified by ZNL-81 in the context line, and the
	// team is not the user's to change from here.
	form.Show(IssueFormOptions{Mode: IssueFormEdit, TeamID: "team-1", Issue: &linearapi.Issue{ID: "issue-1", Identifier: "LIN-1", TeamID: "team-1"}})
	if form.fm.title != "Edit Issue" {
		t.Fatalf("edit title = %q, want Edit Issue", form.fm.title)
	}
}
