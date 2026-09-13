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

func TestSavingSettingsWritesBackToTheLaunchFile(t *testing.T) {
	home := t.TempDir()
	setHomeDir(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	launch := filepath.Join(home, ".zen-linear", "config.json")
	app := newUXTestApp(t)
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

func TestSettingsFormRoundTripsFlags(t *testing.T) {
	for _, want := range []bool{true, false} {
		app := newUXTestApp(t)
		app.config.SessionRestore = want
		app.config.RoundedBorders = want
		app.config.UpdateCheck = want

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
		if settings.UpdateCheck != want {
			t.Errorf("UpdateCheck = %v, want %v", settings.UpdateCheck, want)
		}
	}
}

func TestSettingsSectionsHoldEveryFieldAndKeepOneHeight(t *testing.T) {
	app := newUXTestApp(t)
	app.pages.SetRect(0, 0, 110, 40)
	sm := app.settingsModal
	fm := sm.fm

	if len(fm.sections) < 2 {
		t.Fatal("the settings form is not sectioned")
	}
	for i, row := range fm.rows {
		if row.section < 0 {
			t.Fatalf("row %d belongs to no section, so no rail entry reaches it", i)
		}
	}

	want := fm.contentHeight(40)
	for i := range fm.sections {
		fm.activeSection = i
		if got := fm.contentHeight(40); got != want {
			t.Fatalf("section %q sizes the panel to %d, but %q sizes it to %d",
				fm.sections[i].name, got, fm.sections[0].name, want)
		}
	}

	for i := range fm.sections {
		fm.activeSection = i
		rows := 0
		for _, h := range fm.rowHeights(40) {
			rows += h
		}
		if rows > want-fm.chromeHeight() {
			t.Fatalf("section %q needs %d rows past the %d the panel holds",
				fm.sections[i].name, rows, want-fm.chromeHeight())
		}
	}
}

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

func TestSavingSettingsLeavesNoMachineSpecificLogPathOnDisk(t *testing.T) {
	isolateLogging(t)

	home := t.TempDir()
	setHomeDir(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	launch := filepath.Join(home, ".zen-linear", "config.json")
	app := newUXTestApp(t)
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

	settings, err := config.LoadSettings(launch)
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}
	if got := settings.ResolvedLogFile(); got != config.DefaultLogFile() {
		t.Errorf("ResolvedLogFile() = %q, want %q", got, config.DefaultLogFile())
	}
}

func TestSavingSettingsKeepsAnExplicitLogPathOnDisk(t *testing.T) {
	isolateLogging(t)

	home := t.TempDir()
	setHomeDir(t, home)
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

	if _, got := app.settingsModal.logLevelField.GetCurrentOption(); got != "debug" {
		t.Errorf("log level field = %q, want the effective %q", got, "debug")
	}
	if got := app.settingsModal.fm.contextText; !strings.Contains(got, config.FieldLogLevel) {
		t.Errorf("context line %q does not name the field", got)
	}
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

	if app.config.LogLevel != "debug" {
		t.Errorf("session log level = %q, want the override to survive the save", app.config.LogLevel)
	}
}

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
	if len(notice) > 74 {
		t.Errorf("notice is %d chars and will be cut: %q", len(notice), notice)
	}
	if !strings.Contains(notice, "and 3 more") {
		t.Errorf("notice %q drops the fields it cannot list instead of counting them", notice)
	}
}

func setHomeDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}
