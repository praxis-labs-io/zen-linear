package tui

import (
	"github.com/rivo/tview"
	"github.com/roeyazroel/linear-tui/internal/linearapi"
	"github.com/roeyazroel/linear-tui/internal/logger"
)

// favoriteNavigationNodes maps favorites onto navigation nodes, skipping
// favorite types the navigation tree cannot display (custom views, labels,
// documents, ...).
func favoriteNavigationNodes(favorites []linearapi.Favorite) []*NavigationNode {
	nodes := make([]*NavigationNode, 0, len(favorites))
	for _, favorite := range favorites {
		switch favorite.Type {
		case "issue":
			if favorite.IssueID == "" {
				continue
			}
			nodes = append(nodes, &NavigationNode{
				ID:      favorite.IssueID,
				Text:    favorite.IssueIdentifier + " " + favorite.IssueTitle,
				TeamID:  favorite.IssueTeamID,
				IsIssue: true,
				IssueID: favorite.IssueID,
			})
		case "project":
			if favorite.ProjectID == "" {
				continue
			}
			nodes = append(nodes, &NavigationNode{
				ID:        favorite.ProjectID,
				Text:      favorite.ProjectName,
				TeamID:    favorite.ProjectTeamID,
				IsProject: true,
			})
		case "cycle":
			if favorite.CycleID == "" {
				continue
			}
			name := linearapi.CycleRef{Name: favorite.CycleName, Number: favorite.CycleNumber}.DisplayName()
			nodes = append(nodes, &NavigationNode{
				ID:        favorite.CycleID,
				Text:      name,
				TeamID:    favorite.CycleTeamID,
				IsCycle:   true,
				CycleID:   favorite.CycleID,
				CycleName: name,
			})
		case "team":
			if favorite.TeamID == "" {
				continue
			}
			nodes = append(nodes, &NavigationNode{
				ID:     favorite.TeamID,
				Text:   favorite.TeamName,
				TeamID: favorite.TeamID,
				IsTeam: true,
			})
		case "customView":
			if favorite.CustomViewID == "" {
				continue
			}
			label := favorite.CustomViewName
			if favorite.Title != "" {
				label = favorite.Title
			}
			nodes = append(nodes, &NavigationNode{
				ID:           favorite.CustomViewID,
				Text:         label,
				CustomViewID: favorite.CustomViewID,
			})
		case "predefinedView":
			switch favorite.PredefinedViewType {
			case "triage":
				label := favorite.Title
				if label == "" {
					label = "Triage"
				}
				nodes = append(nodes, &NavigationNode{
					ID:        favorite.ID,
					Text:      label,
					TeamID:    favorite.PredefinedViewTeamID,
					StateType: "triage",
				})
			case "allIssues":
				label := favorite.Title
				if label == "" {
					label = "All Issues"
				}
				nodes = append(nodes, &NavigationNode{ID: "all", Text: label})
			default:
				logger.Debug("tui.favorites: skipping unsupported predefined view type=%s id=%s", favorite.PredefinedViewType, favorite.ID)
			}
		default:
			logger.Debug("tui.favorites: skipping unsupported favorite type=%s id=%s", favorite.Type, favorite.ID)
		}
	}
	return nodes
}

// appendFavoritesSection adds a Favorites group under the navigation root.
// Favorites are additive: with no displayable favorites the section is omitted.
func (a *App) appendFavoritesSection(root *tview.TreeNode, favorites []linearapi.Favorite) {
	nodes := favoriteNavigationNodes(favorites)
	if len(nodes) == 0 {
		return
	}

	group := tview.NewTreeNode("Favorites").
		SetColor(a.theme.Accent).
		SetSelectable(false).
		SetExpanded(true)
	for _, navNode := range nodes {
		child := tview.NewTreeNode("  " + navNode.Text).
			SetColor(a.theme.Foreground).
			SetReference(navNode).
			SetExpanded(false)
		group.AddChild(child)
	}
	root.AddChild(group)
}
