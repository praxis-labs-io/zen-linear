package tui

import (
	"context"
	"testing"
	"time"

	"github.com/roeyazroel/linear-tui/internal/config"
	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

func TestMapViewGrouping(t *testing.T) {
	cases := map[string]struct {
		want string
		ok   bool
	}{
		"workflowState":    {GroupByStatus, true},
		"status":           {GroupByStatus, true},
		"priority":         {GroupByPriority, true},
		"assignee":         {GroupByAssignee, true},
		"cycle":            {GroupByCycle, true},
		"project":          {GroupByProject, true},
		"projectMilestone": {GroupByMilestone, true},
		"milestone":        {GroupByMilestone, true},
		"none":             {GroupByNone, true},
		"noGrouping":       {GroupByNone, true},
		"label":            {"", false},
		"":                 {"", false},
	}
	for value, want := range cases {
		got, ok := mapViewGrouping(value)
		if got != want.want || ok != want.ok {
			t.Errorf("mapViewGrouping(%q) = %q, %v; want %q, %v", value, got, ok, want.want, want.ok)
		}
	}
}

func TestMapViewOrdering(t *testing.T) {
	cases := map[string]struct {
		want SortField
		ok   bool
	}{
		"priority":    {SortByPriority, true},
		"updatedAt":   {SortByUpdatedAt, true},
		"lastUpdated": {SortByUpdatedAt, true},
		"createdAt":   {SortByCreatedAt, true},
		"manual":      {"", false},
		"dueDate":     {"", false},
		"":            {"", false},
	}
	for value, want := range cases {
		got, ok := mapViewOrdering(value)
		if got != want.want || ok != want.ok {
			t.Errorf("mapViewOrdering(%q) = %q, %v; want %q, %v", value, got, ok, want.want, want.ok)
		}
	}
}

func TestResolveViewPrefs(t *testing.T) {
	if got := resolveViewPrefs(nil); got != nil {
		t.Fatalf("resolveViewPrefs(nil) = %+v, want nil", got)
	}
	if got := resolveViewPrefs(&linearapi.ViewPreferencesValues{IssueGrouping: "label", ViewOrdering: "manual"}); got != nil {
		t.Fatalf("resolveViewPrefs(unmappable) = %+v, want nil", got)
	}

	prefs := resolveViewPrefs(&linearapi.ViewPreferencesValues{
		IssueGrouping:    "workflowState",
		IssueSubGrouping: "priority",
		ViewOrdering:     "priority",
	})
	if prefs == nil || !prefs.hasGrouping || prefs.groupBy != GroupByStatus || prefs.subgroupBy != GroupByPriority {
		t.Fatalf("grouping prefs = %+v, want status/priority", prefs)
	}
	if !prefs.hasSort || prefs.sortField != SortByPriority {
		t.Fatalf("sort prefs = %+v, want priority", prefs)
	}

	// A subgrouping equal to the grouping collapses to none, and an
	// unmappable subgrouping does not inherit the config fallback.
	prefs = resolveViewPrefs(&linearapi.ViewPreferencesValues{IssueGrouping: "priority", IssueSubGrouping: "priority"})
	if prefs == nil || prefs.subgroupBy != GroupByNone {
		t.Fatalf("same-dimension subgroup prefs = %+v, want subgroup none", prefs)
	}
}

func TestRefreshAppliesCustomViewPreferences(t *testing.T) {
	cfg := config.Config{
		PageSize: 10,
		CacheTTL: time.Minute,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	refreshDone := installRefreshCompletionHook(app)

	app.selectedNavigation = &NavigationNode{ID: "view-1", Text: "My View", CustomViewID: "view-1"}
	app.fetchViewPrefsFunc = func(ctx context.Context, viewID string) (*linearapi.ViewPreferencesValues, error) {
		if viewID != "view-1" {
			t.Errorf("fetchViewPrefsFunc viewID = %q, want view-1", viewID)
		}
		return &linearapi.ViewPreferencesValues{
			IssueGrouping:    "workflowState",
			IssueSubGrouping: "priority",
			ViewOrdering:     "priority",
		}, nil
	}

	called := make(chan linearapi.FetchIssuesParams, 1)
	issues := []linearapi.Issue{
		{ID: "issue-1", Identifier: "ABC-1", Title: "First", State: "Todo", Priority: 1},
		{ID: "issue-2", Identifier: "ABC-2", Title: "Second", State: "Done", Priority: 3},
	}
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		select {
		case called <- params:
		default:
		}
		return linearapi.IssuePage{Issues: issues}, nil
	}
	app.fetchIssueByID = func(ctx context.Context, id string) (linearapi.Issue, error) {
		return issues[0], nil
	}

	app.refreshIssues()

	select {
	case params := <-called:
		if params.CustomViewID != "view-1" {
			t.Fatalf("CustomViewID = %q, want view-1", params.CustomViewID)
		}
		if params.OrderBy != "priority" {
			t.Fatalf("OrderBy = %q, want the view's priority ordering", params.OrderBy)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for fetchIssuesPage")
	}
	waitForRefreshCompletion(t, refreshDone)

	if app.effectiveGroupBy() != GroupByStatus || app.effectiveSubgroupBy() != GroupByPriority {
		t.Fatalf("effective grouping = %q/%q, want status/priority", app.effectiveGroupBy(), app.effectiveSubgroupBy())
	}
	if chain := app.effectiveSortFields(); len(chain) != 1 || chain[0] != SortByPriority {
		t.Fatalf("effective sort chain = %v, want priority alone", app.effectiveSortFields())
	}
	if len(app.issueRows) == 0 || !app.issueRows[0].IsHeader {
		t.Fatalf("issueRows[0] = %+v, want a group header from the view's grouping", app.issueRows)
	}

	// A manual grouping override outranks the view for the session.
	app.groupingOverridden = true
	if app.effectiveGroupBy() != cfg.GroupBy {
		t.Fatalf("overridden grouping = %q, want config value %q", app.effectiveGroupBy(), cfg.GroupBy)
	}
	app.groupingOverridden = false

	// Leaving the view clears its settings with the next list.
	app.selectedNavigation = &NavigationNode{ID: "team-1", TeamID: "team-1", IsTeam: true}
	app.refreshIssues()
	waitForRefreshCompletion(t, refreshDone)
	if app.viewPrefs != nil {
		t.Fatalf("viewPrefs = %+v after leaving the view, want nil", app.viewPrefs)
	}
	if app.effectiveGroupBy() != cfg.GroupBy {
		t.Fatalf("effective grouping = %q after leaving the view, want config", app.effectiveGroupBy())
	}
}

func TestRefreshFallsBackWhenViewPreferencesFail(t *testing.T) {
	cfg := config.Config{
		PageSize: 10,
		CacheTTL: time.Minute,
		GroupBy:  GroupByAssignee,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	refreshDone := installRefreshCompletionHook(app)

	app.selectedNavigation = &NavigationNode{ID: "view-1", Text: "My View", CustomViewID: "view-1"}
	app.fetchViewPrefsFunc = func(ctx context.Context, viewID string) (*linearapi.ViewPreferencesValues, error) {
		return nil, context.DeadlineExceeded
	}
	fallbackIssue := linearapi.Issue{ID: "issue-1", Identifier: "ABC-1", Title: "First", State: "Todo"}
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		return linearapi.IssuePage{Issues: []linearapi.Issue{fallbackIssue}}, nil
	}
	app.fetchIssueByID = func(ctx context.Context, id string) (linearapi.Issue, error) {
		return fallbackIssue, nil
	}

	app.refreshIssues()
	waitForRefreshCompletion(t, refreshDone)

	if app.viewPrefs != nil {
		t.Fatalf("viewPrefs = %+v after a failed prefs fetch, want nil", app.viewPrefs)
	}
	if app.effectiveGroupBy() != GroupByAssignee {
		t.Fatalf("effective grouping = %q, want the configured assignee fallback", app.effectiveGroupBy())
	}
}
