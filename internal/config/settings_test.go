package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestEnsureSettingsFileCreatesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "nested", "config.json")

	settings, err := EnsureSettingsFile(settingsPath)
	if err != nil {
		t.Fatalf("EnsureSettingsFile() error: %v", err)
	}

	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings file not created: %v", err)
	}

	if _, err := os.Stat(filepath.Dir(settingsPath)); err != nil {
		t.Fatalf("settings directory not created: %v", err)
	}

	assertSettingsEqual(t, settings, DefaultSettings())
}

func TestLoadSettingsAppliesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "config.json")

	data := []byte(`{"page_size":123}`)
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("write settings file: %v", err)
	}

	settings, err := LoadSettings(settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}

	expected := DefaultSettings()
	expected.PageSize = 123
	assertSettingsEqual(t, settings, expected)
}

func TestLoadSettingsAppliesDefaultSearchDebounce(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "config.json")

	data := []byte(`{"page_size":123}`)
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("write settings file: %v", err)
	}

	settings, err := LoadSettings(settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}

	if settings.SearchDebounce != DefaultSearchDebounce.String() {
		t.Fatalf("SearchDebounce = %q, want %q", settings.SearchDebounce, DefaultSearchDebounce.String())
	}
}

func TestConfigFromSettingsParsesSearchDebounce(t *testing.T) {
	settings := DefaultSettings()
	settings.SearchDebounce = "450ms"

	cfg, err := ConfigFromSettings("test-key", settings)
	if err != nil {
		t.Fatalf("ConfigFromSettings() error: %v", err)
	}

	if cfg.SearchDebounce != 450*time.Millisecond {
		t.Fatalf("SearchDebounce = %s, want 450ms", cfg.SearchDebounce)
	}
}

func TestLoadSettingsPreservesEmptyLogFile(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "config.json")

	data := []byte(`{"log_file": ""}`)
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("write settings file: %v", err)
	}

	settings, err := LoadSettings(settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}

	expected := DefaultSettings()
	off := ""
	expected.LogFile = &off
	assertSettingsEqual(t, settings, expected)

	if resolved := settings.ResolvedLogFile(); resolved != "" {
		t.Errorf("ResolvedLogFile() = %q, want %q", resolved, "")
	}
}

func TestConfigFromSettingsAcceptsAllThemes(t *testing.T) {
	for _, theme := range []string{ThemeLinear, ThemeHighContrast, ThemeColorBlind, ThemeRosePineMoon} {
		t.Run(theme, func(t *testing.T) {
			settings := DefaultSettings()
			settings.Theme = theme
			if _, err := ConfigFromSettings("test-key", settings); err != nil {
				t.Errorf("ConfigFromSettings() error for theme %q: %v", theme, err)
			}
		})
	}
}

func TestLoadSettingsParsesRoundedBorders(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "config.json")

	data := []byte(`{"rounded_borders": true}`)
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("write settings file: %v", err)
	}

	settings, err := LoadSettings(settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}
	if !settings.RoundedBorders {
		t.Error("RoundedBorders = false, want true")
	}

	cfg, err := ConfigFromSettings("test-key", settings)
	if err != nil {
		t.Fatalf("ConfigFromSettings() error: %v", err)
	}
	if !cfg.RoundedBorders {
		t.Error("Config.RoundedBorders = false, want true")
	}
	if DefaultSettings().RoundedBorders {
		t.Error("DefaultSettings().RoundedBorders = true, want false")
	}
}

func TestLoadSettingsParsesImages(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "absent takes the default", data: `{}`, want: ImagesAuto},
		{name: "empty takes the default", data: `{"images": ""}`, want: ImagesAuto},
		{name: "off is kept", data: `{"images": "off"}`, want: ImagesOff},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settingsPath := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(settingsPath, []byte(test.data), 0644); err != nil {
				t.Fatalf("write settings file: %v", err)
			}

			settings, err := LoadSettings(settingsPath)
			if err != nil {
				t.Fatalf("LoadSettings() error: %v", err)
			}
			cfg, err := ConfigFromSettings("test-key", settings)
			if err != nil {
				t.Fatalf("ConfigFromSettings() error: %v", err)
			}
			if cfg.Images != test.want {
				t.Errorf("Config.Images = %q, want %q", cfg.Images, test.want)
			}
		})
	}
}

