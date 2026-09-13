package agents

import (
	"fmt"
	"strings"
)

const (
	ProviderCursor = "cursor"
	ProviderClaude = "claude"
)

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

// ProviderForKey returns the provider for a config key, or an error for an unknown one.
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
