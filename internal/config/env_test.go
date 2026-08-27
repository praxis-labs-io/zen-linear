package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The six overrides were documented for months while the only code reading
// them had no production caller. These drive the path the app actually boots
// with, ConfigFromSettings, rather than the env reader in isolation.
func TestEnvOverridesReachTheRunningConfig(t *testing.T) {
	tests := []struct {
		name     string
		variable string
		value    string
		field    string
		want     func(*testing.T, Config)
	}{
		{
			name: "api endpoint", variable: LinearAPIEndpoint,
			value: "http://localhost:8080/graphql", field: FieldAPIEndpoint,
			want: func(t *testing.T, cfg Config) {
				if cfg.APIEndpoint != "http://localhost:8080/graphql" {
					t.Errorf("APIEndpoint = %q", cfg.APIEndpoint)
				}
			},
		},
		{
			name: "timeout", variable: TimeoutEnv, value: "15s", field: FieldTimeout,
			want: func(t *testing.T, cfg Config) {
				if cfg.Timeout != 15*time.Second {
					t.Errorf("Timeout = %s, want 15s", cfg.Timeout)
				}
			},
		},
		{
			name: "page size", variable: PageSizeEnv, value: "100", field: FieldPageSize,
			want: func(t *testing.T, cfg Config) {
				if cfg.PageSize != 100 {
					t.Errorf("PageSize = %d, want 100", cfg.PageSize)
				}
			},
		},
		{
			name: "cache ttl", variable: CacheTTLEnv, value: "10m", field: FieldCacheTTL,
			want: func(t *testing.T, cfg Config) {
				if cfg.CacheTTL != 10*time.Minute {
					t.Errorf("CacheTTL = %s, want 10m", cfg.CacheTTL)
				}
			},
		},
		{
			name: "log level", variable: LogLevelEnv, value: "debug", field: FieldLogLevel,
			want: func(t *testing.T, cfg Config) {
				if cfg.LogLevel != "debug" {
					t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.variable, tt.value)

			settings, overrides, err := ApplyEnvOverrides(DefaultSettings())
			if err != nil {
				t.Fatalf("ApplyEnvOverrides() error: %v", err)
			}
			if !overrides.Has(tt.field) {
				t.Errorf("%s not reported as overridden, got %v", tt.field, overrides)
			}
			if got := overrides[tt.field]; got != tt.variable {
				t.Errorf("%s attributed to %q, want %q", tt.field, got, tt.variable)
			}

			cfg, err := ConfigFromSettings("test-key", settings)
			if err != nil {
				t.Fatalf("ConfigFromSettings() error: %v", err)
			}
			tt.want(t, cfg)
		})
	}
}

// LINEAR_LOG_FILE is the one read with LookupEnv: set-but-empty means logging
// off, where unset leaves whatever the file said.
func TestLogFileOverrideDistinguishesUnsetFromEmpty(t *testing.T) {
	fromFile := filepath.Join(t.TempDir(), "from-file.log")

	t.Run("unset keeps the file value", func(t *testing.T) {
		settings := DefaultSettings()
		settings.LogFile = &fromFile

		settings, overrides, err := ApplyEnvOverrides(settings)
		if err != nil {
			t.Fatalf("ApplyEnvOverrides() error: %v", err)
		}
		if overrides.Has(FieldLogFile) {
			t.Error("log_file reported as overridden with nothing in the environment")
		}
		if got := settings.ResolvedLogFile(); got != fromFile {
			t.Errorf("ResolvedLogFile() = %q, want %q", got, fromFile)
		}
	})

	t.Run("set to empty turns logging off", func(t *testing.T) {
		t.Setenv(LogFileEnv, "")

		settings := DefaultSettings()
		settings.LogFile = &fromFile

		settings, overrides, err := ApplyEnvOverrides(settings)
		if err != nil {
			t.Fatalf("ApplyEnvOverrides() error: %v", err)
		}
		if !overrides.Has(FieldLogFile) {
			t.Error("log_file not reported as overridden")
		}
		if got := settings.ResolvedLogFile(); got != "" {
			t.Errorf("ResolvedLogFile() = %q, want logging off", got)
		}
	})

	t.Run("set to a path uses it", func(t *testing.T) {
		elsewhere := filepath.Join(t.TempDir(), "elsewhere.log")
		t.Setenv(LogFileEnv, elsewhere)

		settings, _, err := ApplyEnvOverrides(DefaultSettings())
		if err != nil {
			t.Fatalf("ApplyEnvOverrides() error: %v", err)
		}
		if got := settings.ResolvedLogFile(); got != elsewhere {
			t.Errorf("ResolvedLogFile() = %q, want %q", got, elsewhere)
		}
	})
}

// A malformed override fails the launch. Ignoring it would be the same silence
// this ticket exists to end.
func TestAMalformedEnvOverrideFailsTheLaunch(t *testing.T) {
	tests := []struct {
		name     string
		variable string
		value    string
	}{
		{name: "timeout", variable: TimeoutEnv, value: "not-a-duration"},
		{name: "page size not a number", variable: PageSizeEnv, value: "abc"},
		{name: "page size too low", variable: PageSizeEnv, value: "0"},
		{name: "page size too high", variable: PageSizeEnv, value: "300"},
		{name: "cache ttl", variable: CacheTTLEnv, value: "bad-duration"},
		{name: "log level", variable: LogLevelEnv, value: "verbose"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.variable, tt.value)

			if _, _, err := ApplyEnvOverrides(DefaultSettings()); err == nil {
				t.Errorf("no error for %s=%s", tt.variable, tt.value)
			}
		})
	}
}

func TestNoEnvironmentLeavesSettingsAlone(t *testing.T) {
	for _, name := range []string{LinearAPIEndpoint, TimeoutEnv, PageSizeEnv, CacheTTLEnv, LogFileEnv, LogLevelEnv} {
		// Setenv first so the restore is registered, then clear it: an empty
		// LINEAR_LOG_FILE means logging off, which is not the same as unset.
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset %s: %v", name, err)
		}
	}

	settings, overrides, err := ApplyEnvOverrides(DefaultSettings())
	if err != nil {
		t.Fatalf("ApplyEnvOverrides() error: %v", err)
	}
	if len(overrides) != 0 {
		t.Errorf("overrides = %v, want none", overrides)
	}
	assertSettingsEqual(t, settings, DefaultSettings())
}
