package linearapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/shurcooL/graphql"
)

// TestBuildIssueCreateInputEncodesEveryField pins the wire keys. IssueCreateInput
// is a map scalar, so nothing in the type system checks these names: a typo
// only shows up as a Linear rejection at runtime.
func TestBuildIssueCreateInputEncodesEveryField(t *testing.T) {
	estimate := 5.0
	got, err := buildIssueCreateInput(CreateIssueInput{
		TeamID:             "team-1",
		Title:              "Title",
		Description:        "Body",
		ProjectID:          "project-1",
		ProjectMilestoneID: "milestone-1",
		StateID:            "state-1",
		CycleID:            "cycle-1",
		AssigneeID:         "user-1",
		Priority:           2,
		ParentID:           "parent-1",
		LabelIDs:           []string{"label-1", "label-2"},
		DueDate:            "2026-12-24",
		Estimate:           &estimate,
	})
	if err != nil {
		t.Fatalf("buildIssueCreateInput returned %v", err)
	}

	want := IssueCreateInput{
		"teamId":             graphql.ID("team-1"),
		"title":              graphql.String("Title"),
		"description":        graphql.String("Body"),
		"projectId":          graphql.ID("project-1"),
		"projectMilestoneId": graphql.ID("milestone-1"),
		"stateId":            graphql.ID("state-1"),
		"cycleId":            graphql.ID("cycle-1"),
		"assigneeId":         graphql.ID("user-1"),
		"priority":           graphql.Int(2),
		"parentId":           graphql.ID("parent-1"),
		"labelIds":           []graphql.ID{"label-1", "label-2"},
		"dueDate":            graphql.String("2026-12-24"),
		"estimate":           graphql.Float(5),
	}
	if !reflect.DeepEqual(map[string]interface{}(got), map[string]interface{}(want)) {
		t.Fatalf("input = %#v, want %#v", got, want)
	}
}

func TestBuildIssueCreateInputOmitsUnsetFields(t *testing.T) {
	got, err := buildIssueCreateInput(CreateIssueInput{TeamID: "team-1", Title: "Title"})
	if err != nil {
		t.Fatalf("buildIssueCreateInput returned %v", err)
	}

	want := IssueCreateInput{
		"teamId": graphql.ID("team-1"),
		"title":  graphql.String("Title"),
	}
	if !reflect.DeepEqual(map[string]interface{}(got), map[string]interface{}(want)) {
		t.Fatalf("input = %#v, want only teamId and title", got)
	}
}

func TestBuildIssueCreateInputRejectsAnOutOfRangePriority(t *testing.T) {
	if _, err := buildIssueCreateInput(CreateIssueInput{TeamID: "team-1", Title: "Title", Priority: 9}); err == nil {
		t.Fatal("buildIssueCreateInput accepted priority 9, want an error")
	}
}

// TestBuildIssueCreateInputKeepsAZeroEstimate covers the reason Estimate is a
// pointer: zero is a real estimate on teams that allow it.
func TestBuildIssueCreateInputKeepsAZeroEstimate(t *testing.T) {
	zero := 0.0
	got, err := buildIssueCreateInput(CreateIssueInput{TeamID: "team-1", Title: "Title", Estimate: &zero})
	if err != nil {
		t.Fatalf("buildIssueCreateInput returned %v", err)
	}
	if got["estimate"] != graphql.Float(0) {
		t.Fatalf("estimate = %#v, want graphql.Float(0)", got["estimate"])
	}
}

// TestUpdateIssue_SendsTeamID pins the wire key for a team move. The update
// input is a map scalar, so a typo here only surfaces as a Linear rejection.
func TestUpdateIssue_SendsTeamID(t *testing.T) {
	var inputs []map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}
		variables, ok := reqBody["variables"].(map[string]interface{})
		if !ok {
			t.Fatalf("Request body missing variables")
		}
		input, ok := variables["input"].(map[string]interface{})
		if !ok {
			t.Fatalf("variables.input = %#v, want object", variables["input"])
		}
		inputs = append(inputs, input)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mutationIssueResponse("issueUpdate")))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		Token:    "test-token",
		Endpoint: server.URL,
	})

	title := "Untouched team"
	empty := ""
	teamID := "team-2"
	for _, tc := range []struct {
		name  string
		input UpdateIssueInput
	}{
		{"move", UpdateIssueInput{ID: "issue-1", TeamID: &teamID}},
		{"no change", UpdateIssueInput{ID: "issue-1", Title: &title}},
		{"empty", UpdateIssueInput{ID: "issue-1", Title: &title, TeamID: &empty}},
	} {
		if _, err := client.UpdateIssue(context.Background(), tc.input); err != nil {
			t.Fatalf("UpdateIssue(%s) error: %v", tc.name, err)
		}
	}

	if len(inputs) != 3 {
		t.Fatalf("inputs length = %d, want 3", len(inputs))
	}
	if inputs[0]["teamId"] != "team-2" {
		t.Fatalf("teamId = %#v, want team-2", inputs[0]["teamId"])
	}
	// An issue has no "no team", so neither a nil nor an empty TeamID may
	// reach the wire: a null or an empty id fails the whole update, taking
	// the other fields in the same input with it.
	for _, i := range []int{1, 2} {
		if value, present := inputs[i]["teamId"]; present {
			t.Fatalf("inputs[%d] teamId = %#v, want it absent", i, value)
		}
	}
}
