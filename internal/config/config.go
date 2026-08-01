package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Environment variable names for configuration.
const (
	LinearAPIKeyEnv   = "LINEAR_API_KEY"
	LinearClientIDEnv = "LINEAR_CLIENT_ID"
	LinearAPIEndpoint = "LINEAR_API_ENDPOINT"
	TimeoutEnv        = "LINEAR_TIMEOUT"
	PageSizeEnv       = "LINEAR_PAGE_SIZE"
	CacheTTLEnv       = "LINEAR_CACHE_TTL"
	LogFileEnv        = "LINEAR_LOG_FILE"
	LogLevelEnv       = "LINEAR_LOG_LEVEL"
)

// Default configuration values.
const (
	DefaultTimeout        = 30 * time.Second
	DefaultPageSize       = 50
	DefaultCacheTTL       = 5 * time.Minute
	DefaultSearchDebounce = 300 * time.Millisecond
	DefaultAPIEndpoint    = "https://api.linear.app/graphql"
	DefaultLogLevel       = "warning" // debug, info, warning, error
	ThemeLinear           = "linear"
	ThemeHighContrast     = "high_contrast"
	ThemeColorBlind       = "color_blind"
	ThemeRosePineMoon     = "rose_pine_moon"
	DefaultTheme          = ThemeLinear
	DensityComfortable    = "comfortable"
	DensityCompact        = "compact"
	DefaultDensity        = DensityComfortable
	DefaultAgentProvider  = "cursor"
	DefaultAgentSandbox   = "enabled"
)

// getDefaultLogFile returns the default log file path: $HOME/.linear-tui/app.log
func getDefaultLogFile() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to empty string if home directory cannot be determined
		return ""
	}
	return filepath.Join(homeDir, ".linear-tui", "app.log")
}

// Config holds runtime configuration for the application.
type Config struct {
	// LinearAPIKey is the credential used for Linear GraphQL auth.
	// It may be a personal API key or an OAuth access token resolved at startup.
	LinearAPIKey string

	// APIEndpoint is the Linear GraphQL API endpoint (useful for testing).
	APIEndpoint string

	// Timeout is the HTTP request timeout for API calls.
	Timeout time.Duration

	// PageSize is the default number of items to fetch per page.
	PageSize int

	// CacheTTL is the time-to-live for cached team metadata.
	CacheTTL time.Duration

	// SearchDebounce is the delay before live search refreshes issue results.
	SearchDebounce time.Duration

	// LogFile is the path to the log file (empty to disable logging).
	LogFile string

	// LogLevel is the minimum log level (debug, info, warning, error).
	LogLevel string

	// Theme controls the active UI theme.
	Theme string

	// Density controls the UI spacing density.
	Density string

	// GroupBy groups the issues list along a dimension
	// (status, priority, assignee, cycle; empty for a flat list).
	GroupBy string

	// SubgroupBy adds a second grouping level beneath GroupBy groups.
	SubgroupBy string

	// SortBy orders the issues list by one or more dimensions
	// (status, priority, updated, created). The first field decides, later
	// fields break ties. Empty sorts by most recently updated.
	SortBy []string

	// Columns selects and orders the issue list columns. Empty uses the
	// default Linear-style layout.
	Columns []string

	// RoundedBorders draws pane borders with rounded corners.
	RoundedBorders bool

	// AgentProvider selects the agent CLI provider (cursor or claude).
	AgentProvider string

	// AgentSandbox configures sandboxing for the agent CLI (enabled or disabled).
	AgentSandbox string

	// AgentModel selects the agent model when supported by the provider.
	AgentModel string

	// AgentWorkspace is the default workspace path for agent runs.
	AgentWorkspace string

	// Keybindings remaps palette command shortcuts and UI action keys by
	// id (e.g. {"refresh": "R", "tab_next": ")"}). Values are single keys.
	Keybindings map[string]string

	// DefaultTeam selects the team (by key or name) to open on startup.
	DefaultTeam string

	// DefaultProject selects the project (by name) to open on startup. Requires DefaultTeam.
	DefaultProject string

	// Workspaces lists Linear workspaces the app can switch between.
	Workspaces []Workspace

	// DefaultWorkspace selects the startup workspace by name. Empty falls
	// back to the first workspace whose key env var is set.
	DefaultWorkspace string
}

