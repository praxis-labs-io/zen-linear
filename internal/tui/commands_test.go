package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

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
// complete ordering, and the issues pane's footer names what is in effect.
func TestSortByPickerAppliesWholeOrdering(t *testing.T) {
	for _, tc := range []struct {
		label      string
		wantFields []SortField
		wantConfig []string
		wantSort   string
	}{
		{label: "Status, then priority", wantFields: []SortField{SortByStatus, SortByPriority}, wantConfig: []string{"status", "priority"}, wantSort: "Sort: status → priority"},
		{label: "Priority", wantFields: []SortField{SortByPriority}, wantConfig: []string{"priority"}, wantSort: "Sort: priority"},
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

			// The ordering is named on the issues pane's own footer.
			app.selectedNavigation = &NavigationNode{ID: "all", Text: "All Issues"}
			if got := stripTags(app.issuesContextText(120)); !strings.Contains(got, tc.wantSort) {
				t.Fatalf("issues footer = %q, want it to name %q", got, tc.wantSort)
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

// TestDefaultShortcutsMatchTheShippedSet pins the keys the app opens with.
// Every entry here moved at least once, and a rune drifting back onto a command
// that gave it up is the kind of change nothing else fails on.
func TestDefaultShortcutsMatchTheShippedSet(t *testing.T) {
	app := newUXTestApp(t)
	byID := make(map[string]rune, len(app.paletteCtrl.commands))
	for _, cmd := range app.paletteCtrl.commands {
		byID[cmd.ID] = cmd.ShortcutRune
	}

	// A zero rune is palette-only: the key it used to hold now belongs
	// elsewhere, and the command answers by name.
	want := map[string]rune{
		"switch_workspace":       'w',
		"toggle_navigation_pane": '<',
		"toggle_details_pane":    '>',
		"edit_labels":            't',
		"change_team":            'T',
		"add_comment":            'c',
		"set_cycle":              'C',
		"set_priority":           'p',
		"set_project":            'P',
		"create_sub_issue":       'N',
		"open_github":            'O',
		"copy_id":                'i',
		"copy_url":               'y',
		"copy_branch":            'Y',
		"view_parent":            0,
		"set_parent":             0,
		"remove_parent":          0,
		"edit_description":       0,
		"expand_all":             0,
		"collapse_all":           0,
	}
	for id, key := range want {
		got, registered := byID[id]
		if !registered {
			t.Errorf("command %q is not registered", id)
			continue
		}
		if got != key {
			t.Errorf("%s binds %q, want %q", id, string(got), string(key))
		}
	}

	// Both were dropped: the toggle for an unpredictable heuristic the explicit
	// pair covers, the title editor for a field the edit form already has.
	for _, id := range []string{"toggle_expand_all", "edit_title"} {
		if _, registered := byID[id]; registered {
			t.Errorf("command %q is still registered, want it removed", id)
		}
	}
}

// TestEditIssueOwnsEAndShortcutsAreUniquePerScope pins the rune assignment. e
// opens the full form, and no two commands reachable from the same pane share a
// rune: runCommandShortcut silently takes the first match. Two scopes that
// never answer together may share one.
func TestEditIssueOwnsEAndShortcutsAreUniquePerScope(t *testing.T) {
	app := newUXTestApp(t)

	byScope := map[CommandScope]map[rune]string{
		ScopeIssue:      {},
		ScopeNavigation: {},
	}
	for _, cmd := range app.paletteCtrl.commands {
		if cmd.ShortcutRune == 0 {
			continue
		}
		for scope, byRune := range byScope {
			if !cmd.appliesIn(scope) {
				continue
			}
			if other, taken := byRune[cmd.ShortcutRune]; taken {
				t.Fatalf("commands %q and %q both bind %q in scope %d", other, cmd.ID, string(cmd.ShortcutRune), scope)
			}
			byRune[cmd.ShortcutRune] = cmd.ID
		}
	}

	if got := byScope[ScopeIssue]['e']; got != "edit_issue" {
		t.Fatalf("e runs %q, want edit_issue", got)
	}
}

// TestEditIssueCommandEntersFieldEditMode drives the shortcut the way the key
// dispatcher does, from the pane a reader presses it in.
func TestEditIssueCommandEntersFieldEditMode(t *testing.T) {
	app := newUXTestApp(t)
	app.selectedIssue = &linearapi.Issue{
		ID:         "issue-1",
		Identifier: "LTUI-1",
		Title:      "Needs an edit",
		TeamID:     "team-1",
	}
	app.updateDetailsView()

	// The shortcut only answers from an issue pane, which is where the key
	// dispatcher runs it.
	app.focusedPane = FocusIssues

	if !app.runCommandShortcut('e') {
		t.Fatal("e did not run a command")
	}

	if !app.detailsEdit.on {
		t.Fatal("e left the pane out of edit mode")
	}
	if app.focusedPane != FocusDetails || app.detailsHidden {
		t.Fatalf("edit mode landed on pane %d, hidden %v", app.focusedPane, app.detailsHidden)
	}
	if got := app.detailsEdit.cursor; got != issueFieldTitle {
		t.Fatalf("cursor = %q, want the title", got)
	}
}

// TestChangeTeamCommandMovesTheIssue drives the shortcut the way the key
// dispatcher does, and pins the write to the issue the picker named.
func TestChangeTeamCommandMovesTheIssue(t *testing.T) {
	app := newUXTestApp(t)
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		t.Error("a single-issue update refetched the whole list")
		return linearapi.IssuePage{}, nil
	}
	app.navTeams = []linearapi.Team{
		{ID: "team-1", Key: "LIN", Name: "Linear"},
		{ID: "team-2", Key: "DES", Name: "Design"},
	}
	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{ID: "issue-1", Identifier: "LIN-1", Title: "Wrong team", TeamID: "team-1"}
	app.issuesMu.Unlock()

	called := make(chan linearapi.UpdateIssueInput, 1)
	app.updateIssueFunc = func(ctx context.Context, input linearapi.UpdateIssueInput) (linearapi.Issue, error) {
		called <- input
		return linearapi.Issue{ID: input.ID}, nil
	}

	// The shortcut only answers from an issue pane, which is where the key
	// dispatcher runs it.
	app.focusedPane = FocusIssues

	if !app.runCommandShortcut('T') {
		t.Fatal("T did not run a command")
	}

	// The selection moves out from under an open picker on a background
	// refresh; the write must still land on the issue the picker named.
	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{ID: "issue-2", Identifier: "LIN-2", Title: "Moved on", TeamID: "team-1"}
	app.issuesMu.Unlock()
	selectPickerItem(t, app, "Design (DES)")

	select {
	case input := <-called:
		if input.ID != "issue-1" {
			t.Fatalf("issue ID = %q, want issue-1", input.ID)
		}
		if input.TeamID == nil || *input.TeamID != "team-2" {
			t.Fatalf("TeamID = %v, want team-2", input.TeamID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the team move")
	}
}

// TestChangeTeamCommandSkipsTheCurrentTeam keeps the command from spending a
// mutation, and a renumbering, on the team the issue is already in.
func TestChangeTeamCommandSkipsTheCurrentTeam(t *testing.T) {
	app := newUXTestApp(t)
	app.navTeams = []linearapi.Team{
		{ID: "team-1", Key: "LIN", Name: "Linear"},
		{ID: "team-2", Key: "DES", Name: "Design"},
	}
	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{ID: "issue-1", Identifier: "LIN-1", Title: "Right team", TeamID: "team-1"}
	app.issuesMu.Unlock()

	called := make(chan linearapi.UpdateIssueInput, 1)
	app.updateIssueFunc = func(ctx context.Context, input linearapi.UpdateIssueInput) (linearapi.Issue, error) {
		called <- input
		return linearapi.Issue{ID: input.ID}, nil
	}

	// The shortcut only answers from an issue pane, which is where the key
	// dispatcher runs it.
	app.focusedPane = FocusIssues

	if !app.runCommandShortcut('T') {
		t.Fatal("T did not run a command")
	}
	selectPickerItem(t, app, "Linear (LIN)")

	select {
	case input := <-called:
		t.Fatalf("update sent for a no-op move: %#v", input)
	case <-time.After(100 * time.Millisecond):
	}
	if got := statusText(app); !strings.Contains(got, "Already in that team") {
		t.Fatalf("status bar = %q, want it to say the issue is already in that team", got)
	}
}
