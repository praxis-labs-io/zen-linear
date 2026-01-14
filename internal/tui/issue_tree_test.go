package tui

import (
	"testing"

	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

func TestBuildIssueRows_NoChildren(t *testing.T) {
	issues := []linearapi.Issue{
		{ID: "1", Identifier: "LIN-1", Title: "Issue 1"},
		{ID: "2", Identifier: "LIN-2", Title: "Issue 2"},
		{ID: "3", Identifier: "LIN-3", Title: "Issue 3"},
	}
	expanded := make(map[string]bool)

	rows, idToIssue := BuildIssueRows(issues, expanded)

	// Should have 3 rows, all at level 0
	if len(rows) != 3 {
		t.Errorf("BuildIssueRows() returned %d rows, want 3", len(rows))
	}

	for i, row := range rows {
		if row.Level != 0 {
			t.Errorf("Row %d level = %d, want 0", i, row.Level)
		}
		if row.HasChildren {
			t.Errorf("Row %d HasChildren = true, want false", i)
		}
		if row.IsExpanded {
			t.Errorf("Row %d IsExpanded = true, want false", i)
		}
	}

	// Check idToIssue map
	if len(idToIssue) != 3 {
		t.Errorf("idToIssue has %d entries, want 3", len(idToIssue))
	}
	if idToIssue["1"] == nil || idToIssue["1"].Identifier != "LIN-1" {
		t.Errorf("idToIssue[1] = %v, want LIN-1", idToIssue["1"])
	}
}

func TestBuildIssueRows_ParentWithChildren(t *testing.T) {
	parent := linearapi.Issue{
		ID:         "parent-1",
		Identifier: "LIN-1",
		Title:      "Parent Issue",
		Children: []linearapi.IssueChildRef{
			{ID: "child-1", Identifier: "LIN-2", Title: "Child 1", State: "Todo"},
			{ID: "child-2", Identifier: "LIN-3", Title: "Child 2", State: "Done"},
		},
	}
	child1 := linearapi.Issue{
		ID:         "child-1",
		Identifier: "LIN-2",
		Title:      "Child 1",
		Parent:     &linearapi.IssueRef{ID: "parent-1", Identifier: "LIN-1", Title: "Parent Issue"},
	}
	child2 := linearapi.Issue{
		ID:         "child-2",
		Identifier: "LIN-3",
		Title:      "Child 2",
		Parent:     &linearapi.IssueRef{ID: "parent-1", Identifier: "LIN-1", Title: "Parent Issue"},
	}

	issues := []linearapi.Issue{parent, child1, child2}
	expanded := make(map[string]bool)

	// Test collapsed state
	rows, _ := BuildIssueRows(issues, expanded)

	// Should only show parent (collapsed)
	if len(rows) != 1 {
		t.Errorf("BuildIssueRows() collapsed returned %d rows, want 1", len(rows))
	}
	if rows[0].IssueID != "parent-1" {
		t.Errorf("Row 0 IssueID = %q, want parent-1", rows[0].IssueID)
	}
	if !rows[0].HasChildren {
		t.Error("Parent row HasChildren = false, want true")
	}
	if rows[0].IsExpanded {
		t.Error("Parent row IsExpanded = true, want false (collapsed)")
	}
}

func TestBuildIssueRows_ExpandedParent(t *testing.T) {
	parent := linearapi.Issue{
		ID:         "parent-1",
		Identifier: "LIN-1",
		Title:      "Parent Issue",
		Children: []linearapi.IssueChildRef{
			{ID: "child-1", Identifier: "LIN-2", Title: "Child 1", State: "Todo"},
			{ID: "child-2", Identifier: "LIN-3", Title: "Child 2", State: "Done"},
		},
	}
	child1 := linearapi.Issue{
		ID:         "child-1",
		Identifier: "LIN-2",
		Title:      "Child 1",
		Parent:     &linearapi.IssueRef{ID: "parent-1", Identifier: "LIN-1", Title: "Parent Issue"},
	}
	child2 := linearapi.Issue{
		ID:         "child-2",
		Identifier: "LIN-3",
		Title:      "Child 2",
		Parent:     &linearapi.IssueRef{ID: "parent-1", Identifier: "LIN-1", Title: "Parent Issue"},
	}

	issues := []linearapi.Issue{parent, child1, child2}
	expanded := map[string]bool{"parent-1": true}

	rows, _ := BuildIssueRows(issues, expanded)

	// Should show parent + 2 children when expanded
	if len(rows) != 3 {
		t.Errorf("BuildIssueRows() expanded returned %d rows, want 3", len(rows))
	}

	// First row should be parent
	if rows[0].IssueID != "parent-1" {
		t.Errorf("Row 0 IssueID = %q, want parent-1", rows[0].IssueID)
	}
	if rows[0].Level != 0 {
		t.Errorf("Row 0 Level = %d, want 0", rows[0].Level)
	}
	if !rows[0].IsExpanded {
		t.Error("Parent row IsExpanded = false, want true")
	}

	// Children should be at level 1
	for i := 1; i < len(rows); i++ {
		if rows[i].Level != 1 {
			t.Errorf("Row %d Level = %d, want 1", i, rows[i].Level)
		}
	}
}

func TestBuildIssueRows_OrphanSubIssue(t *testing.T) {
	// Sub-issue whose parent is not in the fetched list
	orphan := linearapi.Issue{
		ID:         "orphan-1",
		Identifier: "LIN-2",
		Title:      "Orphan Issue",
		Parent:     &linearapi.IssueRef{ID: "missing-parent", Identifier: "LIN-1", Title: "Missing Parent"},
	}

	issues := []linearapi.Issue{orphan}
	expanded := make(map[string]bool)

	rows, _ := BuildIssueRows(issues, expanded)

	// Orphan should appear as top-level
	if len(rows) != 1 {
		t.Errorf("BuildIssueRows() returned %d rows, want 1", len(rows))
	}
	if rows[0].Level != 0 {
		t.Errorf("Orphan row Level = %d, want 0 (treated as top-level)", rows[0].Level)
	}
}

func TestBuildIssueRows_MixedIssues(t *testing.T) {
	// Mix of parent issues, sub-issues, and standalone issues
	standalone := linearapi.Issue{
		ID:         "standalone",
		Identifier: "LIN-1",
		Title:      "Standalone Issue",
	}
	parent := linearapi.Issue{
		ID:         "parent",
		Identifier: "LIN-2",
		Title:      "Parent Issue",
		Children: []linearapi.IssueChildRef{
			{ID: "child", Identifier: "LIN-3", Title: "Child", State: "Todo"},
		},
	}
	child := linearapi.Issue{
		ID:         "child",
		Identifier: "LIN-3",
		Title:      "Child Issue",
		Parent:     &linearapi.IssueRef{ID: "parent", Identifier: "LIN-2", Title: "Parent Issue"},
	}

	issues := []linearapi.Issue{standalone, parent, child}
	expanded := make(map[string]bool)

	rows, _ := BuildIssueRows(issues, expanded)

	// Should show standalone + parent (collapsed), not child
	if len(rows) != 2 {
		t.Errorf("BuildIssueRows() returned %d rows, want 2", len(rows))
	}
}

func TestToggleExpanded(t *testing.T) {
	expanded := make(map[string]bool)

	// First toggle should expand
	newState := ToggleExpanded(expanded, "issue-1")
	if !newState {
		t.Error("First toggle should return true (expanded)")
	}
	if !expanded["issue-1"] {
		t.Error("issue-1 should be expanded")
	}

	// Second toggle should collapse
	newState = ToggleExpanded(expanded, "issue-1")
	if newState {
		t.Error("Second toggle should return false (collapsed)")
	}
	if expanded["issue-1"] {
		t.Error("issue-1 should be collapsed")
	}
}

func TestCollapseAll(t *testing.T) {
	expanded := map[string]bool{
		"issue-1": true,
		"issue-2": true,
		"issue-3": true,
	}

	CollapseAll(expanded)

	if len(expanded) != 0 {
		t.Errorf("CollapseAll() left %d entries, want 0", len(expanded))
	}
}

func TestExpandAll(t *testing.T) {
	issues := []linearapi.Issue{
		{
			ID:       "parent-1",
			Children: []linearapi.IssueChildRef{{ID: "child-1"}},
		},
		{
			ID:     "child-1",
			Parent: &linearapi.IssueRef{ID: "parent-1"},
		},
		{
			ID:       "parent-2",
			Children: []linearapi.IssueChildRef{{ID: "child-2"}},
		},
		{
			ID: "standalone",
		},
	}
	expanded := make(map[string]bool)

	ExpandAll(expanded, issues)

	// Parents with children should be expanded
	if !expanded["parent-1"] {
		t.Error("parent-1 should be expanded")
	}
	if !expanded["parent-2"] {
		t.Error("parent-2 should be expanded")
	}
	// Standalone (no parent, no children) should also be marked (doesn't affect display)
	if !expanded["standalone"] {
		t.Error("standalone should be marked in expanded map")
	}
}

func TestBuildIssueRows_ChildrenSortedByIdentifier(t *testing.T) {
	parent := linearapi.Issue{
		ID:         "parent",
		Identifier: "LIN-1",
		Title:      "Parent",
		Children: []linearapi.IssueChildRef{
			{ID: "child-c", Identifier: "LIN-4", Title: "Child C"},
			{ID: "child-a", Identifier: "LIN-2", Title: "Child A"},
			{ID: "child-b", Identifier: "LIN-3", Title: "Child B"},
		},
	}
	childC := linearapi.Issue{
		ID:         "child-c",
		Identifier: "LIN-4",
		Title:      "Child C",
		Parent:     &linearapi.IssueRef{ID: "parent"},
	}
	childA := linearapi.Issue{
		ID:         "child-a",
		Identifier: "LIN-2",
		Title:      "Child A",
		Parent:     &linearapi.IssueRef{ID: "parent"},
	}
	childB := linearapi.Issue{
		ID:         "child-b",
		Identifier: "LIN-3",
		Title:      "Child B",
		Parent:     &linearapi.IssueRef{ID: "parent"},
	}

	issues := []linearapi.Issue{parent, childC, childA, childB}
	expanded := map[string]bool{"parent": true}

	rows, _ := BuildIssueRows(issues, expanded)

	// Children should be sorted by identifier
	if len(rows) != 4 {
		t.Fatalf("Expected 4 rows, got %d", len(rows))
	}

	expectedOrder := []string{"parent", "child-a", "child-b", "child-c"}
	for i, expected := range expectedOrder {
		if rows[i].IssueID != expected {
			t.Errorf("Row %d IssueID = %q, want %q", i, rows[i].IssueID, expected)
		}
	}
}
