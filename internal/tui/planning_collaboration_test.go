package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/roeyazroel/linear-tui/internal/config"
	"github.com/roeyazroel/linear-tui/internal/linearapi"
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

	row := renderIssueRow(issue)
	if len(row) != 9 {
		t.Fatalf("renderIssueRow() length = %d, want 9: %#v", len(row), row)
	}
	if row[5] != dueDate {
		t.Fatalf("due date column = %q, want %q", row[5], dueDate)
	}
	if row[6] != "5" {
		t.Fatalf("estimate column = %q, want 5", row[6])
	}
	if row[7] != "Beta" {
		t.Fatalf("milestone column = %q, want Beta", row[7])
	}
	if row[8] != issue.Title {
		t.Fatalf("title column = %q, want %q", row[8], issue.Title)
	}
}

func TestUpdateDetailsView_IncludesPlanningAndCollaboration(t *testing.T) {
	cfg := config.Config{
		PageSize: 1,
		CacheTTL: time.Minute,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
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

func TestRefreshIssues_AppliesRichFiltersWithNavigationAndSearch(t *testing.T) {
	cfg := config.Config{
		PageSize: 1,
		CacheTTL: time.Minute,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	refreshDone := installRefreshCompletionHook(app)

	estimate := 3.0
	app.searchQuery = "database"
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
		if params.Search != "database" {
			t.Fatalf("Search = %q, want database", params.Search)
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
		"list_project_milestones", "set_milestone", "clear_milestone",
		"filter_issues", "clear_filters", "filter_assignee", "filter_labels",
		"filter_status", "filter_project", "filter_cycle", "filter_due_date",
		"filter_estimate", "filter_text",
		"add_issue_relation", "remove_issue_relation", "subscribe_issue", "unsubscribe_issue",
		"open_attachment", "copy_attachment_url",
	} {
		if !ids[id] {
			t.Fatalf("command %q missing from DefaultCommands", id)
		}
	}
}

func TestIssueRelationActionDispatchesExpectedAPIInput(t *testing.T) {
	app := NewApp(&linearapi.Client{}, config.Config{PageSize: 1, CacheTTL: time.Minute}, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	refreshDone := installRefreshCompletionHook(app)
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		return linearapi.IssuePage{Issues: nil, HasNext: false}, nil
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
	waitForRefreshCompletion(t, refreshDone)
}

func TestAttachmentActionsUseInjectedOpenAndCopyFunctions(t *testing.T) {
	app := NewApp(&linearapi.Client{}, config.Config{PageSize: 1, CacheTTL: time.Minute}, nil)
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
