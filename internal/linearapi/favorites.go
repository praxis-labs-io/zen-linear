package linearapi

import (
	"context"
	"fmt"
	"sort"

	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/shurcooL/graphql"
)

// favoriteNode is the favorite selection shared by the list query and the
// create mutation. Linear rejects an entire query over one misplaced field, so
// the selection lives in exactly one place.
type favoriteNode struct {
	ID         graphql.String
	Type       graphql.String
	SortOrder  graphql.Float
	Title      graphql.String
	FolderName *graphql.String
	Parent     *struct {
		ID graphql.String
	}
	PredefinedViewType *graphql.String
	PredefinedViewTeam *struct {
		ID graphql.String
	}
	CustomView *struct {
		ID   graphql.String
		Name graphql.String
	}
	Issue *struct {
		ID         graphql.String
		Identifier graphql.String
		Title      graphql.String
		Team       struct {
			ID graphql.String
		}
	}
	Project *struct {
		ID    graphql.String
		Name  graphql.String
		Teams struct {
			Nodes []struct {
				ID graphql.String
			}
		} `graphql:"teams(first: 1)"`
	}
	Cycle *struct {
		ID     graphql.String
		Name   *graphql.String
		Number graphql.Float
		Team   struct {
			ID graphql.String
		}
	}
	Team *struct {
		ID   graphql.String
		Name graphql.String
	}
}

// parseFavoriteNode flattens a favorite selection into the Favorite struct.
func parseFavoriteNode(node favoriteNode) Favorite {
	favorite := Favorite{
		ID:        string(node.ID),
		Type:      string(node.Type),
		SortOrder: float64(node.SortOrder),
		Title:     string(node.Title),
	}
	if node.Parent != nil {
		favorite.ParentID = string(node.Parent.ID)
	}
	if node.FolderName != nil {
		favorite.FolderName = string(*node.FolderName)
	}
	if node.PredefinedViewType != nil {
		favorite.PredefinedViewType = string(*node.PredefinedViewType)
	}
	if node.PredefinedViewTeam != nil {
		favorite.PredefinedViewTeamID = string(node.PredefinedViewTeam.ID)
	}
	if node.CustomView != nil {
		favorite.CustomViewID = string(node.CustomView.ID)
		favorite.CustomViewName = string(node.CustomView.Name)
	}
	if node.Issue != nil {
		favorite.IssueID = string(node.Issue.ID)
		favorite.IssueIdentifier = string(node.Issue.Identifier)
		favorite.IssueTitle = string(node.Issue.Title)
		favorite.IssueTeamID = string(node.Issue.Team.ID)
	}
	if node.Project != nil {
		favorite.ProjectID = string(node.Project.ID)
		favorite.ProjectName = string(node.Project.Name)
		if len(node.Project.Teams.Nodes) > 0 {
			favorite.ProjectTeamID = string(node.Project.Teams.Nodes[0].ID)
		}
	}
	if node.Cycle != nil {
		favorite.CycleID = string(node.Cycle.ID)
		if node.Cycle.Name != nil {
			favorite.CycleName = string(*node.Cycle.Name)
		}
		favorite.CycleNumber = int(node.Cycle.Number)
		favorite.CycleTeamID = string(node.Cycle.Team.ID)
	}
	if node.Team != nil {
		favorite.TeamID = string(node.Team.ID)
		favorite.TeamName = string(node.Team.Name)
	}
	return favorite
}

// ListFavorites fetches the viewer's favorites, ordered as in Linear's sidebar.
func (c *Client) ListFavorites(ctx context.Context) ([]Favorite, error) {
	var after *graphql.String
	favorites := make([]Favorite, 0)

	for {
		var query struct {
			Favorites struct {
				Nodes    []favoriteNode
				PageInfo struct {
					HasNextPage graphql.Boolean
					EndCursor   graphql.String
				}
			} `graphql:"favorites(first: $first, after: $after)"`
		}

		variables := map[string]interface{}{
			"first": graphql.Int(50),
			"after": after,
		}

		if err := c.client.query(ctx, &query, variables); err != nil {
			logger.ErrorWithErr(err, "linearapi.client: ListFavorites failed")
			return nil, fmt.Errorf("list favorites: %w", err)
		}

		for _, node := range query.Favorites.Nodes {
			favorites = append(favorites, parseFavoriteNode(node))
		}

		if !bool(query.Favorites.PageInfo.HasNextPage) {
			break
		}
		cursor := query.Favorites.PageInfo.EndCursor
		after = &cursor
	}

	SortFavorites(favorites)

	return favorites, nil
}

