package linearapi

import (
	"context"
	"fmt"

	"github.com/shurcooL/graphql"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// CreateComment creates a new comment on an issue.
func (c *Client) CreateComment(ctx context.Context, input CreateCommentInput) (Comment, error) {
	var mutation struct {
		CommentCreate struct {
			Success graphql.Boolean
			Comment struct {
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
		} `graphql:"commentCreate(input: $input)"`
	}

	// Build input object
	commentInput := make(CommentCreateInput)
	commentInput["issueId"] = graphql.ID(input.IssueID)
	commentInput["body"] = graphql.String(input.Body)
	if input.ParentID != "" {
		commentInput["parentId"] = graphql.ID(input.ParentID)
	}

	variables := map[string]interface{}{
		"input": commentInput,
	}

	err := c.client.mutate(ctx, &mutation, variables)
	if err != nil {
		logger.ErrorWithErr(err, "linearapi.client: CreateComment failed issue_id=%s", input.IssueID)
		return Comment{}, fmt.Errorf("create comment: %w", err)
	}

	if !bool(mutation.CommentCreate.Success) {
		logger.Error("linearapi.client: CreateComment operation failed success=false issue_id=%s", input.IssueID)
		return Comment{}, fmt.Errorf("create comment: operation failed")
	}

	node := mutation.CommentCreate.Comment
	createdAt := parseTime(string(node.CreatedAt))
	updatedAt := parseTime(string(node.UpdatedAt))
	parentID := ""
	if node.ParentID != nil {
		parentID = string(*node.ParentID)
	}

	return Comment{
		ID:        string(node.ID),
		Body:      string(node.Body),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		ParentID:  parentID,
		URL:       string(node.URL),
		Author: User{
			ID:          string(node.User.ID),
			Name:        string(node.User.Name),
			DisplayName: string(node.User.DisplayName),
			Email:       string(node.User.Email),
			IsMe:        bool(node.User.IsMe),
		},
		IssueID: input.IssueID,
	}, nil
}
