package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// TestEnsureSettingsFileCreatesDefaults verifies missing settings are created with defaults.
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

// TestLoadSettingsAppliesDefaults verifies missing fields use default values.
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

// TestLoadSettingsPreservesEmptyLogFile ensures an empty log file disables logging.
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
	expected.LogFile = ""
	assertSettingsEqual(t, settings, expected)
}

// TestConfigFromSettingsAcceptsAllThemes checks every registered theme name validates.
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

// TestLoadSettingsParsesRoundedBorders verifies the flag loads from JSON and
// defaults to false.
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

// TestLoadSettingsParsesSessionRestore verifies the flag defaults to on and
// that an explicit false in the file survives, which is what the pointer field
// on SettingsFile buys.
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

// TestConfigFromSettingsValidation checks invalid settings are rejected.
func TestConfigFromSettingsValidation(t *testing.T) {
	base := DefaultSettings()

	tests := []struct {
		name   string
		mutate func(Settings) Settings
	}{
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

// TestConfigFromSettingsAcceptsWorkspaces verifies a valid workspace list
// passes validation and reaches the config.
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

// TestLoadSettingsParsesWorkspaces verifies workspaces load from JSON.
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

// TestStartupWorkspace verifies the configured default wins when its key is
// available and falls back to the first available workspace otherwise.
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

// TestStartupWorkspaceName verifies the last session's workspace outranks the
// configured default, and only while restore is on.
func TestStartupWorkspaceName(t *testing.T) {
	tests := []struct {
		name        string
		restore     bool
		configured  string
		lastSession string
		want        string
	}{
		{name: "session wins", restore: true, configured: "Acme", lastSession: "Side", want: "Side"},
		{name: "no session falls back", restore: true, configured: "Acme", lastSession: "", want: "Acme"},
		{name: "blank session falls back", restore: true, configured: "Acme", lastSession: "   ", want: "Acme"},
		{name: "restore off ignores session", restore: false, configured: "Acme", lastSession: "Side", want: "Acme"},
		{name: "nothing configured", restore: true, configured: "", lastSession: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := DefaultSettings()
			settings.SessionRestore = tt.restore
			settings.DefaultWorkspace = tt.configured

			if got := StartupWorkspaceName(settings, tt.lastSession); got != tt.want {
				t.Errorf("StartupWorkspaceName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFirstAvailableWorkspace verifies startup default selection skips
// workspaces whose env var is unset.
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

// TestConfigFromSettingsRequiresAPIKey verifies API key is mandatory.
func TestConfigFromSettingsRequiresAPIKey(t *testing.T) {
	_, err := ConfigFromSettings("", DefaultSettings())
	if err == nil {
		t.Error("ConfigFromSettings() expected error when API key is empty")
	}
}

// TestDefaultSettingsAgentDefaults verifies agent defaults are set.
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

// TestLoadSettingsParsesDefaultNavigation verifies default team/project keys load from JSON.
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

// TestConfigFromSettingsPassesDefaultNavigation verifies default team/project reach the config.
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

// TestSettingsFromConfigCarriesDefaultNavigation verifies round-tripping config to settings.
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

// TestSortByRoundTrip verifies the sort chain survives load, config, and the
// trip back to settings.
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

// TestValidateSortByMatchesParserSpellings verifies the validator accepts
// every spelling the TUI parser handles. A stricter validator would abort
// startup on a config the app is written to understand.
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

// assertSettingsEqual compares settings values in tests.
func assertSettingsEqual(t *testing.T, got Settings, want Settings) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Settings mismatch: got %+v, want %+v", got, want)
	}
}

// TestValidateKeybindings verifies single-key values and duplicate rejection.
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
