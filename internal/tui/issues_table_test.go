package tui

import (
	"testing"
	"time"

	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

func TestRenderIssueRow(t *testing.T) {
	tests := []struct {
		name      string
		issue     linearapi.Issue
		wantLen   int
		wantID    string
		wantState string
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
			},
			wantLen:   5,
			wantID:    "LIN-1",
			wantState: "Todo",
		},
		{
			name: "unassigned issue",
			issue: linearapi.Issue{
				ID:         "test-2",
				Identifier: "LIN-2",
				Title:      "Another Issue",
				State:      "In Progress",
				Assignee:   "",
				Priority:   2, // High priority
			},
			wantLen:   5,
			wantID:    "LIN-2",
			wantState: "In Progres", // truncated to 10 chars
		},
		{
			name: "long identifier truncated",
			issue: linearapi.Issue{
				ID:         "test-3",
				Identifier: "VERY-LONG-IDENTIFIER-123",
				Title:      "Long ID Issue",
				State:      "Done",
				Assignee:   "Jane",
				Priority:   1, // Urgent priority
			},
			wantLen:   5,
			wantID:    "VERY-LONG-", // truncated to 10 chars
			wantState: "Done",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := renderIssueRow(tt.issue)
			if len(row) != tt.wantLen {
				t.Errorf("renderIssueRow() length = %d, want %d", len(row), tt.wantLen)
			}
			if len(row) > 0 && row[0] != tt.wantID {
				t.Errorf("renderIssueRow()[0] = %q, want %q", row[0], tt.wantID)
			}
			if len(row) > 1 && row[1] != tt.wantState {
				t.Errorf("renderIssueRow()[1] = %q, want %q", row[1], tt.wantState)
			}
			// Column 2 is now Priority
			if len(row) > 3 && tt.issue.Assignee == "" && row[3] != "Unassigned" {
				t.Errorf("renderIssueRow()[3] = %q, want %q", row[3], "Unassigned")
			}
		})
	}
}

func TestRenderIssueRow_Truncation(t *testing.T) {
	issue := linearapi.Issue{
		ID:         "test",
		Identifier: "ABCDEFGHIJKLMNOP", // 16 chars
		Title:      "Test",
		State:      "ABCDEFGHIJKLMNOP", // 16 chars
		Assignee:   "ABCDEFGHIJKLMNOP", // 16 chars
		Priority:   1,
		UpdatedAt:  time.Now(),
	}

	row := renderIssueRow(issue)

	// Identifier should be truncated to 10 chars
	if len(row[0]) > 10 {
		t.Errorf("Identifier length = %d, want <= 10", len(row[0]))
	}

	// State should be truncated to 10 chars
	if len(row[1]) > 10 {
		t.Errorf("State length = %d, want <= 10", len(row[1]))
	}

	// Priority is column 2 (no truncation needed for formatted priority)
	// Assignee should be truncated to 10 chars (now column 3)
	if len(row[3]) > 10 {
		t.Errorf("Assignee length = %d, want <= 10", len(row[3]))
	}
}
