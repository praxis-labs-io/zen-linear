package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
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

// toggleLabel focuses the multi-select, moves its highlight, and toggles the
// row through the form's own key handling.
func toggleLabel(app *App, form *IssueFormModal, index int) {
	app.app.SetFocus(form.labelsField.list)
	form.labelsField.list.SetCurrentItem(index)
	form.HandleKey(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
}

func TestIssueFormProjectChangeClearsAndReloadsMilestone(t *testing.T) {
	app, pending := newIssueFormTestApp(t)
	fetched := make(chan string, 4)
	app.fetchMilestonesFunc = func(_ context.Context, projectID string) ([]linearapi.ProjectMilestone, error) {
		fetched <- projectID
		return []linearapi.ProjectMilestone{{ID: "milestone-" + projectID, Name: "Milestone " + projectID}}, nil
	}
	created := make(chan linearapi.CreateIssueInput, 2)
	app.createIssueFunc = func(_ context.Context, input linearapi.CreateIssueInput) (linearapi.Issue, error) {
		created <- input
		return linearapi.Issue{ID: "issue-9"}, nil
	}

	// Seeded from a project in the navigation tree, which is the only way a
	// create opens with one already chosen.
	form := app.issueFormModal
	form.Show(IssueFormOptions{TeamID: "team-1", ProjectID: "project-1"})
	if got := <-fetched; got != "project-1" {
		t.Fatalf("first milestone fetch = %q, want project-1", got)
	}
	// The options have to land before one can be picked, or the milestone this
	// test is about clearing was never set.
	runNextUpdate(t, pending)
	form.milestoneField.SetCurrentOption(1)
	if form.milestone.id != "milestone-project-1" {
		t.Fatalf("milestone = %+v, want project-1's picked before the move", form.milestone)
	}

	form.projectField.SetCurrentOption(2) // "No project", "Alpha", "Beta"

	if got := <-fetched; got != "project-2" {
		t.Fatalf("second milestone fetch = %q, want project-2", got)
	}
	if form.project.id != "project-2" {
		t.Fatalf("project = %q, want project-2", form.project.id)
	}

	form.titleField.SetText("Fresh issue")
	form.submit()

	select {
	case input := <-created:
		if input.ProjectID != "project-2" {
			t.Fatalf("input.ProjectID = %q, want project-2", input.ProjectID)
		}
		// A milestone belongs to one project, so moving project orphans it.
		if input.ProjectMilestoneID != "" {
			t.Fatalf("input.ProjectMilestoneID = %q, want it cleared", input.ProjectMilestoneID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the create")
	}
}

// awaitFetch reads the team one option load asked for, failing rather than
// blocking when the form answered from a cache instead of fetching.
func awaitFetch(t *testing.T, what string, asked chan string) {
	t.Helper()
	select {
	case got := <-asked:
		if got != "team-2" {
			t.Fatalf("%s fetched for %q, want the team being created in", what, got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("%s never fetched: the form offered the navigation team's cache", what)
	}
}

// The warm caches follow the navigation tree and lag a team switch, so a form
// opened for a team they do not belong to has to fetch rather than offer them.
func TestIssueFormFetchesForTheTeamItCreatesIn(t *testing.T) {
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

	// The tree warmed team-1; the create is for the team it has moved to.
	form := app.issueFormModal
	form.Show(IssueFormOptions{TeamID: "team-2"})

	// Timed rather than a bare receive: a form that wrongly reads the warm cache
	// never fetches at all, and this has to fail rather than block.
	awaitFetch(t, "statuses", states)
	awaitFetch(t, "labels", labels)
	runUpdatesUntil(t, pending, func() bool { return len(form.statusField.options) > 1 })

	for _, option := range form.statusField.options {
		if option == "Todo" || option == "In Progress" {
			t.Fatalf("status options = %v, want team-2's, not the cached team-1 ones", form.statusField.options)
		}
	}
}

// TestIssueFormStaleMilestoneLoadIsIgnored covers the flip from one project to
// another and back: the first fetch must not paint its milestones under the
// project the form has moved on to.
func TestIssueFormStaleMilestoneLoadIsIgnored(t *testing.T) {
	app, pending := newIssueFormTestApp(t)
	form := app.issueFormModal
	form.Show(IssueFormOptions{TeamID: "team-1", ProjectID: "project-1"})

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
			created := make(chan linearapi.CreateIssueInput, 2)
			app.createIssueFunc = func(_ context.Context, input linearapi.CreateIssueInput) (linearapi.Issue, error) {
				created <- input
				return linearapi.Issue{ID: "issue-9"}, nil
			}
			form := app.issueFormModal
			form.Show(IssueFormOptions{TeamID: "team-1"})
			// A valid title, so the two field cases fail on their own field.
			form.titleField.SetText("Fresh issue")

			tc.mutate(form)
			form.submit()

			if len(created) != 0 {
				t.Fatalf("create calls = %d, want 0", len(created))
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
	form := app.issueFormModal
	// A form left half filled in, which is what the next opening must not show.
	form.Show(IssueFormOptions{TeamID: "team-1"})
	form.titleField.SetText("Abandoned")
	form.descField.SetText("Abandoned body", true)
	form.estimateField.SetText("8")
	form.dueDateField.SetText("2026-09-01")
	toggleLabel(app, form, 0)
	form.Hide()

	form.Show(IssueFormOptions{
		TeamID: "team-1",
		Parent: &linearapi.IssueRef{ID: "parent-1", Identifier: "ZNL-1", Title: "Parent issue"},
	})

	if app.app.GetFocus() != form.titleField {
		t.Fatal("Show did not focus the title field")
	}
	if form.titleField.GetText() != "" || form.descField.GetText() != "" {
		t.Fatalf("fields carried over the abandoned form: %q / %q", form.titleField.GetText(), form.descField.GetText())
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

// A cold cache must not clear a field: an empty project list would otherwise
// throw away the project the navigation tree seeded the create from.
func TestIssueFormKeepsAProjectTheOptionsCannotShow(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	app.teamProjects = nil
	app.fetchProjectsFunc = func(context.Context, string) ([]linearapi.Project, error) {
		return nil, nil
	}
	created := make(chan linearapi.CreateIssueInput, 2)
	app.createIssueFunc = func(_ context.Context, input linearapi.CreateIssueInput) (linearapi.Issue, error) {
		created <- input
		return linearapi.Issue{ID: "issue-9"}, nil
	}
	form := app.issueFormModal
	form.Show(IssueFormOptions{TeamID: "team-1", ProjectID: "project-1"})

	if form.project.id != "project-1" {
		t.Fatalf("project = %+v, want the seeded project kept", form.project)
	}
	form.titleField.SetText("Fresh issue")
	form.submit()

	select {
	case input := <-created:
		if input.ProjectID != "project-1" {
			t.Fatalf("input.ProjectID = %q, want the seeded project", input.ProjectID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the create")
	}
}

func TestIssueFormEscapeClosesTheOpenMenuBeforeModal(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	form := app.issueFormModal
	form.Show(IssueFormOptions{TeamID: "team-1"})
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
	form.Show(IssueFormOptions{TeamID: "team-1"})

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

// TestIssueFormIgnoresASecondSubmitWhileSaving keeps a slow write from being
// fired twice.
func TestIssueFormIgnoresASecondSubmitWhileSaving(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	created := make(chan linearapi.CreateIssueInput, 4)
	app.createIssueFunc = func(_ context.Context, input linearapi.CreateIssueInput) (linearapi.Issue, error) {
		created <- input
		return linearapi.Issue{ID: "issue-9"}, nil
	}
	form := app.issueFormModal
	form.Show(IssueFormOptions{TeamID: "team-1"})

	form.titleField.SetText("Fresh issue")
	form.submit()
	form.submit()

	select {
	case <-created:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the create")
	}
	select {
	case extra := <-created:
		t.Fatalf("a second submit went out while saving: %+v", extra)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestIssueFormStaleSaveDoesNotCloseAReopenedForm covers escaping out of a
// slow create and reopening: the first write must not tear down the second form.
func TestIssueFormStaleSaveDoesNotCloseAReopenedForm(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	form := app.issueFormModal
	form.Show(IssueFormOptions{TeamID: "team-1"})

	// The result handler the first create would carry.
	stale := form.completion()

	form.Hide()
	form.Show(IssueFormOptions{TeamID: "team-1"})
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

	form.Show(IssueFormOptions{TeamID: "team-2"})
	if form.fm.title != "New Issue · Design" {
		t.Fatalf("modal title = %q, want it to name the Design team", form.fm.title)
	}
	form.Hide()

	form.Show(IssueFormOptions{
		TeamID: "team-1",
		Parent: &linearapi.IssueRef{ID: "parent-1", Identifier: "LIN-1", Title: "Parent issue"},
	})
	if form.fm.title != "New Sub-Issue · Linear" {
		t.Fatalf("sub-issue title = %q, want it to name the Linear team", form.fm.title)
	}
	form.Hide()

	// A team the tree does not carry leaves the base title rather than a
	// dangling separator.
	form.Show(IssueFormOptions{TeamID: "team-unknown"})
	if form.fm.title != "New Issue" {
		t.Fatalf("title = %q, want the base with no team", form.fm.title)
	}
}
