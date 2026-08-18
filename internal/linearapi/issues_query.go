package linearapi

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/shurcooL/graphql"
)

// FetchIssuesPage fetches a single page of issues with optional filtering and sorting.
// It returns pagination metadata to allow callers to continue fetching.
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

// FetchIssues fetches every page of issues for params, sorting by priority
// client-side when requested (the Linear API paginates only by created/updated
// time). FetchIssuesPage routes each page to the standard issues query, the
// searchIssues query, or a custom view, depending on params.
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

// searchIssuesPage fetches a single page of issues using Linear's searchIssues query.
func (c *Client) searchIssuesPage(ctx context.Context, params FetchIssuesParams, after *string) (IssuePage, error) {
	first := params.First
	if first <= 0 {
		first = 50
	}

	searchTerm := strings.TrimSpace(params.Search)
	// Linear's search term does the text matching, so the filter carries the rest.
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

// ViewPreferencesValues carries the display settings of a custom view.
// Values are Linear's raw strings; empty means the setting is unset.
type ViewPreferencesValues struct {
	IssueGrouping         string
	IssueSubGrouping      string
	ViewOrdering          string
	ViewOrderingDirection string
}

// viewPreferencesSelection is the shared GraphQL selection for one layer of
// view preferences.
type viewPreferencesSelection struct {
	IssueGrouping         graphql.String
	IssueSubGrouping      graphql.String
	ViewOrdering          graphql.String
	ViewOrderingDirection graphql.String
}

// FetchCustomViewPreferences fetches the effective display settings of a
// custom view. Linear's computed viewPreferencesValues drops layers (a
// subgrouping stored on the organization layer comes back "none"), so the
// user, organization, and computed layers are fetched and merged here, most
// specific first. Returns nil when the view has no preferences at all.
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

// IssueMatchesScope reports whether an issue is inside the scope the params
// describe, asking the server the same question the list query asks so a
// custom view's own filter is honored. The params' IDs field is ignored.
//
// The caller gets an error rather than a false when the check itself fails, so
// a failed check never removes a row the user is looking at.
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

	// A custom view keeps its filter server-side, so the id filter has to ride
	// along on the view's own connection. This is a separate selection from
	// customViewIssuesPage on purpose: if the schema rejects the argument here,
	// only the check breaks, not the list.
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

// customViewIssuesPage fetches a single page of a Linear custom view's issues.
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

	// Linear API only supports "createdAt" and "updatedAt" for
	// PaginationOrderBy; other sorts happen client-side.
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

// cycleRefNode is the cycle selection carried on an issue.
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

// projectMilestoneRefNode is the milestone selection carried on an issue. It is
// narrower than the one ListProjectMilestones uses.
type projectMilestoneRefNode struct {
	ID         graphql.String
	Name       graphql.String
	TargetDate *graphql.String
	Status     graphql.String
	Project    struct {
		ID graphql.String
	}
}

// issueQueryNode is the GraphQL selection for a single issue, shared by the
// filtered, search, and custom-view queries and by the issue mutations. The
// mutations return it so the TUI can splice the result into the list instead
// of refetching; a narrower selection there would quietly drop fields the list
// renders.
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

// fetchIssuesWithFilterPage fetches a single page of issues using the standard issues query.
func (c *Client) fetchIssuesWithFilterPage(ctx context.Context, params FetchIssuesParams, after *string) (IssuePage, error) {
	first := params.First
	if first <= 0 {
		first = 50
	}

	// Build filter.
	filter := buildIssueFilter(params)

	// Linear API only supports "createdAt" and "updatedAt" for PaginationOrderBy.
	orderBy := PaginationOrderBy(params.OrderBy)
	if orderBy != OrderByCreatedAt && orderBy != OrderByUpdatedAt {
		// "priority" and "status" sort client-side; fetch by updatedAt.
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

// sortByPriority sorts issues by priority.
// Linear priority: 0 = No priority, 1 = Urgent, 2 = High, 3 = Normal, 4 = Low.
// We sort with Urgent (1) first, then High (2), Normal (3), Low (4), and No priority (0) last.
func (c *Client) sortByPriority(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		pi, pj := issues[i].Priority, issues[j].Priority
		// Map 0 (no priority) to a high value so it sorts last
		if pi == 0 {
			pi = 5
		}
		if pj == 0 {
			pj = 5
		}
		return pi < pj
	})
}