// Workspace describes a switchable Linear workspace. The API key is read from
// the named environment variable so credentials never live in the config file.
type Workspace struct {
	Name      string `json:"name"`
	APIKeyEnv string `json:"api_key_env"`
}

// APIKey returns the workspace's API key from the environment.
func (w Workspace) APIKey() string {
	return os.Getenv(w.APIKeyEnv)
}

// FirstAvailableWorkspace returns the first workspace whose API key
// environment variable is set, for use as the startup default.
func FirstAvailableWorkspace(workspaces []Workspace) (Workspace, bool) {
	for _, workspace := range workspaces {
		if workspace.APIKey() != "" {
			return workspace, true
		}
	}
	return Workspace{}, false
}

// StartupWorkspace resolves the workspace to use at startup: the configured
// default when its key is available, else the first workspace with a key.
func StartupWorkspace(workspaces []Workspace, defaultName string) (Workspace, bool) {
	if defaultName != "" {
		for _, workspace := range workspaces {
			if strings.EqualFold(workspace.Name, defaultName) && workspace.APIKey() != "" {
				return workspace, true
			}
		}
	}
	return FirstAvailableWorkspace(workspaces)
}

// LoadFromEnv loads configuration from environment variables.
// Returns an error if LINEAR_API_KEY is not set.
// Other values use sensible defaults if not specified.
func LoadFromEnv() (Config, error) {
	apiKey := os.Getenv(LinearAPIKeyEnv)
	if apiKey == "" {
		return Config{}, fmt.Errorf("%s environment variable is not set", LinearAPIKeyEnv)
	}

	cfg := Config{
		LinearAPIKey:   apiKey,
		APIEndpoint:    DefaultAPIEndpoint,
		Timeout:        DefaultTimeout,
		PageSize:       DefaultPageSize,
		CacheTTL:       DefaultCacheTTL,
		SearchDebounce: DefaultSearchDebounce,
		LogFile:        getDefaultLogFile(), // Default: $HOME/.linear-tui/app.log
		LogLevel:       DefaultLogLevel,
		Theme:          DefaultTheme,
		Density:        DefaultDensity,
		AgentProvider:  DefaultAgentProvider,
		AgentSandbox:   DefaultAgentSandbox,
		AgentModel:     "",
		AgentWorkspace: "",
	}

	// Parse optional API endpoint override.
	if endpoint := os.Getenv(LinearAPIEndpoint); endpoint != "" {
		cfg.APIEndpoint = endpoint
	}

	// Parse optional timeout.
	if timeoutStr := os.Getenv(TimeoutEnv); timeoutStr != "" {
		timeout, err := parseDuration(timeoutStr, TimeoutEnv)
		if err != nil {
			return Config{}, err
		}
		cfg.Timeout = timeout
	}

	// Parse optional page size.
	if pageSizeStr := os.Getenv(PageSizeEnv); pageSizeStr != "" {
		pageSize, err := strconv.Atoi(pageSizeStr)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s value %q: %w", PageSizeEnv, pageSizeStr, err)
		}
		if err := validatePageSize(pageSize, PageSizeEnv); err != nil {
			return Config{}, err
		}
		cfg.PageSize = pageSize
	}

	// Parse optional cache TTL.
	if cacheTTLStr := os.Getenv(CacheTTLEnv); cacheTTLStr != "" {
		cacheTTL, err := parseDuration(cacheTTLStr, CacheTTLEnv)
		if err != nil {
			return Config{}, err
		}
		cfg.CacheTTL = cacheTTL
	}

	// Parse optional log file path.
	// If LINEAR_LOG_FILE is set to empty string, disable logging.
	// If not set, use default: $HOME/.linear-tui/app.log
	if logFile, ok := os.LookupEnv(LogFileEnv); ok {
		if logFile == "" {
			cfg.LogFile = "" // Explicitly disable logging
		} else {
			cfg.LogFile = logFile
		}
	}
	// If LINEAR_LOG_FILE is not set, cfg.LogFile already has the default value

	// Parse optional log level.
	if logLevel := os.Getenv(LogLevelEnv); logLevel != "" {
		if err := validateLogLevel(logLevel, LogLevelEnv); err != nil {
			return Config{}, err
		}
		cfg.LogLevel = logLevel
	}

	return cfg, nil
}
