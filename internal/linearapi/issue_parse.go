package linearapi

import "slices"

// toRef converts the cycle selection into a CycleRef, treating a missing id as
// no cycle.
func (n *cycleRefNode) toRef() *CycleRef {
	if n == nil || n.ID == "" {
		return nil
	}

	name := ""
	if n.Name != nil {
		name = string(*n.Name)
	}

	return &CycleRef{
		ID:         string(n.ID),
		Name:       name,
		Number:     int(n.Number),
		StartsAt:   parseTime(string(n.StartsAt)),
		EndsAt:     parseTime(string(n.EndsAt)),
		IsActive:   bool(n.IsActive),
		IsFuture:   bool(n.IsFuture),
		IsPast:     bool(n.IsPast),
		IsNext:     bool(n.IsNext),
		IsPrevious: bool(n.IsPrevious),
	}
}

// toRef converts the milestone selection into a ProjectMilestoneRef, treating a
// missing id as no milestone. SortOrder and Progress stay zero because no issue
// selection requests them, so anything ranking milestones by SortOrder ties.
func (n *projectMilestoneRefNode) toRef() *ProjectMilestoneRef {
	if n == nil || n.ID == "" {
		return nil
	}

	var targetDate *string
	if n.TargetDate != nil && *n.TargetDate != "" {
		value := string(*n.TargetDate)
		targetDate = &value
	}

	return &ProjectMilestoneRef{
		ID:         string(n.ID),
		Name:       string(n.Name),
		ProjectID:  string(n.Project.ID),
		TargetDate: targetDate,
		Status:     string(n.Status),
	}
}

// toRelation converts a relation node from the perspective of the issue that
// was fetched; inverse marks the ones that point back at it.
func (n issueRelationNode) toRelation(inverse bool) IssueRelation {
	return IssueRelation{
		ID:   string(n.ID),
		Type: string(n.Type),
		Issue: IssueRef{
			ID:         string(n.Issue.ID),
			Identifier: string(n.Issue.Identifier),
			Title:      string(n.Issue.Title),
		},
		RelatedIssue: IssueRef{
			ID:         string(n.RelatedIssue.ID),
			Identifier: string(n.RelatedIssue.Identifier),
			Title:      string(n.RelatedIssue.Title),
		},
		Inverse: inverse,
	}
}

// toIssue converts the shared issue selection into an Issue. Relations,
// Subscribers and Attachments are not part of this selection; issueDetailNode
// fills them.
func (n issueQueryNode) toIssue() Issue {
	issue := Issue{
		ID:               string(n.ID),
		Identifier:       string(n.Identifier),
		Title:            string(n.Title),
		State:            string(n.State.Name),
		StateID:          string(n.State.ID),
		Priority:         int(n.Priority),
		UpdatedAt:        parseTime(string(n.UpdatedAt)),
		CreatedAt:        parseTime(string(n.CreatedAt)),
		TeamID:           string(n.Team.ID),
		Cycle:            n.Cycle.toRef(),
		ProjectMilestone: n.ProjectMilestone.toRef(),
		URL:              string(n.URL),
		BranchName:       string(n.BranchName),
		Archived:         n.ArchivedAt != nil,
		Relations:        make([]IssueRelation, 0),
	}

	if n.Assignee != nil {
		issue.AssigneeID = string(n.Assignee.ID)
		issue.Assignee = string(n.Assignee.Name)
	}
	if n.Description != nil {
		issue.Description = string(*n.Description)
	}
	if n.Project != nil {
		issue.ProjectID = string(n.Project.ID)
		issue.ProjectName = string(n.Project.Name)
	}
	if n.DueDate != nil && *n.DueDate != "" {
		value := string(*n.DueDate)
		issue.DueDate = &value
	}
	if n.Estimate != nil {
		value := float64(*n.Estimate)
		issue.Estimate = &value
	}
	if n.Parent != nil {
		issue.Parent = &IssueRef{
			ID:         string(n.Parent.ID),
			Identifier: string(n.Parent.Identifier),
			Title:      string(n.Parent.Title),
		}
	}

	issue.Labels = make([]IssueLabel, 0, len(n.Labels.Nodes))
	for _, label := range n.Labels.Nodes {
		issue.Labels = append(issue.Labels, IssueLabel{
			ID:    string(label.ID),
			Name:  string(label.Name),
			Color: string(label.Color),
		})
	}

	issue.Children = make([]IssueChildRef, 0, len(n.Children.Nodes))
	for _, child := range n.Children.Nodes {
		issue.Children = append(issue.Children, IssueChildRef{
			ID:         string(child.ID),
			Identifier: string(child.Identifier),
			Title:      string(child.Title),
			State:      string(child.State.Name),
			StateID:    string(child.State.ID),
		})
	}

	return issue
}

// toIssue converts the detail selection, adding the connections the shared
// selection leaves out.
func (n issueDetailNode) toIssue() Issue {
	issue := n.issueQueryNode.toIssue()

	for _, relation := range n.Relations.Nodes {
		issue.Relations = append(issue.Relations, relation.toRelation(false))
	}
	for _, relation := range n.InverseRelations.Nodes {
		issue.Relations = append(issue.Relations, relation.toRelation(true))
	}

	issue.Subscribers = make([]User, 0, len(n.Subscribers.Nodes))
	for _, node := range n.Subscribers.Nodes {
		issue.Subscribers = append(issue.Subscribers, User{
			ID:          string(node.ID),
			Name:        string(node.Name),
			DisplayName: string(node.DisplayName),
			Email:       string(node.Email),
			IsMe:        bool(node.IsMe),
		})
	}

	issue.Attachments = make([]Attachment, 0, len(n.Attachments.Nodes))
	for _, node := range n.Attachments.Nodes {
		subtitle := ""
		if node.Subtitle != nil {
			subtitle = string(*node.Subtitle)
		}
		sourceType := ""
		if node.SourceType != nil {
			sourceType = string(*node.SourceType)
		}
		issue.Attachments = append(issue.Attachments, Attachment{
			ID:         string(node.ID),
			Title:      string(node.Title),
			Subtitle:   subtitle,
			URL:        string(node.URL),
			SourceType: sourceType,
			CreatedAt:  parseTime(string(node.CreatedAt)),
			UpdatedAt:  parseTime(string(node.UpdatedAt)),
		})
	}

	issue.Comments = make([]Comment, 0, len(n.Comments.Nodes))
	for _, node := range n.Comments.Nodes {
		issue.Comments = append(issue.Comments, Comment{
			ID:        string(node.ID),
			Body:      string(node.Body),
			CreatedAt: parseTime(string(node.CreatedAt)),
			UpdatedAt: parseTime(string(node.UpdatedAt)),
			Author: User{
				ID:          string(node.User.ID),
				Name:        string(node.User.Name),
				DisplayName: string(node.User.DisplayName),
				Email:       string(node.User.Email),
				IsMe:        bool(node.User.IsMe),
			},
			IssueID: string(n.ID),
		})
	}
	// A thread reads oldest first. The query takes them newest first on purpose:
	// with a cap of 100 that keeps the most recent hundred on a long issue,
	// where asking for them ascending would return the first hundred instead.
	slices.SortStableFunc(issue.Comments, func(a, b Comment) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	})

	return issue
}
