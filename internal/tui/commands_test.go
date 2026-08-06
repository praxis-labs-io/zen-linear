package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/linearapi"
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

// TestSortByPickerAppliesWholeOrdering drives the sort picker: one row is one
// complete ordering, and the status bar names what is in effect.
func TestSortByPickerAppliesWholeOrdering(t *testing.T) {
	for _, tc := range []struct {
		label      string
		wantFields []SortField
		wantConfig []string
		wantStatus string
	}{
		{label: "Status, then priority", wantFields: []SortField{SortByStatus, SortByPriority}, wantConfig: []string{"status", "priority"}, wantStatus: "Sort: status → priority"},
		{label: "Priority", wantFields: []SortField{SortByPriority}, wantConfig: []string{"priority"}, wantStatus: "Sort: priority"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			app := newUXTestApp(t)
			refreshDone := installRefreshCompletionHook(app)
			app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
				return linearapi.IssuePage{}, nil
			}

			app.showSortByPicker()
			selectPickerItem(t, app, tc.label)

			if !reflect.DeepEqual(app.sortFields, tc.wantFields) {
				t.Fatalf("sort chain = %v, want %v", app.sortFields, tc.wantFields)
			}
			if !app.sortOverridden {
				t.Fatal("sortOverridden = false, want the manual choice to outrank the view")
			}
			// Without this an in-app settings save rewrites the file from
			// config and silently reverts the pick.
			if !reflect.DeepEqual(app.config.SortBy, tc.wantConfig) {
				t.Fatalf("config.SortBy = %v, want %v", app.config.SortBy, tc.wantConfig)
			}
			waitForRefreshCompletion(t, refreshDone)

			app.updateStatusBar()
			if got := app.statusBar.GetText(true); !strings.Contains(got, tc.wantStatus) {
				t.Fatalf("status bar = %q, want it to name %q", got, tc.wantStatus)
			}
		})
	}
}

// selectPickerItem moves to a row and presses Enter, the way the picker is
// actually driven. Going through HandleKey keeps the list index, the item
// slice, and the dismissal in the test's path.
func selectPickerItem(t *testing.T, app *App, label string) {
	t.Helper()
	for index, item := range app.pickerModal.items {
		if item.Label != label {
			continue
		}
		app.pickerModal.list.SetCurrentItem(index)
		app.pickerModal.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
		if app.pages.HasPage("picker") {
			t.Fatalf("picker still on the page stack after selecting %q", label)
		}
		return
	}
	t.Fatalf("picker has no %q item: %#v", label, app.pickerModal.items)
}

// TestEditDescriptionCommandFocusesModalNotNav reproduces the palette flow:
// running "Edit issue description" must leave keyboard focus in the modal,
// not on the pane the palette restored.
func TestEditDescriptionCommandFocusesModalNotNav(t *testing.T) {
	app := newUXTestApp(t)
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
	app := newUXTestApp(t)
	modal := app.editDescriptionModal
	app.app.SetFocus(modal.fm.order[len(modal.fm.order)-1])

	modal.Show("issue-1", "text", "ZEN-1 · Test issue", func(issueID, description string) {})

	if app.app.GetFocus() != modal.bodyField {
		t.Fatal("Show did not focus the description field")
	}
}

// TestEditIssueOwnsEAndShortcutsAreUnique pins the rune reassignment. e opens
// the full form, edit_title survives in the palette without one, and no two
// commands share a rune: runCommandShortcut silently takes the first match.
func TestEditIssueOwnsEAndShortcutsAreUnique(t *testing.T) {
	app := newUXTestApp(t)

	byRune := make(map[rune]string)
	for _, cmd := range app.paletteCtrl.commands {
		if cmd.ShortcutRune == 0 {
			continue
		}
		if other, taken := byRune[cmd.ShortcutRune]; taken {
			t.Fatalf("commands %q and %q both bind %q", other, cmd.ID, string(cmd.ShortcutRune))
		}
		byRune[cmd.ShortcutRune] = cmd.ID
	}

	if got := byRune['e']; got != "edit_issue" {
		t.Fatalf("e runs %q, want edit_issue", got)
	}
	for _, cmd := range app.paletteCtrl.commands {
		if cmd.ID == "edit_title" && cmd.ShortcutRune != 0 {
			t.Fatalf("edit_title still binds %q, want palette only", string(cmd.ShortcutRune))
		}
	}
}

// TestEditIssueCommandOpensThePrefilledForm drives the shortcut the way the
// key dispatcher does.
func TestEditIssueCommandOpensThePrefilledForm(t *testing.T) {
	app := newUXTestApp(t)
	app.selectedIssue = &linearapi.Issue{
		ID:         "issue-1",
		Identifier: "LTUI-1",
		Title:      "Needs an edit",
		TeamID:     "team-1",
	}

	if !app.runCommandShortcut('e') {
		t.Fatal("e did not run a command")
	}

	if !app.pages.HasPage("issue_form") {
		t.Fatal("issue form did not open")
	}
	if got := app.issueFormModal.titleField.GetText(); got != "Needs an edit" {
		t.Fatalf("title field = %q, want the selected issue's title", got)
	}
	if app.issueFormModal.fm.title != "Edit Issue" {
		t.Fatalf("modal title = %q, want Edit Issue", app.issueFormModal.fm.title)
	}
}
