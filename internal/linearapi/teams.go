package linearapi

import (
	"context"
	"fmt"

	"github.com/shurcooL/graphql"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// ListTeams fetches all teams the user has access to.
func (c *Client) ListTeams(ctx context.Context) ([]Team, error) {
	var query struct {
		Teams struct {
			Nodes []struct {
				ID   graphql.String
				Key  graphql.String
				Name graphql.String
			}
		} `graphql:"teams"`
	}

	err := c.client.query(ctx, &query, nil)
	if err != nil {
		logger.ErrorWithErr(err, "linearapi.client: ListTeams failed")
		return nil, fmt.Errorf("list teams: %w", err)
	}

	teams := make([]Team, 0, len(query.Teams.Nodes))
	for _, node := range query.Teams.Nodes {
		teams = append(teams, Team{
			ID:   string(node.ID),
			Key:  string(node.Key),
			Name: string(node.Name),
		})
	}

	return teams, nil
}
