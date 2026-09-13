package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SettingsFile struct {
	APIEndpoint      *string           `json:"api_endpoint"`
	Timeout          *string           `json:"timeout"`
	PageSize         *int              `json:"page_size"`
	CacheTTL         *string           `json:"cache_ttl"`
	SearchDebounce   *string           `json:"search_debounce"`
	LogFile          *string           `json:"log_file"`
	LogLevel         *string           `json:"log_level"`
	Theme            *string           `json:"theme"`
	Density          *string           `json:"density"`
	GroupBy          *string           `json:"group_by"`
	SubgroupBy       *string           `json:"subgroup_by"`
	SortBy           []string          `json:"sort_by"`
	Columns          []string          `json:"columns"`
	RoundedBorders   *bool             `json:"rounded_borders"`
	Images           *string           `json:"images"`
	Workspaces       []Workspace       `json:"workspaces"`
	DefaultWorkspace *string           `json:"default_workspace"`
	AgentProvider    *string           `json:"agent_provider"`
	AgentSandbox     *string           `json:"agent_sandbox"`
	AgentModel       *string           `json:"agent_model"`
	AgentWorkspace   *string           `json:"agent_workspace"`
	Keybindings      map[string]string `json:"keybindings"`
	DefaultTeam      *string           `json:"default_team"`
	DefaultProject   *string           `json:"default_project"`
	SessionRestore   *bool             `json:"session_restore"`
	UpdateCheck      *bool             `json:"update_check"`
}

type Settings struct {
	APIEndpoint    string `json:"api_endpoint"`
	Timeout        string `json:"timeout"`
	PageSize       int    `json:"page_size"`
	CacheTTL       string `json:"cache_ttl"`
	SearchDebounce string `json:"search_debounce"`
	// LogFile is nil for this machine's default and "" for logging off.
	LogFile          *string           `json:"log_file,omitempty"`
	LogLevel         string            `json:"log_level"`
	Theme            string            `json:"theme"`
	Density          string            `json:"density"`
	GroupBy          string            `json:"group_by"`
	SubgroupBy       string            `json:"subgroup_by"`
	SortBy           []string          `json:"sort_by,omitempty"`
	Columns          []string          `json:"columns,omitempty"`
	RoundedBorders   bool              `json:"rounded_borders"`
	Images           string            `json:"images"`
	Workspaces       []Workspace       `json:"workspaces,omitempty"`
	DefaultWorkspace string            `json:"default_workspace,omitempty"`
	AgentProvider    string            `json:"agent_provider"`
	AgentSandbox     string            `json:"agent_sandbox"`
	AgentModel       string            `json:"agent_model"`
	AgentWorkspace   string            `json:"agent_workspace"`
	Keybindings      map[string]string `json:"keybindings,omitempty"`
	DefaultTeam      string            `json:"default_team"`
	DefaultProject   string            `json:"default_project"`
	SessionRestore   bool              `json:"session_restore"`
	UpdateCheck      bool              `json:"update_check"`
}

func DefaultSettings() Settings {
	return Settings{
		APIEndpoint:    DefaultAPIEndpoint,
		Timeout:        DefaultTimeout.String(),
		PageSize:       DefaultPageSize,
		CacheTTL:       DefaultCacheTTL.String(),
		SearchDebounce: DefaultSearchDebounce.String(),
		LogLevel:       DefaultLogLevel,
		Theme:          DefaultTheme,
		Density:        DefaultDensity,
		GroupBy:        "",
		SubgroupBy:     "",
		RoundedBorders: false,
		Images:         DefaultImages,
		AgentProvider:  DefaultAgentProvider,
		AgentSandbox:   DefaultAgentSandbox,
		AgentModel:     "",
		AgentWorkspace: "",
		DefaultTeam:    "",
		DefaultProject: "",
		SessionRestore: true,
		UpdateCheck:    true,
	}
}

func (s Settings) ResolvedLogFile() string {
	if s.LogFile == nil {
		return DefaultLogFile()
	}
	return *s.LogFile
}

// LogFileSetting maps a path equal to DefaultLogFile back to nil, so a save
// never writes it.
func LogFileSetting(logFile string) *string {
	if logFile == DefaultLogFile() {
		return nil
	}
	return &logFile
}

