package tui

import "github.com/zen-linear/zen-linear/internal/linearapi"

// myIssues returns the issues belonging in the My tab: those assigned to the
// current user, plus the sub-issues beneath them so a subtree the user owns
// stays intact. Descent only ever adds, so an issue assigned to the user stays
// in My whoever owns its parent. If currentUserID is empty, My is empty.
func myIssues(issues []linearapi.Issue, currentUserID string) []linearapi.Issue {
	mine := make([]linearapi.Issue, 0)
	if currentUserID == "" {
		return mine
	}

	member := make(map[string]bool)
	for i := range issues {
		if issues[i].AssigneeID == currentUserID {
			member[issues[i].ID] = true
		}
	}

	// Descend one generation per sweep, so nested children reach their
	// ancestor however the pages ordered them.
	for changed := true; changed; {
		changed = false
		for i := range issues {
			issue := &issues[i]
			if issue.Parent != nil && !member[issue.ID] && member[issue.Parent.ID] {
				member[issue.ID] = true
				changed = true
			}
		}
	}

	for i := range issues {
		if member[issues[i].ID] {
			mine = append(mine, issues[i])
		}
	}
	return mine
}
