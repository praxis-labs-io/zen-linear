package tui

import (
	"sort"
	"strings"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

// SortField values double as the API's orderBy argument.
type SortField string

const (
	SortByUpdatedAt SortField = "updatedAt"
	SortByCreatedAt SortField = "createdAt"
	SortByPriority  SortField = "priority"
	SortByStatus    SortField = "status"
)

const (
	sortNameStatus   = "status"
	sortNamePriority = "priority"
	sortNameUpdated  = "updated"
	sortNameCreated  = "created"
)

var sortFieldConfigNames = map[string]SortField{
	sortNameStatus:   SortByStatus,
	sortNamePriority: SortByPriority,
	sortNameUpdated:  SortByUpdatedAt,
	"updatedat":      SortByUpdatedAt,
	sortNameCreated:  SortByCreatedAt,
	"createdat":      SortByCreatedAt,
}

func parseSortFields(names []string) []SortField {
	fields := make([]SortField, 0, len(names))
	seen := make(map[SortField]bool, len(names))
	for _, name := range names {
		field, ok := sortFieldConfigNames[strings.ToLower(strings.TrimSpace(name))]
		if !ok || seen[field] {
			continue
		}
		seen[field] = true
		fields = append(fields, field)
	}
	if len(fields) == 0 {
		return []SortField{SortByUpdatedAt}
	}
	return fields
}

func sortFieldLabel(field SortField) string {
	switch field {
	case SortByStatus:
		return sortNameStatus
	case SortByPriority:
		return sortNamePriority
	case SortByCreatedAt:
		return sortNameCreated
	default:
		return sortNameUpdated
	}
}

func sortChainLabel(fields []SortField) string {
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		labels = append(labels, sortFieldLabel(field))
	}
	return strings.Join(labels, " → ")
}

var sortOrderings = [][]SortField{
	{SortByPriority},
	{SortByStatus, SortByPriority},
	{SortByPriority, SortByUpdatedAt},
	{SortByUpdatedAt},
	{SortByCreatedAt},
	{SortByStatus},
}

func sortOrderingLabel(fields []SortField) string {
	if len(fields) == 0 {
		return ""
	}
	label := sortFieldLabel(fields[0])
	label = strings.ToUpper(label[:1]) + label[1:]
	for _, field := range fields[1:] {
		label += ", then " + sortFieldLabel(field)
	}
	return label
}

func sortConfigNames(fields []SortField) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, sortFieldLabel(field))
	}
	return names
}

func sortOrderingID(fields []SortField) string {
	return strings.Join(sortConfigNames(fields), ",")
}

func sortOrderingPickerItems(configured []SortField) []PickerItem {
	items := make([]PickerItem, 0, len(sortOrderings)+1)
	if len(configured) > 0 && !isPresetOrdering(configured) {
		items = append(items, sortOrderingItem(configured))
	}
	for _, fields := range sortOrderings {
		items = append(items, sortOrderingItem(fields))
	}
	return items
}

func sortOrderingItem(fields []SortField) PickerItem {
	return PickerItem{ID: sortOrderingID(fields), Label: sortOrderingLabel(fields)}
}

func isPresetOrdering(fields []SortField) bool {
	id := sortOrderingID(fields)
	for _, preset := range sortOrderings {
		if sortOrderingID(preset) == id {
			return true
		}
	}
	return false
}

func compareIssues(field SortField, a, b linearapi.Issue) int {
	switch field {
	case SortByPriority:
		return comparePriority(a.Priority, b.Priority)
	case SortByStatus:
		if ra, rb := statusRank(a.State), statusRank(b.State); ra != rb {
			return ra - rb
		}
		return strings.Compare(a.State, b.State)
	case SortByCreatedAt:
		return b.CreatedAt.Compare(a.CreatedAt)
	default:
		return b.UpdatedAt.Compare(a.UpdatedAt)
	}
}

func comparePriority(a, b int) int {
	if a == 0 {
		a = 5
	}
	if b == 0 {
		b = 5
	}
	return a - b
}

func sortIssuesByFields(issues []linearapi.Issue, fields []SortField) {
	if len(fields) == 0 {
		return
	}
	sort.SliceStable(issues, func(i, j int) bool {
		for _, field := range fields {
			if result := compareIssues(field, issues[i], issues[j]); result != 0 {
				return result < 0
			}
		}
		return false
	})
}
