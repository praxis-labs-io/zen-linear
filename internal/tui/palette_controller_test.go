package tui

import (
	"testing"
)

func TestPaletteController_FilterCommands(t *testing.T) {
	commands := []Command{
		{
			ID:       "refresh",
			Title:    "Refresh issues",
			Keywords: []string{"refresh", "reload", "r"},
		},
		{
			ID:       "search",
			Title:    "Search issues",
			Keywords: []string{"search", "find", "s"},
		},
	}

	pc := NewPaletteController(commands)

	tests := []struct {
		name      string
		query     string
		wantLen   int
		wantFirst string
	}{
		{
			name:      "empty query returns all",
			query:     "",
			wantLen:   2,
			wantFirst: "Refresh issues",
		},
		{
			name:      "filter by title",
			query:     "refresh",
			wantLen:   1,
			wantFirst: "Refresh issues",
		},
		{
			name:      "filter by keyword",
			query:     "ref",
			wantLen:   1,
			wantFirst: "Refresh issues",
		},
		{
			name:      "filter by partial keyword",
			query:     "sea",
			wantLen:   1,
			wantFirst: "Search issues",
		},
		{
			name:      "no matches",
			query:     "xyz",
			wantLen:   0,
			wantFirst: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc.SetQuery(tt.query)
			filtered := pc.Filtered()
			if len(filtered) != tt.wantLen {
				t.Errorf("Filtered() length = %d, want %d", len(filtered), tt.wantLen)
			}
			if tt.wantLen > 0 && filtered[0].Title != tt.wantFirst {
				t.Errorf("Filtered()[0].Title = %q, want %q", filtered[0].Title, tt.wantFirst)
			}
		})
	}
}

func TestPaletteController_Cursor(t *testing.T) {
	commands := []Command{
		{ID: "1", Title: "Command 1"},
		{ID: "2", Title: "Command 2"},
		{ID: "3", Title: "Command 3"},
	}

	pc := NewPaletteController(commands)

	// Test initial cursor position
	if pc.Cursor() != 0 {
		t.Errorf("Initial cursor = %d, want 0", pc.Cursor())
	}

	// Test MoveCursorDown
	pc.MoveCursorDown()
	if pc.Cursor() != 1 {
		t.Errorf("After MoveCursorDown, cursor = %d, want 1", pc.Cursor())
	}

	// Test MoveCursorUp
	pc.MoveCursorUp()
	if pc.Cursor() != 0 {
		t.Errorf("After MoveCursorUp, cursor = %d, want 0", pc.Cursor())
	}

	// Test SetCursor
	pc.SetCursor(2)
	if pc.Cursor() != 2 {
		t.Errorf("After SetCursor(2), cursor = %d, want 2", pc.Cursor())
	}

	// Test SetCursor beyond bounds
	pc.SetCursor(10)
	if pc.Cursor() != 2 {
		t.Errorf("After SetCursor(10), cursor = %d, want 2 (clamped)", pc.Cursor())
	}

	// Test SetCursor negative
	pc.SetCursor(-1)
	if pc.Cursor() != 0 {
		t.Errorf("After SetCursor(-1), cursor = %d, want 0 (clamped)", pc.Cursor())
	}
}

func TestPaletteController_Selected(t *testing.T) {
	commands := []Command{
		{ID: "1", Title: "Command 1"},
		{ID: "2", Title: "Command 2"},
	}

	pc := NewPaletteController(commands)

	// Test selected with valid cursor
	cmd, ok := pc.Selected()
	if !ok {
		t.Error("Selected() = false, want true")
	}
	if cmd.ID != "1" {
		t.Errorf("Selected().ID = %q, want %q", cmd.ID, "1")
	}

	// Test with empty filtered list
	pc.SetQuery("xyz")
	_, ok = pc.Selected()
	if ok {
		t.Error("Selected() with empty list = true, want false")
	}
}

func TestPaletteController_Reset(t *testing.T) {
	commands := []Command{
		{ID: "1", Title: "Command 1"},
		{ID: "2", Title: "Command 2"},
	}

	pc := NewPaletteController(commands)
	pc.SetQuery("test")
	pc.SetCursor(1)

	pc.Reset()

	if pc.Query() != "" {
		t.Errorf("After Reset(), Query() = %q, want empty string", pc.Query())
	}
	if pc.Cursor() != 0 {
		t.Errorf("After Reset(), Cursor() = %d, want 0", pc.Cursor())
	}
	if len(pc.Filtered()) != len(commands) {
		t.Errorf("After Reset(), Filtered() length = %d, want %d", len(pc.Filtered()), len(commands))
	}
}

func TestPaletteController_SearchMode(t *testing.T) {
	commands := []Command{
		{ID: "1", Title: "Command 1"},
		{ID: "2", Title: "Command 2"},
	}

	pc := NewPaletteController(commands)

	// Initially not in search mode
	if pc.IsSearchMode() {
		t.Error("Initially IsSearchMode() = true, want false")
	}

	// Enable search mode
	pc.SetSearchMode(true)
	if !pc.IsSearchMode() {
		t.Error("After SetSearchMode(true), IsSearchMode() = false, want true")
	}

	// In search mode, Selected should return false
	_, ok := pc.Selected()
	if ok {
		t.Error("In search mode, Selected() should return false")
	}

	// In search mode, filtered list should be nil/empty
	if pc.Filtered() != nil {
		t.Error("In search mode, Filtered() should be nil")
	}

	// Disable search mode
	pc.SetSearchMode(false)
	if pc.IsSearchMode() {
		t.Error("After SetSearchMode(false), IsSearchMode() = true, want false")
	}

	// After disabling, filtered should have commands again
	if len(pc.Filtered()) != len(commands) {
		t.Errorf("After disabling search mode, Filtered() length = %d, want %d", len(pc.Filtered()), len(commands))
	}
}

func TestPaletteController_QueryInSearchMode(t *testing.T) {
	commands := []Command{
		{ID: "1", Title: "Command 1"},
	}

	pc := NewPaletteController(commands)
	pc.SetSearchMode(true)
	pc.SetQuery("test search query")

	if pc.Query() != "test search query" {
		t.Errorf("Query() = %q, want %q", pc.Query(), "test search query")
	}

	// Query should be preserved even though we're in search mode
	// (the query is used for issue search, not command filtering)
}

func TestPaletteController_CaseInsensitiveFilter(t *testing.T) {
	commands := []Command{
		{ID: "1", Title: "UPPERCASE Command", Keywords: []string{"UPPER"}},
		{ID: "2", Title: "lowercase command", Keywords: []string{"lower"}},
	}

	pc := NewPaletteController(commands)

	// Search with lowercase should find uppercase
	pc.SetQuery("uppercase")
	if len(pc.Filtered()) != 1 {
		t.Errorf("Searching 'uppercase' returned %d results, want 1", len(pc.Filtered()))
	}

	// Search with uppercase should find lowercase
	pc.SetQuery("LOWERCASE")
	if len(pc.Filtered()) != 1 {
		t.Errorf("Searching 'LOWERCASE' returned %d results, want 1", len(pc.Filtered()))
	}

	// Keyword search should also be case insensitive
	pc.SetQuery("upper")
	if len(pc.Filtered()) != 1 {
		t.Errorf("Searching keyword 'upper' returned %d results, want 1", len(pc.Filtered()))
	}
}
