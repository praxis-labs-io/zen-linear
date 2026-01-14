package tui

import (
	"testing"

	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

func TestSplitIssuesByAssignee(t *testing.T) {
	currentUserID := "user-123"

	tests := []struct {
		name         string
		issues       []linearapi.Issue
		currentUserID string
		wantMyCount  int
		wantOtherCount int
	}{
		{
			name: "no current user - all issues go to other",
			issues: []linearapi.Issue{
				{ID: "1", AssigneeID: "user-123"},
				{ID: "2", AssigneeID: "user-456"},
			},
			currentUserID: "",
			wantMyCount:   0,
			wantOtherCount: 2,
		},
		{
			name: "mixed assignment - correct partition",
			issues: []linearapi.Issue{
				{ID: "1", AssigneeID: "user-123"},
				{ID: "2", AssigneeID: "user-456"},
				{ID: "3", AssigneeID: "user-123"},
				{ID: "4", AssigneeID: ""},
			},
			currentUserID: currentUserID,
			wantMyCount:   2,
			wantOtherCount: 2,
		},
		{
			name: "unassigned issues go to other",
			issues: []linearapi.Issue{
				{ID: "1", AssigneeID: ""},
				{ID: "2", AssigneeID: ""},
				{ID: "3", AssigneeID: currentUserID},
			},
			currentUserID: currentUserID,
			wantMyCount:   1,
			wantOtherCount: 2,
		},
		{
			name: "all my issues",
			issues: []linearapi.Issue{
				{ID: "1", AssigneeID: currentUserID},
				{ID: "2", AssigneeID: currentUserID},
			},
			currentUserID: currentUserID,
			wantMyCount:   2,
			wantOtherCount: 0,
		},
		{
			name: "all other issues",
			issues: []linearapi.Issue{
				{ID: "1", AssigneeID: "user-456"},
				{ID: "2", AssigneeID: "user-789"},
			},
			currentUserID: currentUserID,
			wantMyCount:   0,
			wantOtherCount: 2,
		},
		{
			name: "empty issues list",
			issues: []linearapi.Issue{},
			currentUserID: currentUserID,
			wantMyCount:   0,
			wantOtherCount: 0,
		},
		{
			name: "children follow parent section - parent in my, children unassigned",
			issues: []linearapi.Issue{
				{ID: "parent-1", AssigneeID: currentUserID},
				{ID: "child-1", AssigneeID: "", Parent: &linearapi.IssueRef{ID: "parent-1"}},
				{ID: "child-2", AssigneeID: "", Parent: &linearapi.IssueRef{ID: "parent-1"}},
			},
			currentUserID: currentUserID,
			wantMyCount:   3, // Parent + 2 children
			wantOtherCount: 0,
		},
		{
			name: "children follow parent section - parent unassigned, children assigned to me",
			issues: []linearapi.Issue{
				{ID: "parent-2", AssigneeID: ""},
				{ID: "child-3", AssigneeID: currentUserID, Parent: &linearapi.IssueRef{ID: "parent-2"}},
				{ID: "child-4", AssigneeID: currentUserID, Parent: &linearapi.IssueRef{ID: "parent-2"}},
			},
			currentUserID: currentUserID,
			wantMyCount:   0,
			wantOtherCount: 3, // Parent + 2 children
		},
		{
			name: "nested children follow parent section",
			issues: []linearapi.Issue{
				{ID: "parent-3", AssigneeID: currentUserID},
				{ID: "child-5", AssigneeID: "", Parent: &linearapi.IssueRef{ID: "parent-3"}},
				{ID: "grandchild-1", AssigneeID: "", Parent: &linearapi.IssueRef{ID: "child-5"}},
			},
			currentUserID: currentUserID,
			wantMyCount:   3, // Parent + child + grandchild
			wantOtherCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			my, other := splitIssuesByAssignee(tt.issues, tt.currentUserID)

			if len(my) != tt.wantMyCount {
				t.Errorf("splitIssuesByAssignee() my count = %d, want %d", len(my), tt.wantMyCount)
			}

			if len(other) != tt.wantOtherCount {
				t.Errorf("splitIssuesByAssignee() other count = %d, want %d", len(other), tt.wantOtherCount)
			}

			// Build a map of issue IDs to their section (true = my, false = other)
			myIDs := make(map[string]bool)
			for _, issue := range my {
				myIDs[issue.ID] = true
			}
			otherIDs := make(map[string]bool)
			for _, issue := range other {
				otherIDs[issue.ID] = true
			}

			// Verify correctness: top-level my issues have correct assignee
			// Children may have different assignees but should follow parent
			for _, issue := range my {
				if issue.Parent == nil {
					// Top-level issue must have correct assignee
					if issue.AssigneeID != tt.currentUserID {
						t.Errorf("splitIssuesByAssignee() my top-level issue %s has AssigneeID %s, want %s", issue.ID, issue.AssigneeID, tt.currentUserID)
					}
				} else {
					// Child issue - verify parent is also in "my" section
					if !myIDs[issue.Parent.ID] {
						t.Errorf("splitIssuesByAssignee() my child issue %s has parent %s not in my section", issue.ID, issue.Parent.ID)
					}
				}
			}

			// Verify correctness: top-level other issues don't have current user as assignee
			// Children may have current user as assignee but should follow parent
			for _, issue := range other {
				if issue.Parent == nil {
					// Top-level issue must not have current user as assignee
					if issue.AssigneeID == tt.currentUserID {
						t.Errorf("splitIssuesByAssignee() other top-level issue %s has AssigneeID %s, should not match current user", issue.ID, issue.AssigneeID)
					}
				} else {
					// Child issue - verify parent is also in "other" section
					if !otherIDs[issue.Parent.ID] {
						t.Errorf("splitIssuesByAssignee() other child issue %s has parent %s not in other section", issue.ID, issue.Parent.ID)
					}
				}
			}

			// Verify all issues are accounted for
			if len(my)+len(other) != len(tt.issues) {
				t.Errorf("splitIssuesByAssignee() total issues = %d, want %d", len(my)+len(other), len(tt.issues))
			}
		})
	}
}
