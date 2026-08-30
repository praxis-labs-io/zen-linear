package config

import (
	"os"
	"path/filepath"
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
	DefaultSearchDebounce = 200 * time.Millisecond
	DefaultAPIEndpoint    = "https://api.linear.app/graphql"
	DefaultLogLevel       = "warning" // debug, info, warning, error
	ThemeTerminal         = "terminal"
	ThemeLinear           = "linear"
	ThemeHighContrast     = "high_contrast"
	ThemeColorBlind       = "color_blind"
	ThemeRosePineMoon     = "rose_pine_moon"
	DefaultTheme          = ThemeTerminal
	DensityComfortable    = "comfortable"
	DensityCompact        = "compact"
	DefaultDensity        = DensityComfortable
	DefaultAgentProvider  = "cursor"
	DefaultAgentSandbox   = "enabled"
)

// DefaultLogFile returns this machine's default log path: $HOME/.zen-linear/app.log.
// It is resolved at use rather than stored, so a config file shared between
// machines does not carry one machine's home directory to another.
func DefaultLogFile() string {
	dir, err := Dir()
	if err != nil {
		// Fallback to empty string if home directory cannot be determined
		return ""
	}
	return filepath.Join(dir, "app.log")
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
	// id (e.g. {"refresh": "R", "comment_next": ")"}). Values are single keys.
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

	// SessionRestore reopens the last workspace, navigation selection, and
	// focused issue on startup. When off, DefaultWorkspace, DefaultTeam, and
	// DefaultProject decide where the app opens.
	SessionRestore bool

	// UpdateCheck asks GitHub once a day whether a newer release has been
	// published and says so on the status bar. Nothing is downloaded, and no
	// token or identifier goes with the request.
	UpdateCheck bool
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

// StartupWorkspace resolves the workspace to use at startup: the first name
// whose key is available, else the first workspace with a key. Names are tried
// in order so a stale one falls through to the next rather than skipping
// straight to whatever workspace happens to come first.
func StartupWorkspace(workspaces []Workspace, names ...string) (Workspace, bool) {
	for _, name := range names {
		if name == "" {
			continue
		}
		for _, workspace := range workspaces {
			if strings.EqualFold(workspace.Name, name) && workspace.APIKey() != "" {
				return workspace, true
			}
		}
	}
	return FirstAvailableWorkspace(workspaces)
}

// StartupWorkspaceNames lists the workspace names to try at startup, best
// first: the last session's when restore is on, then the configured default.
// Both are offered because a session name whose key env var is gone must cost
// the user their default, not silently open an unrelated workspace.
func StartupWorkspaceNames(settings Settings, lastSession string) []string {
	if settings.SessionRestore && strings.TrimSpace(lastSession) != "" {
		return []string{lastSession, settings.DefaultWorkspace}
	}
	return []string{settings.DefaultWorkspace}
}
