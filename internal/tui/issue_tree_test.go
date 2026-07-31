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

// TestStatusRank verifies lifecycle ordering, including that "Unstarted" does
// not match the "started" category.
func TestStatusRank(t *testing.T) {
	ordered := []string{"Triage", "Backlog", "Unstarted", "In Progress", "Done", "Canceled"}
	for i := 1; i < len(ordered); i++ {
		if statusRank(ordered[i-1]) > statusRank(ordered[i]) {
			t.Errorf("statusRank(%q)=%d > statusRank(%q)=%d", ordered[i-1], statusRank(ordered[i-1]), ordered[i], statusRank(ordered[i]))
		}
	}
	if statusRank("Unstarted") == statusRank("In Progress") {
		t.Error("Unstarted must not rank with started states")
	}
	if statusRank("In Review") != statusRank("In Progress") {
		t.Error("In Review should rank with started states")
	}
}

// TestBuildGroupedIssueRows verifies headers, group order, and counts.
func TestBuildGroupedIssueRows(t *testing.T) {
	issues := []linearapi.Issue{
		{ID: "1", Identifier: "LIN-1", State: "Done"},
		{ID: "2", Identifier: "LIN-2", State: "Todo"},
		{ID: "3", Identifier: "LIN-3", State: "Todo"},
		{ID: "4", Identifier: "LIN-4", State: "In Progress"},
	}

	rows, idToIssue := BuildGroupedIssueRows(issues, map[string]bool{})
	if len(idToIssue) != 4 {
		t.Fatalf("idToIssue size = %d, want 4", len(idToIssue))
	}
	// Expect: [Todo header, 2, 3, In Progress header, 4, Done header, 1]
	if len(rows) != 7 {
		t.Fatalf("rows = %d, want 7: %#v", len(rows), rows)
	}
	wantHeaders := map[int]string{0: "Todo", 3: "In Progress", 5: "Done"}
	for index, state := range wantHeaders {
		if !rows[index].IsHeader || rows[index].HeaderText != state {
			t.Errorf("rows[%d] = %+v, want header %q", index, rows[index], state)
		}
	}
	if rows[0].HeaderCount != 2 {
		t.Errorf("Todo header count = %d, want 2", rows[0].HeaderCount)
	}
	if rows[1].IssueID != "2" || rows[2].IssueID != "3" || rows[4].IssueID != "4" || rows[6].IssueID != "1" {
		t.Errorf("unexpected issue placement: %#v", rows)
	}
}

// TestNextIssueRow verifies header rows are skipped in both directions.
func TestNextIssueRow(t *testing.T) {
	rows := []IssueRow{
		{IsHeader: true, HeaderText: "Todo"},
		{IssueID: "a"},
		{IsHeader: true, HeaderText: "Done"},
		{IssueID: "b"},
	}
	if got := nextIssueRow(rows, 0, 1); got != 2 {
		t.Errorf("first issue row = %d, want 2", got)
	}
	if got := nextIssueRow(rows, 2, 1); got != 4 {
		t.Errorf("next after row 2 = %d, want 4", got)
	}
	if got := nextIssueRow(rows, 4, -1); got != 2 {
		t.Errorf("previous before row 4 = %d, want 2", got)
	}
	if got := nextIssueRow(rows, 4, 1); got != 0 {
		t.Errorf("past end = %d, want 0", got)
	}
}
