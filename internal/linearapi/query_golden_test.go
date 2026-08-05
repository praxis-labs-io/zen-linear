package linearapi

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGoldens = flag.Bool("update", false, "rewrite the golden query files under testdata/")

// captureQuery runs invoke against a server that records the GraphQL request
// body and answers with an empty data object. Empty data leaves every field
// zero, so a paginating caller stops after one request. The error invoke
// returns is ignored on purpose: the request is the assertion here, and every
// case fails its response check by design.
func captureQuery(t *testing.T, invoke func(context.Context, *Client)) string {
	t.Helper()

	var captured string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
			return
		}
		var request struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("decoding request body: %v", err)
			return
		}
		if captured == "" {
			captured = request.Query
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer server.Close()

	invoke(context.Background(), NewClient(ClientConfig{Endpoint: server.URL}))
	if captured == "" {
		t.Fatal("no request reached the server")
	}
	return captured
}

// TestQueryGoldens pins the selection set every call site sends. Linear rejects
// a whole query over one misplaced field, and the canned-JSON tests elsewhere in
// this package never see the request, so this is the only check that a
// refactor of the query structs left the wire format alone.
func TestQueryGoldens(t *testing.T) {
	title := "Golden"
	listParams := FetchIssuesParams{TeamID: "team-1", First: 25, OrderBy: "updatedAt"}
	searchParams := FetchIssuesParams{TeamID: "team-1", Search: "term", First: 25}
	viewParams := FetchIssuesParams{CustomViewID: "view-1", First: 25, OrderBy: "updatedAt"}

	cases := []struct {
		name   string
		invoke func(context.Context, *Client)
	}{
		{"list_teams", func(ctx context.Context, c *Client) { _, _ = c.ListTeams(ctx) }},
		{"create_comment", func(ctx context.Context, c *Client) {
			_, _ = c.CreateComment(ctx, CreateCommentInput{IssueID: "issue-1", Body: "body"})
		}},
		{"list_favorites", func(ctx context.Context, c *Client) { _, _ = c.ListFavorites(ctx) }},
		{"create_favorite", func(ctx context.Context, c *Client) {
			_, _ = c.CreateFavorite(ctx, FavoriteTarget{ProjectID: "project-1"})
		}},
		{"delete_favorite", func(ctx context.Context, c *Client) { _ = c.DeleteFavorite(ctx, "favorite-1") }},
		{"update_favorite", func(ctx context.Context, c *Client) {
			_ = c.UpdateFavoriteSortOrder(ctx, "favorite-1", 1)
		}},
		{"list_projects", func(ctx context.Context, c *Client) { _, _ = c.ListProjects(ctx, "team-1") }},
		{"list_project_milestones", func(ctx context.Context, c *Client) {
			_, _ = c.ListProjectMilestones(ctx, "project-1")
		}},
		{"list_cycles", func(ctx context.Context, c *Client) { _, _ = c.ListCycles(ctx, "team-1") }},
		{"list_users", func(ctx context.Context, c *Client) { _, _ = c.ListUsers(ctx, "team-1") }},
		{"get_current_user", func(ctx context.Context, c *Client) { _, _ = c.GetCurrentUser(ctx) }},
		{"list_workflow_states", func(ctx context.Context, c *Client) { _, _ = c.ListWorkflowStates(ctx, "team-1") }},
		{"list_workspace_labels", func(ctx context.Context, c *Client) { _, _ = c.ListWorkspaceLabels(ctx) }},
		{"list_team_labels", func(ctx context.Context, c *Client) { _, _ = c.ListTeamLabels(ctx, "team-1") }},
		{"fetch_issue_by_id", func(ctx context.Context, c *Client) { _, _ = c.FetchIssueByID(ctx, "issue-1") }},
		{"search_issues_page", func(ctx context.Context, c *Client) { _, _ = c.searchIssuesPage(ctx, searchParams, nil) }},
		{"fetch_custom_view_preferences", func(ctx context.Context, c *Client) {
			_, _ = c.FetchCustomViewPreferences(ctx, "view-1")
		}},
		{"issue_matches_scope", func(ctx context.Context, c *Client) {
			_, _ = c.IssueMatchesScope(ctx, viewParams, "issue-1")
		}},
		{"custom_view_issues_page", func(ctx context.Context, c *Client) {
			_, _ = c.customViewIssuesPage(ctx, viewParams, nil)
		}},
		{"fetch_issues_with_filter_page", func(ctx context.Context, c *Client) {
			_, _ = c.fetchIssuesWithFilterPage(ctx, listParams, nil)
		}},
		{"create_issue", func(ctx context.Context, c *Client) {
			_, _ = c.CreateIssue(ctx, CreateIssueInput{TeamID: "team-1", Title: title})
		}},
		{"update_issue", func(ctx context.Context, c *Client) {
			_, _ = c.UpdateIssue(ctx, UpdateIssueInput{ID: "issue-1", Title: &title})
		}},
		{"create_issue_relation", func(ctx context.Context, c *Client) {
			_, _ = c.CreateIssueRelation(ctx, CreateIssueRelationInput{
				IssueID:        "issue-1",
				RelatedIssueID: "issue-2",
				Type:           IssueRelationBlocks,
			})
		}},
		{"delete_issue_relation", func(ctx context.Context, c *Client) { _ = c.DeleteIssueRelation(ctx, "relation-1") }},
		{"subscribe_to_issue", func(ctx context.Context, c *Client) { _, _ = c.SubscribeToIssue(ctx, "issue-1") }},
		{"unsubscribe_from_issue", func(ctx context.Context, c *Client) { _, _ = c.UnsubscribeFromIssue(ctx, "issue-1") }},
		{"archive_issue", func(ctx context.Context, c *Client) { _ = c.ArchiveIssue(ctx, "issue-1") }},
		{"unarchive_issue", func(ctx context.Context, c *Client) { _ = c.UnarchiveIssue(ctx, "issue-1") }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := captureQuery(t, tc.invoke)
			path := filepath.Join("testdata", tc.name+".graphql")

			if *updateGoldens {
				if err := os.WriteFile(path, []byte(got+"\n"), 0o644); err != nil {
					t.Fatalf("writing %s: %v", path, err)
				}
				return
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %s (regenerate with: go test ./internal/linearapi -run TestQueryGoldens -update): %v", path, err)
			}
			if got != strings.TrimSuffix(string(want), "\n") {
				t.Errorf("query drifted\n got: %s\nwant: %s", got, want)
			}
		})
	}
}
