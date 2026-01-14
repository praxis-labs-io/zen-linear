package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Environment variable names for configuration.
const (
	LinearAPIKeyEnv   = "LINEAR_API_KEY"
	LinearAPIEndpoint = "LINEAR_API_ENDPOINT"
	TimeoutEnv        = "LINEAR_TIMEOUT"
	PageSizeEnv       = "LINEAR_PAGE_SIZE"
	CacheTTLEnv       = "LINEAR_CACHE_TTL"
	LogFileEnv        = "LINEAR_LOG_FILE"
	LogLevelEnv       = "LINEAR_LOG_LEVEL"
)

// Default configuration values.
const (
	DefaultTimeout     = 30 * time.Second
	DefaultPageSize    = 50
	DefaultCacheTTL    = 5 * time.Minute
	DefaultAPIEndpoint = "https://api.linear.app/graphql"
	DefaultLogLevel    = "warning" // debug, info, warning, error
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
	// LinearAPIKey is the API key for authenticating with Linear.
	LinearAPIKey string

	// APIEndpoint is the Linear GraphQL API endpoint (useful for testing).
	APIEndpoint string

	// Timeout is the HTTP request timeout for API calls.
	Timeout time.Duration

	// PageSize is the default number of items to fetch per page.
	PageSize int

	// CacheTTL is the time-to-live for cached team metadata.
	CacheTTL time.Duration

	// LogFile is the path to the log file (empty to disable logging).
	LogFile string

	// LogLevel is the minimum log level (debug, info, warning, error).
	LogLevel string
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
		LinearAPIKey: apiKey,
		APIEndpoint:  DefaultAPIEndpoint,
		Timeout:      DefaultTimeout,
		PageSize:     DefaultPageSize,
		CacheTTL:     DefaultCacheTTL,
		LogFile:      getDefaultLogFile(), // Default: $HOME/.linear-tui/app.log
		LogLevel:     DefaultLogLevel,
	}

	// Parse optional API endpoint override.
	if endpoint := os.Getenv(LinearAPIEndpoint); endpoint != "" {
		cfg.APIEndpoint = endpoint
	}

	// Parse optional timeout.
	if timeoutStr := os.Getenv(TimeoutEnv); timeoutStr != "" {
		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s value %q: %w", TimeoutEnv, timeoutStr, err)
		}
		cfg.Timeout = timeout
	}

	// Parse optional page size.
	if pageSizeStr := os.Getenv(PageSizeEnv); pageSizeStr != "" {
		pageSize, err := strconv.Atoi(pageSizeStr)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s value %q: %w", PageSizeEnv, pageSizeStr, err)
		}
		if pageSize < 1 || pageSize > 250 {
			return Config{}, fmt.Errorf("%s must be between 1 and 250, got %d", PageSizeEnv, pageSize)
		}
		cfg.PageSize = pageSize
	}

	// Parse optional cache TTL.
	if cacheTTLStr := os.Getenv(CacheTTLEnv); cacheTTLStr != "" {
		cacheTTL, err := time.ParseDuration(cacheTTLStr)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s value %q: %w", CacheTTLEnv, cacheTTLStr, err)
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
		// Validate log level
		switch logLevel {
		case "debug", "info", "warning", "error":
			cfg.LogLevel = logLevel
		default:
			return Config{}, fmt.Errorf("invalid %s value %q: must be debug, info, warning, or error", LogLevelEnv, logLevel)
		}
	}

	return cfg, nil
}
