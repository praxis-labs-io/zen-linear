package linearapi

import (
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