func TestLoadSettingsParsesSessionRestore(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{name: "absent defaults to on", data: `{}`, want: true},
		{name: "explicit false stays off", data: `{"session_restore": false}`, want: false},
		{name: "explicit true stays on", data: `{"session_restore": true}`, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settingsPath := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(settingsPath, []byte(tt.data), 0644); err != nil {
				t.Fatalf("write settings file: %v", err)
			}

			settings, err := LoadSettings(settingsPath)
			if err != nil {
				t.Fatalf("LoadSettings() error: %v", err)
			}
			if settings.SessionRestore != tt.want {
				t.Errorf("SessionRestore = %v, want %v", settings.SessionRestore, tt.want)
			}

			cfg, err := ConfigFromSettings("test-key", settings)
			if err != nil {
				t.Fatalf("ConfigFromSettings() error: %v", err)
			}
			if cfg.SessionRestore != tt.want {
				t.Errorf("Config.SessionRestore = %v, want %v", cfg.SessionRestore, tt.want)
			}
			if got := SettingsFromConfig(cfg).SessionRestore; got != tt.want {
				t.Errorf("SettingsFromConfig().SessionRestore = %v, want %v", got, tt.want)
			}
		})
	}

	if !DefaultSettings().SessionRestore {
		t.Error("DefaultSettings().SessionRestore = false, want true")
	}
}

func TestLoadSettingsParsesUpdateCheck(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{name: "absent defaults to on", data: `{}`, want: true},
		{name: "explicit false stays off", data: `{"update_check": false}`, want: false},
		{name: "explicit true stays on", data: `{"update_check": true}`, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settingsPath := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(settingsPath, []byte(tt.data), 0644); err != nil {
				t.Fatalf("write settings file: %v", err)
			}

			settings, err := LoadSettings(settingsPath)
			if err != nil {
				t.Fatalf("LoadSettings() error: %v", err)
			}
			if settings.UpdateCheck != tt.want {
				t.Errorf("UpdateCheck = %v, want %v", settings.UpdateCheck, tt.want)
			}

			cfg, err := ConfigFromSettings("test-key", settings)
			if err != nil {
				t.Fatalf("ConfigFromSettings() error: %v", err)
			}
			if cfg.UpdateCheck != tt.want {
				t.Errorf("Config.UpdateCheck = %v, want %v", cfg.UpdateCheck, tt.want)
			}
			if got := SettingsFromConfig(cfg).UpdateCheck; got != tt.want {
				t.Errorf("SettingsFromConfig().UpdateCheck = %v, want %v", got, tt.want)
			}
		})
	}

	if !DefaultSettings().UpdateCheck {
		t.Error("DefaultSettings().UpdateCheck = false, want true")
	}
}

func TestConfigFromSettingsValidation(t *testing.T) {
	base := DefaultSettings()

	tests := []struct {
		name   string
		mutate func(Settings) Settings
	}{
		{
			name: "invalid images",
			mutate: func(settings Settings) Settings {
				settings.Images = "sixel"
				return settings
			},
		},
		{
			name: "invalid timeout",
			mutate: func(settings Settings) Settings {
				settings.Timeout = "not-a-duration"
				return settings
			},
		},
		{
			name: "invalid cache ttl",
			mutate: func(settings Settings) Settings {
				settings.CacheTTL = "bad-duration"
				return settings
			},
		},
		{
			name: "invalid search debounce",
			mutate: func(settings Settings) Settings {
				settings.SearchDebounce = "bad-duration"
				return settings
			},
		},
		{
			name: "zero search debounce",
			mutate: func(settings Settings) Settings {
				settings.SearchDebounce = "0s"
				return settings
			},
		},
		{
			name: "negative search debounce",
			mutate: func(settings Settings) Settings {
				settings.SearchDebounce = "-1ms"
				return settings
			},
		},
		{
			name: "page size too low",
			mutate: func(settings Settings) Settings {
				settings.PageSize = 0
				return settings
			},
		},
		{
			name: "page size too high",
			mutate: func(settings Settings) Settings {
				settings.PageSize = 300
				return settings
			},
		},
		{
			name: "invalid log level",
			mutate: func(settings Settings) Settings {
				settings.LogLevel = "verbose"
				return settings
			},
		},
		{
			name: "invalid theme",
			mutate: func(settings Settings) Settings {
				settings.Theme = "rainbow"
				return settings
			},
		},
		{
			name: "invalid density",
			mutate: func(settings Settings) Settings {
				settings.Density = "ultra"
				return settings
			},
		},
		{
			name: "invalid agent provider",
			mutate: func(settings Settings) Settings {
				settings.AgentProvider = "unknown"
				return settings
			},
		},
		{
			name: "invalid group_by",
			mutate: func(settings Settings) Settings {
				settings.GroupBy = "labels"
				return settings
			},
		},
		{
			name: "invalid subgroup_by",
			mutate: func(settings Settings) Settings {
				settings.SubgroupBy = "rainbow"
				return settings
			},
		},
		{
			name: "invalid sort_by field",
			mutate: func(settings Settings) Settings {
				settings.SortBy = []string{"status", "labels"}
				return settings
			},
		},
		{
			name: "duplicate sort_by field",
			mutate: func(settings Settings) Settings {
				settings.SortBy = []string{"status", "status"}
				return settings
			},
		},
		{
			name: "invalid agent sandbox",
			mutate: func(settings Settings) Settings {
				settings.AgentSandbox = "maybe"
				return settings
			},
		},
		{
			name: "workspace missing name",
			mutate: func(settings Settings) Settings {
				settings.Workspaces = []Workspace{{APIKeyEnv: "LINEAR_KEY_A"}}
				return settings
			},
		},
		{
			name: "workspace missing api_key_env",
			mutate: func(settings Settings) Settings {
				settings.Workspaces = []Workspace{{Name: "Acme"}}
				return settings
			},
		},
		{
			name: "duplicate workspace names",
			mutate: func(settings Settings) Settings {
				settings.Workspaces = []Workspace{
					{Name: "Acme", APIKeyEnv: "LINEAR_KEY_A"},
					{Name: "acme", APIKeyEnv: "LINEAR_KEY_B"},
				}
				return settings
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := tt.mutate(base)
			_, err := ConfigFromSettings("test-key", settings)
			if err == nil {
				t.Errorf("ConfigFromSettings() expected error for %s", tt.name)
			}
		})
	}
}

