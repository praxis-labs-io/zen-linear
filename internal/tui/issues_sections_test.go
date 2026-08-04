package tui

import (
	"testing"

	"github.com/zen-linear/zen-linear/internal/linearapi"
)

func TestMyIssues(t *testing.T) {
	currentUserID := "user-123"

	tests := []struct {
		name          string
		issues        []linearapi.Issue
		currentUserID string
		wantIDs       []string
	}{
		{
			name: "no current user - My is empty",
			issues: []linearapi.Issue{
				{ID: "1", AssigneeID: "user-123"},
				{ID: "2", AssigneeID: "user-456"},
			},
			currentUserID: "",
			wantIDs:       nil,
		},
		{
			name: "mixed assignment - only mine",
			issues: []linearapi.Issue{
				{ID: "1", AssigneeID: currentUserID},
				{ID: "2", AssigneeID: "user-456"},
				{ID: "3", AssigneeID: currentUserID},
				{ID: "4", AssigneeID: ""},
			},
			currentUserID: currentUserID,
			wantIDs:       []string{"1", "3"},
		},
		{
			name: "unassigned issues stay out",
			issues: []linearapi.Issue{
				{ID: "1", AssigneeID: ""},
				{ID: "2", AssigneeID: ""},
				{ID: "3", AssigneeID: currentUserID},
			},
			currentUserID: currentUserID,
			wantIDs:       []string{"3"},
		},
		{
			name:          "empty issues list",
			issues:        []linearapi.Issue{},
			currentUserID: currentUserID,
			wantIDs:       nil,
		},
		{
			name: "my parent brings its children",
			issues: []linearapi.Issue{
				{ID: "parent-1", AssigneeID: currentUserID},
				{ID: "child-1", AssigneeID: "", Parent: &linearapi.IssueRef{ID: "parent-1"}},
				{ID: "child-2", AssigneeID: "user-456", Parent: &linearapi.IssueRef{ID: "parent-1"}},
			},
			currentUserID: currentUserID,
			wantIDs:       []string{"parent-1", "child-1", "child-2"},
		},
		{
			name: "descent reaches grandchildren",
			issues: []linearapi.Issue{
				{ID: "parent-3", AssigneeID: currentUserID},
				{ID: "child-5", AssigneeID: "", Parent: &linearapi.IssueRef{ID: "parent-3"}},
				{ID: "grandchild-1", AssigneeID: "", Parent: &linearapi.IssueRef{ID: "child-5"}},
			},
			currentUserID: currentUserID,
			wantIDs:       []string{"parent-3", "child-5", "grandchild-1"},
		},
		{
			// ZNL-26: the parent's owner used to win, dropping the child out of My.
			name: "my child stays when its parent belongs to someone else",
			issues: []linearapi.Issue{
				{ID: "parent-2", AssigneeID: "user-456"},
				{ID: "child-3", AssigneeID: currentUserID, Parent: &linearapi.IssueRef{ID: "parent-2"}},
			},
			currentUserID: currentUserID,
			wantIDs:       []string{"child-3"},
		},
		{
			name: "a child of mine is not dragged out by an unassigned parent",
			issues: []linearapi.Issue{
				{ID: "parent-4", AssigneeID: ""},
				{ID: "child-6", AssigneeID: currentUserID, Parent: &linearapi.IssueRef{ID: "parent-4"}},
			},
			currentUserID: currentUserID,
			wantIDs:       []string{"child-6"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := myIssues(tt.issues, tt.currentUserID)

			gotIDs := make([]string, 0, len(got))
			for _, issue := range got {
				gotIDs = append(gotIDs, issue.ID)
			}
			if len(gotIDs) != len(tt.wantIDs) {
				t.Fatalf("myIssues() = %v, want %v", gotIDs, tt.wantIDs)
			}
			for i, id := range tt.wantIDs {
				if gotIDs[i] != id {
					t.Fatalf("myIssues() = %v, want %v", gotIDs, tt.wantIDs)
				}
			}
		})
	}
}

// TestMyIssues_ChildKeepsItsPlaceWhenTheParentPaginatesIn is ZNL-26 as the user
// hit it: the child arrived on an early page and showed in My, then a later
// page brought its parent in and the old split moved it out from under the
// cursor.
func TestMyIssues_ChildKeepsItsPlaceWhenTheParentPaginatesIn(t *testing.T) {
	currentUserID := "user-123"
	child := linearapi.Issue{ID: "child", AssigneeID: currentUserID, Parent: &linearapi.IssueRef{ID: "parent"}}
	parent := linearapi.Issue{ID: "parent", AssigneeID: "user-456"}

	firstPage := myIssues([]linearapi.Issue{child}, currentUserID)
	if len(firstPage) != 1 || firstPage[0].ID != "child" {
		t.Fatalf("first page: My = %v, want the child", firstPage)
	}

	secondPage := myIssues([]linearapi.Issue{child, parent}, currentUserID)
	if len(secondPage) != 1 || secondPage[0].ID != "child" {
		t.Fatalf("after the parent loads: My = %v, want the child to stay", secondPage)
	}
}
