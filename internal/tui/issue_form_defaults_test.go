package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

// captureCreates routes the form's writes to a channel instead of the network.
func captureCreates(app *App) chan linearapi.CreateIssueInput {
	created := make(chan linearapi.CreateIssueInput, 2)
	app.createIssueFunc = func(_ context.Context, input linearapi.CreateIssueInput) (linearapi.Issue, error) {
		created <- input
		return linearapi.Issue{ID: "issue-9", Identifier: "ZNL-9"}, nil
	}
	return created
}

// submitCreate titles the form, sends it, and returns what it sent.
func submitCreate(t *testing.T, form *IssueFormModal, created chan linearapi.CreateIssueInput) linearapi.CreateIssueInput {
	t.Helper()
	form.titleField.SetText("Fresh issue")
	form.submit()
	select {
	case input := <-created:
		return input
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the create")
	}
	return linearapi.CreateIssueInput{}
}

// The form used to say "Team default" and send nothing, so the state a new
// issue landed in was only visible once it had landed.
func TestTheCreateFormOpensOnTheTeamsDefaultState(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	app.workflowStates = []linearapi.WorkflowState{
		{ID: "state-todo", Name: "Todo"},
		{ID: "state-backlog", Name: "Backlog", IsDefault: true},
	}
	created := captureCreates(app)
	form := app.issueFormModal

	form.Show(IssueFormOptions{TeamID: "team-1"})

	if _, label := form.statusField.GetCurrentOption(); label != "Backlog" {
		t.Fatalf("status opened on %q, want the team's default state by name", label)
	}
	for _, option := range form.statusField.options {
		if option == statusSentinel {
			t.Fatalf("status options = %v, want no %q row beside the state it stood for", form.statusField.options, statusSentinel)
		}
	}
	if got := submitCreate(t, form, created).StateID; got != "state-backlog" {
		t.Fatalf("create StateID = %q, want the default the form named", got)
	}
}

// A team can have no default set, and then nothing can name the state Linear
// will pick.
func TestTheSentinelStaysWhereTheTeamHasNoDefault(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	created := captureCreates(app)
	form := app.issueFormModal

	form.Show(IssueFormOptions{TeamID: "team-1"})

	if _, label := form.statusField.GetCurrentOption(); label != statusSentinel {
		t.Fatalf("status opened on %q, want %q where no state is marked default", label, statusSentinel)
	}
	if got := submitCreate(t, form, created).StateID; got != "" {
		t.Fatalf("create StateID = %q, want it left to Linear", got)
	}
}

func TestTheCreateFormOpensAssignedToMe(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	app.teamUsers = []linearapi.User{
		{ID: "user-2", Name: "Someone Else"},
		{ID: "user-1", Name: "Test User", IsMe: true},
	}
	created := captureCreates(app)
	form := app.issueFormModal

	form.Show(IssueFormOptions{TeamID: "team-1"})

	if _, label := form.assigneeField.GetCurrentOption(); label != "Test User (me)" {
		t.Fatalf("assignee opened on %q, want the current user", label)
	}
	if got := submitCreate(t, form, created).AssigneeID; got != "user-1" {
		t.Fatalf("create AssigneeID = %q, want the current user", got)
	}
}

// Linear refuses an assignee off the team, so a create in one the user is not
// a member of would fail on a default nobody typed.
func TestACreateInATeamWithoutMeFallsBackToUnassigned(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	app.teamUsers = []linearapi.User{{ID: "user-2", Name: "Someone Else"}}
	created := captureCreates(app)
	form := app.issueFormModal

	form.Show(IssueFormOptions{TeamID: "team-1"})

	if _, label := form.assigneeField.GetCurrentOption(); label != "Unassigned" {
		t.Fatalf("assignee opened on %q, want Unassigned in a team the user is not on", label)
	}
	if got := submitCreate(t, form, created).AssigneeID; got != "" {
		t.Fatalf("create AssigneeID = %q, want none", got)
	}
}

// coldMemberList leaves the member list as the form's only background load, so
// one queued callback is the whole of the fetch answering.
func coldMemberList(app *App) {
	app.teamUsers = nil
	app.currentUser = &linearapi.User{ID: "user-1", Name: "Test User", IsMe: true}
}

// Nothing confirmed the user is on the team, so opening on them would send an
// assignee Linear refuses and fail a create that used to work unassigned.
func TestAFailedMemberFetchOpensUnassigned(t *testing.T) {
	app, pending := newIssueFormTestApp(t)
	coldMemberList(app)
	app.fetchUsersFunc = func(context.Context, string) ([]linearapi.User, error) {
		return nil, errors.New("member list unavailable")
	}
	created := captureCreates(app)
	form := app.issueFormModal

	form.Show(IssueFormOptions{TeamID: "team-1"})
	runNextUpdate(t, pending)

	if _, label := form.assigneeField.GetCurrentOption(); label != "Unassigned" {
		t.Fatalf("assignee opened on %q, want Unassigned where the member list never arrived", label)
	}
	if got := submitCreate(t, form, created).AssigneeID; got != "" {
		t.Fatalf("create AssigneeID = %q, want none", got)
	}
}

// The window between opening the form and the member list landing. A create
// sent in it must not carry an assignee nothing has checked.
func TestASubmitBeforeTheMemberListLandsSendsNoAssignee(t *testing.T) {
	app, _ := newIssueFormTestApp(t)
	coldMemberList(app)
	app.fetchUsersFunc = func(context.Context, string) ([]linearapi.User, error) {
		return []linearapi.User{{ID: "user-1", Name: "Test User", IsMe: true}}, nil
	}
	created := captureCreates(app)
	form := app.issueFormModal

	form.Show(IssueFormOptions{TeamID: "team-1"})

	if got := submitCreate(t, form, created).AssigneeID; got != "" {
		t.Fatalf("create AssigneeID = %q, want none before the member list confirms it", got)
	}
}
