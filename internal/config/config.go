package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

const (
	DefaultTimeout        = 30 * time.Second
	DefaultPageSize       = 50
	DefaultCacheTTL       = 5 * time.Minute
	DefaultSearchDebounce = 200 * time.Millisecond
	DefaultAPIEndpoint    = "https://api.linear.app/graphql"
	DefaultLogLevel       = "warning"
	ThemeTerminal         = "terminal"
	ThemeLinear           = "linear"
	ThemeHighContrast     = "high_contrast"
	ThemeColorBlind       = "color_blind"
	ThemeRosePineMoon     = "rose_pine_moon"
	DefaultTheme          = ThemeTerminal
	DensityComfortable    = "comfortable"
	DensityCompact        = "compact"
	DefaultDensity        = DensityComfortable
	ImagesAuto            = "auto"
	ImagesOff             = "off"
	DefaultImages         = ImagesAuto
	DefaultAgentProvider  = "cursor"
	DefaultAgentSandbox   = "enabled"
)

// DefaultLogFile returns this machine's default log path, or "" when the home
// directory is unknown.
func DefaultLogFile() string {
	dir, err := Dir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "app.log")
}

type Config struct {
	LinearAPIKey string

	APIEndpoint string

	Timeout time.Duration

	PageSize int

	CacheTTL time.Duration

	SearchDebounce time.Duration

	// LogFile empty disables logging.
	LogFile string

	LogLevel string

	Theme string

	Density string

	GroupBy string

	SubgroupBy string

	SortBy []string

	Columns []string

	RoundedBorders bool

	Images string

	AgentProvider string

	AgentSandbox string

	AgentModel string

	AgentWorkspace string

	Keybindings map[string]string

	DefaultTeam string

	// DefaultProject requires DefaultTeam.
	DefaultProject string

	Workspaces []Workspace

	// DefaultWorkspace empty falls back to the first workspace whose key is set.
	DefaultWorkspace string

	SessionRestore bool

	UpdateCheck bool
}

// Workspace is a switchable Linear workspace whose API key is read from the
// APIKeyEnv environment variable.
type Workspace struct {
	Name      string `json:"name"`
	APIKeyEnv string `json:"api_key_env"`
}

func (w Workspace) APIKey() string {
	return os.Getenv(w.APIKeyEnv)
}

func FirstAvailableWorkspace(workspaces []Workspace) (Workspace, bool) {
	for _, workspace := range workspaces {
		if workspace.APIKey() != "" {
			return workspace, true
		}
	}
	return Workspace{}, false
}

// StartupWorkspace returns the first of names whose key is set, else the first
// workspace with a key.
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

// StartupWorkspaceNames lists names to try at startup, best first: the last
// session's when restore is on, then the configured default.
func StartupWorkspaceNames(settings Settings, lastSession string) []string {
	if settings.SessionRestore && strings.TrimSpace(lastSession) != "" {
		return []string{lastSession, settings.DefaultWorkspace}
	}
	return []string{settings.DefaultWorkspace}
}
