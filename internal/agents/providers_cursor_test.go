package agents

import (
	"strings"
	"testing"
)

// TestCursorProvider_BuildArgs verifies CLI args include print + stream-json.
func TestCursorProvider_BuildArgs(t *testing.T) {
	provider := NewCursorProvider(nil)
	options := AgentRunOptions{
		Sandbox:   "enabled",
		Model:     "gpt-5.2",
		Workspace: "/tmp/workspace",
	}
	args := provider.BuildArgs("Summarize", "Issue context", options)

	if len(args) == 0 {
		t.Fatal("expected args, got none")
	}

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--force") {
		t.Fatalf("expected --force in args: %s", joined)
	}
	if !strings.Contains(joined, "-p") && !strings.Contains(joined, "--print") {
		t.Fatalf("expected print mode in args: %s", joined)
	}
	if !strings.Contains(joined, "--output-format") || !strings.Contains(joined, "stream-json") {
		t.Fatalf("expected stream-json output in args: %s", joined)
	}
	if !strings.Contains(joined, "--sandbox") || !strings.Contains(joined, "enabled") {
		t.Fatalf("expected sandbox option in args: %s", joined)
	}
	if !strings.Contains(joined, "--model") || !strings.Contains(joined, "gpt-5.2") {
		t.Fatalf("expected model option in args: %s", joined)
	}
	if !strings.Contains(joined, "--workspace") || !strings.Contains(joined, "/tmp/workspace") {
		t.Fatalf("expected workspace option in args: %s", joined)
	}
	if !strings.Contains(joined, "Summarize") || !strings.Contains(joined, "Issue context") {
		t.Fatalf("expected prompt and context in args: %s", joined)
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
