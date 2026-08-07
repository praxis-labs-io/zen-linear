package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

func TestRenderIssueRow_IncludesPlanningFields(t *testing.T) {
	dueDate := "2026-06-15"
	estimate := 5.0
	issue := linearapi.Issue{
		ID:         "issue-1",
		Identifier: "LIN-1",
		Title:      "Ship planning fields",
		State:      "Todo",
		Assignee:   "Ada Lovelace",
		Priority:   2,
		Cycle:      &linearapi.CycleRef{ID: "cycle-1", Name: "Launch"},
		DueDate:    &dueDate,
		Estimate:   &estimate,
		ProjectMilestone: &linearapi.ProjectMilestoneRef{
			ID:   "milestone-1",
			Name: "Beta",
		},
	}

	// Planning fields (due date, estimate, milestone) intentionally stay out
	// of the list row — the layout matches Linear's list view and planning
	// data lives in the details pane (covered below).
	row := renderIssueRow(issue)
	if len(row) != 7 {
		t.Fatalf("renderIssueRow() length = %d, want 7: %#v", len(row), row)
	}
	if row[3] != issue.Title {
		t.Fatalf("title column = %q, want %q", row[3], issue.Title)
	}
	for _, cell := range row {
		if cell == dueDate || cell == "5" || cell == "Beta" {
			t.Fatalf("planning field %q leaked into the list row: %#v", cell, row)
		}
	}
}

func TestUpdateDetailsView_IncludesPlanningAndCollaboration(t *testing.T) {
	cfg := config.Config{
		PageSize: 1,
		CacheTTL: time.Minute,
	}
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
	app.queueUpdateDraw = func(f func()) { f() }

	dueDate := "2026-06-15"
	estimate := 8.0
	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{
		ID:         "issue-1",
		Identifier: "LIN-1",
		Title:      "Planning and collaboration",
		State:      "Todo",
		DueDate:    &dueDate,
		Estimate:   &estimate,
		ProjectMilestone: &linearapi.ProjectMilestoneRef{
			ID:   "milestone-1",
			Name: "Beta",
		},
		Relations: []linearapi.IssueRelation{
			{
				ID:   "relation-1",
				Type: string(linearapi.IssueRelationBlocks),
				RelatedIssue: linearapi.IssueRef{
					ID:         "issue-2",
					Identifier: "LIN-2",
					Title:      "Dependency",
				},
			},
		},
		Subscribers: []linearapi.User{
			{ID: "user-1", DisplayName: "Ada Lovelace"},
		},
		Attachments: []linearapi.Attachment{
			{ID: "attachment-1", Title: "Pull request", SourceType: "github", URL: "https://github.com/example/pr/1"},
		},
	}
	app.issuesMu.Unlock()

	app.updateDetailsView()
	text := app.detailsDescriptionView.GetText(true)
	for _, want := range []string{
		"Due date:", "2026-06-15",
		"Estimate:", "8",
		"Milestone:", "Beta",
		"Relations:", "blocking", "LIN-2",
		"Subscribers:", "Ada Lovelace",
		"Attachments:", "Pull request", "https://github.com/example/pr/1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("details text missing %q:\n%s", want, text)
		}
	}
}

func TestRefreshIssues_AppliesRichFiltersWithNavigation(t *testing.T) {
	cfg := config.Config{
		PageSize: 1,
		CacheTTL: time.Minute,
	}
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
	app.queueUpdateDraw = func(f func()) { f() }
	refreshDone := installRefreshCompletionHook(app)

	estimate := 3.0
	app.richFilters = IssueFilters{
		AssigneeID: "user-1",
		LabelIDs:   []string{"label-1", "label-2"},
		StateID:    "state-1",
		CycleID:    "cycle-1",
		DueDate:    linearapi.DateFilter{Eq: "2026-06-15"},
		Estimate:   linearapi.NumberFilter{Eq: &estimate},
	}
	app.selectedNavigation = &NavigationNode{
		ID:        "project-1",
		Text:      "Project One",
		IsProject: true,
		TeamID:    "team-1",
	}

	called := make(chan linearapi.FetchIssuesParams, 1)
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		select {
		case called <- params:
		default:
		}
		return linearapi.IssuePage{Issues: nil, HasNext: false}, nil
	}

	app.refreshIssues()

	select {
	case params := <-called:
		if params.TeamID != "team-1" || params.ProjectID != "project-1" {
			t.Fatalf("navigation params = team %q project %q, want team-1 project-1", params.TeamID, params.ProjectID)
		}
		if params.Search != "" {
			t.Fatalf("Search = %q, want empty (main tabs never search)", params.Search)
		}
		if params.AssigneeID != "user-1" {
			t.Fatalf("AssigneeID = %q, want user-1", params.AssigneeID)
		}
		if strings.Join(params.LabelIDs, ",") != "label-1,label-2" {
			t.Fatalf("LabelIDs = %#v, want label-1,label-2", params.LabelIDs)
		}
		if params.StateID != "state-1" || params.CycleID != "cycle-1" {
			t.Fatalf("state/cycle = %q/%q, want state-1/cycle-1", params.StateID, params.CycleID)
		}
		if params.DueDate.Eq != "2026-06-15" {
			t.Fatalf("DueDate = %#v, want eq 2026-06-15", params.DueDate)
		}
		if params.Estimate.Eq == nil || *params.Estimate.Eq != 3 {
			t.Fatalf("Estimate = %#v, want eq 3", params.Estimate)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for fetchIssuesPage")
	}
	waitForRefreshCompletion(t, refreshDone)
}