func SettingsFromConfig(cfg Config) Settings {
	return Settings{
		APIEndpoint:      cfg.APIEndpoint,
		Timeout:          cfg.Timeout.String(),
		PageSize:         cfg.PageSize,
		CacheTTL:         cfg.CacheTTL.String(),
		SearchDebounce:   cfg.SearchDebounce.String(),
		LogFile:          LogFileSetting(cfg.LogFile),
		LogLevel:         cfg.LogLevel,
		Theme:            cfg.Theme,
		Density:          cfg.Density,
		GroupBy:          cfg.GroupBy,
		SubgroupBy:       cfg.SubgroupBy,
		SortBy:           cfg.SortBy,
		Columns:          cfg.Columns,
		RoundedBorders:   cfg.RoundedBorders,
		Images:           cfg.Images,
		Workspaces:       cfg.Workspaces,
		DefaultWorkspace: cfg.DefaultWorkspace,
		AgentProvider:    cfg.AgentProvider,
		AgentSandbox:     cfg.AgentSandbox,
		AgentModel:       cfg.AgentModel,
		AgentWorkspace:   cfg.AgentWorkspace,
		Keybindings:      cfg.Keybindings,
		DefaultTeam:      cfg.DefaultTeam,
		DefaultProject:   cfg.DefaultProject,
		SessionRestore:   cfg.SessionRestore,
		UpdateCheck:      cfg.UpdateCheck,
	}
}

// ConfigFromSettings validates settings and builds a Config around an already
// resolved token.
func ConfigFromSettings(apiKey string, settings Settings) (Config, error) {
	if apiKey == "" {
		return Config{}, fmt.Errorf("auth token is empty")
	}

	timeout, err := parseDuration(settings.Timeout, "timeout")
	if err != nil {
		return Config{}, err
	}

	cacheTTL, err := parseDuration(settings.CacheTTL, "cache_ttl")
	if err != nil {
		return Config{}, err
	}

	searchDebounce, err := parsePositiveDuration(settings.SearchDebounce, "search_debounce")
	if err != nil {
		return Config{}, err
	}

	if err := validatePageSize(settings.PageSize, "page_size"); err != nil {
		return Config{}, err
	}

	if err := validateLogLevel(settings.LogLevel, "log_level"); err != nil {
		return Config{}, err
	}

	theme := strings.TrimSpace(settings.Theme)
	if theme == "" {
		theme = DefaultTheme
	}
	if err := validateTheme(theme, "theme"); err != nil {
		return Config{}, err
	}

	density := strings.TrimSpace(settings.Density)
	if density == "" {
		density = DefaultDensity
	}
	if err := validateDensity(density, "density"); err != nil {
		return Config{}, err
	}

	images := strings.TrimSpace(settings.Images)
	if images == "" {
		images = DefaultImages
	}
	if err := validateImages(images, "images"); err != nil {
		return Config{}, err
	}

	if err := validateAgentProvider(settings.AgentProvider, "agent_provider"); err != nil {
		return Config{}, err
	}

	if err := validateAgentSandbox(settings.AgentSandbox, "agent_sandbox"); err != nil {
		return Config{}, err
	}

	if err := validateGroupDimension(settings.GroupBy, "group_by"); err != nil {
		return Config{}, err
	}
	if err := validateGroupDimension(settings.SubgroupBy, "subgroup_by"); err != nil {
		return Config{}, err
	}

	if err := validateSortBy(settings.SortBy, "sort_by"); err != nil {
		return Config{}, err
	}

	if err := validateColumns(settings.Columns, "columns"); err != nil {
		return Config{}, err
	}

	if err := validateKeybindings(settings.Keybindings, "keybindings"); err != nil {
		return Config{}, err
	}

	if err := validateWorkspaces(settings.Workspaces, "workspaces"); err != nil {
		return Config{}, err
	}

	return Config{
		LinearAPIKey:     apiKey,
		APIEndpoint:      settings.APIEndpoint,
		Timeout:          timeout,
		PageSize:         settings.PageSize,
		CacheTTL:         cacheTTL,
		SearchDebounce:   searchDebounce,
		LogFile:          settings.ResolvedLogFile(),
		LogLevel:         settings.LogLevel,
		Theme:            theme,
		Density:          density,
		GroupBy:          settings.GroupBy,
		SubgroupBy:       settings.SubgroupBy,
		SortBy:           settings.SortBy,
		Columns:          settings.Columns,
		RoundedBorders:   settings.RoundedBorders,
		Images:           images,
		Workspaces:       settings.Workspaces,
		DefaultWorkspace: settings.DefaultWorkspace,
		AgentProvider:    settings.AgentProvider,
		AgentSandbox:     settings.AgentSandbox,
		AgentModel:       settings.AgentModel,
		AgentWorkspace:   settings.AgentWorkspace,
		Keybindings:      settings.Keybindings,
		DefaultTeam:      settings.DefaultTeam,
		DefaultProject:   settings.DefaultProject,
		SessionRestore:   settings.SessionRestore,
		UpdateCheck:      settings.UpdateCheck,
	}, nil
}

