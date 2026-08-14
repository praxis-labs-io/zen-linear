package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/agents"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// TestIssueShortcutIsDeadInTheNavigationPane drives the key the dispatcher
// drives. x on a tree node used to archive whatever issue happened to be
// selected in a pane the user was not looking at.
func TestIssueShortcutIsDeadInTheNavigationPane(t *testing.T) {
	app := newUXTestApp(t)
	app.selectedIssue = &linearapi.Issue{ID: "issue-1", Identifier: "LIN-1", Title: "Not yours to archive"}
	archive := tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone)

	app.focusedPane = FocusNavigation
	if got := app.handleNavigationKey(archive); got == nil {
		t.Fatal("navigation pane swallowed x")
	}
	if app.pages.HasPage("confirmation") {
		t.Fatal("x archived from the navigation pane")
	}

	app.focusedPane = FocusIssues
	if got := app.handleIssuesKey(archive); got != nil {
		t.Fatal("issues pane did not take x")
	}
	if !app.pages.HasPage("confirmation") {
		t.Fatal("x did not open the archive confirmation from the issues pane")
	}
}

// TestNavigationShortcutIsDeadInTheIssuesPane is the mirror: F belongs to the
// tree.
func TestNavigationShortcutIsDeadInTheIssuesPane(t *testing.T) {
	app := newUXTestApp(t)
	favorite := tcell.NewEventKey(tcell.KeyRune, 'F', tcell.ModNone)

	app.focusedPane = FocusIssues
	if got := app.handleIssuesKey(favorite); got == nil {
		t.Fatal("issues pane swallowed F")
	}

	app.focusedPane = FocusNavigation
	if got := app.handleNavigationKey(favorite); got != nil {
		t.Fatal("navigation pane did not take F")
	}
}

// TestPaletteOpensInTheScopeOfThePaneBehindIt pins the reading openPalette
// takes: the palette acts on the pane it was opened from, not on itself.
func TestPaletteOpensInTheScopeOfThePaneBehindIt(t *testing.T) {
	app := newUXTestApp(t)

	app.focusedPane = FocusDetails
	app.openPalette()
	if got := app.paletteCtrl.scope; got != ScopeIssue {
		t.Fatalf("palette opened from details in scope %v, want ScopeIssue", got)
	}
	for _, cmd := range app.paletteCtrl.Filtered() {
		if cmd.ID == "toggle_favorite" {
			t.Fatal("details palette lists toggle_favorite")
		}
	}

	app.closePalette()
	app.focusedPane = FocusNavigation
	app.openPalette()
	if got := app.paletteCtrl.scope; got != ScopeNavigation {
		t.Fatalf("palette opened from navigation in scope %v, want ScopeNavigation", got)
	}
	for _, cmd := range app.paletteCtrl.Filtered() {
		if cmd.ID == "archive" {
			t.Fatal("navigation palette lists archive")
		}
	}
}

// TestCommandScopes pins the registry. A command added without a scope is
// global, which is the permissive reading, so the ones that act on something
// have to say so here.
func TestCommandScopes(t *testing.T) {
	tests := []struct {
		id   string
		want CommandScope
	}{
		{"refresh", ScopeGlobal},
		{"settings", ScopeGlobal},
		{"switch_workspace", ScopeGlobal},
		{"create_issue", ScopeGlobal},
		{"zoom_details", ScopeGlobal},
		{"toggle_navigation_pane", ScopeGlobal},
		// List controls stay global so grouping and filtering work from the
		// navigation pane.
		{"group_by", ScopeGlobal},
		{"sort_by", ScopeGlobal},
		{"filter_status", ScopeGlobal},
		{"clear_filters", ScopeGlobal},
		{"expand_all", ScopeGlobal},
		{"archive", ScopeIssue},
		{"change_status", ScopeIssue},
		{"copy_url", ScopeIssue},
		{"add_comment", ScopeIssue},
		{"ask_agent", ScopeIssue},
		{"edit_labels", ScopeIssue},
		{"toggle_favorite", ScopeNavigation},
	}

	// ask_agent is gated out when no agent CLI is on PATH, so the registry
	// would be short one row on a machine without one. Stub the lookup rather
	// than let the host decide which commands this table can see.
	app := newUXTestApp(t)
	app.agentRunner = &agents.Runner{
		LookPath: func(string) (string, error) { return "agent", nil },
	}

	commands := DefaultCommands(app)
	for _, tt := range tests {
		cmd := findCommandByID(commands, tt.id)
		if cmd == nil {
			t.Errorf("%s is not in the registry", tt.id)
			continue
		}
		if cmd.Scope != tt.want {
			t.Errorf("%s scope = %v, want %v", tt.id, cmd.Scope, tt.want)
		}
	}
}