func TestConfigFromSettingsAcceptsWorkspaces(t *testing.T) {
	settings := DefaultSettings()
	settings.Workspaces = []Workspace{
		{Name: "Acme", APIKeyEnv: "LINEAR_KEY_A"},
		{Name: "Side", APIKeyEnv: "LINEAR_KEY_B"},
	}

	cfg, err := ConfigFromSettings("test-key", settings)
	if err != nil {
		t.Fatalf("ConfigFromSettings() error: %v", err)
	}
	if !reflect.DeepEqual(cfg.Workspaces, settings.Workspaces) {
		t.Errorf("Workspaces = %+v, want %+v", cfg.Workspaces, settings.Workspaces)
	}
}

func TestLoadSettingsParsesWorkspaces(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "config.json")

	data := []byte(`{"workspaces":[{"name":"Acme","api_key_env":"LINEAR_KEY_A"}]}`)
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("write settings file: %v", err)
	}

	settings, err := LoadSettings(settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}

	expected := DefaultSettings()
	expected.Workspaces = []Workspace{{Name: "Acme", APIKeyEnv: "LINEAR_KEY_A"}}
	assertSettingsEqual(t, settings, expected)
}

func TestStartupWorkspace(t *testing.T) {
	t.Setenv("LINEAR_KEY_A", "k-a")
	t.Setenv("LINEAR_KEY_B", "k-b")
	workspaces := []Workspace{
		{Name: "Acme", APIKeyEnv: "LINEAR_KEY_A"},
		{Name: "Side", APIKeyEnv: "LINEAR_KEY_B"},
		{Name: "Ghost", APIKeyEnv: "LINEAR_KEY_UNSET"},
	}

	if workspace, ok := StartupWorkspace(workspaces, "side"); !ok || workspace.Name != "Side" {
		t.Errorf("StartupWorkspace(side) = %+v, %v; want Side", workspace, ok)
	}
	if workspace, ok := StartupWorkspace(workspaces, "Ghost"); !ok || workspace.Name != "Acme" {
		t.Errorf("StartupWorkspace(Ghost, key unset) = %+v, %v; want Acme fallback", workspace, ok)
	}
	if workspace, ok := StartupWorkspace(workspaces, ""); !ok || workspace.Name != "Acme" {
		t.Errorf("StartupWorkspace(empty) = %+v, %v; want Acme", workspace, ok)
	}
}

