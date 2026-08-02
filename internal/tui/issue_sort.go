package tui

import (
	"sort"
	"strings"

	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// Config names for the sort fields. The field values themselves double as the
// API's orderBy argument, so they stay camelCase while the config spells the
// timestamps "updated" and "created".
const (
	sortNameStatus   = "status"
	sortNamePriority = "priority"
	sortNameUpdated  = "updated"
	sortNameCreated  = "created"
)

// sortFieldConfigNames maps the config file's sort_by values onto sort fields.
var sortFieldConfigNames = map[string]SortField{
	sortNameStatus:   SortByStatus,
	sortNamePriority: SortByPriority,
	sortNameUpdated:  SortByUpdatedAt,
	"updatedat":      SortByUpdatedAt,
	sortNameCreated:  SortByCreatedAt,
	"createdat":      SortByCreatedAt,
}

// parseSortFields maps configured sort names onto a sort chain, dropping
// unknown and repeated entries. An empty result falls back to most recently
// updated.
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

// sortFieldLabel names a sort field for the status bar and pickers.
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

// sortChainLabel renders a chain as "status → priority".
func sortChainLabel(fields []SortField) string {
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		labels = append(labels, sortFieldLabel(field))
	}
	return strings.Join(labels, " → ")
}

// sortOrderings are the whole orderings the sort picker offers. Each row is a
// complete answer, so picking one takes a single keystroke.
var sortOrderings = [][]SortField{
	{SortByPriority},
	{SortByStatus, SortByPriority},
	{SortByPriority, SortByUpdatedAt},
	{SortByUpdatedAt},
	{SortByCreatedAt},
	{SortByStatus},
}

// sortOrderingLabel renders a chain as a menu row: "Status, then priority".
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

// sortConfigNames renders a chain in the names the config file uses.
func sortConfigNames(fields []SortField) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, sortFieldLabel(field))
	}
	return names
}

// sortOrderingID encodes a chain as picker item identity, in the same names
// the config file uses.
func sortOrderingID(fields []SortField) string {
	return strings.Join(sortConfigNames(fields), ",")
}

// sortOrderingPickerItems lists the orderings for the sort picker. A
// configured chain the menu does not already cover leads the list, so a
// hand-written sort_by stays one keystroke away after a detour.
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

// compareIssues orders two issues along one field: negative when a sorts
// first, positive when b does, zero when the field cannot separate them.
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

// comparePriority orders by Linear's priority semantics: urgent first, no
// priority last.
func comparePriority(a, b int) int {
	if a == 0 {
		a = 5
	}
	if b == 0 {
		b = 5
	}
	return a - b
}

// sortIssuesByFields sorts issues along a chain: the first field decides,
// later fields break ties.
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
