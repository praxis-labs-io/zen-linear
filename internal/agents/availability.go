package agents

import (
	"fmt"
	"strings"
)

// AvailableProviderKeys returns provider keys with resolvable binaries.
func AvailableProviderKeys(lookPath func(string) (string, error)) []string {
	providers := []struct {
		key      string
		provider Provider
	}{
		{key: "cursor", provider: NewCursorProvider(lookPath)},
		{key: "claude", provider: NewClaudeProvider(lookPath)},
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
	case "cursor":
		return NewCursorProvider(lookPath), nil
	case "claude":
		return NewClaudeProvider(lookPath), nil
	default:
		return nil, fmt.Errorf("invalid agent provider %q", key)
	}
}
