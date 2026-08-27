package config

import (
	"fmt"
	"os"
	"strconv"
)

// Field ids for the settings an environment variable can override. They are
// the config file's own keys, so a message naming one names something the
// reader can find in config.json.
const (
	FieldAPIEndpoint = "api_endpoint"
	FieldTimeout     = "timeout"
	FieldPageSize    = "page_size"
	FieldCacheTTL    = "cache_ttl"
	FieldLogFile     = "log_file"
	FieldLogLevel    = "log_level"
)

// EnvOverrides maps a field id to the variable that set it. Empty when the
// environment says nothing, which is the usual case.
type EnvOverrides map[string]string

// Has reports whether the environment owns this field.
func (e EnvOverrides) Has(field string) bool {
	_, ok := e[field]
	return ok
}

// ApplyEnvOverrides returns settings with the environment applied and a map of
// what it took over. The environment wins over the file, as the docs say.
//
// It runs on Settings rather than Config because a settings save rebuilds the
// config from the form (`ConfigFromSettings`): an override layered onto Config
// would be discarded the moment the user saved an unrelated field.
//
// A malformed value fails the launch rather than being ignored. An override
// that silently does nothing is what this whole ticket is about.
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

	// LookupEnv rather than Getenv: an empty LINEAR_LOG_FILE is a deliberate
	// "logging off", the same as an empty log_file in the file, where an unset
	// one leaves whatever the file said.
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