func TestDefaultCommands_IncludesPlanningCommandsWithoutReactions(t *testing.T) {
	commands := DefaultCommands(nil)
	ids := make(map[string]bool, len(commands))
	for _, command := range commands {
		if strings.Contains(strings.ToLower(command.ID), "reaction") ||
			strings.Contains(strings.ToLower(command.Title), "reaction") {
			t.Fatalf("reaction command should not be present: %#v", command)
		}
		ids[command.ID] = true
	}

	for _, id := range []string{
		"set_due_date", "clear_due_date", "edit_estimate", "clear_estimate",
		"set_priority",
		"set_project", "clear_project",
		"list_project_milestones", "set_milestone", "clear_milestone",
		"filter_issues", "clear_filters", "filter_assignee", "filter_labels",
		"filter_status", "filter_project", "filter_cycle", "filter_due_date",
		"filter_estimate",
		"add_issue_relation", "remove_issue_relation", "subscribe_issue", "unsubscribe_issue",
		"open_attachment", "copy_attachment_url",
	} {
		if !ids[id] {
			t.Fatalf("command %q missing from DefaultCommands", id)
		}
	}
}

func TestSetPriorityPickerDispatchesSelectedPriority(t *testing.T) {
	for _, tc := range []struct {
		label string
		want  int
	}{
		{label: "Urgent", want: 1},
		{label: "No priority", want: 0},
	} {
		app := newUXTestApp(t)
		app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
			t.Error("a single-issue update refetched the whole list")
			return linearapi.IssuePage{}, nil
		}
		app.issuesMu.Lock()
		app.selectedIssue = &linearapi.Issue{ID: "issue-1", Identifier: "LIN-1", Title: "Current"}
		app.issuesMu.Unlock()

		called := make(chan linearapi.UpdateIssueInput, 1)
		app.updateIssueFunc = func(ctx context.Context, input linearapi.UpdateIssueInput) (linearapi.Issue, error) {
			called <- input
			return linearapi.Issue{ID: input.ID}, nil
		}

		app.showSetPriorityPicker()

		if len(app.pickerModal.items) != len(priorityLabels) {
			t.Fatalf("picker item count = %d, want %d", len(app.pickerModal.items), len(priorityLabels))
		}
		// The selection moves out from under an open picker on a background
		// refresh; the write must still land on the issue the picker named.
		app.issuesMu.Lock()
		app.selectedIssue = &linearapi.Issue{ID: "issue-2", Identifier: "LIN-2", Title: "Moved on"}
		app.issuesMu.Unlock()
		selectPickerItem(t, app, tc.label)

		select {
		case input := <-called:
			if input.Priority == nil {
				t.Fatal("Priority = nil, want a value")
			}
			if *input.Priority != tc.want {
				t.Fatalf("Priority = %d, want %d", *input.Priority, tc.want)
			}
			if input.ID != "issue-1" {
				t.Fatalf("issue ID = %q, want issue-1", input.ID)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for priority update")
		}
	}
}

