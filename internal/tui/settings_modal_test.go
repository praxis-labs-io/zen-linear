package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// The log path is the one setting whose default is machine-specific. Showing it
// resolved is right — the field should name where logs really go — but saving
// it back verbatim is what pinned a shared config.json to one machine's home.
func TestSettingsFormDropsTheMachineDefaultLogPath(t *testing.T) {
	isolateLogging(t)

	app := newUXTestApp(t)
	app.config.LogFile = config.DefaultLogFile()

	sm := app.settingsModal
	sm.Show()

	if got := sm.logFileField.GetText(); got != config.DefaultLogFile() {
		t.Fatalf("log file field = %q, want the resolved default %q", got, config.DefaultLogFile())
	}

	settings, err := sm.settingsFromForm()
	if err != nil {
		t.Fatalf("settingsFromForm: %v", err)
	}
	if settings.LogFile != nil {
		t.Fatalf("log file saved as %q, want unset", *settings.LogFile)
	}
	if got := settings.ResolvedLogFile(); got != config.DefaultLogFile() {
		t.Fatalf("ResolvedLogFile() = %q, want %q", got, config.DefaultLogFile())
	}
}

// A path the user typed is theirs, and an empty field stays "logging off".
func TestSettingsFormKeepsAnExplicitLogPath(t *testing.T) {
	app := newUXTestApp(t)
	custom := filepath.Join(t.TempDir(), "elsewhere.log")

	tests := []struct {
		name string
		text string
		want *string
	}{
		{name: "explicit path survives", text: custom, want: &custom},
		{name: "blank is logging off", text: "", want: new(string)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := app.settingsModal
			sm.Show()
			sm.logFileField.SetText(tt.text)

			settings, err := sm.settingsFromForm()
			if err != nil {
				t.Fatalf("settingsFromForm: %v", err)
			}
			if settings.LogFile == nil {
				t.Fatalf("log file saved as unset, want %q", *tt.want)
			}
			if *settings.LogFile != *tt.want {
				t.Fatalf("log file saved as %q, want %q", *settings.LogFile, *tt.want)
			}
		})
	}
}

// The log level has a form control but no coverage; it rides along here so a
// picker read off the wrong index cannot silently reset it.
func TestSettingsFormRoundTripsLogLevel(t *testing.T) {
	app := newUXTestApp(t)
	app.config.LogLevel = "debug"

	sm := app.settingsModal
	sm.Show()
	settings, err := sm.settingsFromForm()
	if err != nil {
		t.Fatalf("settingsFromForm: %v", err)
	}
	if settings.LogLevel != "debug" {
		t.Fatalf("log level = %q, want %q", settings.LogLevel, "debug")
	}
}

// The end of the chain: what an in-app save actually leaves on disk. The form
// shows the resolved path, so only the normalization on the way out keeps this
// machine's home directory out of a config file shared with another machine.
func TestSavingSettingsLeavesNoMachineSpecificLogPathOnDisk(t *testing.T) {
	isolateLogging(t)

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
	app.config.LogFile = config.DefaultLogFile()
	app.UseSettingsFile(launch)

	app.settingsModal.Show()
	app.settingsModal.saveSettings()

	data, err := os.ReadFile(launch)
	if err != nil {
		t.Fatalf("reading the saved config: %v", err)
	}
	var written map[string]any
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("unmarshal saved config: %v", err)
	}
	if value, ok := written["log_file"]; ok {
		t.Errorf("log_file written as %q, want the key omitted", value)
	}

	// Omitted still has to come back as a usable path on the next launch.
	settings, err := config.LoadSettings(launch)
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}
	if got := settings.ResolvedLogFile(); got != config.DefaultLogFile() {
		t.Errorf("ResolvedLogFile() = %q, want %q", got, config.DefaultLogFile())
	}
}

// A path the user typed is theirs and does get written out.
func TestSavingSettingsKeepsAnExplicitLogPathOnDisk(t *testing.T) {
	isolateLogging(t)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	launch := filepath.Join(home, ".zen-linear", "config.json")
	custom := filepath.Join(home, "logs", "zen.log")
	app := newUXTestApp(t)
	app.config.LinearAPIKey = "k-acme"
	app.config.APIEndpoint = "https://api.linear.app/graphql"
	app.config.Timeout = 30 * time.Second
	app.config.SearchDebounce = 300 * time.Millisecond
	app.UseSettingsFile(launch)

	app.settingsModal.Show()
	app.settingsModal.logFileField.SetText(custom)
	app.settingsModal.saveSettings()

	data, err := os.ReadFile(launch)
	if err != nil {
		t.Fatalf("reading the saved config: %v", err)
	}
	var written map[string]any
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("unmarshal saved config: %v", err)
	}
	if written["log_file"] != custom {
		t.Errorf("log_file = %v, want %q", written["log_file"], custom)
	}
}

