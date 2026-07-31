package tui

import (
	"testing"
	"time"

	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

// Column order matches Linear's list view: priority, id, state, title,
// labels, assignee, updated.
const (
	rowColPriority = iota
	rowColID
	rowColState
	rowColTitle
	rowColLabels
	rowColAssignee
	rowColUpdated
)

func TestRenderIssueRow(t *testing.T) {
	tests := []struct {
		name         string
		issue        linearapi.Issue
		wantID       string
		wantState    string
		wantPriority string
		wantAssignee string
		wantLabels   string
	}{
		{
			name: "normal issue",
			issue: linearapi.Issue{
				ID:         "test-1",
				Identifier: "LIN-1",
				Title:      "Test Issue",
				State:      "Todo",
				Assignee:   "John Doe",
				Priority:   3, // Normal priority
				Labels:     []linearapi.IssueLabel{{Name: "Bug"}, {Name: "UI"}},
			},
			wantID:       "LIN-1",
			wantState:    "○",
			wantPriority: "=",
			wantAssignee: "John Doe",
			wantLabels:   "Bug, UI",
		},
		{
			name: "unassigned urgent issue",
			issue: linearapi.Issue{
				ID:         "test-2",
				Identifier: "LIN-2",
				Title:      "Another Issue",
				State:      "In Progress",
				Assignee:   "",
				Priority:   1, // Urgent priority
			},
			wantID:       "LIN-2",
			wantState:    "◉",
			wantPriority: "▲",
			wantAssignee: "-",
			wantLabels:   "-",
		},
		{
			name: "long identifier truncated",
			issue: linearapi.Issue{
				ID:         "test-3",
				Identifier: "VERY-LONG-IDENTIFIER-123",
				Title:      "Long ID Issue",
				State:      "Done",
				Assignee:   "Jane",
				Priority:   0, // No priority
			},
			wantID:       "VERY-LONG-", // truncated to 10 chars
			wantState:    "●",
			wantPriority: "-",
			wantAssignee: "Jane",
			wantLabels:   "-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := renderIssueRow(tt.issue)
			if len(row) != 7 {
				t.Fatalf("renderIssueRow() length = %d, want 7", len(row))
			}
			if row[rowColPriority] != tt.wantPriority {
				t.Errorf("priority = %q, want %q", row[rowColPriority], tt.wantPriority)
			}
			if row[rowColID] != tt.wantID {
				t.Errorf("id = %q, want %q", row[rowColID], tt.wantID)
			}
			if row[rowColState] != tt.wantState {
				t.Errorf("state = %q, want %q", row[rowColState], tt.wantState)
			}
			if row[rowColTitle] != tt.issue.Title {
				t.Errorf("title = %q, want %q", row[rowColTitle], tt.issue.Title)
			}
			if row[rowColLabels] != tt.wantLabels {
				t.Errorf("labels = %q, want %q", row[rowColLabels], tt.wantLabels)
			}
			if row[rowColAssignee] != tt.wantAssignee {
				t.Errorf("assignee = %q, want %q", row[rowColAssignee], tt.wantAssignee)
			}
		})
	}
}

func TestRenderIssueRow_Truncation(t *testing.T) {
	issue := linearapi.Issue{
		ID:         "test",
		Identifier: "ABCDEFGHIJKLMNOP", // 16 chars
		Title:      "Test",
		State:      "In Progress",
		Assignee:   "ABCDEFGHIJKLMNOP", // 16 chars
		Priority:   1,
		UpdatedAt:  time.Now(),
	}

	row := renderIssueRow(issue)

	if len(row[rowColID]) > 10 {
		t.Errorf("Identifier length = %d, want <= 10", len(row[rowColID]))
	}
	if len(row[rowColAssignee]) > 14 {
		t.Errorf("Assignee length = %d, want <= 14", len(row[rowColAssignee]))
	}
}

func TestFormatUpdatedAt(t *testing.T) {
	if got := formatUpdatedAt(time.Time{}); got != "-" {
		t.Errorf("formatUpdatedAt(zero) = %q, want -", got)
	}
	now := time.Date(time.Now().Year(), time.July, 28, 12, 0, 0, 0, time.UTC)
	if got := formatUpdatedAt(now); got != "Jul 28" {
		t.Errorf("formatUpdatedAt(current year) = %q, want Jul 28", got)
	}
	old := time.Date(2023, time.December, 3, 12, 0, 0, 0, time.UTC)
	if got := formatUpdatedAt(old); got != "Dec 2023" {
		t.Errorf("formatUpdatedAt(old year) = %q, want Dec 2023", got)
	}
}
