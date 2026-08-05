package agents

import (
	"slices"
	"strings"
	"testing"
)

// TestCursorProvider_BuildArgs pins the whole argv rather than grepping it. A
// substring check is what let --sandbox and --workspace ship for months against
// a cursor-agent that has neither, failing every run. Adding a flag has to
// break this test.
func TestCursorProvider_BuildArgs(t *testing.T) {
	prompt := buildAgentPrompt("Summarize", "Issue context")

	tests := []struct {
		name    string
		options AgentRunOptions
		want    []string
	}{
		{
			name:    "sandbox disabled forces commands through",
			options: AgentRunOptions{Sandbox: "disabled", Model: "gpt-5.2", Workspace: "/tmp/workspace"},
			want:    []string{"--force", "--print", "--output-format", "stream-json", "--model", "gpt-5.2", "-p", prompt},
		},
		{
			name:    "sandbox enabled leaves the agent asking",
			options: AgentRunOptions{Sandbox: "enabled", Model: "gpt-5.2", Workspace: "/tmp/workspace"},
			want:    []string{"--print", "--output-format", "stream-json", "--model", "gpt-5.2", "-p", prompt},
		},
		{
			name:    "unset sandbox asks",
			options: AgentRunOptions{Model: "gpt-5.2"},
			want:    []string{"--print", "--output-format", "stream-json", "--model", "gpt-5.2", "-p", prompt},
		},
		{
			name:    "no model drops the flag",
			options: AgentRunOptions{Sandbox: "disabled"},
			want:    []string{"--force", "--print", "--output-format", "stream-json", "-p", prompt},
		},
	}

	provider := NewCursorProvider(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := provider.BuildArgs("Summarize", "Issue context", tt.options)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("BuildArgs() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCursorProvider_ParseStreamLine verifies text extraction.
func TestCursorProvider_ParseStreamLine(t *testing.T) {
	provider := NewCursorProvider(nil)

	display, ok := provider.ParseStreamLine([]byte(`{"type":"system","subtype":"init"}`))
	if !ok || !strings.Contains(display, "System init") {
		t.Fatalf("expected system event line, got %q (ok=%v)", display, ok)
	}

	display, ok = provider.ParseStreamLine([]byte(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"prompt text"}]}}`))
	if !ok || !strings.Contains(display, "User:") || !strings.Contains(display, "prompt text") {
		t.Fatalf("expected user event line, got %q (ok=%v)", display, ok)
	}

	display, ok = provider.ParseStreamLine([]byte(`{"delta":{"content":"hello"}}`))
	if !ok || !strings.Contains(display, "Assistant delta") {
		t.Fatalf("expected delta line, got %q (ok=%v)", display, ok)
	}

	display, ok = provider.ParseStreamLine([]byte(`{"type":"thinking","text":"working "}`))
	if !ok || !strings.Contains(display, "Thinking:") {
		t.Fatalf("expected thinking line, got %q (ok=%v)", display, ok)
	}

	display, ok = provider.ParseStreamLine([]byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"message text"}]}}`))
	if !ok || !strings.Contains(display, "Assistant:") {
		t.Fatalf("expected assistant message line, got %q (ok=%v)", display, ok)
	}

	display, ok = provider.ParseStreamLine([]byte(`{"type":"result","result":"world"}`))
	if !ok || !strings.Contains(display, "Result") {
		t.Fatalf("expected result event line, got %q (ok=%v)", display, ok)
	}

	display, ok = provider.ParseStreamLine([]byte(`{"type":"tool_call","subtype":"started","tool_call":{"readToolCall":{"args":{"path":"README.md"}}}}`))
	if !ok || !strings.Contains(display, "Tool call started") {
		t.Fatalf("expected tool call started line, got %q (ok=%v)", display, ok)
	}

	_, ok = provider.ParseStreamLine([]byte("not-json"))
	if ok {
		t.Fatalf("expected non-json to return ok=false")
	}
}