// An environment override is where this session's value came from, not where
// the next one's should. The modal shows it, so without protection a save
// would write $LINEAR_LOG_LEVEL into config.json permanently — the same shape
// as the log path ZNL-145 froze into a config shared between machines.
func TestSavingSettingsDoesNotWriteAnEnvOverrideToDisk(t *testing.T) {
	isolateLogging(t)
	t.Setenv("LINEAR_LOG_LEVEL", "debug")

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	launch := filepath.Join(home, ".zen-linear", "config.json")

	fileSettings := config.DefaultSettings()
	fileSettings.LogLevel = "warning"

	effective, overrides, err := config.ApplyEnvOverrides(fileSettings)
	if err != nil {
		t.Fatalf("ApplyEnvOverrides() error: %v", err)
	}
	if !overrides.Has(config.FieldLogLevel) {
		t.Fatal("log_level not taken over by the environment")
	}

	app := newUXTestApp(t)
	app.config.LinearAPIKey = "k-acme"
	app.config.APIEndpoint = "https://api.linear.app/graphql"
	app.config.Timeout = 30 * time.Second
	app.config.SearchDebounce = 300 * time.Millisecond
	app.config.LogLevel = effective.LogLevel
	app.UseSettingsFile(launch)
	app.UseFileSettings(fileSettings, overrides)

	app.settingsModal.Show()

	// Shown: the field reads what the session is actually running with, and the
	// line above the fields says where that came from.
	if _, got := app.settingsModal.logLevelField.GetCurrentOption(); got != "debug" {
		t.Errorf("log level field = %q, want the effective %q", got, "debug")
	}
	if got := app.settingsModal.fm.contextText; !strings.Contains(got, config.FieldLogLevel) {
		t.Errorf("context line %q does not name the field", got)
	}
	// And it refuses the typing rather than taking it and dropping it later.
	if !app.settingsModal.fm.isLocked(app.settingsModal.logLevelField.View()) {
		t.Error("an overridden field is still editable")
	}
	if app.settingsModal.fm.isLocked(app.settingsModal.themeField.View()) {
		t.Error("a field the environment does not own was locked")
	}

	app.settingsModal.saveSettings()

	data, err := os.ReadFile(launch)
	if err != nil {
		t.Fatalf("reading the saved config: %v", err)
	}
	var written map[string]any
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("unmarshal saved config: %v", err)
	}
	if written["log_level"] != "warning" {
		t.Errorf("log_level written as %v, want the file's %q", written["log_level"], "warning")
	}

	// And the session keeps the override rather than reverting to the file.
	if app.config.LogLevel != "debug" {
		t.Errorf("session log level = %q, want the override to survive the save", app.config.LogLevel)
	}
}

// With nothing in the environment the notice line stays empty, so an ordinary
// settings modal is unchanged.
func TestTheEnvOverrideNoticeNamesOnlyWhatIsSet(t *testing.T) {
	tests := []struct {
		name      string
		overrides config.EnvOverrides
		want      string
	}{
		{name: "nothing set", overrides: config.EnvOverrides{}, want: ""},
		{
			name:      "one field",
			overrides: config.EnvOverrides{config.FieldLogLevel: "LINEAR_LOG_LEVEL"},
			want:      "From the environment: log_level",
		},
		{
			name: "sorted, so the line does not reshuffle between opens",
			overrides: config.EnvOverrides{
				config.FieldLogLevel: "LINEAR_LOG_LEVEL",
				config.FieldTimeout:  "LINEAR_TIMEOUT",
			},
			want: "From the environment: log_level, timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := envOverrideNotice(tt.overrides); got != tt.want {
				t.Errorf("envOverrideNotice() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The notice line is one fixed row that does not grow, so it has to fit a
// narrow terminal at every length. Naming the variable beside each field
// overflowed it at two overrides, which told the reader about one field and
// hid the rest.
func TestTheEnvOverrideNoticeStaysOnOneLine(t *testing.T) {
	all := config.EnvOverrides{
		config.FieldAPIEndpoint: "LINEAR_API_ENDPOINT",
		config.FieldCacheTTL:    "LINEAR_CACHE_TTL",
		config.FieldLogFile:     "LINEAR_LOG_FILE",
		config.FieldLogLevel:    "LINEAR_LOG_LEVEL",
		config.FieldPageSize:    "LINEAR_PAGE_SIZE",
		config.FieldTimeout:     "LINEAR_TIMEOUT",
	}

	notice := envOverrideNotice(all)
	// 74 is what an 80-column terminal leaves after the border and padding.
	if len(notice) > 74 {
		t.Errorf("notice is %d chars and will be cut: %q", len(notice), notice)
	}
	if !strings.Contains(notice, "and 3 more") {
		t.Errorf("notice %q drops the fields it cannot list instead of counting them", notice)
	}
}
