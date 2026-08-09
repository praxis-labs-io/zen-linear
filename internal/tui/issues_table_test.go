package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
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
			wantAssignee: "JD",
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
			wantState:    "⊙",
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
			wantAssignee: "J",
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
	if row[rowColAssignee] != "A" {
		t.Errorf("Assignee = %q, want the initial A", row[rowColAssignee])
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

func TestBuildFlatSearchRowsPreservesOrder(t *testing.T) {
	parent := linearapi.Issue{ID: "parent", Identifier: "ABC-1", Title: "Parent", State: "Todo"}
	child := linearapi.Issue{
		ID: "child", Identifier: "ABC-2", Title: "Child", State: "Todo",
		Parent: &linearapi.IssueRef{ID: "parent"},
	}
	// Relevance order puts the child first; flat rows must keep it there
	// instead of re-nesting it under its parent.
	rows, idToIssue := buildFlatSearchRows([]linearapi.Issue{child, parent})

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].IssueID != "child" || rows[1].IssueID != "parent" {
		t.Fatalf("row order = %q, %q; want child, parent", rows[0].IssueID, rows[1].IssueID)
	}
	for _, row := range rows {
		if row.IsHeader || row.Level != 0 || row.HasChildren {
			t.Fatalf("row %q not flat: %+v", row.IssueID, row)
		}
	}
	if idToIssue["child"] == nil || idToIssue["parent"] == nil {
		t.Fatal("idToIssue missing entries")
	}
}

// TestFormatStateIcon pins one distinct icon and color per lifecycle state.
// Triage used to fall through to the Todo default and render identically.
func TestFormatStateIcon(t *testing.T) {
	tests := []struct {
		state     string
		wantIcon  string
		wantColor tcell.Color
	}{
		{"Triage", "◎", LinearTheme.StatusTriage},
		{"Todo", "○", LinearTheme.StatusTodo},
		{"In Progress", "⊙", LinearTheme.StatusInProgress},
		{"In Review", "◉", LinearTheme.StatusReview},
		{"Done", "●", LinearTheme.StatusDone},
		{"Canceled", "⊘", LinearTheme.StatusCanceled},
		{"Duplicate", "⊘", LinearTheme.StatusCanceled},
		{"Backlog", "◌", LinearTheme.SecondaryText},
	}

	for _, tt := range tests {
		icon, color := formatStateIcon(tt.state, LinearTheme)
		if icon != tt.wantIcon || color != tt.wantColor {
			t.Errorf("formatStateIcon(%q) = %q, %v; want %q, %v", tt.state, icon, color, tt.wantIcon, tt.wantColor)
		}
	}
}

// TestFormatStateIconTriageFallsBackToTodo covers themes that predate
// StatusTriage: the icon still separates triage from todo.
func TestFormatStateIconTriageFallsBackToTodo(t *testing.T) {
	legacy := LinearTheme
	legacy.StatusTriage = tcell.ColorDefault

	icon, color := formatStateIcon("Triage", legacy)
	if icon != "◎" {
		t.Errorf("icon = %q, want ◎", icon)
	}
	if color != legacy.StatusTodo {
		t.Errorf("color = %v, want StatusTodo %v", color, legacy.StatusTodo)
	}
}

// TestFormatAssigneeInitials covers the name shapes the column has to render
// in two cells.
func TestFormatAssigneeInitials(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Drew White", "DW"},
		{"drew white", "DW"},
		{"drucial", "D"},
		{"Jean Luc Picard", "JP"},
		{"Mary-Jane Watson", "MW"},
		{"  Drew   White  ", "DW"},
		{"Åsa Öberg", "ÅÖ"},
		{"Jane Doe (she/her)", "JD"},
		{"Linear (bot)", "L"},
		{"(bot)", ""},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatAssigneeInitials(tt.name); got != tt.want {
				t.Errorf("formatAssigneeInitials(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// TestAssigneeColumnCell pins what the list actually renders, including the
// dash an unassigned issue gets.
func TestAssigneeColumnCell(t *testing.T) {
	assigned := &linearapi.Issue{Assignee: "Drew White"}
	if text, color := issueColumnCell(ColumnAssignee, assigned, "", LinearTheme); text != "DW" || color != LinearTheme.AssigneeText {
		t.Errorf("assigned cell = %q, %v; want DW, AssigneeText", text, color)
	}

	unassigned := &linearapi.Issue{}
	if text, color := issueColumnCell(ColumnAssignee, unassigned, "", LinearTheme); text != "-" || color != LinearTheme.SecondaryText {
		t.Errorf("unassigned cell = %q, %v; want -, SecondaryText", text, color)
	}
}

// TestAssigneeTextColorFallsBackToForeground covers themes that predate the
// AssigneeText field.
func TestAssigneeTextColorFallsBackToForeground(t *testing.T) {
	legacy := LinearTheme
	legacy.AssigneeText = tcell.ColorDefault

	if got := legacy.AssigneeTextColor(); got != legacy.Foreground {
		t.Errorf("AssigneeTextColor() = %v, want Foreground %v", got, legacy.Foreground)
	}
}

// renderTableLines draws an issues table and returns the screen as text, one
// string per row.
func renderTableLines(t *testing.T, issues []linearapi.Issue, columns []string) []string {
	t.Helper()

	rows, idToIssue := BuildIssueRows(issues, map[string]bool{})
	table := tview.NewTable()
	renderIssuesTableModel(table, rows, idToIssue, "", LinearTheme, columns)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	t.Cleanup(screen.Fini)

	const width, height = 100, 10
	screen.SetSize(width, height)
	table.SetRect(0, 0, width, height)
	table.Draw(screen)
	screen.Show()

	cells, screenWidth, screenHeight := screen.GetContents()
	lines := make([]string, 0, screenHeight)
	for y := 0; y < screenHeight; y++ {
		line := make([]rune, 0, screenWidth)
		for x := 0; x < screenWidth; x++ {
			runes := cells[y*screenWidth+x].Runes
			if len(runes) == 0 || runes[0] == 0 {
				line = append(line, ' ')
				continue
			}
			line = append(line, runes[0])
		}
		lines = append(lines, string(line))
	}
	return lines
}

// TestIssueColumnHeadersAlignWithCells covers the header row sitting one cell
// left of its values: every ID cell leads with the space the tree icon uses,
// and the header has to lead with one too.
func TestIssueColumnHeadersAlignWithCells(t *testing.T) {
	issues := []linearapi.Issue{
		{ID: "1", Identifier: "ZNL-82", Title: "Triage status icons", State: "Triage"},
	}

	tests := []struct {
		name    string
		columns []string
		header  string
		value   string
	}{
		{"id after priority", DefaultIssueColumns, "ID", "ZNL-82"},
		{"id first", []string{ColumnID, ColumnTitle}, "ID", "ZNL-82"},
		{"title first", []string{ColumnTitle, ColumnID}, "Title", "Triage status icons"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := renderTableLines(t, issues, tt.columns)
			headerAt := strings.Index(lines[0], tt.header)
			valueAt := strings.Index(lines[1], tt.value)
			if headerAt < 0 || valueAt < 0 {
				t.Fatalf("header %q at %d, value %q at %d in:\n%s\n%s", tt.header, headerAt, tt.value, valueAt, lines[0], lines[1])
			}
			if headerAt != valueAt {
				t.Errorf("header %q starts at column %d, value starts at %d:\n%s\n%s", tt.header, headerAt, valueAt, lines[0], lines[1])
			}
		})
	}
}
