package config

import (
	"fmt"
	"os"
	"strconv"
)

const (
	FieldAPIEndpoint = "api_endpoint"
	FieldTimeout     = "timeout"
	FieldPageSize    = "page_size"
	FieldCacheTTL    = "cache_ttl"
	FieldLogFile     = "log_file"
	FieldLogLevel    = "log_level"
)

// EnvOverrides maps a settings field id to the environment variable that set it.
type EnvOverrides map[string]string

func (e EnvOverrides) Has(field string) bool {
	_, ok := e[field]
	return ok
}

// ApplyEnvOverrides returns settings with the environment applied and the fields
// it set. A malformed value is an error.
func ApplyEnvOverrides(settings Settings) (Settings, EnvOverrides, error) {
	overrides := EnvOverrides{}

	if endpoint := os.Getenv(LinearAPIEndpoint); endpoint != "" {
		settings.APIEndpoint = endpoint
		overrides[FieldAPIEndpoint] = LinearAPIEndpoint
	}

	if timeout := os.Getenv(TimeoutEnv); timeout != "" {
		if _, err := parseDuration(timeout, TimeoutEnv); err != nil {
			return Settings{}, nil, err
		}
		settings.Timeout = timeout
		overrides[FieldTimeout] = TimeoutEnv
	}

	if pageSize := os.Getenv(PageSizeEnv); pageSize != "" {
		parsed, err := strconv.Atoi(pageSize)
		if err != nil {
			return Settings{}, nil, fmt.Errorf("invalid %s value %q: %w", PageSizeEnv, pageSize, err)
		}
		if err := validatePageSize(parsed, PageSizeEnv); err != nil {
			return Settings{}, nil, err
		}
		settings.PageSize = parsed
		overrides[FieldPageSize] = PageSizeEnv
	}

	if cacheTTL := os.Getenv(CacheTTLEnv); cacheTTL != "" {
		if _, err := parseDuration(cacheTTL, CacheTTLEnv); err != nil {
			return Settings{}, nil, err
		}
		settings.CacheTTL = cacheTTL
		overrides[FieldCacheTTL] = CacheTTLEnv
	}

	if logFile, ok := os.LookupEnv(LogFileEnv); ok {
		settings.LogFile = &logFile
		overrides[FieldLogFile] = LogFileEnv
	}

	if logLevel := os.Getenv(LogLevelEnv); logLevel != "" {
		if err := validateLogLevel(logLevel, LogLevelEnv); err != nil {
			return Settings{}, nil, err
		}
		settings.LogLevel = logLevel
		overrides[FieldLogLevel] = LogLevelEnv
	}

	return settings, overrides, nil
}
