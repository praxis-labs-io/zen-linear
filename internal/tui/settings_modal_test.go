package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/config"
)

// A save has to land on the file the app launched from. Re-resolving the path
// followed an XDG copy that appeared mid-session and wrote to a stranger.
func TestSavingSettingsWritesBackToTheLaunchFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	launch := filepath.Join(home, ".zen-linear", "config.json")
	app := newUXTestApp(t)
	// The save validates before it writes, so the form needs values that pass.
	app.config.LinearAPIKey = "k-acme"
	app.config.APIEndpoint = "https://api.linear.app/graphql"
	app.config.Timeout = 30 * time.Second
	app.config.SearchDebounce = 300 * time.Millisecond
	app.UseSettingsFile(launch)

	intruder := filepath.Join(home, ".config", "zen-linear", "config.json")
	if err := os.MkdirAll(filepath.Dir(intruder), 0o755); err != nil {
		t.Fatalf("creating the XDG dir: %v", err)
	}
	const untouched = `{"theme":"linear"}`
	if err := os.WriteFile(intruder, []byte(untouched), 0o600); err != nil {
		t.Fatalf("writing the XDG config: %v", err)
	}

	app.settingsModal.Show()
	app.settingsModal.saveSettings()

	if _, err := os.Stat(launch); err != nil {
		t.Fatalf("nothing written to the launch file: %v", err)
	}
	data, err := os.ReadFile(intruder)
	if err != nil {
		t.Fatalf("reading the XDG config: %v", err)
	}
	if string(data) != untouched {
		t.Errorf("the XDG copy was overwritten: %s", data)
	}
}

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

// TestSettingsPickersSpanMultipleRows guards the row breaks between the
// settings pickers. Consecutive AddPicker calls pack into one row, so dropping
// an EndRow silently squeezes every picker into a single row and clips the
// labels and values rather than failing.
func TestSettingsPickersSpanMultipleRows(t *testing.T) {
	app := newUXTestApp(t)
	sm := app.settingsModal

	widest := 0
	for _, row := range sm.fm.rows {
		if row.columns > widest {
			widest = row.columns
		}
	}
	if widest > 3 {
		t.Fatalf("widest picker row = %d columns, want at most 3: the settings pickers lost a row break and will clip", widest)
	}
}
