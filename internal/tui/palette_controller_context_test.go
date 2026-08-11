package tui

import "testing"

// TestPaletteScopeFiltersCommands verifies the palette lists only what applies
// where it was opened: global commands everywhere, issue commands from an issue
// pane, navigation commands from the tree.
func TestPaletteScopeFiltersCommands(t *testing.T) {
	commands := []Command{
		{ID: "refresh", Title: "Refresh issues"},
		{ID: "copy_id", Title: "Copy issue ID", Scope: ScopeIssue},
		{ID: "archive", Title: "Archive issue", Scope: ScopeIssue},
		{ID: "toggle_favorite", Title: "Favorite", Scope: ScopeNavigation},
	}

	tests := []struct {
		name  string
		scope CommandScope
		want  []string
	}{
		{"issue pane", ScopeIssue, []string{"refresh", "copy_id", "archive"}},
		{"navigation pane", ScopeNavigation, []string{"refresh", "toggle_favorite"}},
		{"no pane", ScopeGlobal, []string{"refresh"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := NewPaletteController(commands)
			pc.SetScope(tt.scope)
			if got := commandIDs(pc.Filtered()); !equalIDs(got, tt.want) {
				t.Errorf("listed %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPaletteScopeSurvivesAQuery pins the filtered path to the same rule as the
// empty-query path. A search that reached out-of-scope commands would put the
// keyboard back where the scope took it away.
func TestPaletteScopeSurvivesAQuery(t *testing.T) {
	pc := NewPaletteController([]Command{
		{ID: "archive", Title: "Archive issue", Scope: ScopeIssue},
		{ID: "toggle_favorite", Title: "Favorite navigation item", Scope: ScopeNavigation},
	})
	pc.SetScope(ScopeNavigation)
	pc.SetQuery("i")

	if got := commandIDs(pc.Filtered()); !equalIDs(got, []string{"toggle_favorite"}) {
		t.Errorf("query listed %v, want only toggle_favorite", got)
	}
}

func commandIDs(commands []Command) []string {
	ids := make([]string, 0, len(commands))
	for _, cmd := range commands {
		ids = append(ids, cmd.ID)
	}
	return ids
}

func equalIDs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
