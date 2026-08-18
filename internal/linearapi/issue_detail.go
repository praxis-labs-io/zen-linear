package linearapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/shurcooL/graphql"
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

// historyUserNode is the user selection on a history entry: the actor, and both
// sides of an assignee change.
type historyUserNode struct {
	ID          graphql.String
	Name        graphql.String
	DisplayName graphql.String
	Email       graphql.String
	IsMe        graphql.Boolean
}

// actorBotNode is the integration that made a change, in place of a user.
// avatarUrl, type and subType are left out: the feed names an actor, it does
// not draw one or route on what kind of bot it is.
type actorBotNode struct {
	ID              graphql.String
	Name            graphql.String
	UserDisplayName graphql.String
}

// historyStateNode is narrower than the workflow state selection elsewhere:
// naming a state that was moved through needs no position or team.
type historyStateNode struct {
	ID   graphql.String
	Name graphql.String
	Type graphql.String
}

// historyCycleNode is narrower than cycleRefNode for the same reason. Number is
// what CycleRef.DisplayName falls back to when a cycle is unnamed.
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

// issueHistoryNode is one entry in an issue's history. Linear records every
// change saved together as a single entry, so each from/to pair below is
// independently null and several may be set at once.
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

	// The priorities are pointers because 0 is Linear's "No priority", a real
	// target a change can name. A value type reads a demotion to it as no
	// change at all.
	FromPriority *graphql.Float
	ToPriority   *graphql.Float

	ToTitle            *graphql.String
	UpdatedDescription *graphql.Boolean
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
	// Creator and BotActor answer the one activity event with no history entry:
	// Linear records an issue's creation on the issue itself.
	Creator  *historyUserNode
	BotActor *actorBotNode
	History  struct {
		Nodes []issueHistoryNode
	} `graphql:"history(first: 50, orderBy: createdAt)"`
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
