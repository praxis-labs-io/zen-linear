package tui

import (
	"sort"
	"strings"

	"github.com/Drucial/zen-linear/internal/linearapi"
)

// IssueRow represents a single row in the issues table with hierarchy info.
type IssueRow struct {
	IssueID         string // Reference to the issue
	Level           int    // Nesting level (0 = top-level, 1 = child, etc.)
	IsParent        bool   // True if this issue has children
	HasChildren     bool   // True if this issue has children (same as IsParent for now)
	IsExpanded      bool   // True if children are shown (only meaningful when HasChildren is true)
	IsHeader        bool   // True for a group header row (no issue)
	IsSpacer        bool   // True for a blank gap row above a header (no issue, not selectable)
	HeaderText      string // Group label for header rows
	HeaderCount     int    // Number of top-level issues in the group
	HeaderDimension string // Grouping dimension the header belongs to
	HeaderLevel     int    // 0 for group headers, 1 for subgroup headers
	HeaderKey       string // Stable identity for collapse tracking
	HeaderCollapsed bool   // True when the group's rows are hidden
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

// BuildIssueRows constructs a flattened list of rows for table rendering.
// It builds a hierarchical view where parent issues can be expanded/collapsed.
// Returns the rows and a map for quick issue lookup by ID.
func BuildIssueRows(issues []linearapi.Issue, expanded map[string]bool) ([]IssueRow, map[string]*linearapi.Issue) {
	idToIssue, topLevel, childrenByParent := indexIssues(issues)
	rows := appendIssueRows(nil, topLevel, childrenByParent, expanded)
	return rows, idToIssue
}

// Grouping dimensions supported by the grouped list view.
const (
	GroupByNone      = ""
	GroupByStatus    = "status"
	GroupByPriority  = "priority"
	GroupByAssignee  = "assignee"
	GroupByCycle     = "cycle"
	GroupByProject   = "project"
	GroupByMilestone = "milestone"
)

// groupKeyFor returns the group label and ordering rank of an issue along a
// grouping dimension. Groups sort by rank, then label.
func groupKeyFor(issue *linearapi.Issue, dimension string) (string, int) {
	switch dimension {
	case GroupByPriority:
		switch issue.Priority {
		case 1:
			return "Urgent", 1
		case 2:
			return "High", 2
		case 3:
			return "Normal", 3
		case 4:
			return "Low", 4
		default:
			return "No priority", 5
		}
	case GroupByAssignee:
		if issue.Assignee == "" {
			return "Unassigned", 1
		}
		return issue.Assignee, 0
	case GroupByCycle:
		if issue.Cycle == nil {
			return "No cycle", 1_000_000
		}
		// Recent cycles first.
		return issue.Cycle.DisplayName(), -issue.Cycle.Number
	case GroupByProject:
		if issue.ProjectName == "" {
			return "No project", 1
		}
		return issue.ProjectName, 0
	case GroupByMilestone:
		if issue.ProjectMilestone == nil || issue.ProjectMilestone.Name == "" {
			return "No milestone", 1_000_000
		}
		// Milestones order by their project-defined sort order.
		return issue.ProjectMilestone.Name, int(issue.ProjectMilestone.SortOrder)
	default: // GroupByStatus
		return issue.State, statusRank(issue.State)
	}
}

// groupTopLevel buckets top-level issues along a dimension, returning group
// labels in display order.
func groupTopLevel(topLevel []*linearapi.Issue, dimension string) ([]string, map[string][]*linearapi.Issue) {
	groups := make(map[string][]*linearapi.Issue)
	ranks := make(map[string]int)
	var order []string
	for _, issue := range topLevel {
		label, rank := groupKeyFor(issue, dimension)
		if _, seen := groups[label]; !seen {
			order = append(order, label)
			ranks[label] = rank
		}
		groups[label] = append(groups[label], issue)
	}
	sort.SliceStable(order, func(i, j int) bool {
		if ranks[order[i]] != ranks[order[j]] {
			return ranks[order[i]] < ranks[order[j]]
		}
		return order[i] < order[j]
	})
	return order, groups
}

// BuildGroupedIssueRows constructs rows grouped along a dimension with a
// header row per group — like Linear's grouped list view — and optionally
// sub-grouped along a second dimension beneath each header. Hierarchy behaves
// as in BuildIssueRows; a parent's subtree stays under the parent's group.
func BuildGroupedIssueRows(issues []linearapi.Issue, expanded map[string]bool, groupBy string, subgroupBy string, collapsed map[string]bool) ([]IssueRow, map[string]*linearapi.Issue) {
	idToIssue, topLevel, childrenByParent := indexIssues(issues)
	if groupBy == GroupByNone {
		return appendIssueRows(nil, topLevel, childrenByParent, expanded), idToIssue
	}
	if subgroupBy == groupBy {
		subgroupBy = GroupByNone
	}

	var rows []IssueRow
	order, groups := groupTopLevel(topLevel, groupBy)
	for _, label := range order {
		group := groups[label]
		key := groupBy + "\x1f" + label
		// Groups breathe: a gap row above every header except the first.
		if len(rows) > 0 {
			rows = append(rows, IssueRow{IsSpacer: true})
		}
		rows = append(rows, IssueRow{
			IsHeader:        true,
			HeaderText:      label,
			HeaderCount:     len(group),
			HeaderDimension: groupBy,
			HeaderKey:       key,
			HeaderCollapsed: collapsed[key],
		})
		if collapsed[key] {
			continue
		}
		if subgroupBy == GroupByNone {
			rows = appendIssueRows(rows, group, childrenByParent, expanded)
			continue
		}
		subOrder, subGroups := groupTopLevel(group, subgroupBy)
		for _, subLabel := range subOrder {
			subGroup := subGroups[subLabel]
			subKey := key + "\x1f" + subgroupBy + "\x1f" + subLabel
			// Subgroups gap too, except directly under their group header.
			if last := rows[len(rows)-1]; !last.IsHeader || last.HeaderLevel != 0 {
				rows = append(rows, IssueRow{IsSpacer: true})
			}
			rows = append(rows, IssueRow{
				IsHeader:        true,
				HeaderText:      subLabel,
				HeaderCount:     len(subGroup),
				HeaderDimension: subgroupBy,
				HeaderLevel:     1,
				HeaderKey:       subKey,
				HeaderCollapsed: collapsed[subKey],
			})
			if collapsed[subKey] {
				continue
			}
			rows = appendIssueRows(rows, subGroup, childrenByParent, expanded)
		}
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
