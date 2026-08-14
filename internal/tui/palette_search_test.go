package tui

import (
	"strings"
	"testing"
)

// rankedTitles ranks the commands against the query and names what came back.
func rankedTitles(commands []Command, query string) []string {
	titles := make([]string, 0, len(commands))
	for _, cmd := range rankCommands(commands, query) {
		titles = append(titles, cmd.Title)
	}
	return titles
}

func TestRankCommandsOrdersByWhereTheTokenSits(t *testing.T) {
	commands := []Command{
		{Title: "Group issues by…", Keywords: []string{"cycle"}},
		{Title: "Set cycle"},
		{Title: "Clear filters"},
		{Title: "Filter by cycle"},
		{Title: "Unassign issue", Keywords: []string{"clear assignee"}},
		{Title: "Clear due date"},
	}

	want := []string{
		"Clear due date",   // title prefix
		"Clear filters",    // title prefix
		"Filter by cycle",  // a title word starts with it
		"Set cycle",        // the title holds it mid-word
		"Unassign issue",   // a keyword word starts with it
		"Group issues by…", // a keyword holds it mid-word
	}
	if got := rankedTitles(commands, "cle"); !equalTitles(got, want) {
		t.Errorf("ranked %v, want %v", got, want)
	}
}

func TestRankCommandsBreaksTiesAlphabetically(t *testing.T) {
	commands := []Command{
		{Title: "Clear project"},
		{Title: "Clear cycle"},
		{Title: "Clear milestone"},
	}

	want := []string{"Clear cycle", "Clear milestone", "Clear project"}
	if got := rankedTitles(commands, "clear"); !equalTitles(got, want) {
		t.Errorf("ranked %v, want %v", got, want)
	}
}

func TestRankCommandsNeedsEveryToken(t *testing.T) {
	commands := []Command{
		{Title: "Clear cycle"},
		{Title: "Clear due date"},
	}

	if got := rankedTitles(commands, "clear cy"); !equalTitles(got, []string{"Clear cycle"}) {
		t.Errorf("ranked %v, want only Clear cycle", got)
	}
	if got := rankedTitles(commands, "clear nothing"); len(got) != 0 {
		t.Errorf("ranked %v, want nothing", got)
	}
}

// TestRankCommandsFallsBackToScatteredCharacters pins the fuzzy pass to being
// a fallback. It has to stay out of the way of a run of characters that a
// title actually holds, or it would list half the palette under every query.
func TestRankCommandsFallsBackToScatteredCharacters(t *testing.T) {
	commands := []Command{
		{Title: "Settings"},
		{Title: "Set cycle"},
		{Title: "Switch workspace", Keywords: []string{"organization"}},
	}

	if got := rankedTitles(commands, "stng"); !equalTitles(got, []string{"Settings"}) {
		t.Errorf("ranked %v, want Settings from the scattered characters", got)
	}
	// "set" is a run both titles hold, so the fuzzy pass never runs and
	// Switch workspace stays out even though s-e-t scatters through it.
	if got := rankedTitles(commands, "set"); !equalTitles(got, []string{"Set cycle", "Settings"}) {
		t.Errorf("ranked %v, want only the two titles holding \"set\"", got)
	}
}

func TestRankCommandsScattersThroughKeywordsToo(t *testing.T) {
	commands := []Command{
		{Title: "Switch workspace", Keywords: []string{"organization"}},
		{Title: "Settings"},
	}

	if got := rankedTitles(commands, "ogniz"); !equalTitles(got, []string{"Switch workspace"}) {
		t.Errorf("ranked %v, want the keyword match", got)
	}
}

func TestHasWordPrefixStartsAtEveryBreak(t *testing.T) {
	tests := []struct {
		text  string
		token string
		want  bool
	}{
		{"create sub-issue", "create", true},
		{"create sub-issue", "sub", true},
		{"create sub-issue", "issue", true},
		{"create sub-issue", "ssue", false},
		{"clear assignee", "assignee", true},
		{"unassign issue", "assign", false},
	}
	for _, tt := range tests {
		if got := hasWordPrefix(tt.text, tt.token); got != tt.want {
			t.Errorf("hasWordPrefix(%q, %q) = %v, want %v", tt.text, tt.token, got, tt.want)
		}
	}
}

func equalTitles(got, want []string) bool {
	return strings.Join(got, "|") == strings.Join(want, "|")
}
