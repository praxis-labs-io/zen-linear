package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
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
			name: "invalid agent sandbox",
			mutate: func(settings Settings) Settings {
				settings.AgentSandbox = "maybe"
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

// assertSettingsEqual compares settings values in tests.
func assertSettingsEqual(t *testing.T, got Settings, want Settings) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Settings mismatch: got %+v, want %+v", got, want)
	}
}