// ConfigFilePath returns $XDG_CONFIG_HOME/zen-linear/config.json when it exists,
// else config.json under Dir().
func ConfigFilePath() (string, error) {
	const name = "config.json"

	xdg, err := xdgDir()
	if err != nil {
		return "", err
	}
	preferred := filepath.Join(xdg, name)
	if _, err := os.Stat(preferred); err == nil {
		return preferred, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("stat settings file %s: %w", preferred, err)
	}

	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, name), nil
}

func EnsureSettingsFile(path string) (Settings, error) {
	if path == "" {
		return Settings{}, fmt.Errorf("settings path is empty")
	}

	if _, err := os.Stat(path); err == nil {
		return LoadSettings(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Settings{}, fmt.Errorf("stat settings file: %w", err)
	}

	settings := DefaultSettings()
	if err := SaveSettings(path, settings); err != nil {
		return Settings{}, err
	}

	return settings, nil
}

func LoadSettings(path string) (Settings, error) {
	if path == "" {
		return Settings{}, fmt.Errorf("settings path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Settings{}, fmt.Errorf("read settings file: %w", err)
	}

	var file SettingsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return Settings{}, fmt.Errorf("parse settings file: %w", err)
	}

	settings := DefaultSettings()
	if file.APIEndpoint != nil {
		settings.APIEndpoint = *file.APIEndpoint
	}
	if file.Timeout != nil {
		settings.Timeout = *file.Timeout
	}
	if file.PageSize != nil {
		settings.PageSize = *file.PageSize
	}
	if file.CacheTTL != nil {
		settings.CacheTTL = *file.CacheTTL
	}
	if file.SearchDebounce != nil {
		settings.SearchDebounce = *file.SearchDebounce
	}
	settings.LogFile = file.LogFile
	if file.LogLevel != nil {
		settings.LogLevel = *file.LogLevel
	}
	if file.Theme != nil {
		settings.Theme = *file.Theme
	}
	if file.Density != nil {
		settings.Density = *file.Density
	}
	if file.GroupBy != nil {
		settings.GroupBy = *file.GroupBy
	}
	if file.SubgroupBy != nil {
		settings.SubgroupBy = *file.SubgroupBy
	}
	if file.SortBy != nil {
		settings.SortBy = file.SortBy
	}
	if file.Columns != nil {
		settings.Columns = file.Columns
	}
	if file.RoundedBorders != nil {
		settings.RoundedBorders = *file.RoundedBorders
	}
	if file.Images != nil {
		settings.Images = *file.Images
	}
	if file.AgentProvider != nil {
		settings.AgentProvider = *file.AgentProvider
	}
	if file.AgentSandbox != nil {
		settings.AgentSandbox = *file.AgentSandbox
	}
	if file.AgentModel != nil {
		settings.AgentModel = *file.AgentModel
	}
	if file.AgentWorkspace != nil {
		settings.AgentWorkspace = *file.AgentWorkspace
	}
	if file.Keybindings != nil {
		settings.Keybindings = file.Keybindings
	}
	if file.DefaultTeam != nil {
		settings.DefaultTeam = *file.DefaultTeam
	}
	if file.DefaultProject != nil {
		settings.DefaultProject = *file.DefaultProject
	}
	if file.Workspaces != nil {
		settings.Workspaces = file.Workspaces
	}
	if file.DefaultWorkspace != nil {
		settings.DefaultWorkspace = *file.DefaultWorkspace
	}
	if file.SessionRestore != nil {
		settings.SessionRestore = *file.SessionRestore
	}
	if file.UpdateCheck != nil {
		settings.UpdateCheck = *file.UpdateCheck
	}

	return settings, nil
}

// Written in place, not atomically, so a hard link to the config keeps its inode.
func SaveSettings(path string, settings Settings) error {
	if path == "" {
		return fmt.Errorf("settings path is empty")
	}

	if _, err := EnsureDirFor(path); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write settings file: %w", err)
	}

	return nil
}

func parseDuration(value string, label string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: %w", label, value, err)
	}

	return duration, nil
}