// SortFavorites orders favorites the way Linear's sidebar does.
func SortFavorites(favorites []Favorite) {
	sort.SliceStable(favorites, func(i, j int) bool {
		return favorites[i].SortOrder < favorites[j].SortOrder
	})
}

// CreateFavorite adds a favorite for the target entity. Linear upserts, so
// favoriting something twice returns the existing favorite.
func (c *Client) CreateFavorite(ctx context.Context, target FavoriteTarget) (Favorite, error) {
	input := target.input()
	if len(input) == 0 {
		return Favorite{}, fmt.Errorf("create favorite: no target entity")
	}

	var mutation struct {
		FavoriteCreate struct {
			Success  graphql.Boolean
			Favorite favoriteNode
		} `graphql:"favoriteCreate(input: $input)"`
	}

	variables := map[string]interface{}{"input": input}
	if err := c.client.mutate(ctx, &mutation, variables); err != nil {
		logger.ErrorWithErr(err, "linearapi.client: CreateFavorite failed target=%v", input)
		return Favorite{}, fmt.Errorf("create favorite: %w", err)
	}
	if !bool(mutation.FavoriteCreate.Success) {
		return Favorite{}, fmt.Errorf("create favorite: operation failed")
	}
	return parseFavoriteNode(mutation.FavoriteCreate.Favorite), nil
}

// DeleteFavorite removes a favorite from the viewer's sidebar. Linear treats a
// missing favorite as success.
func (c *Client) DeleteFavorite(ctx context.Context, favoriteID string) error {
	var mutation struct {
		FavoriteDelete struct {
			Success graphql.Boolean
		} `graphql:"favoriteDelete(id: $id)"`
	}

	variables := map[string]interface{}{"id": graphql.String(favoriteID)}
	if err := c.client.mutate(ctx, &mutation, variables); err != nil {
		logger.ErrorWithErr(err, "linearapi.client: DeleteFavorite failed favorite_id=%s", favoriteID)
		return fmt.Errorf("delete favorite %s: %w", favoriteID, err)
	}
	if !bool(mutation.FavoriteDelete.Success) {
		return fmt.Errorf("delete favorite %s: operation failed", favoriteID)
	}
	return nil
}

// UpdateFavoriteSortOrder repositions a favorite in Linear's sidebar.
func (c *Client) UpdateFavoriteSortOrder(ctx context.Context, favoriteID string, sortOrder float64) error {
	return c.updateFavorite(ctx, favoriteID, FavoriteUpdateInput{
		"sortOrder": graphql.Float(sortOrder),
	})
}

// MoveFavorite reparents a favorite and positions it. An empty parentID moves
// it back to the top level, which needs an explicit null rather than a blank
// id.
func (c *Client) MoveFavorite(ctx context.Context, favoriteID, parentID string, sortOrder float64) error {
	input := FavoriteUpdateInput{"sortOrder": graphql.Float(sortOrder)}
	if parentID == "" {
		input["parentId"] = nil
	} else {
		input["parentId"] = graphql.String(parentID)
	}
	return c.updateFavorite(ctx, favoriteID, input)
}

func (c *Client) updateFavorite(ctx context.Context, favoriteID string, input FavoriteUpdateInput) error {
	var mutation struct {
		FavoriteUpdate struct {
			Success graphql.Boolean
		} `graphql:"favoriteUpdate(id: $id, input: $input)"`
	}

	variables := map[string]interface{}{
		"id":    graphql.String(favoriteID),
		"input": input,
	}
	if err := c.client.mutate(ctx, &mutation, variables); err != nil {
		logger.ErrorWithErr(err, "linearapi.client: updateFavorite failed favorite_id=%s", favoriteID)
		return fmt.Errorf("update favorite %s: %w", favoriteID, err)
	}
	if !bool(mutation.FavoriteUpdate.Success) {
		return fmt.Errorf("update favorite %s: operation failed", favoriteID)
	}
	return nil
}
