package linearapi

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/shurcooL/graphql"
)

func (c *Client) FetchIssuesPage(ctx context.Context, params FetchIssuesParams, after *string) (IssuePage, error) {
	if params.CustomViewID != "" {
		return c.customViewIssuesPage(ctx, params, after)
	}

	searchTerm := strings.TrimSpace(params.Search)
	if searchTerm != "" {
		params.Search = searchTerm
		return c.searchIssuesPage(ctx, params, after)
	}

	return c.fetchIssuesWithFilterPage(ctx, params, after)
}

func (c *Client) FetchIssues(ctx context.Context, params FetchIssuesParams) ([]Issue, error) {
	sortByPriority := params.OrderBy == "priority"

	var after *string
	page := 0
	issues := make([]Issue, 0)
	for {
		pageResult, err := c.FetchIssuesPage(ctx, params, after)
		if err != nil {
			return nil, err
		}

		issues = append(issues, pageResult.Issues...)
		page++
		if params.OnProgress != nil {
			params.OnProgress(IssueFetchProgress{
				Page:    page,
				Fetched: len(issues),
			})
		}

		if !pageResult.HasNext {
			break
		}
		after = pageResult.EndCursor
	}

	if sortByPriority {
		c.sortByPriority(issues)
	}

	return issues, nil
}

func (c *Client) searchIssuesPage(ctx context.Context, params FetchIssuesParams, after *string) (IssuePage, error) {
	first := params.First
	if first <= 0 {
		first = 50
	}

	searchTerm := strings.TrimSpace(params.Search)
	filter := buildStructuredIssueFilter(params)

	var afterCursor *graphql.String
	if after != nil {
		cursor := graphql.String(*after)
		afterCursor = &cursor
	}

	var query struct {
		SearchIssues struct {
			Nodes    []issueQueryNode
			PageInfo struct {
				HasNextPage graphql.Boolean
				EndCursor   graphql.String
			}
		} `graphql:"searchIssues(term: $term, first: $first, after: $after, filter: $filter)"`
	}

	variables := map[string]interface{}{
		"term":   graphql.String(searchTerm),
		"first":  graphql.Int(first),
		"filter": filter,
		"after":  afterCursor,
	}

	err := c.client.query(ctx, &query, variables)
	if err != nil {
		logger.ErrorWithErr(err, "linearapi.client: searchIssues failed")
		return IssuePage{}, fmt.Errorf("search issues: %w", err)
	}

	issues := make([]Issue, 0, len(query.SearchIssues.Nodes))
	for _, node := range query.SearchIssues.Nodes {
		issue := node.toIssue()
		issues = append(issues, issue)
	}

	hasNext := bool(query.SearchIssues.PageInfo.HasNextPage)
	var endCursor *string
	if hasNext {
		cursor := string(query.SearchIssues.PageInfo.EndCursor)
		endCursor = &cursor
	}

	return IssuePage{
		Issues:    issues,
		HasNext:   hasNext,
		EndCursor: endCursor,
	}, nil
}

type ViewPreferencesValues struct {
	IssueGrouping         string
	IssueSubGrouping      string
	ViewOrdering          string
	ViewOrderingDirection string
}

type viewPreferencesSelection struct {
	IssueGrouping         graphql.String
	IssueSubGrouping      graphql.String
	ViewOrdering          graphql.String
	ViewOrderingDirection graphql.String
}

// FetchCustomViewPreferences returns a view's display settings merged from its
// user, organization and computed layers, or nil when it has none.
func (c *Client) FetchCustomViewPreferences(ctx context.Context, viewID string) (*ViewPreferencesValues, error) {
	var query struct {
		CustomView struct {
			UserViewPreferences *struct {
				Preferences viewPreferencesSelection
			}
			OrganizationViewPreferences *struct {
				Preferences viewPreferencesSelection
			}
			ViewPreferencesValues *viewPreferencesSelection
		} `graphql:"customView(id: $id)"`
	}

	variables := map[string]interface{}{
		"id": graphql.String(viewID),
	}

	if err := c.client.query(ctx, &query, variables); err != nil {
		logger.ErrorWithErr(err, "linearapi.client: fetchCustomViewPreferences failed view_id=%s", viewID)
		return nil, fmt.Errorf("fetch custom view preferences: %w", err)
	}

	var layers []viewPreferencesSelection
	if query.CustomView.UserViewPreferences != nil {
		layers = append(layers, query.CustomView.UserViewPreferences.Preferences)
	}
	if query.CustomView.OrganizationViewPreferences != nil {
		layers = append(layers, query.CustomView.OrganizationViewPreferences.Preferences)
	}
	if query.CustomView.ViewPreferencesValues != nil {
		layers = append(layers, *query.CustomView.ViewPreferencesValues)
	}
	if len(layers) == 0 {
		return nil, nil
	}

	firstSet := func(pick func(viewPreferencesSelection) graphql.String) string {
		for _, layer := range layers {
			if value := string(pick(layer)); value != "" {
				return value
			}
		}
		return ""
	}
	return &ViewPreferencesValues{
		IssueGrouping:         firstSet(func(l viewPreferencesSelection) graphql.String { return l.IssueGrouping }),
		IssueSubGrouping:      firstSet(func(l viewPreferencesSelection) graphql.String { return l.IssueSubGrouping }),
		ViewOrdering:          firstSet(func(l viewPreferencesSelection) graphql.String { return l.ViewOrdering }),
		ViewOrderingDirection: firstSet(func(l viewPreferencesSelection) graphql.String { return l.ViewOrderingDirection }),
	}, nil
}

