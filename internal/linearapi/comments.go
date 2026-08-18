package linearapi

import (
	"context"
	"fmt"

	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/shurcooL/graphql"
)

// commentNode is one comment as a mutation hands it back. Create and update
// share it so a comment just written and one written an hour ago are the same
// shape on the way in.
//
// It carries no issue: the payload is asked for the comment alone, and the
// caller already names the issue it wrote to.
type commentNode struct {
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

// toComment converts the node, stamped with the issue the caller wrote to.
func (n commentNode) toComment(issueID string) Comment {
	parentID := ""
	if n.ParentID != nil {
		parentID = string(*n.ParentID)
	}
	return Comment{
		ID:        string(n.ID),
		Body:      string(n.Body),
		CreatedAt: parseTime(string(n.CreatedAt)),
		UpdatedAt: parseTime(string(n.UpdatedAt)),
		ParentID:  parentID,
		URL:       string(n.URL),
		Author: User{
			ID:          string(n.User.ID),
			Name:        string(n.User.Name),
			DisplayName: string(n.User.DisplayName),
			Email:       string(n.User.Email),
			IsMe:        bool(n.User.IsMe),
		},
		IssueID: issueID,
	}
}

// CreateComment creates a new comment on an issue.
func (c *Client) CreateComment(ctx context.Context, input CreateCommentInput) (Comment, error) {
	var mutation struct {
		CommentCreate struct {
			Success graphql.Boolean
			Comment commentNode
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

	return mutation.CommentCreate.Comment.toComment(input.IssueID), nil
}

// UpdateComment rewrites a comment's body and returns it as Linear recorded it,
// which is where the updatedAt behind the "edited" marker comes from.
func (c *Client) UpdateComment(ctx context.Context, input UpdateCommentInput) (Comment, error) {
	var mutation struct {
		CommentUpdate struct {
			Success graphql.Boolean
			Comment commentNode
		} `graphql:"commentUpdate(id: $id, input: $input)"`
	}

	commentInput := make(CommentUpdateInput)
	commentInput["body"] = graphql.String(input.Body)

	variables := map[string]interface{}{
		"id":    graphql.String(input.ID),
		"input": commentInput,
	}

	if err := c.client.mutate(ctx, &mutation, variables); err != nil {
		logger.ErrorWithErr(err, "linearapi.client: UpdateComment failed comment_id=%s", input.ID)
		return Comment{}, fmt.Errorf("update comment %s: %w", input.ID, err)
	}

	if !bool(mutation.CommentUpdate.Success) {
		logger.Error("linearapi.client: UpdateComment operation failed success=false comment_id=%s", input.ID)
		return Comment{}, fmt.Errorf("update comment %s: operation failed", input.ID)
	}

	return mutation.CommentUpdate.Comment.toComment(input.IssueID), nil
}

// DeleteComment removes a comment. Linear keeps the replies under a deleted
// parent, so a thread outlives its root.
//
// The payload carries nothing worth reading back: the comment is gone, and the
// error is the whole of the answer.
func (c *Client) DeleteComment(ctx context.Context, commentID string) error {
	var mutation struct {
		CommentDelete struct {
			Success graphql.Boolean
		} `graphql:"commentDelete(id: $id)"`
	}

	variables := map[string]interface{}{
		"id": graphql.String(commentID),
	}

	if err := c.client.mutate(ctx, &mutation, variables); err != nil {
		logger.ErrorWithErr(err, "linearapi.client: DeleteComment failed comment_id=%s", commentID)
		return fmt.Errorf("delete comment %s: %w", commentID, err)
	}

	if !bool(mutation.CommentDelete.Success) {
		logger.Error("linearapi.client: DeleteComment operation failed success=false comment_id=%s", commentID)
		return fmt.Errorf("delete comment %s: operation failed", commentID)
	}
	return nil
}
