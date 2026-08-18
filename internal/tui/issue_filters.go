package tui

import (
	"strings"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

// IssueFilters contains structured filters applied in addition to navigation.
type IssueFilters struct {
	AssigneeID   string
	AssigneeName string
	LabelIDs     []string
	LabelNames   []string
	StateID      string
	StateName    string
	ProjectID    string
	ProjectName  string
	CycleID      string
	CycleName    string
	DueDate      linearapi.DateFilter
	Estimate     linearapi.NumberFilter
}

func (f IssueFilters) Empty() bool {
	return f.AssigneeID == "" &&
		len(f.LabelIDs) == 0 &&
		f.StateID == "" &&
		f.ProjectID == "" &&
		f.CycleID == "" &&
		f.DueDate.Empty() &&
		f.Estimate.Empty()
}

func (f IssueFilters) Summary() string {
	parts := make([]string, 0, 8)
	if f.AssigneeID != "" {
		label := f.AssigneeName
		if label == "" {
			label = f.AssigneeID
		}
		parts = append(parts, "assignee="+label)
	}
	if len(f.LabelIDs) > 0 {
		labels := f.LabelNames
		if len(labels) == 0 {
			labels = f.LabelIDs
		}
		parts = append(parts, "labels="+strings.Join(labels, ","))
	}
	if f.StateID != "" {
		label := f.StateName
		if label == "" {
			label = f.StateID
		}
		parts = append(parts, "status="+label)
	}
	if f.ProjectID != "" {
		label := f.ProjectName
		if label == "" {
			label = f.ProjectID
		}
		parts = append(parts, "project="+label)
	}
	if f.CycleID != "" {
		label := f.CycleName
		if label == "" {
			label = f.CycleID
		}
		parts = append(parts, "cycle="+label)
	}
	if !f.DueDate.Empty() {
		parts = append(parts, "due="+formatDateFilterSummary(f.DueDate))
	}
	if !f.Estimate.Empty() {
		parts = append(parts, "estimate="+formatNumberFilterSummary(f.Estimate))
	}
	return strings.Join(parts, ", ")
}

func formatDateFilterSummary(filter linearapi.DateFilter) string {
	switch {
	case filter.Eq != "":
		return filter.Eq
	case filter.GTE != "":
		return ">=" + filter.GTE
	case filter.GT != "":
		return ">" + filter.GT
	case filter.LTE != "":
		return "<=" + filter.LTE
	case filter.LT != "":
		return "<" + filter.LT
	case filter.Null != nil && *filter.Null:
		return "none"
	case filter.Null != nil:
		return "set"
	default:
		return ""
	}
}

func formatNumberFilterSummary(filter linearapi.NumberFilter) string {
	switch {
	case filter.Eq != nil:
		return formatEstimate(filter.Eq)
	case filter.GTE != nil:
		return ">=" + formatEstimate(filter.GTE)
	case filter.GT != nil:
		return ">" + formatEstimate(filter.GT)
	case filter.LTE != nil:
		return "<=" + formatEstimate(filter.LTE)
	case filter.LT != nil:
		return "<" + formatEstimate(filter.LT)
	case filter.Null != nil && *filter.Null:
		return "none"
	case filter.Null != nil:
		return "set"
	default:
		return ""
	}
}
