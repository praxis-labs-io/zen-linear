package linearapi

import (
	"strings"
)

// buildBaseIssueFilter builds the base issue filter without search terms.
func buildBaseIssueFilter(params FetchIssuesParams) IssueFilter {
	filter := make(IssueFilter)
	if len(params.IDs) > 0 {
		filter["id"] = map[string]interface{}{"in": params.IDs}
	}
	if params.TeamID != "" {
		filter["team"] = map[string]interface{}{"id": map[string]interface{}{"eq": params.TeamID}}
	}
	if params.ProjectID != "" {
		filter["project"] = map[string]interface{}{"id": map[string]interface{}{"eq": params.ProjectID}}
	}
	if params.StateID != "" {
		filter["state"] = map[string]interface{}{"id": map[string]interface{}{"eq": params.StateID}}
	} else if params.StateType != "" {
		filter["state"] = map[string]interface{}{"type": map[string]interface{}{"eq": params.StateType}}
	}
	if params.CycleID != "" {
		filter["cycle"] = map[string]interface{}{"id": map[string]interface{}{"eq": params.CycleID}}
	}
	return filter
}

// buildStructuredIssueFilter builds issue filters that can be passed alongside
// Linear's full-text search term.
func buildStructuredIssueFilter(params FetchIssuesParams) IssueFilter {
	filter := buildBaseIssueFilter(params)
	if params.AssigneeID != "" {
		filter["assignee"] = map[string]interface{}{"id": map[string]interface{}{"eq": params.AssigneeID}}
	}
	if params.ProjectMilestoneID != "" {
		filter["projectMilestone"] = map[string]interface{}{"id": map[string]interface{}{"eq": params.ProjectMilestoneID}}
	}
	if !params.DueDate.Empty() {
		filter["dueDate"] = buildDateComparator(params.DueDate)
	}
	if !params.Estimate.Empty() {
		filter["estimate"] = buildNumberComparator(params.Estimate)
	}
	if len(params.LabelIDs) > 0 {
		andFilters := make([]map[string]interface{}, 0, len(params.LabelIDs))
		for _, labelID := range params.LabelIDs {
			labelID = strings.TrimSpace(labelID)
			if labelID == "" {
				continue
			}
			andFilters = append(andFilters, map[string]interface{}{
				"labels": map[string]interface{}{
					"some": map[string]interface{}{
						"id": map[string]interface{}{"eq": labelID},
					},
				},
			})
		}
		appendIssueAndFilters(filter, andFilters...)
	}
	return filter
}

func buildDateComparator(dateFilter DateFilter) map[string]interface{} {
	comparator := make(map[string]interface{})
	if dateFilter.Eq != "" {
		comparator["eq"] = dateFilter.Eq
	}
	if dateFilter.GT != "" {
		comparator["gt"] = dateFilter.GT
	}
	if dateFilter.GTE != "" {
		comparator["gte"] = dateFilter.GTE
	}
	if dateFilter.LT != "" {
		comparator["lt"] = dateFilter.LT
	}
	if dateFilter.LTE != "" {
		comparator["lte"] = dateFilter.LTE
	}
	if dateFilter.Null != nil {
		comparator["null"] = *dateFilter.Null
	}
	return comparator
}

func buildNumberComparator(numberFilter NumberFilter) map[string]interface{} {
	comparator := make(map[string]interface{})
	if numberFilter.Eq != nil {
		comparator["eq"] = *numberFilter.Eq
	}
	if numberFilter.GT != nil {
		comparator["gt"] = *numberFilter.GT
	}
	if numberFilter.GTE != nil {
		comparator["gte"] = *numberFilter.GTE
	}
	if numberFilter.LT != nil {
		comparator["lt"] = *numberFilter.LT
	}
	if numberFilter.LTE != nil {
		comparator["lte"] = *numberFilter.LTE
	}
	if numberFilter.Null != nil {
		comparator["null"] = *numberFilter.Null
	}
	return comparator
}

func appendIssueAndFilters(filter IssueFilter, filters ...map[string]interface{}) {
	if len(filters) == 0 {
		return
	}
	existing, _ := filter["and"].([]map[string]interface{})
	existing = append(existing, filters...)
	filter["and"] = existing
}

// buildIssueFilter builds the GraphQL issue filter for the given params.
func buildIssueFilter(params FetchIssuesParams) IssueFilter {
	filter := buildStructuredIssueFilter(params)

	searchTerm := strings.TrimSpace(params.Search)
	if searchTerm == "" {
		return filter
	}

	terms := strings.Fields(searchTerm)
	if len(terms) == 1 {
		filter["or"] = buildSearchOrFilters(terms[0])
		return filter
	}

	// Require every term to match at least one field for free-text search.
	andFilters := make([]map[string]interface{}, 0, len(terms))
	for _, term := range terms {
		andFilters = append(andFilters, map[string]interface{}{
			"or": buildSearchOrFilters(term),
		})
	}
	appendIssueAndFilters(filter, andFilters...)
	return filter
}

// buildSearchOrFilters returns per-term OR filters for issue search.
// Note: identifier is not a filterable field in Linear's IssueFilter type,
// so we only filter by title and description.
func buildSearchOrFilters(term string) []map[string]interface{} {
	return []map[string]interface{}{
		{"title": map[string]interface{}{"containsIgnoreCase": term}},
		{"description": map[string]interface{}{"containsIgnoreCase": term}},
	}
}
