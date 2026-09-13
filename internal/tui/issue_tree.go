package tui

import (
	"sort"
	"strings"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

type IssueRow struct {
	IssueID         string
	Level           int
	IsParent        bool
	HasChildren     bool
	IsExpanded      bool
	IsHeader        bool
	IsSpacer        bool
	HeaderText      string
	HeaderCount     int
	HeaderDimension string
	HeaderLevel     int
	HeaderKey       string
	HeaderCollapsed bool
}

func statusRank(state string) int {
	lowerState := strings.ToLower(state)
	switch {
	case strings.Contains(lowerState, "triage"):
		return 0
	case strings.Contains(lowerState, "unstarted"):
		return 3
	case strings.Contains(lowerState, "review"):
		return 1
	case strings.Contains(lowerState, "progress") || strings.Contains(lowerState, "started"):
		return 2
	case strings.Contains(lowerState, "backlog"):
		return 4
	case strings.Contains(lowerState, "done") || strings.Contains(lowerState, "complete"):
		return 5
	case strings.Contains(lowerState, "cancel") || strings.Contains(lowerState, "duplicate"):
		return 6
	default:
		return 3
	}
}

// BuildIssueRows flattens issues into table rows, children under expanded parents, with an id lookup.
func BuildIssueRows(issues []linearapi.Issue, expanded map[string]bool) ([]IssueRow, map[string]*linearapi.Issue) {
	idToIssue, topLevel, childrenByParent := indexIssues(issues)
	rows := appendIssueRows(nil, topLevel, childrenByParent, expanded)
	return rows, idToIssue
}

const (
	GroupByNone      = ""
	GroupByStatus    = "status"
	GroupByPriority  = "priority"
	GroupByAssignee  = "assignee"
	GroupByCycle     = "cycle"
	GroupByProject   = "project"
	GroupByMilestone = "milestone"
)

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
		return issue.ProjectMilestone.Name, int(issue.ProjectMilestone.SortOrder)
	default:
		return issue.State, statusRank(issue.State)
	}
}

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

// BuildGroupedIssueRows is BuildIssueRows with a header row per group and subgroup; collapsed hides a header's rows.
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

func appendIssueRows(rows []IssueRow, topLevel []*linearapi.Issue, childrenByParent map[string][]*linearapi.Issue, expanded map[string]bool) []IssueRow {
	for _, issue := range topLevel {
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

		if hasChildren && isExpanded {
			if len(children) > 0 {
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

// ToggleExpanded flips an issue's expanded state and returns the new one.
func ToggleExpanded(expanded map[string]bool, issueID string) bool {
	newState := !expanded[issueID]
	expanded[issueID] = newState
	return newState
}

func CollapseAll(expanded map[string]bool) {
	for k := range expanded {
		delete(expanded, k)
	}
}

func ExpandAll(expanded map[string]bool, issues []linearapi.Issue) {
	for _, issue := range issues {
		if len(issue.Children) > 0 || issue.Parent == nil {
			expanded[issue.ID] = true
		}
	}
}
