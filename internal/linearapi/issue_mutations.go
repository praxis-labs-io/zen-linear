package linearapi

import (
	"context"
	"fmt"

	"github.com/shurcooL/graphql"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// maxPriority is Linear's highest priority value (0=None, 1=Urgent, 2=High, 3=Medium, 4=Low).
const maxPriority = 4

// priorityValue bounds-checks a priority before narrowing it to graphql.Int (an int32),
// so the API boundary rejects out-of-range values instead of trusting the caller.
func priorityValue(p int) (graphql.Int, error) {
	if p < 0 || p > maxPriority {
		return 0, fmt.Errorf("priority %d out of range [0,%d]", p, maxPriority)
	}
	return graphql.Int(p), nil
}

// CreateIssue creates a new issue.
func (c *Client) CreateIssue(ctx context.Context, input CreateIssueInput) (Issue, error) {
	var mutation struct {
		IssueCreate struct {
			Success graphql.Boolean
			Issue   issueQueryNode
		} `graphql:"issueCreate(input: $input)"`
	}

	// Build input object
	issueInput := make(IssueCreateInput)
	issueInput["teamId"] = graphql.ID(input.TeamID)
	issueInput["title"] = graphql.String(input.Title)
	if input.Description != "" {
		issueInput["description"] = graphql.String(input.Description)
	}
	if input.ProjectID != "" {
		issueInput["projectId"] = graphql.ID(input.ProjectID)
	}
	if input.StateID != "" {
		issueInput["stateId"] = graphql.ID(input.StateID)
	}
	if input.CycleID != "" {
		issueInput["cycleId"] = graphql.ID(input.CycleID)
	}
	if input.AssigneeID != "" {
		issueInput["assigneeId"] = graphql.ID(input.AssigneeID)
	}
	if input.Priority > 0 {
		priority, err := priorityValue(input.Priority)
		if err != nil {
			return Issue{}, fmt.Errorf("create issue: %w", err)
		}
		issueInput["priority"] = priority
	}
	if input.ParentID != "" {
		issueInput["parentId"] = graphql.ID(input.ParentID)
	}

	variables := map[string]interface{}{
		"input": issueInput,
	}

	err := c.client.mutate(ctx, &mutation, variables)
	if err != nil {
		logger.ErrorWithErr(err, "linearapi.client: CreateIssue failed")
		return Issue{}, fmt.Errorf("create issue: %w", err)
	}

	if !bool(mutation.IssueCreate.Success) {
		logger.Error("linearapi.client: CreateIssue operation failed success=false")
		return Issue{}, fmt.Errorf("create issue: operation failed")
	}

	return mutation.IssueCreate.Issue.toIssue(), nil
}

// UpdateIssue updates an existing issue.
func (c *Client) UpdateIssue(ctx context.Context, input UpdateIssueInput) (Issue, error) {
	var mutation struct {
		IssueUpdate struct {
			Success graphql.Boolean
			Issue   issueQueryNode
		} `graphql:"issueUpdate(id: $id, input: $input)"`
	}

	// Build input object with only provided fields
	issueInput := make(IssueUpdateInput)
	if input.Title != nil {
		issueInput["title"] = graphql.String(*input.Title)
	}
	if input.Description != nil {
		issueInput["description"] = graphql.String(*input.Description)
	}
	if input.StateID != nil {
		issueInput["stateId"] = graphql.ID(*input.StateID)
	}
	if input.CycleID != nil {
		if *input.CycleID == "" {
			issueInput["cycleId"] = (*graphql.ID)(nil)
		} else {
			issueInput["cycleId"] = graphql.ID(*input.CycleID)
		}
	}
	if input.AssigneeID != nil {
		if *input.AssigneeID == "" {
			// Unassign by passing null
			issueInput["assigneeId"] = (*graphql.ID)(nil)
		} else {
			issueInput["assigneeId"] = graphql.ID(*input.AssigneeID)
		}
	}
	if input.Priority != nil {
		priority, err := priorityValue(*input.Priority)
		if err != nil {
			return Issue{}, fmt.Errorf("update issue %s: %w", input.ID, err)
		}
		issueInput["priority"] = priority
	}
	if input.LabelIDs != nil {
		// Convert string slice to []graphql.ID for the GraphQL mutation
		labelIDs := make([]graphql.ID, len(*input.LabelIDs))
		for i, id := range *input.LabelIDs {
			labelIDs[i] = graphql.ID(id)
		}
		issueInput["labelIds"] = labelIDs
	}
	if input.ParentID != nil {
		if *input.ParentID == "" {
			// Remove parent by passing null
			issueInput["parentId"] = (*graphql.ID)(nil)
		} else {
			issueInput["parentId"] = graphql.ID(*input.ParentID)
		}
	}
	if input.DueDate != nil {
		if *input.DueDate == "" {
			issueInput["dueDate"] = (*graphql.String)(nil)
		} else {
			issueInput["dueDate"] = graphql.String(*input.DueDate)
		}
	}
	if input.ClearEstimate {
		issueInput["estimate"] = (*graphql.Float)(nil)
	} else if input.Estimate != nil {
		issueInput["estimate"] = graphql.Float(*input.Estimate)
	}
	if input.ProjectID != nil {
		if *input.ProjectID == "" {
			issueInput["projectId"] = (*graphql.ID)(nil)
		} else {
			issueInput["projectId"] = graphql.ID(*input.ProjectID)
		}
	}
	if input.ProjectMilestoneID != nil {
		if *input.ProjectMilestoneID == "" {
			issueInput["projectMilestoneId"] = (*graphql.ID)(nil)
		} else {
			issueInput["projectMilestoneId"] = graphql.ID(*input.ProjectMilestoneID)
		}
	}

	variables := map[string]interface{}{
		"id":    graphql.String(input.ID),
		"input": issueInput,
	}

	err := c.client.mutate(ctx, &mutation, variables)
	if err != nil {
		logger.ErrorWithErr(err, "linearapi.client: UpdateIssue failed issue_id=%s", input.ID)
		return Issue{}, fmt.Errorf("update issue %s: %w", input.ID, err)
	}

	if !bool(mutation.IssueUpdate.Success) {
		logger.Error("linearapi.client: UpdateIssue operation failed success=false issue_id=%s", input.ID)
		return Issue{}, fmt.Errorf("update issue %s: operation failed", input.ID)
	}

	return mutation.IssueUpdate.Issue.toIssue(), nil
}

// CreateIssueRelation creates a relation between two issues.
func (c *Client) CreateIssueRelation(ctx context.Context, input CreateIssueRelationInput) (IssueRelation, error) {
	var mutation struct {
		IssueRelationCreate struct {
			Success       graphql.Boolean
			IssueRelation struct {
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
		} `graphql:"issueRelationCreate(input: $input)"`
	}

	relationInput := IssueRelationCreateInput{
		"issueId":        graphql.String(input.IssueID),
		"relatedIssueId": graphql.String(input.RelatedIssueID),
		"type":           string(input.Type),
	}
	variables := map[string]interface{}{
		"input": relationInput,
	}

	if err := c.client.mutate(ctx, &mutation, variables); err != nil {
		logger.ErrorWithErr(err, "linearapi.client: CreateIssueRelation failed issue_id=%s related_issue_id=%s", input.IssueID, input.RelatedIssueID)
		return IssueRelation{}, fmt.Errorf("create issue relation: %w", err)
	}
	if !bool(mutation.IssueRelationCreate.Success) {
		return IssueRelation{}, fmt.Errorf("create issue relation: operation failed")
	}

	node := mutation.IssueRelationCreate.IssueRelation
	return IssueRelation{
		ID:   string(node.ID),
		Type: string(node.Type),
		Issue: IssueRef{
			ID:         string(node.Issue.ID),
			Identifier: string(node.Issue.Identifier),
			Title:      string(node.Issue.Title),
		},
		RelatedIssue: IssueRef{
			ID:         string(node.RelatedIssue.ID),
			Identifier: string(node.RelatedIssue.Identifier),
			Title:      string(node.RelatedIssue.Title),
		},
	}, nil
}

// DeleteIssueRelation deletes an issue relation.
func (c *Client) DeleteIssueRelation(ctx context.Context, relationID string) error {
	var mutation struct {
		IssueRelationDelete struct {
			Success graphql.Boolean
		} `graphql:"issueRelationDelete(id: $id)"`
	}

	variables := map[string]interface{}{
		"id": graphql.String(relationID),
	}
	if err := c.client.mutate(ctx, &mutation, variables); err != nil {
		logger.ErrorWithErr(err, "linearapi.client: DeleteIssueRelation failed relation_id=%s", relationID)
		return fmt.Errorf("delete issue relation %s: %w", relationID, err)
	}
	if !bool(mutation.IssueRelationDelete.Success) {
		return fmt.Errorf("delete issue relation %s: operation failed", relationID)
	}
	return nil
}

// SubscribeToIssue subscribes the current user to an issue.
func (c *Client) SubscribeToIssue(ctx context.Context, issueID string) (Issue, error) {
	return c.setIssueSubscription(ctx, issueID, true)
}

// UnsubscribeFromIssue unsubscribes the current user from an issue.
func (c *Client) UnsubscribeFromIssue(ctx context.Context, issueID string) (Issue, error) {
	return c.setIssueSubscription(ctx, issueID, false)
}

func (c *Client) setIssueSubscription(ctx context.Context, issueID string, subscribe bool) (Issue, error) {
	if subscribe {
		var mutation struct {
			IssueSubscribe struct {
				Success graphql.Boolean
				Issue   issueQueryNode
			} `graphql:"issueSubscribe(id: $id)"`
		}
		variables := map[string]interface{}{"id": graphql.String(issueID)}
		if err := c.client.mutate(ctx, &mutation, variables); err != nil {
			return Issue{}, fmt.Errorf("subscribe to issue %s: %w", issueID, err)
		}
		if !bool(mutation.IssueSubscribe.Success) {
			return Issue{}, fmt.Errorf("subscribe to issue %s: operation failed", issueID)
		}
		return mutation.IssueSubscribe.Issue.toIssue(), nil
	}

	var mutation struct {
		IssueUnsubscribe struct {
			Success graphql.Boolean
			Issue   issueQueryNode
		} `graphql:"issueUnsubscribe(id: $id)"`
	}
	variables := map[string]interface{}{"id": graphql.String(issueID)}
	if err := c.client.mutate(ctx, &mutation, variables); err != nil {
		return Issue{}, fmt.Errorf("unsubscribe from issue %s: %w", issueID, err)
	}
	if !bool(mutation.IssueUnsubscribe.Success) {
		return Issue{}, fmt.Errorf("unsubscribe from issue %s: operation failed", issueID)
	}
	return mutation.IssueUnsubscribe.Issue.toIssue(), nil
}

// ArchiveIssue archives an issue.
func (c *Client) ArchiveIssue(ctx context.Context, issueID string) error {
	var mutation struct {
		IssueArchive struct {
			Success graphql.Boolean
		} `graphql:"issueArchive(id: $id)"`
	}

	variables := map[string]interface{}{
		"id": graphql.String(issueID),
	}

	err := c.client.mutate(ctx, &mutation, variables)
	if err != nil {
		logger.ErrorWithErr(err, "linearapi.client: ArchiveIssue failed issue_id=%s", issueID)
		return fmt.Errorf("archive issue %s: %w", issueID, err)
	}

	if !bool(mutation.IssueArchive.Success) {
		logger.Error("linearapi.client: ArchiveIssue operation failed success=false issue_id=%s", issueID)
		return fmt.Errorf("archive issue %s: operation failed", issueID)
	}

	return nil
}

// UnarchiveIssue unarchives an issue.
func (c *Client) UnarchiveIssue(ctx context.Context, issueID string) error {
	var mutation struct {
		IssueUnarchive struct {
			Success graphql.Boolean
		} `graphql:"issueUnarchive(id: $id)"`
	}

	variables := map[string]interface{}{
		"id": graphql.String(issueID),
	}

	err := c.client.mutate(ctx, &mutation, variables)
	if err != nil {
		logger.ErrorWithErr(err, "linearapi.client: UnarchiveIssue failed issue_id=%s", issueID)
		return fmt.Errorf("unarchive issue %s: %w", issueID, err)
	}

	if !bool(mutation.IssueUnarchive.Success) {
		logger.Error("linearapi.client: UnarchiveIssue operation failed success=false issue_id=%s", issueID)
		return fmt.Errorf("unarchive issue %s: operation failed", issueID)
	}

	return nil
}