func TestSetProjectPickerDispatchesProjectAndClearsMilestone(t *testing.T) {
	app := newUXTestApp(t)
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		t.Error("a single-issue update refetched the whole list")
		return linearapi.IssuePage{}, nil
	}
	app.teamProjects = []linearapi.Project{
		{ID: "project-1", Name: "Alpha"},
		{ID: "project-2", Name: "Beta"},
	}
	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{
		ID: "issue-1", Identifier: "LIN-1", Title: "Current",
		ProjectID:        "project-1",
		ProjectName:      "Alpha",
		ProjectMilestone: &linearapi.ProjectMilestoneRef{ID: "milestone-1", Name: "M1"},
	}
	app.issuesMu.Unlock()

	called := make(chan linearapi.UpdateIssueInput, 1)
	app.updateIssueFunc = func(ctx context.Context, input linearapi.UpdateIssueInput) (linearapi.Issue, error) {
		called <- input
		return linearapi.Issue{ID: input.ID}, nil
	}

	app.showSetProjectPicker()

	// The selection moves out from under an open picker on a background
	// refresh; the write must still land on the issue the picker named.
	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{ID: "issue-2", Identifier: "LIN-2", Title: "Moved on"}
	app.issuesMu.Unlock()
	selectPickerItem(t, app, "Beta")

	select {
	case input := <-called:
		if input.ID != "issue-1" {
			t.Fatalf("issue ID = %q, want issue-1", input.ID)
		}
		if input.ProjectID == nil || *input.ProjectID != "project-2" {
			t.Fatalf("ProjectID = %v, want project-2", input.ProjectID)
		}
		if input.ProjectMilestoneID == nil || *input.ProjectMilestoneID != "" {
			t.Fatalf("ProjectMilestoneID = %v, want cleared", input.ProjectMilestoneID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for project update")
	}
}

func TestClearProjectLeavesMilestoneAloneWhenAbsent(t *testing.T) {
	app := newUXTestApp(t)
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		t.Error("a single-issue update refetched the whole list")
		return linearapi.IssuePage{}, nil
	}
	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{
		ID: "issue-1", Identifier: "LIN-1", Title: "Current",
		ProjectID: "project-1", ProjectName: "Alpha",
	}
	app.issuesMu.Unlock()

	called := make(chan linearapi.UpdateIssueInput, 1)
	app.updateIssueFunc = func(ctx context.Context, input linearapi.UpdateIssueInput) (linearapi.Issue, error) {
		called <- input
		return linearapi.Issue{ID: input.ID}, nil
	}

	app.clearProjectForSelectedIssue()

	select {
	case input := <-called:
		if input.ProjectID == nil || *input.ProjectID != "" {
			t.Fatalf("ProjectID = %v, want cleared", input.ProjectID)
		}
		if input.ProjectMilestoneID != nil {
			t.Fatalf("ProjectMilestoneID = %v, want untouched", *input.ProjectMilestoneID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for project clear")
	}
}

func TestShowProjectFilterReportsMissingTeamContext(t *testing.T) {
	app := newUXTestApp(t)

	app.showProjectFilter()

	if text := app.statusBar.GetText(true); !strings.Contains(text, "team context is required") {
		t.Fatalf("status bar = %q, want the team-context error, not a silent no-op", text)
	}
	if app.pages.HasPage("picker") {
		t.Fatal("picker opened without a team to list projects for")
	}
}

func TestIssueRelationActionDispatchesExpectedAPIInput(t *testing.T) {
	app := NewApp(linearapi.ClientConfig{}, config.Config{PageSize: 1, CacheTTL: time.Minute}, nil)
	stopBackgroundWorkOnCleanup(t, app)
	app.queueUpdateDraw = func(f func()) { f() }
	// A relation changes nothing the list renders, so it must refetch the one
	// issue for the details pane instead of the whole list.
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		t.Error("relation change refetched the issue list")
		return linearapi.IssuePage{}, nil
	}
	detailFetches := make(chan string, 1)
	app.fetchIssueByID = func(ctx context.Context, issueID string) (linearapi.Issue, error) {
		detailFetches <- issueID
		return linearapi.Issue{ID: issueID, Identifier: "LIN-1"}, nil
	}
	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{ID: "issue-1", Identifier: "LIN-1", Title: "Current"}
	app.issuesMu.Unlock()

	called := make(chan linearapi.CreateIssueRelationInput, 1)
	app.createIssueRelationFunc = func(ctx context.Context, input linearapi.CreateIssueRelationInput) (linearapi.IssueRelation, error) {
		called <- input
		return linearapi.IssueRelation{ID: "relation-1"}, nil
	}

	app.createIssueRelationForSelectedIssue("blocked by", "issue-2")

	select {
	case input := <-called:
		if input.Type != linearapi.IssueRelationBlocks {
			t.Fatalf("Type = %q, want blocks", input.Type)
		}
		if input.IssueID != "issue-2" || input.RelatedIssueID != "issue-1" {
			t.Fatalf("relation input = %#v, want issue-2 blocks issue-1", input)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for relation API call")
	}

	select {
	case issueID := <-detailFetches:
		if issueID != "issue-1" {
			t.Fatalf("detail refetch issue = %q, want issue-1", issueID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for issue detail refetch")
	}
}

func TestAttachmentActionsUseInjectedOpenAndCopyFunctions(t *testing.T) {
	app := NewApp(linearapi.ClientConfig{}, config.Config{PageSize: 1, CacheTTL: time.Minute}, nil)
	stopBackgroundWorkOnCleanup(t, app)
	app.queueUpdateDraw = func(f func()) { f() }
	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{
		ID:         "issue-1",
		Identifier: "LIN-1",
		Attachments: []linearapi.Attachment{
			{ID: "attachment-1", Title: "Spec", URL: "https://example.com/spec"},
		},
	}
	app.issuesMu.Unlock()

	opened := make(chan string, 1)
	copied := make(chan string, 1)
	app.openURLFunc = func(url string) error {
		opened <- url
		return nil
	}
	app.copyToClipboardFunc = func(text string) error {
		copied <- text
		return nil
	}

	app.openSelectedAttachment()
	app.copySelectedAttachmentURL()

	select {
	case url := <-opened:
		if url != "https://example.com/spec" {
			t.Fatalf("opened URL = %q, want attachment URL", url)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for open URL call")
	}

	select {
	case text := <-copied:
		if text != "https://example.com/spec" {
			t.Fatalf("copied text = %q, want attachment URL", text)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for copy call")
	}
}
