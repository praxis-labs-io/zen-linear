package linearapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/shurcooL/graphql"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// FetchIssueByID fetches a single issue by its ID.
func (c *Client) FetchIssueByID(ctx context.Context, id string) (Issue, error) {
	var query struct {
		Issue struct {
			ID         graphql.String
			Identifier graphql.String
			Title      graphql.String
			State      struct {
				ID   graphql.String
				Name graphql.String
			}
			Assignee *struct {
				ID   graphql.String
				Name graphql.String
			}
			Priority    graphql.Float
			UpdatedAt   graphql.String
			CreatedAt   graphql.String
			Description *graphql.String
			Team        struct {
				ID graphql.String
			}
			Project *struct {
				ID   graphql.String
				Name graphql.String
			}
			Cycle *struct {
				ID         graphql.String
				Name       *graphql.String
				Number     graphql.Float
				StartsAt   graphql.String
				EndsAt     graphql.String
				IsActive   graphql.Boolean
				IsFuture   graphql.Boolean
				IsPast     graphql.Boolean
				IsNext     graphql.Boolean
				IsPrevious graphql.Boolean
			}
			DueDate          *graphql.String
			Estimate         *graphql.Float
			ProjectMilestone *struct {
				ID         graphql.String
				Name       graphql.String
				TargetDate *graphql.String
				Status     graphql.String
				Project    struct {
					ID graphql.String
				}
			}
			Labels struct {
				Nodes []struct {
					ID    graphql.String
					Name  graphql.String
					Color graphql.String
				}
			}
			URL        graphql.String
			BranchName graphql.String
			ArchivedAt *graphql.String
			Parent     *struct {
				ID         graphql.String
				Identifier graphql.String
				Title      graphql.String
			}
			Children struct {
				Nodes []struct {
					ID         graphql.String
					Identifier graphql.String
					Title      graphql.String
					State      struct {
						ID   graphql.String
						Name graphql.String
					}
				}
			}
			Relations struct {
				Nodes []struct {
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
			} `graphql:"relations(first: 50)"`
			InverseRelations struct {
				Nodes []struct {
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
					User      struct {
						ID          graphql.String
						Name        graphql.String
						DisplayName graphql.String
						Email       graphql.String
						IsMe        graphql.Boolean
					}
				}
			} `graphql:"comments(first: 100, orderBy: createdAt)"`
		} `graphql:"issue(id: $id)"`
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

	issue := c.parseIssueNode(query.Issue)

	// Parse comments
	comments := make([]Comment, 0, len(query.Issue.Comments.Nodes))
	for _, node := range query.Issue.Comments.Nodes {
		commentCreatedAt := parseTime(string(node.CreatedAt))
		commentUpdatedAt := parseTime(string(node.UpdatedAt))
		comments = append(comments, Comment{
			ID:        string(node.ID),
			Body:      string(node.Body),
			CreatedAt: commentCreatedAt,
			UpdatedAt: commentUpdatedAt,
			Author: User{
				ID:          string(node.User.ID),
				Name:        string(node.User.Name),
				DisplayName: string(node.User.DisplayName),
				Email:       string(node.User.Email),
				IsMe:        bool(node.User.IsMe),
			},
			IssueID: string(query.Issue.ID),
		})
	}

	issue.Comments = comments
	return issue, nil
}
