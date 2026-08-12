package linearapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/shurcooL/graphql"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// issueRelationNode is the relation selection, shared by an issue's relations
// and inverseRelations connections.
type issueRelationNode struct {
	ID    graphql.String
	Type  graphql.String
	Issue struct {
		ID         graphql.String
		Identifier graphql.String
		Title      graphql.String
	}
	RelatedIssue struct {
		ID         graphql.String
		Identifier graphql.String
		Title      graphql.String
	}
}

// issueDetailNode is issueQueryNode plus the connections only the details pane
// needs. The embedded selection is untagged and declared first, which is what
// makes shurcooL/graphql inline its fields flat and in the original order.
type issueDetailNode struct {
	issueQueryNode
	Relations struct {
		Nodes []issueRelationNode
	} `graphql:"relations(first: 50)"`
	InverseRelations struct {
		Nodes []issueRelationNode
	} `graphql:"inverseRelations(first: 50)"`
	Subscribers struct {
		Nodes []struct {
			ID          graphql.String
			Name        graphql.String
			DisplayName graphql.String
			Email       graphql.String
			IsMe        graphql.Boolean
		}
	} `graphql:"subscribers(first: 50)"`
	Attachments struct {
		Nodes []struct {
			ID         graphql.String
			Title      graphql.String
			Subtitle   *graphql.String
			URL        graphql.String
			SourceType *graphql.String
			CreatedAt  graphql.String
			UpdatedAt  graphql.String
		}
	} `graphql:"attachments(first: 50)"`
	Comments struct {
		Nodes []struct {
			ID        graphql.String
			Body      graphql.String
			CreatedAt graphql.String
			UpdatedAt graphql.String
			ParentID  *graphql.String
			URL       graphql.String
			User      struct {
				ID          graphql.String
				Name        graphql.String
				DisplayName graphql.String
				Email       graphql.String
				IsMe        graphql.Boolean
			}
		}
	} `graphql:"comments(first: 100, orderBy: createdAt)"`
}

// FetchIssueByID fetches a single issue by its ID.
func (c *Client) FetchIssueByID(ctx context.Context, id string) (Issue, error) {
	var query struct {
		Issue issueDetailNode `graphql:"issue(id: $id)"`
	}

	variables := map[string]interface{}{
		"id": graphql.String(id),
	}

	err := c.client.query(ctx, &query, variables)
	if err != nil {
		// The details pane cancels this query on every superseded selection, so
		// a cancellation here is the design working, not a failure to report.
		// Test the error rather than the context: a query that fails for a real
		// reason while a newer selection happens to have canceled it still has
		// to reach the log at Error.
		if errors.Is(err, context.Canceled) {
			logger.Debug("linearapi.client: FetchIssueByID canceled issue_id=%s", id)
		} else {
			logger.ErrorWithErr(err, "linearapi.client: FetchIssueByID failed issue_id=%s", id)
		}
		return Issue{}, fmt.Errorf("fetch issue %s: %w", id, err)
	}

	return query.Issue.toIssue(), nil
}
