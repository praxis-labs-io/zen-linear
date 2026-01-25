package agents

import (
	"os/exec"
	"reflect"
	"testing"
)

// stubLookPath returns a lookPath implementation that reports configured binaries.
func stubLookPath(available map[string]bool) func(string) (string, error) {
	return func(name string) (string, error) {
		if available[name] {
			return "/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	}
}

// TestAvailableProviderKeys verifies availability detection across providers.
func TestAvailableProviderKeys(t *testing.T) {
	tests := []struct {
		name      string
		available map[string]bool
		want      []string
	}{
		{
			name: "cursor_and_claude",
			available: map[string]bool{
				"cursor-agent": true,
				"claude":       true,
			},
			want: []string{"cursor", "claude"},
		},
		{
			name: "cursor_agent_fallback",
			available: map[string]bool{
				"agent": true,
			},
			want: []string{"cursor"},
		},
		{
			name: "claude_only",
			available: map[string]bool{
				"claude": true,
			},
			want: []string{"claude"},
		},
		{
			name:      "none",
			available: map[string]bool{},
			want:      []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AvailableProviderKeys(stubLookPath(tt.available))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("AvailableProviderKeys() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestProviderForKey verifies provider construction by key.
func TestProviderForKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		wantName string
		wantErr  bool
	}{
		{
			name:     "cursor",
			key:      "cursor",
			wantName: "Cursor",
		},
		{
			name:     "claude_normalized",
			key:      "  Claude ",
			wantName: "Claude",
		},
		{
			name:    "invalid",
			key:     "unknown",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := ProviderForKey(tt.key, stubLookPath(map[string]bool{}))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ProviderForKey() expected error")
				}
				if provider != nil {
					t.Fatalf("ProviderForKey() expected nil provider")
				}
				return
			}
			if err != nil {
				t.Fatalf("ProviderForKey() error: %v", err)
			}
			if provider == nil {
				t.Fatalf("ProviderForKey() returned nil provider")
			}
			if provider.Name() != tt.wantName {
				t.Fatalf("ProviderForKey() name = %q, want %q", provider.Name(), tt.wantName)
			}
		})
	}
}
