package tui

import (
	"reflect"
	"testing"
	"time"

	"github.com/Drucial/zen-linear/internal/linearapi"
)

func TestParseSortFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		names []string
		want  []SortField
	}{
		{
			name:  "config names map onto sort fields",
			names: []string{"status", "priority"},
			want:  []SortField{SortByStatus, SortByPriority},
		},
		{
			name:  "timestamps accept either spelling",
			names: []string{"updated", "createdAt"},
			want:  []SortField{SortByUpdatedAt, SortByCreatedAt},
		},
		{
			name:  "unknown and repeated entries drop out",
			names: []string{"labels", "priority", "Priority"},
			want:  []SortField{SortByPriority},
		},
		{
			name:  "nothing usable falls back to updated",
			names: []string{"labels"},
			want:  []SortField{SortByUpdatedAt},
		},
		{
			name:  "empty falls back to updated",
			names: nil,
			want:  []SortField{SortByUpdatedAt},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseSortFields(tc.names); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseSortFields(%v) = %v, want %v", tc.names, got, tc.want)
			}
		})
	}
}

func TestSortIssuesByFieldsBreaksTiesWithLaterFields(t *testing.T) {
	issues := []linearapi.Issue{
		{Identifier: "A", State: "Done", Priority: 1},
		{Identifier: "B", State: "In Progress", Priority: 0},
		{Identifier: "C", State: "In Progress", Priority: 2},
		{Identifier: "D", State: "In Progress", Priority: 1},
	}

	sortIssuesByFields(issues, []SortField{SortByStatus, SortByPriority})

	got := identifiers(issues)
	want := []string{"D", "C", "B", "A"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("status → priority order = %v, want %v", got, want)
	}
}

func TestSortIssuesByFieldsOrdersTimestampsNewestFirst(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	issues := []linearapi.Issue{
		{Identifier: "old", UpdatedAt: now.Add(-2 * time.Hour), CreatedAt: now},
		{Identifier: "new", UpdatedAt: now, CreatedAt: now.Add(-2 * time.Hour)},
	}

	sortIssuesByFields(issues, []SortField{SortByUpdatedAt})
	if got := identifiers(issues); !reflect.DeepEqual(got, []string{"new", "old"}) {
		t.Fatalf("updated order = %v, want [new old]", got)
	}

	sortIssuesByFields(issues, []SortField{SortByCreatedAt})
	if got := identifiers(issues); !reflect.DeepEqual(got, []string{"old", "new"}) {
		t.Fatalf("created order = %v, want [old new]", got)
	}
}

// TestSortOrderingPickerItemsOffersConfiguredChain guards the way back: a
// configured ordering the menu does not already list leads the rows, so a
// session detour does not strand it until restart.
func TestSortOrderingPickerItemsOffersConfiguredChain(t *testing.T) {
	items := sortOrderingPickerItems([]SortField{SortByStatus, SortByUpdatedAt})
	if len(items) != len(sortOrderings)+1 {
		t.Fatalf("item count = %d, want %d", len(items), len(sortOrderings)+1)
	}
	if items[0].Label != "Status, then updated" || items[0].ID != "status,updated" {
		t.Fatalf("first item = %+v, want the configured chain", items[0])
	}

	preset := sortOrderingPickerItems([]SortField{SortByPriority})
	if len(preset) != len(sortOrderings) {
		t.Fatalf("a configured chain already in the menu was duplicated: %d items", len(preset))
	}
}

func TestSortChainLabel(t *testing.T) {
	got := sortChainLabel([]SortField{SortByStatus, SortByPriority})
	if got != "status → priority" {
		t.Fatalf("sortChainLabel = %q, want %q", got, "status → priority")
	}
}

func identifiers(issues []linearapi.Issue) []string {
	ids := make([]string, 0, len(issues))
	for _, issue := range issues {
		ids = append(ids, issue.Identifier)
	}
	return ids
}
