package tui

import (
	"sort"
	"strings"

	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

// IssueRow represents a single row in the issues table with hierarchy info.
type IssueRow struct {
	IssueID     string // Reference to the issue
	Level       int    // Nesting level (0 = top-level, 1 = child, etc.)
	IsParent    bool   // True if this issue has children
	HasChildren bool   // True if this issue has children (same as IsParent for now)
	IsExpanded  bool   // True if children are shown (only meaningful when HasChildren is true)
	IsHeader    bool   // True for a status group header row (no issue)
	HeaderText  string // Workflow state name for header rows
	HeaderCount int    // Number of top-level issues in the group
}

// statusRank orders workflow states by lifecycle category the way Linear's
// grouped list does: triage, started, unstarted, backlog, completed,
// canceled. The state type is not fetched, so the category is derived from
// the state name.
func statusRank(state string) int {
	lowerState := strings.ToLower(state)
	switch {
	case strings.Contains(lowerState, "triage"):
		return 0
	case strings.Contains(lowerState, "unstarted"):
		return 2
	case strings.Contains(lowerState, "progress") || strings.Contains(lowerState, "review") || strings.Contains(lowerState, "started"):
		return 1
	case strings.Contains(lowerState, "backlog"):
		return 3
	case strings.Contains(lowerState, "done") || strings.Contains(lowerState, "complete"):
		return 4
	case strings.Contains(lowerState, "cancel") || strings.Contains(lowerState, "duplicate"):
		return 5
	default:
		return 2 // todo and anything unrecognized sit with unstarted
	}
}

// sortIssuesByStatus sorts issues by workflow state category, then state name.
func sortIssuesByStatus(issues []linearapi.Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		ri, rj := statusRank(issues[i].State), statusRank(issues[j].State)
		if ri != rj {
			return ri < rj
		}
		return issues[i].State < issues[j].State
	})
}

// BuildIssueRows constructs a flattened list of rows for table rendering.
// It builds a hierarchical view where parent issues can be expanded/collapsed.
// Returns the rows and a map for quick issue lookup by ID.
func BuildIssueRows(issues []linearapi.Issue, expanded map[string]bool) ([]IssueRow, map[string]*linearapi.Issue) {
	idToIssue, topLevel, childrenByParent := indexIssues(issues)
	rows := appendIssueRows(nil, topLevel, childrenByParent, expanded)
	return rows, idToIssue
}

// BuildGroupedIssueRows constructs rows grouped by workflow state with a
// header row per group, like Linear's grouped list view. Hierarchy behaves as
// in BuildIssueRows; a parent's subtree stays under the parent's group.
func BuildGroupedIssueRows(issues []linearapi.Issue, expanded map[string]bool) ([]IssueRow, map[string]*linearapi.Issue) {
	idToIssue, topLevel, childrenByParent := indexIssues(issues)

	groupsByState := make(map[string][]*linearapi.Issue)
	var stateOrder []string
	for _, issue := range topLevel {
		if _, seen := groupsByState[issue.State]; !seen {
			stateOrder = append(stateOrder, issue.State)
		}
		groupsByState[issue.State] = append(groupsByState[issue.State], issue)
	}
	sort.SliceStable(stateOrder, func(i, j int) bool {
		ri, rj := statusRank(stateOrder[i]), statusRank(stateOrder[j])
		if ri != rj {
			return ri < rj
		}
		return stateOrder[i] < stateOrder[j]
	})

	var rows []IssueRow
	for _, state := range stateOrder {
		group := groupsByState[state]
		rows = append(rows, IssueRow{
			IsHeader:    true,
			HeaderText:  state,
			HeaderCount: len(group),
		})
		rows = appendIssueRows(rows, group, childrenByParent, expanded)
	}
	return rows, idToIssue
}

// indexIssues splits issues into top-level entries and children keyed by
// parent, alongside the id lookup map.
// An issue is "top-level" if it has no parent or its parent is not in the
// fetched list (orphan sub-issue).
func indexIssues(issues []linearapi.Issue) (map[string]*linearapi.Issue, []*linearapi.Issue, map[string][]*linearapi.Issue) {
	idToIssue := make(map[string]*linearapi.Issue, len(issues))
	for i := range issues {
		idToIssue[issues[i].ID] = &issues[i]
	}

	var topLevel []*linearapi.Issue
	childrenByParent := make(map[string][]*linearapi.Issue)
	for i := range issues {
		issue := &issues[i]
		if issue.Parent == nil {
			topLevel = append(topLevel, issue)
		} else if _, parentInList := idToIssue[issue.Parent.ID]; parentInList {
			childrenByParent[issue.Parent.ID] = append(childrenByParent[issue.Parent.ID], issue)
		} else {
			topLevel = append(topLevel, issue)
		}
	}
	return idToIssue, topLevel, childrenByParent
}

// appendIssueRows emits table rows for the given top-level issues, expanding
// children where requested.
func appendIssueRows(rows []IssueRow, topLevel []*linearapi.Issue, childrenByParent map[string][]*linearapi.Issue, expanded map[string]bool) []IssueRow {
	for _, issue := range topLevel {
		// Check if this issue has children in our list
		children := childrenByParent[issue.ID]
		hasChildren := len(children) > 0 || len(issue.Children) > 0
		isExpanded := expanded[issue.ID]

		rows = append(rows, IssueRow{
			IssueID:     issue.ID,
			Level:       0,
			IsParent:    hasChildren,
			HasChildren: hasChildren,
			IsExpanded:  isExpanded,
		})

		// If expanded, add children
		if hasChildren && isExpanded {
			// Use children from our fetched list if available
			if len(children) > 0 {
				// Sort children by identifier for consistent ordering
				sort.Slice(children, func(i, j int) bool {
					return children[i].Identifier < children[j].Identifier
				})

				for _, child := range children {
					childHasChildren := len(child.Children) > 0
					childExpanded := expanded[child.ID]

					rows = append(rows, IssueRow{
						IssueID:     child.ID,
						Level:       1,
						IsParent:    childHasChildren,
						HasChildren: childHasChildren,
						IsExpanded:  childExpanded,
					})
				}
			}
		}
	}

	return rows
}

// ToggleExpanded toggles the expanded state for an issue.
// Returns the new expanded state.
func ToggleExpanded(expanded map[string]bool, issueID string) bool {
	newState := !expanded[issueID]
	expanded[issueID] = newState
	return newState
}

// CollapseAll sets all issues to collapsed state.
func CollapseAll(expanded map[string]bool) {
	for k := range expanded {
		delete(expanded, k)
	}
}

// ExpandAll expands all parent issues.
func ExpandAll(expanded map[string]bool, issues []linearapi.Issue) {
	for _, issue := range issues {
		if len(issue.Children) > 0 || issue.Parent == nil {
			// Expand issues that have children
			expanded[issue.ID] = true
		}
	}
}