func parsePositiveDuration(value string, label string) (time.Duration, error) {
	duration, err := parseDuration(value, label)
	if err != nil {
		return 0, err
	}
	if duration <= 0 {
		return 0, fmt.Errorf("%s must be greater than 0, got %s", label, duration)
	}
	return duration, nil
}

func validatePageSize(pageSize int, label string) error {
	if pageSize < 1 || pageSize > 250 {
		return fmt.Errorf("%s must be between 1 and 250, got %d", label, pageSize)
	}

	return nil
}

func validateLogLevel(logLevel string, label string) error {
	switch logLevel {
	case "debug", "info", "warning", "error":
		return nil
	default:
		return fmt.Errorf("invalid %s value %q: must be debug, info, warning, or error", label, logLevel)
	}
}

func validateTheme(theme string, label string) error {
	switch theme {
	case ThemeTerminal, ThemeLinear, ThemeHighContrast, ThemeColorBlind, ThemeRosePineMoon:
		return nil
	default:
		return fmt.Errorf("invalid %s value %q: must be terminal, linear, high_contrast, color_blind, or rose_pine_moon", label, theme)
	}
}

func validateKeybindings(bindings map[string]string, label string) error {
	used := make(map[string]string, len(bindings))
	for action, key := range bindings {
		if len([]rune(key)) != 1 {
			return fmt.Errorf("invalid %s entry %q: key %q must be a single character", label, action, key)
		}
		if previous, taken := used[key]; taken {
			return fmt.Errorf("invalid %s: key %q bound to both %q and %q", label, key, previous, action)
		}
		used[key] = action
	}
	return nil
}

func validateColumns(columns []string, label string) error {
	known := map[string]bool{
		"priority": true, "id": true, "state": true, "title": true,
		"labels": true, "assignee": true, "updated": true,
		"cycle": true, "due": true, "estimate": true,
		"project": true, "milestone": true,
	}
	seen := make(map[string]bool, len(columns))
	for _, column := range columns {
		if !known[column] {
			return fmt.Errorf("invalid %s entry %q: unknown column", label, column)
		}
		if seen[column] {
			return fmt.Errorf("invalid %s entry %q: duplicate column", label, column)
		}
		seen[column] = true
	}
	return nil
}

func validateSortBy(fields []string, label string) error {
	canonical := map[string]string{
		"status": "status", "priority": "priority",
		"updated": "updated", "updatedat": "updated",
		"created": "created", "createdat": "created",
	}
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		name, known := canonical[strings.ToLower(strings.TrimSpace(field))]
		if !known {
			return fmt.Errorf("invalid %s entry %q: must be status, priority, updated, or created", label, field)
		}
		if seen[name] {
			return fmt.Errorf("invalid %s entry %q: duplicate sort field", label, field)
		}
		seen[name] = true
	}
	return nil
}

func validateGroupDimension(dimension string, label string) error {
	switch dimension {
	case "", "status", "priority", "assignee", "cycle", "project", "milestone":
		return nil
	default:
		return fmt.Errorf("invalid %s value %q: must be status, priority, assignee, cycle, project, milestone, or empty", label, dimension)
	}
}

func validateWorkspaces(workspaces []Workspace, label string) error {
	seen := make(map[string]bool, len(workspaces))
	for i, workspace := range workspaces {
		name := strings.TrimSpace(workspace.Name)
		if name == "" {
			return fmt.Errorf("invalid %s entry %d: name is required", label, i)
		}
		if strings.TrimSpace(workspace.APIKeyEnv) == "" {
			return fmt.Errorf("invalid %s entry %q: api_key_env is required", label, name)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("invalid %s entry %q: duplicate workspace name", label, name)
		}
		seen[key] = true
	}
	return nil
}

func validateDensity(density string, label string) error {
	switch density {
	case DensityComfortable, DensityCompact:
		return nil
	default:
		return fmt.Errorf("invalid %s value %q: must be comfortable or compact", label, density)
	}
}

func validateImages(images string, label string) error {
	switch images {
	case ImagesAuto, ImagesOff:
		return nil
	default:
		return fmt.Errorf("invalid %s value %q: must be auto or off", label, images)
	}
}

func validateAgentProvider(provider string, label string) error {
	switch provider {
	case "cursor", "claude":
		return nil
	default:
		return fmt.Errorf("invalid %s value %q: must be cursor or claude", label, provider)
	}
}

func validateAgentSandbox(sandbox string, label string) error {
	switch sandbox {
	case "enabled", "disabled":
		return nil
	default:
		return fmt.Errorf("invalid %s value %q: must be enabled or disabled", label, sandbox)
	}
}
