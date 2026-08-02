package agents

import (
	"fmt"
	"strings"
)

// Provider keys as they appear in config and on the agent CLIs' own binaries.
const (
	ProviderCursor = "cursor"
	ProviderClaude = "claude"
)

// AvailableProviderKeys returns provider keys with resolvable binaries.
func AvailableProviderKeys(lookPath func(string) (string, error)) []string {
	providers := []struct {
		key      string
		provider Provider
	}{
		{key: ProviderCursor, provider: NewCursorProvider(lookPath)},
		{key: ProviderClaude, provider: NewClaudeProvider(lookPath)},
	}

	available := make([]string, 0, len(providers))
	for _, entry := range providers {
		if _, ok := entry.provider.ResolveBinary(); ok {
			available = append(available, entry.key)
		}
	}
	return available
}

// ProviderForKey constructs a provider for the given config key.
func ProviderForKey(key string, lookPath func(string) (string, error)) (Provider, error) {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case ProviderCursor:
		return NewCursorProvider(lookPath), nil
	case ProviderClaude:
		return NewClaudeProvider(lookPath), nil
	default:
		return nil, fmt.Errorf("invalid agent provider %q", key)
	}
}
