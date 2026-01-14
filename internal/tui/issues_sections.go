package tui

import "github.com/roeyazroel/linear-tui/internal/linearapi"

// splitIssuesByAssignee partitions issues into "My Issues" and "Other Issues" based on assignee.
// Issues where AssigneeID matches currentUserID go into "my", all others go into "other".
// Children always follow their parent's section to preserve parent-child relationships.
// If currentUserID is empty, all issues go into "other".
func splitIssuesByAssignee(issues []linearapi.Issue, currentUserID string) (my []linearapi.Issue, other []linearapi.Issue) {
	my = make([]linearapi.Issue, 0)
	other = make([]linearapi.Issue, 0)

	// If no current user, all issues go to "other"
	if currentUserID == "" {
		other = issues
		return my, other
	}

	// Build a map of issue ID to section assignment
	// true = "my", false = "other", not present = not yet assigned
	sectionMap := make(map[string]bool)

	// First pass: assign top-level issues (no parent) to sections based on assignee
	for i := range issues {
		issue := &issues[i]
		if issue.Parent == nil {
			if issue.AssigneeID == currentUserID {
				sectionMap[issue.ID] = true
			} else {
				sectionMap[issue.ID] = false
			}
		}
	}

	// Second pass: assign children to the same section as their parent
	// Process in multiple iterations to handle nested children
	changed := true
	for changed {
		changed = false
		for i := range issues {
			issue := &issues[i]
			if issue.Parent != nil {
				if _, assigned := sectionMap[issue.ID]; !assigned {
					// Check if parent is assigned
					if parentInMy, parentAssigned := sectionMap[issue.Parent.ID]; parentAssigned {
						sectionMap[issue.ID] = parentInMy
						changed = true
					}
				}
			}
		}
	}

	// Final pass: assign any remaining orphan children based on their own assignee
	for i := range issues {
		issue := &issues[i]
		if _, assigned := sectionMap[issue.ID]; !assigned {
			if issue.AssigneeID == currentUserID {
				sectionMap[issue.ID] = true
			} else {
				sectionMap[issue.ID] = false
			}
		}
	}

	// Build result slices
	for i := range issues {
		issue := &issues[i]
		if sectionMap[issue.ID] {
			my = append(my, *issue)
		} else {
			other = append(other, *issue)
		}
	}

	return my, other
}
