package linearapi

import (
	"context"
	"fmt"
	"sort"

	"github.com/shurcooL/graphql"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// ListProjects fetches all projects for a team.
func (c *Client) ListProjects(ctx context.Context, teamID string) ([]Project, error) {
	var query struct {
		Team struct {
			Projects struct {
				Nodes []struct {
					ID   graphql.String
					Name graphql.String
				}
			}
		} `graphql:"team(id: $teamId)"`
	}

	variables := map[string]interface{}{
		"teamId": graphql.String(teamID),
	}

	err := c.client.query(ctx, &query, variables)
	if err != nil {
		logger.ErrorWithErr(err, "linearapi.client: ListProjects failed team_id=%s", teamID)
		return nil, fmt.Errorf("list projects for team %s: %w", teamID, err)
	}

	projects := make([]Project, 0, len(query.Team.Projects.Nodes))
	for _, node := range query.Team.Projects.Nodes {
		projects = append(projects, Project{
			ID:     string(node.ID),
			Name:   string(node.Name),
			TeamID: teamID,
		})
	}

	return projects, nil
}

// ListProjectMilestones fetches all non-archived milestones for a project.
func (c *Client) ListProjectMilestones(ctx context.Context, projectID string) ([]ProjectMilestone, error) {
	var after *string
	milestones := make([]ProjectMilestone, 0)

	for {
		var query struct {
			ProjectMilestones struct {
				Nodes []struct {
					ID         graphql.String
					Name       graphql.String
					TargetDate *graphql.String
					Status     graphql.String
					SortOrder  graphql.Float
					Progress   graphql.Float
					Project    struct {
						ID graphql.String
					}
				}
				PageInfo struct {
					HasNextPage graphql.Boolean
					EndCursor   graphql.String
				}
			} `graphql:"projectMilestones(first: $first, after: $after, filter: $filter, includeArchived: $includeArchived)"`
		}

		var afterCursor *graphql.String
		if after != nil {
			cursor := graphql.String(*after)
			afterCursor = &cursor
		}

		filter := ProjectMilestoneFilter{
			"project": map[string]interface{}{"id": map[string]interface{}{"eq": projectID}},
		}
		variables := map[string]interface{}{
			"first":           graphql.Int(50),
			"after":           afterCursor,
			"filter":          filter,
			"includeArchived": graphql.Boolean(false),
		}

		if err := c.client.query(ctx, &query, variables); err != nil {
			logger.ErrorWithErr(err, "linearapi.client: ListProjectMilestones failed project_id=%s", projectID)
			return nil, fmt.Errorf("list project milestones for project %s: %w", projectID, err)
		}

		for _, node := range query.ProjectMilestones.Nodes {
			var targetDate *string
			if node.TargetDate != nil {
				value := string(*node.TargetDate)
				targetDate = &value
			}
			milestones = append(milestones, ProjectMilestone{
				ID:         string(node.ID),
				Name:       string(node.Name),
				ProjectID:  string(node.Project.ID),
				TargetDate: targetDate,
				Status:     string(node.Status),
				SortOrder:  float64(node.SortOrder),
				Progress:   float64(node.Progress),
			})
		}

		if !bool(query.ProjectMilestones.PageInfo.HasNextPage) {
			break
		}
		cursor := string(query.ProjectMilestones.PageInfo.EndCursor)
		after = &cursor
	}

	return milestones, nil
}

// ListCycles fetches all non-archived cycles for a team.
func (c *Client) ListCycles(ctx context.Context, teamID string) ([]Cycle, error) {
	var after *string
	cycles := make([]Cycle, 0)

	for {
		var query struct {
			Team struct {
				Cycles struct {
					Nodes []struct {
						ID          graphql.String
						Name        *graphql.String
						Number      graphql.Float
						Description *graphql.String
						StartsAt    graphql.String
						EndsAt      graphql.String
						IsActive    graphql.Boolean
						IsFuture    graphql.Boolean
						IsPast      graphql.Boolean
						IsNext      graphql.Boolean
						IsPrevious  graphql.Boolean
						CreatedAt   graphql.String
						UpdatedAt   graphql.String
						Team        struct {
							ID graphql.String
						}
					}
					PageInfo struct {
						HasNextPage graphql.Boolean
						EndCursor   graphql.String
					}
				} `graphql:"cycles(first: $first, after: $after, includeArchived: $includeArchived)"`
			} `graphql:"team(id: $teamId)"`
		}

		var afterCursor *graphql.String
		if after != nil {
			cursor := graphql.String(*after)
			afterCursor = &cursor
		}

		variables := map[string]interface{}{
			"teamId":          graphql.String(teamID),
			"first":           graphql.Int(50),
			"after":           afterCursor,
			"includeArchived": graphql.Boolean(false),
		}

		if err := c.client.query(ctx, &query, variables); err != nil {
			logger.ErrorWithErr(err, "linearapi.client: ListCycles failed team_id=%s", teamID)
			return nil, fmt.Errorf("list cycles for team %s: %w", teamID, err)
		}

		for _, node := range query.Team.Cycles.Nodes {
			name := ""
			if node.Name != nil {
				name = string(*node.Name)
			}
			description := ""
			if node.Description != nil {
				description = string(*node.Description)
			}
			cycles = append(cycles, Cycle{
				ID:          string(node.ID),
				Name:        name,
				Number:      int(node.Number),
				StartsAt:    parseTime(string(node.StartsAt)),
				EndsAt:      parseTime(string(node.EndsAt)),
				IsActive:    bool(node.IsActive),
				IsFuture:    bool(node.IsFuture),
				IsPast:      bool(node.IsPast),
				IsNext:      bool(node.IsNext),
				IsPrevious:  bool(node.IsPrevious),
				Description: description,
				TeamID:      string(node.Team.ID),
				CreatedAt:   parseTime(string(node.CreatedAt)),
				UpdatedAt:   parseTime(string(node.UpdatedAt)),
			})
		}

		if !bool(query.Team.Cycles.PageInfo.HasNextPage) {
			break
		}
		cursor := string(query.Team.Cycles.PageInfo.EndCursor)
		after = &cursor
	}

	return cycles, nil
}

// ListUsers fetches all users in a team.
func (c *Client) ListUsers(ctx context.Context, teamID string) ([]User, error) {
	var query struct {
		Team struct {
			Members struct {
				Nodes []struct {
					ID          graphql.String
					Name        graphql.String
					DisplayName graphql.String
					Email       graphql.String
					IsMe        graphql.Boolean
				}
			}
		} `graphql:"team(id: $teamId)"`
	}

	variables := map[string]interface{}{
		"teamId": graphql.String(teamID),
	}

	err := c.client.query(ctx, &query, variables)
	if err != nil {
		logger.ErrorWithErr(err, "linearapi.client: ListUsers failed team_id=%s", teamID)
		return nil, fmt.Errorf("list users for team %s: %w", teamID, err)
	}

	users := make([]User, 0, len(query.Team.Members.Nodes))
	for _, node := range query.Team.Members.Nodes {
		users = append(users, User{
			ID:          string(node.ID),
			Name:        string(node.Name),
			DisplayName: string(node.DisplayName),
			Email:       string(node.Email),
			IsMe:        bool(node.IsMe),
		})
	}

	return users, nil
}

// GetCurrentUser fetches the current authenticated user.
func (c *Client) GetCurrentUser(ctx context.Context) (User, error) {
	var query struct {
		Viewer struct {
			ID          graphql.String
			Name        graphql.String
			DisplayName graphql.String
			Email       graphql.String
		}
	}

	err := c.client.query(ctx, &query, nil)
	if err != nil {
		logger.ErrorWithErr(err, "linearapi.client: GetCurrentUser failed")
		return User{}, fmt.Errorf("get current user: %w", err)
	}

	return User{
		ID:          string(query.Viewer.ID),
		Name:        string(query.Viewer.Name),
		DisplayName: string(query.Viewer.DisplayName),
		Email:       string(query.Viewer.Email),
		IsMe:        true,
	}, nil
}

// ListWorkflowStates fetches all workflow states for a team.
func (c *Client) ListWorkflowStates(ctx context.Context, teamID string) ([]WorkflowState, error) {
	var query struct {
		Team struct {
			States struct {
				Nodes []struct {
					ID       graphql.String
					Name     graphql.String
					Type     graphql.String
					Position graphql.Float
				}
			}
		} `graphql:"team(id: $teamId)"`
	}

	variables := map[string]interface{}{
		"teamId": graphql.String(teamID),
	}

	err := c.client.query(ctx, &query, variables)
	if err != nil {
		logger.ErrorWithErr(err, "linearapi.client: ListWorkflowStates failed team_id=%s", teamID)
		return nil, fmt.Errorf("list workflow states for team %s: %w", teamID, err)
	}

	states := make([]WorkflowState, 0, len(query.Team.States.Nodes))
	for _, node := range query.Team.States.Nodes {
		states = append(states, WorkflowState{
			ID:       string(node.ID),
			Name:     string(node.Name),
			Type:     string(node.Type),
			Position: float64(node.Position),
			TeamID:   teamID,
		})
	}

	return states, nil
}

// ListWorkspaceLabels fetches all workspace-level labels (not scoped to a team).
func (c *Client) ListWorkspaceLabels(ctx context.Context) ([]IssueLabel, error) {
	var query struct {
		IssueLabels struct {
			Nodes []struct {
				ID    graphql.String
				Name  graphql.String
				Color graphql.String
			}
		} `graphql:"issueLabels(first: 250)"`
	}

	err := c.client.query(ctx, &query, nil)
	if err != nil {
		logger.ErrorWithErr(err, "linearapi.client: ListWorkspaceLabels failed")
		return nil, fmt.Errorf("list workspace labels: %w", err)
	}

	labels := make([]IssueLabel, 0, len(query.IssueLabels.Nodes))
	for _, node := range query.IssueLabels.Nodes {
		labels = append(labels, IssueLabel{
			ID:    string(node.ID),
			Name:  string(node.Name),
			Color: string(node.Color),
		})
	}

	return labels, nil
}

// ListTeamLabels fetches labels scoped to a specific team.
func (c *Client) ListTeamLabels(ctx context.Context, teamID string) ([]IssueLabel, error) {
	var query struct {
		Team struct {
			Labels struct {
				Nodes []struct {
					ID    graphql.String
					Name  graphql.String
					Color graphql.String
				}
			}
		} `graphql:"team(id: $teamId)"`
	}

	variables := map[string]interface{}{
		"teamId": graphql.String(teamID),
	}

	err := c.client.query(ctx, &query, variables)
	if err != nil {
		logger.ErrorWithErr(err, "linearapi.client: ListTeamLabels failed team_id=%s", teamID)
		return nil, fmt.Errorf("list team labels for team %s: %w", teamID, err)
	}

	labels := make([]IssueLabel, 0, len(query.Team.Labels.Nodes))
	for _, node := range query.Team.Labels.Nodes {
		labels = append(labels, IssueLabel{
			ID:    string(node.ID),
			Name:  string(node.Name),
			Color: string(node.Color),
		})
	}

	return labels, nil
}

// ListIssueLabels fetches both workspace and team labels, merges them, and returns a sorted list.
// Labels are de-duplicated by ID, with team labels taking precedence.
func (c *Client) ListIssueLabels(ctx context.Context, teamID string) ([]IssueLabel, error) {
	// Fetch workspace labels
	workspaceLabels, err := c.ListWorkspaceLabels(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch team labels
	teamLabels, err := c.ListTeamLabels(ctx, teamID)
	if err != nil {
		return nil, err
	}

	// Merge and de-duplicate by ID (team labels override workspace labels if same ID)
	labelMap := make(map[string]IssueLabel)
	for _, lbl := range workspaceLabels {
		labelMap[lbl.ID] = lbl
	}
	for _, lbl := range teamLabels {
		labelMap[lbl.ID] = lbl
	}

	// Convert to slice and sort by name
	labels := make([]IssueLabel, 0, len(labelMap))
	for _, lbl := range labelMap {
		labels = append(labels, lbl)
	}
	sort.Slice(labels, func(i, j int) bool {
		return labels[i].Name < labels[j].Name
	})

	return labels, nil
}
