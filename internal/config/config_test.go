package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadFromEnv(t *testing.T) {
	tests := []struct {
		name       string
		setEnv     bool
		envValue   string
		wantErr    bool
		wantAPIKey string
	}{
		{
			name:       "API key set",
			setEnv:     true,
			envValue:   "test-api-key-123",
			wantErr:    false,
			wantAPIKey: "test-api-key-123",
		},
		{
			name:       "API key not set",
			setEnv:     false,
			wantErr:    true,
			wantAPIKey: "",
		},
		{
			name:       "API key empty string",
			setEnv:     true,
			envValue:   "",
			wantErr:    true,
			wantAPIKey: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up environment
			oldValue := os.Getenv(LinearAPIKeyEnv)
			defer func() {
				_ = os.Setenv(LinearAPIKeyEnv, oldValue)
			}()

			if tt.setEnv {
				_ = os.Setenv(LinearAPIKeyEnv, tt.envValue)
			} else {
				_ = os.Unsetenv(LinearAPIKeyEnv)
			}

			cfg, err := LoadFromEnv()

			if tt.wantErr {
				if err == nil {
					t.Errorf("LoadFromEnv() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("LoadFromEnv() unexpected error: %v", err)
				return
			}

			if cfg.LinearAPIKey != tt.wantAPIKey {
				t.Errorf("LoadFromEnv() LinearAPIKey = %v, want %v", cfg.LinearAPIKey, tt.wantAPIKey)
			}
		})
	}
}

func TestLoadFromEnv_Defaults(t *testing.T) {
	// Save and restore environment
	oldAPIKey := os.Getenv(LinearAPIKeyEnv)
	oldEndpoint := os.Getenv(LinearAPIEndpoint)
	oldTimeout := os.Getenv(TimeoutEnv)
	oldPageSize := os.Getenv(PageSizeEnv)
	oldCacheTTL := os.Getenv(CacheTTLEnv)
	oldLogFile := os.Getenv(LogFileEnv)
	defer func() {
		_ = os.Setenv(LinearAPIKeyEnv, oldAPIKey)
		_ = os.Setenv(LinearAPIEndpoint, oldEndpoint)
		_ = os.Setenv(TimeoutEnv, oldTimeout)
		_ = os.Setenv(PageSizeEnv, oldPageSize)
		_ = os.Setenv(CacheTTLEnv, oldCacheTTL)
		if oldLogFile != "" {
			_ = os.Setenv(LogFileEnv, oldLogFile)
		} else {
			_ = os.Unsetenv(LogFileEnv)
		}
	}()

	// Clear all optional env vars
	_ = os.Setenv(LinearAPIKeyEnv, "test-key")
	_ = os.Unsetenv(LinearAPIEndpoint)
	_ = os.Unsetenv(TimeoutEnv)
	_ = os.Unsetenv(PageSizeEnv)
	_ = os.Unsetenv(CacheTTLEnv)
	_ = os.Unsetenv(LogFileEnv)

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error: %v", err)
	}

	if cfg.APIEndpoint != DefaultAPIEndpoint {
		t.Errorf("APIEndpoint = %q, want %q", cfg.APIEndpoint, DefaultAPIEndpoint)
	}
	if cfg.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, DefaultTimeout)
	}
	if cfg.PageSize != DefaultPageSize {
		t.Errorf("PageSize = %d, want %d", cfg.PageSize, DefaultPageSize)
	}
	if cfg.CacheTTL != DefaultCacheTTL {
		t.Errorf("CacheTTL = %v, want %v", cfg.CacheTTL, DefaultCacheTTL)
	}
	// Verify default log file path
	homeDir, err := os.UserHomeDir()
	if err == nil {
		expectedLogFile := filepath.Join(homeDir, ".linear-tui", "app.log")
		if cfg.LogFile != expectedLogFile {
			t.Errorf("LogFile = %q, want %q", cfg.LogFile, expectedLogFile)
		}
	} else if cfg.LogFile != "" {
		// If home directory cannot be determined, log file should be empty
		t.Errorf("LogFile = %q, want empty string (home dir unavailable)", cfg.LogFile)
	}
}

func TestLoadFromEnv_CustomValues(t *testing.T) {
	// Save and restore environment
	oldAPIKey := os.Getenv(LinearAPIKeyEnv)
	oldEndpoint := os.Getenv(LinearAPIEndpoint)
	oldTimeout := os.Getenv(TimeoutEnv)
	oldPageSize := os.Getenv(PageSizeEnv)
	oldCacheTTL := os.Getenv(CacheTTLEnv)
	defer func() {
		_ = os.Setenv(LinearAPIKeyEnv, oldAPIKey)
		_ = os.Setenv(LinearAPIEndpoint, oldEndpoint)
		_ = os.Setenv(TimeoutEnv, oldTimeout)
		_ = os.Setenv(PageSizeEnv, oldPageSize)
		_ = os.Setenv(CacheTTLEnv, oldCacheTTL)
	}()

	// Set custom values
	_ = os.Setenv(LinearAPIKeyEnv, "test-key")
	_ = os.Setenv(LinearAPIEndpoint, "http://localhost:8080/graphql")
	_ = os.Setenv(TimeoutEnv, "15s")
	_ = os.Setenv(PageSizeEnv, "100")
	_ = os.Setenv(CacheTTLEnv, "10m")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error: %v", err)
	}

	if cfg.APIEndpoint != "http://localhost:8080/graphql" {
		t.Errorf("APIEndpoint = %q, want %q", cfg.APIEndpoint, "http://localhost:8080/graphql")
	}
	if cfg.Timeout != 15*time.Second {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, 15*time.Second)
	}
	if cfg.PageSize != 100 {
		t.Errorf("PageSize = %d, want %d", cfg.PageSize, 100)
	}
	if cfg.CacheTTL != 10*time.Minute {
		t.Errorf("CacheTTL = %v, want %v", cfg.CacheTTL, 10*time.Minute)
	}
}

func TestLoadFromEnv_InvalidValues(t *testing.T) {
	tests := []struct {
		name     string
		envVar   string
		envValue string
	}{
		{
			name:     "invalid timeout",
			envVar:   TimeoutEnv,
			envValue: "not-a-duration",
		},
		{
			name:     "invalid page size",
			envVar:   PageSizeEnv,
			envValue: "not-a-number",
		},
		{
			name:     "page size too low",
			envVar:   PageSizeEnv,
			envValue: "0",
		},
		{
			name:     "page size too high",
			envVar:   PageSizeEnv,
			envValue: "300",
		},
		{
			name:     "invalid cache TTL",
			envVar:   CacheTTLEnv,
			envValue: "not-a-duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore environment
			oldAPIKey := os.Getenv(LinearAPIKeyEnv)
			oldEnvValue := os.Getenv(tt.envVar)
			defer func() {
				_ = os.Setenv(LinearAPIKeyEnv, oldAPIKey)
				_ = os.Setenv(tt.envVar, oldEnvValue)
			}()

			_ = os.Setenv(LinearAPIKeyEnv, "test-key")
			_ = os.Setenv(tt.envVar, tt.envValue)

			_, err := LoadFromEnv()
			if err == nil {
				t.Errorf("LoadFromEnv() expected error for %s=%s, got none", tt.envVar, tt.envValue)
			}
		})
	}
}
