package tui

import (
	"testing"

	"github.com/zen-linear/zen-linear/internal/config"
)

// TestSettingsFormRoundTripPreservesConfig guards the settings save path:
// settingsFromForm rebuilds the config file from form controls, so any field
// without a control must be carried through or an in-app save silently
// strips it from the user's config.
func TestSettingsFormRoundTripPreservesConfig(t *testing.T) {
	app := newUXTestApp(t)
	app.config.GroupBy = "status"
	app.config.SubgroupBy = "project"
	app.config.SortBy = []string{"status", "priority"}
	app.config.Columns = []string{"priority", "id", "title"}
	app.config.Keybindings = map[string]string{"switch_workspace": "w"}
	app.config.Workspaces = []config.Workspace{{Name: "Zenterm", APIKeyEnv: "LINEAR_API_KEY_FAKE"}}
	app.config.DefaultWorkspace = "Zenterm"

	sm := app.settingsModal
	sm.Show()
	settings, err := sm.settingsFromForm()
	if err != nil {
		t.Fatalf("settingsFromForm: %v", err)
	}
	if settings.GroupBy != "status" || settings.SubgroupBy != "project" {
		t.Fatalf("grouping stripped on save: group_by=%q subgroup_by=%q", settings.GroupBy, settings.SubgroupBy)
	}
	if len(settings.SortBy) != 2 || settings.SortBy[0] != "status" || settings.SortBy[1] != "priority" {
		t.Fatalf("sort chain stripped on save: %v", settings.SortBy)
	}
	if len(settings.Columns) != 3 {
		t.Fatalf("columns stripped on save: %v", settings.Columns)
	}
	if len(settings.Workspaces) != 1 || settings.Workspaces[0].Name != "Zenterm" {
		t.Fatalf("workspaces stripped on save: %v", settings.Workspaces)
	}
	if settings.DefaultWorkspace != "Zenterm" {
		t.Fatalf("default workspace stripped on save: %q", settings.DefaultWorkspace)
	}
	if settings.Keybindings["switch_workspace"] != "w" {
		t.Fatalf("keybindings stripped on save: %v", settings.Keybindings)
	}
}

// TestSettingsFormRoundTripsFlags verifies both on/off pickers survive a save
// in both directions rather than reverting to a default. They read their value
// off a selected index, so an off-by-one reads as the opposite setting.
func TestSettingsFormRoundTripsFlags(t *testing.T) {
	for _, want := range []bool{true, false} {
		app := newUXTestApp(t)
		app.config.SessionRestore = want
		app.config.RoundedBorders = want

		sm := app.settingsModal
		sm.Show()
		settings, err := sm.settingsFromForm()
		if err != nil {
			t.Fatalf("settingsFromForm: %v", err)
		}
		if settings.SessionRestore != want {
			t.Errorf("SessionRestore = %v, want %v", settings.SessionRestore, want)
		}
		if settings.RoundedBorders != want {
			t.Errorf("RoundedBorders = %v, want %v", settings.RoundedBorders, want)
		}
	}
}
