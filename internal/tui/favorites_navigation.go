package tui

import (
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/rivo/tview"
)

func favoriteLeafNode(favorite linearapi.Favorite) *NavigationNode {
	switch favorite.Type {
	case "issue":
		if favorite.IssueID == "" {
			return nil
		}
		return &NavigationNode{
			ID:      favorite.IssueID,
			Text:    favorite.IssueIdentifier + " " + favorite.IssueTitle,
			TeamID:  favorite.IssueTeamID,
			IsIssue: true,
			IssueID: favorite.IssueID,
		}
	case "project":
		if favorite.ProjectID == "" {
			return nil
		}
		return &NavigationNode{
			ID:        favorite.ProjectID,
			Text:      favorite.ProjectName,
			IsProject: true,
		}
	case "cycle":
		if favorite.CycleID == "" {
			return nil
		}
		name := linearapi.CycleRef{Name: favorite.CycleName, Number: favorite.CycleNumber}.DisplayName()
		return &NavigationNode{
			ID:        favorite.CycleID,
			Text:      name,
			TeamID:    favorite.CycleTeamID,
			IsCycle:   true,
			CycleID:   favorite.CycleID,
			CycleName: name,
		}
	case "team":
		if favorite.TeamID == "" {
			return nil
		}
		return &NavigationNode{
			ID:     favorite.TeamID,
			Text:   favorite.TeamName,
			TeamID: favorite.TeamID,
			IsTeam: true,
		}
	case "customView":
		if favorite.CustomViewID == "" {
			return nil
		}
		label := favorite.CustomViewName
		if favorite.Title != "" {
			label = favorite.Title
		}
		return &NavigationNode{
			ID:           favorite.CustomViewID,
			Text:         label,
			CustomViewID: favorite.CustomViewID,
		}
	case "predefinedView":
		switch favorite.PredefinedViewType {
		case "triage":
			label := favorite.Title
			if label == "" {
				label = "Triage"
			}
			return &NavigationNode{
				ID:        favorite.ID,
				Text:      label,
				TeamID:    favorite.PredefinedViewTeamID,
				StateType: "triage",
			}
		case "allIssues":
			label := favorite.Title
			if label == "" {
				label = "All Issues"
			}
			return &NavigationNode{ID: "all", Text: label, TeamID: favorite.PredefinedViewTeamID}
		default:
			logger.Debug("tui.favorites: skipping unsupported predefined view type=%s id=%s", favorite.PredefinedViewType, favorite.ID)
			return nil
		}
	default:
		logger.Debug("tui.favorites: skipping unsupported favorite type=%s id=%s", favorite.Type, favorite.ID)
		return nil
	}
}

const favoriteTypeFolder = "folder"

func isRenderableFavorite(favorite linearapi.Favorite) bool {
	if favorite.Type == favoriteTypeFolder {
		return true
	}
	return favoriteLeafNode(favorite) != nil
}

func favoriteParentIDs(favorites []linearapi.Favorite) map[string]string {
	isFolder := make(map[string]bool)
	for _, favorite := range favorites {
		if favorite.Type == favoriteTypeFolder {
			isFolder[favorite.ID] = true
		}
	}
	parents := make(map[string]string, len(favorites))
	for _, favorite := range favorites {
		if favorite.ParentID != favorite.ID && isFolder[favorite.ParentID] {
			parents[favorite.ID] = favorite.ParentID
		}
	}
	return parents
}

func favoriteNavigationNodes(favorites []linearapi.Favorite) []*NavigationNode {
	parents := favoriteParentIDs(favorites)

	folders := make(map[string]*NavigationNode)
	for _, favorite := range favorites {
		if favorite.Type != favoriteTypeFolder {
			continue
		}
		label := favorite.Title
		if label == "" {
			label = favorite.FolderName
		}
		folders[favorite.ID] = &NavigationNode{
			ID:       favorite.ID,
			Text:     label,
			IsFolder: true,
		}
	}

	var roots []*NavigationNode
	place := func(favorite linearapi.Favorite, node *NavigationNode) {
		node.FavoriteID = favorite.ID
		node.FavoriteParentID = parents[favorite.ID]
		if parent, ok := folders[node.FavoriteParentID]; ok {
			parent.Children = append(parent.Children, node)
			return
		}
		roots = append(roots, node)
	}

	for _, favorite := range favorites {
		if favorite.Type == favoriteTypeFolder {
			place(favorite, folders[favorite.ID])
			continue
		}
		if node := favoriteLeafNode(favorite); node != nil {
			place(favorite, node)
		}
	}
	return roots
}

func (a *App) buildFavoritesGroup(favorites []linearapi.Favorite) *tview.TreeNode {
	nodes := favoriteNavigationNodes(favorites)
	if len(nodes) == 0 {
		return nil
	}

	group := tview.NewTreeNode("Favorites").
		SetColor(a.theme.Accent).
		SetSelectable(false).
		SetExpanded(true)
	a.addFavoriteNodes(group, nodes)
	return group
}

func (a *App) addFavoriteNodes(parent *tview.TreeNode, nodes []*NavigationNode) {
	for _, navNode := range nodes {
		expanded := !navNode.IsTeam
		child := tview.NewTreeNode(navNode.Text).
			SetReference(navNode).
			SetExpanded(expanded)
		if len(navNode.Children) > 0 {
			a.addFavoriteNodes(child, navNode.Children)
		}
		parent.AddChild(child)
	}
}
