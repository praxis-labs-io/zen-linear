package linearapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/shurcooL/graphql"
)

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

type historyUserNode struct {
	ID          graphql.String
	Name        graphql.String
	DisplayName graphql.String
	Email       graphql.String
	IsMe        graphql.Boolean
}

type actorBotNode struct {
	ID              graphql.String
	Name            graphql.String
	UserDisplayName graphql.String
}

type historyStateNode struct {
	ID   graphql.String
	Name graphql.String
	Type graphql.String
}

type historyCycleNode struct {
	ID     graphql.String
	Name   *graphql.String
	Number graphql.Float
}

type historyNamedNode struct {
	ID   graphql.String
	Name graphql.String
}

type historyLabelNode struct {
	ID    graphql.String
	Name  graphql.String
	Color graphql.String
}

type historyIssueNode struct {
	ID         graphql.String
	Identifier graphql.String
	Title      graphql.String
}

type issueHistoryNode struct {
	ID        graphql.String
	CreatedAt graphql.String

	Actor    *historyUserNode
	BotActor *actorBotNode

	FromState *historyStateNode
	ToState   *historyStateNode

	FromAssignee *historyUserNode
	ToAssignee   *historyUserNode

	FromCycle *historyCycleNode
	ToCycle   *historyCycleNode

	FromProject *historyNamedNode
	ToProject   *historyNamedNode

	FromProjectMilestone *historyNamedNode
	ToProjectMilestone   *historyNamedNode

	FromParent *historyIssueNode
	ToParent   *historyIssueNode

	AddedLabels     []historyLabelNode
	RemovedLabels   []historyLabelNode
	RelationChanges []struct {
		Identifier graphql.String
		Type       graphql.String
	}

	// Pointers because 0 is Linear's "No priority", a real target.
	FromPriority *graphql.Float
	ToPriority   *graphql.Float

	ToTitle            *graphql.String
	UpdatedDescription *graphql.Boolean
}

// The embedded selection must stay untagged and first, or shurcooL stops inlining it flat.
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
	Creator  *historyUserNode
	BotActor *actorBotNode
	History  struct {
		Nodes []issueHistoryNode
	} `graphql:"history(first: 50, orderBy: createdAt)"`
}

func (c *Client) FetchIssueByID(ctx context.Context, id string) (Issue, error) {
	var query struct {
		Issue issueDetailNode `graphql:"issue(id: $id)"`
	}

	variables := map[string]interface{}{
		"id": graphql.String(id),
	}

	err := c.client.query(ctx, &query, variables)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Debug("linearapi.client: FetchIssueByID canceled issue_id=%s", id)
		} else {
			logger.ErrorWithErr(err, "linearapi.client: FetchIssueByID failed issue_id=%s", id)
		}
		return Issue{}, fmt.Errorf("fetch issue %s: %w", id, err)
	}

	return query.Issue.toIssue(), nil
}