// IssueMatchesScope asks the server whether issueID is inside the scope params
// describe, ignoring params.IDs. A failed check is an error, never false.
func (c *Client) IssueMatchesScope(ctx context.Context, params FetchIssuesParams, issueID string) (bool, error) {
	if issueID == "" {
		return false, fmt.Errorf("issue id is required")
	}
	params.IDs = []string{issueID}
	params.First = 1
	params.OnProgress = nil

	if params.CustomViewID == "" {
		page, err := c.fetchIssuesWithFilterPage(ctx, params, nil)
		if err != nil {
			return false, err
		}
		return len(page.Issues) > 0, nil
	}

	var query struct {
		CustomView struct {
			Issues struct {
				Nodes []struct {
					ID graphql.String
				}
			} `graphql:"issues(first: $first, filter: $filter)"`
		} `graphql:"customView(id: $id)"`
	}
	variables := map[string]interface{}{
		"id":     graphql.String(params.CustomViewID),
		"first":  graphql.Int(1),
		"filter": IssueFilter{"id": map[string]interface{}{"in": params.IDs}},
	}
	if err := c.client.query(ctx, &query, variables); err != nil {
		logger.ErrorWithErr(err, "linearapi.client: IssueMatchesScope failed view_id=%s issue_id=%s", params.CustomViewID, issueID)
		return false, fmt.Errorf("check issue %s against view %s: %w", issueID, params.CustomViewID, err)
	}
	return len(query.CustomView.Issues.Nodes) > 0, nil
}

func (c *Client) customViewIssuesPage(ctx context.Context, params FetchIssuesParams, after *string) (IssuePage, error) {
	first := params.First
	if first <= 0 {
		first = 50
	}

	var afterCursor *graphql.String
	if after != nil {
		cursor := graphql.String(*after)
		afterCursor = &cursor
	}

	orderBy := PaginationOrderBy(params.OrderBy)
	if orderBy != OrderByCreatedAt && orderBy != OrderByUpdatedAt {
		orderBy = OrderByUpdatedAt
	}

	var query struct {
		CustomView struct {
			Issues struct {
				Nodes    []issueQueryNode
				PageInfo struct {
					HasNextPage graphql.Boolean
					EndCursor   graphql.String
				}
			} `graphql:"issues(first: $first, after: $after, orderBy: $orderBy)"`
		} `graphql:"customView(id: $id)"`
	}

	variables := map[string]interface{}{
		"id":      graphql.String(params.CustomViewID),
		"first":   graphql.Int(first),
		"after":   afterCursor,
		"orderBy": orderBy,
	}

	if err := c.client.query(ctx, &query, variables); err != nil {
		logger.ErrorWithErr(err, "linearapi.client: customViewIssuesPage failed view_id=%s", params.CustomViewID)
		return IssuePage{}, fmt.Errorf("fetch custom view issues: %w", err)
	}

	issues := make([]Issue, 0, len(query.CustomView.Issues.Nodes))
	for _, node := range query.CustomView.Issues.Nodes {
		issues = append(issues, node.toIssue())
	}

	hasNext := bool(query.CustomView.Issues.PageInfo.HasNextPage)
	var endCursor *string
	if hasNext {
		cursor := string(query.CustomView.Issues.PageInfo.EndCursor)
		endCursor = &cursor
	}

	return IssuePage{
		Issues:    issues,
		HasNext:   hasNext,
		EndCursor: endCursor,
	}, nil
}

type cycleRefNode struct {
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

type projectMilestoneRefNode struct {
	ID         graphql.String
	Name       graphql.String
	TargetDate *graphql.String
	Status     graphql.String
	Project    struct {
		ID graphql.String
	}
}

type issueQueryNode struct {
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
	Cycle            *cycleRefNode
	DueDate          *graphql.String
	Estimate         *graphql.Float
	ProjectMilestone *projectMilestoneRefNode
	Labels           struct {
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
}

func (c *Client) fetchIssuesWithFilterPage(ctx context.Context, params FetchIssuesParams, after *string) (IssuePage, error) {
	first := params.First
	if first <= 0 {
		first = 50
	}

	filter := buildIssueFilter(params)

	orderBy := PaginationOrderBy(params.OrderBy)
	if orderBy != OrderByCreatedAt && orderBy != OrderByUpdatedAt {
		orderBy = OrderByUpdatedAt
	}

	var afterCursor *graphql.String
	if after != nil {
		cursor := graphql.String(*after)
		afterCursor = &cursor
	}

	var query struct {
		Issues struct {
			Nodes    []issueQueryNode
			PageInfo struct {
				HasNextPage graphql.Boolean
				EndCursor   graphql.String
			}
		} `graphql:"issues(first: $first, after: $after, filter: $filter, orderBy: $orderBy)"`
	}

	variables := map[string]interface{}{
		"first":   graphql.Int(first),
		"filter":  filter,
		"orderBy": orderBy,
		"after":   afterCursor,
	}

	err := c.client.query(ctx, &query, variables)
	if err != nil {
		logger.ErrorWithErr(err, "linearapi.client: FetchIssues failed")
		return IssuePage{}, fmt.Errorf("fetch issues: %w", err)
	}

	issues := make([]Issue, 0, len(query.Issues.Nodes))
	for _, node := range query.Issues.Nodes {
		issue := node.toIssue()
		issues = append(issues, issue)
	}

	hasNext := bool(query.Issues.PageInfo.HasNextPage)
	var endCursor *string
	if hasNext {
		cursor := string(query.Issues.PageInfo.EndCursor)
		endCursor = &cursor
	}

	return IssuePage{
		Issues:    issues,
		HasNext:   hasNext,
		EndCursor: endCursor,
	}, nil
}

func (c *Client) sortByPriority(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		pi, pj := issues[i].Priority, issues[j].Priority
		if pi == 0 {
			pi = 5
		}
		if pj == 0 {
			pj = 5
		}
		return pi < pj
	})
}