func TestStartupWorkspaceNames(t *testing.T) {
	tests := []struct {
		name        string
		restore     bool
		configured  string
		lastSession string
		want        []string
	}{
		{name: "session first, default behind it", restore: true, configured: "Acme", lastSession: "Side", want: []string{"Side", "Acme"}},
		{name: "no session leaves the default", restore: true, configured: "Acme", lastSession: "", want: []string{"Acme"}},
		{name: "blank session leaves the default", restore: true, configured: "Acme", lastSession: "   ", want: []string{"Acme"}},
		{name: "restore off ignores session", restore: false, configured: "Acme", lastSession: "Side", want: []string{"Acme"}},
		{name: "nothing configured", restore: true, configured: "", lastSession: "", want: []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := DefaultSettings()
			settings.SessionRestore = tt.restore
			settings.DefaultWorkspace = tt.configured

			got := StartupWorkspaceNames(settings, tt.lastSession)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StartupWorkspaceNames() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStartupWorkspaceFallsBackToDefault(t *testing.T) {
	t.Setenv("LINEAR_KEY_FIRST", "k-first")
	t.Setenv("LINEAR_KEY_DEFAULT", "k-default")
	workspaces := []Workspace{
		{Name: "Gamma", APIKeyEnv: "LINEAR_KEY_FIRST"},
		{Name: "Alpha", APIKeyEnv: "LINEAR_KEY_DEFAULT"},
		{Name: "Beta", APIKeyEnv: "LINEAR_KEY_UNSET"},
	}

	settings := DefaultSettings()
	settings.SessionRestore = true
	settings.DefaultWorkspace = "Alpha"

	names := StartupWorkspaceNames(settings, "Beta")
	workspace, ok := StartupWorkspace(workspaces, names...)
	if !ok || workspace.Name != "Alpha" {
		t.Fatalf("StartupWorkspace(%v) = %+v, %v; want Alpha, not the first workspace with a key", names, workspace, ok)
	}
}

func TestFirstAvailableWorkspace(t *testing.T) {
	t.Setenv("LINEAR_KEY_B", "k-side")
	workspaces := []Workspace{
		{Name: "Acme", APIKeyEnv: "LINEAR_KEY_A_UNSET"},
		{Name: "Side", APIKeyEnv: "LINEAR_KEY_B"},
	}

	workspace, ok := FirstAvailableWorkspace(workspaces)
	if !ok || workspace.Name != "Side" {
		t.Errorf("FirstAvailableWorkspace() = %+v, %v; want Side, true", workspace, ok)
	}

	_, ok = FirstAvailableWorkspace([]Workspace{{Name: "Acme", APIKeyEnv: "LINEAR_KEY_A_UNSET"}})
	if ok {
		t.Error("FirstAvailableWorkspace() = true with no env vars set, want false")
	}
}

func TestConfigFromSettingsRequiresAPIKey(t *testing.T) {
	_, err := ConfigFromSettings("", DefaultSettings())
	if err == nil {
		t.Error("ConfigFromSettings() expected error when API key is empty")
	}
}

func TestDefaultSettingsAgentDefaults(t *testing.T) {
	settings := DefaultSettings()
	if settings.AgentProvider != DefaultAgentProvider {
		t.Errorf("AgentProvider = %q, want %q", settings.AgentProvider, DefaultAgentProvider)
	}
	if settings.AgentSandbox != DefaultAgentSandbox {
		t.Errorf("AgentSandbox = %q, want %q", settings.AgentSandbox, DefaultAgentSandbox)
	}
	if settings.AgentModel != "" {
		t.Errorf("AgentModel = %q, want empty string", settings.AgentModel)
	}
	if settings.AgentWorkspace != "" {
		t.Errorf("AgentWorkspace = %q, want empty string", settings.AgentWorkspace)
	}
	if settings.Theme != DefaultTheme {
		t.Errorf("Theme = %q, want %q", settings.Theme, DefaultTheme)
	}
	if settings.Density != DefaultDensity {
		t.Errorf("Density = %q, want %q", settings.Density, DefaultDensity)
	}
}

func TestLoadSettingsParsesDefaultNavigation(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "config.json")

	data := []byte(`{"default_team":"NEX","default_project":"Website"}`)
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("write settings file: %v", err)
	}

	settings, err := LoadSettings(settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}

	expected := DefaultSettings()
	expected.DefaultTeam = "NEX"
	expected.DefaultProject = "Website"
	assertSettingsEqual(t, settings, expected)
}

func TestConfigFromSettingsPassesDefaultNavigation(t *testing.T) {
	settings := DefaultSettings()
	settings.DefaultTeam = "NEX"
	settings.DefaultProject = "Website"

	cfg, err := ConfigFromSettings("test-key", settings)
	if err != nil {
		t.Fatalf("ConfigFromSettings() error: %v", err)
	}

	if cfg.DefaultTeam != "NEX" {
		t.Errorf("DefaultTeam = %q, want %q", cfg.DefaultTeam, "NEX")
	}
	if cfg.DefaultProject != "Website" {
		t.Errorf("DefaultProject = %q, want %q", cfg.DefaultProject, "Website")
	}
}

func TestSettingsFromConfigCarriesDefaultNavigation(t *testing.T) {
	cfg := Config{DefaultTeam: "NEX", DefaultProject: "Website"}

	settings := SettingsFromConfig(cfg)

	if settings.DefaultTeam != "NEX" {
		t.Errorf("DefaultTeam = %q, want %q", settings.DefaultTeam, "NEX")
	}
	if settings.DefaultProject != "Website" {
		t.Errorf("DefaultProject = %q, want %q", settings.DefaultProject, "Website")
	}
}

func TestSortByRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "config.json")

	data := []byte(`{"sort_by":["status","priority"]}`)
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("write settings file: %v", err)
	}

	settings, err := LoadSettings(settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}

	expected := DefaultSettings()
	expected.SortBy = []string{"status", "priority"}
	assertSettingsEqual(t, settings, expected)

	cfg, err := ConfigFromSettings("test-key", settings)
	if err != nil {
		t.Fatalf("ConfigFromSettings() error: %v", err)
	}
	if !reflect.DeepEqual(cfg.SortBy, []string{"status", "priority"}) {
		t.Errorf("SortBy = %v, want [status priority]", cfg.SortBy)
	}
	if !reflect.DeepEqual(SettingsFromConfig(cfg).SortBy, cfg.SortBy) {
		t.Errorf("SettingsFromConfig dropped sort_by: %v", SettingsFromConfig(cfg).SortBy)
	}
}

