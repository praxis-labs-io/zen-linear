package tui

import (
	"testing"

	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

// TestFormatShortcutPreservesCase verifies uppercase and lowercase shortcut
// runes render distinctly in the palette so case-sensitive binds are legible.
func TestFormatShortcutPreservesCase(t *testing.T) {
	if got := FormatShortcut('w'); got != "w" {
		t.Errorf("FormatShortcut('w') = %q, want %q", got, "w")
	}
	if got := FormatShortcut('W'); got != "W" {
		t.Errorf("FormatShortcut('W') = %q, want %q", got, "W")
	}
	if got := FormatShortcut(0); got != "" {
		t.Errorf("FormatShortcut(0) = %q, want empty", got)
	}
}

// TestEditDescriptionCommandFocusesModalNotNav reproduces the palette flow:
// running "Edit issue description" must leave keyboard focus in the modal,
// not on the pane the palette restored.
func TestEditDescriptionCommandFocusesModalNotNav(t *testing.T) {
	app := newUXTestApp()
	app.selectedIssue = &linearapi.Issue{ID: "issue-1", Identifier: "LTUI-1", Title: "T", Description: "old"}
	app.focusedPane = FocusNavigation
	app.openPalette()

	var cmd Command
	for _, c := range app.paletteCtrl.commands {
		if c.ID == "edit_description" {
			cmd = c
			break
		}
	}
	if cmd.ID == "" {
		t.Fatal("edit_description command not registered")
	}
	app.closePalette()
	cmd.Run(app)

	if !app.pages.HasPage("edit_description") {
		t.Fatal("edit description modal did not open")
	}
	if focused := app.app.GetFocus(); focused == app.navigationTree {
		t.Fatal("focus is on the navigation tree, want the edit description form")
	}
}

// TestEditDescriptionModalShowResetsFocusToTextArea verifies reopening the
// modal focuses the description field even after a prior submit left focus
// on a button.
func TestEditDescriptionModalShowResetsFocusToTextArea(t *testing.T) {
	app := newUXTestApp()
	modal := app.editDescriptionModal
	app.app.SetFocus(modal.fm.order[len(modal.fm.order)-1])

	modal.Show("issue-1", "text", func(issueID, description string) {})

	if app.app.GetFocus() != modal.bodyField {
		t.Fatal("Show did not focus the description field")
	}
}