func TestValidateSortByMatchesParserSpellings(t *testing.T) {
	for _, fields := range [][]string{
		{"status", "priority"},
		{"createdAt"},
		{"updatedAt", "priority"},
		{"Status", " priority "},
	} {
		if err := validateSortBy(fields, "sort_by"); err != nil {
			t.Errorf("validateSortBy(%v) = %v, want accepted", fields, err)
		}
	}

	for _, fields := range [][]string{
		{"labels"},
		{"updated", "updatedAt"},
	} {
		if err := validateSortBy(fields, "sort_by"); err == nil {
			t.Errorf("validateSortBy(%v) = nil, want rejected", fields)
		}
	}
}

func assertSettingsEqual(t *testing.T, got Settings, want Settings) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Settings mismatch: got %+v, want %+v", got, want)
	}
}

func TestValidateKeybindings(t *testing.T) {
	settings := DefaultSettings()
	settings.Keybindings = map[string]string{"refresh": "R", "copy_id": "c"}
	if _, err := ConfigFromSettings("test-key", settings); err != nil {
		t.Fatalf("ConfigFromSettings() error for valid keybindings: %v", err)
	}

	settings.Keybindings = map[string]string{"refresh": "ctrl-r"}
	if _, err := ConfigFromSettings("test-key", settings); err == nil {
		t.Error("expected error for multi-character key")
	}

	settings.Keybindings = map[string]string{"refresh": "x", "archive": "x"}
	if _, err := ConfigFromSettings("test-key", settings); err == nil {
		t.Error("expected error for duplicate key")
	}
}

func TestEnsureSettingsFileOmitsTheDefaultLogPath(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "config.json")

	if _, err := EnsureSettingsFile(settingsPath); err != nil {
		t.Fatalf("EnsureSettingsFile() error: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}

	var written map[string]any
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("unmarshal settings file: %v", err)
	}
	if value, ok := written["log_file"]; ok {
		t.Errorf("log_file written as %q, want the key omitted", value)
	}

	settings, err := LoadSettings(settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}
	if got := settings.ResolvedLogFile(); got != DefaultLogFile() {
		t.Errorf("ResolvedLogFile() = %q, want %q", got, DefaultLogFile())
	}
}

func TestLogFileRoundTripDropsTheMachineDefault(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "elsewhere.log")
	machineDefault := DefaultLogFile()
	off := ""

	tests := []struct {
		name string
		file *string
		want *string
	}{
		{name: "unset stays unset", file: nil, want: nil},
		{name: "machine default goes back to unset", file: &machineDefault, want: nil},
		{name: "explicit path survives", file: &custom, want: &custom},
		{name: "logging off survives", file: &off, want: &off},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := DefaultSettings()
			settings.LogFile = tt.file

			cfg, err := ConfigFromSettings("test-key", settings)
			if err != nil {
				t.Fatalf("ConfigFromSettings() error: %v", err)
			}
			if cfg.LogFile != settings.ResolvedLogFile() {
				t.Errorf("Config.LogFile = %q, want %q", cfg.LogFile, settings.ResolvedLogFile())
			}

			if got := SettingsFromConfig(cfg).LogFile; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SettingsFromConfig().LogFile = %v, want %v", derefLogFile(got), derefLogFile(tt.want))
			}
		})
	}
}

func derefLogFile(logFile *string) string {
	if logFile == nil {
		return "<unset>"
	}
	return *logFile
}
