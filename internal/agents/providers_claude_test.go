package agents

import (
	"strings"
	"testing"
)

// TestClaudeProvider_BuildArgs verifies CLI args include print + stream-json.
func TestClaudeProvider_BuildArgs(t *testing.T) {
	provider := NewClaudeProvider(nil)
	options := AgentRunOptions{
		Sandbox:   "disabled",
		Model:     "claude-test",
		Workspace: "/tmp/workspace",
	}
	args := provider.BuildArgs("Do the thing", "Context text", options)

	if len(args) == 0 {
		t.Fatal("expected args, got none")
	}

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-p") && !strings.Contains(joined, "--print") {
		t.Fatalf("expected print mode in args: %s", joined)
	}
	if !strings.Contains(joined, "--verbose") {
		t.Fatalf("expected verbose flag in args: %s", joined)
	}
	if !strings.Contains(joined, "--output-format") || !strings.Contains(joined, "stream-json") {
		t.Fatalf("expected stream-json output in args: %s", joined)
	}
	if !strings.Contains(joined, "--include-partial-messages") {
		t.Fatalf("expected include-partial-messages in args: %s", joined)
	}
	if !strings.Contains(joined, "--model") || !strings.Contains(joined, "claude-test") {
		t.Fatalf("expected model in args: %s", joined)
	}
	if !strings.Contains(joined, "--add-dir") || !strings.Contains(joined, "/tmp/workspace") {
		t.Fatalf("expected add-dir in args: %s", joined)
	}
	if !strings.Contains(joined, "--permission-mode") || !strings.Contains(joined, "bypassPermissions") {
		t.Fatalf("expected permission-mode in args: %s", joined)
	}
	if !strings.Contains(joined, "Do the thing") || !strings.Contains(joined, "Context text") {
		t.Fatalf("expected prompt and context in args: %s", joined)
	}
}

// TestClaudeProvider_ParseEvent_System verifies system init parsing.
func TestClaudeProvider_ParseEvent_System(t *testing.T) {
	provider := NewClaudeProvider(nil)
	line := []byte(`{"type":"system","subtype":"init","session_id":"abc","model":"claude-sonnet"}`)

	event, ok := provider.ParseEvent(line)
	if !ok || event == nil {
		t.Fatalf("expected system event to parse")
	}
	if event.Type != AgentEventSystem {
		t.Fatalf("expected system type, got %s", event.Type)
	}
	if event.SessionID != "abc" {
		t.Fatalf("expected session id to be parsed, got %q", event.SessionID)
	}
	if event.ResumeCommand != "claude --resume abc" {
		t.Fatalf("expected resume command, got %q", event.ResumeCommand)
	}
}

// TestClaudeProvider_ParseEvent_Assistant verifies assistant text parsing.
func TestClaudeProvider_ParseEvent_Assistant(t *testing.T) {
	provider := NewClaudeProvider(nil)
	line := []byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"assistant reply"}]}}`)

	event, ok := provider.ParseEvent(line)
	if !ok || event == nil {
		t.Fatalf("expected assistant event to parse")
	}
	if event.Type != AgentEventAssistant {
		t.Fatalf("expected assistant type, got %s", event.Type)
	}
	if event.Text != "assistant reply" {
		t.Fatalf("expected assistant text, got %q", event.Text)
	}
}

// TestClaudeProvider_ParseEvent_ToolUseAndResult verifies tool call parsing and correlation.
func TestClaudeProvider_ParseEvent_ToolUseAndResult(t *testing.T) {
	provider := NewClaudeProvider(nil)
	toolUseLine := []byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Glob","input":{"pattern":"README*"}}]}}`)

	event, ok := provider.ParseEvent(toolUseLine)
	if !ok || event == nil {
		t.Fatalf("expected tool use event to parse")
	}
	if event.Type != AgentEventToolCall {
		t.Fatalf("expected tool call type, got %s", event.Type)
	}
	if event.Subtype != "started" {
		t.Fatalf("expected subtype started, got %q", event.Subtype)
	}
	if event.Tool == nil || event.Tool.Name != "Glob" || event.Tool.Path != "README*" {
		t.Fatalf("unexpected tool details: %#v", event.Tool)
	}

	toolResultLine := []byte(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"README.md"}]}}`)
	event, ok = provider.ParseEvent(toolResultLine)
	if !ok || event == nil {
		t.Fatalf("expected tool result event to parse")
	}
	if event.Type != AgentEventToolCall {
		t.Fatalf("expected tool call type, got %s", event.Type)
	}
	if event.Subtype != "completed" {
		t.Fatalf("expected subtype completed, got %q", event.Subtype)
	}
	if event.Tool == nil || event.Tool.Name != "Glob" || event.Tool.Path != "README*" {
		t.Fatalf("unexpected tool details: %#v", event.Tool)
	}
	if !strings.Contains(event.Tool.Summary, "README.md") {
		t.Fatalf("expected tool summary to include README.md, got %q", event.Tool.Summary)
	}
}

// TestClaudeProvider_ParseEvent_ToolResultString verifies string tool_use_result parsing.
func TestClaudeProvider_ParseEvent_ToolResultString(t *testing.T) {
	provider := NewClaudeProvider(nil)
	toolUseLine := []byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_2","name":"Read","input":{"path":"README.md"}}]}}`)

	_, ok := provider.ParseEvent(toolUseLine)
	if !ok {
		t.Fatalf("expected tool use event to parse")
	}

	toolResultLine := []byte(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_2","content":"ok"}]},"tool_use_result":"ok"}`)
	event, ok := provider.ParseEvent(toolResultLine)
	if !ok || event == nil {
		t.Fatalf("expected tool result event to parse")
	}
	if event.Type != AgentEventToolCall {
		t.Fatalf("expected tool call type, got %s", event.Type)
	}
	if event.Tool == nil || !strings.Contains(event.Tool.Summary, "ok") {
		t.Fatalf("expected tool summary to include ok, got %#v", event.Tool)
	}
}

// TestClaudeProvider_ParseEvent_Result verifies result parsing.
func TestClaudeProvider_ParseEvent_Result(t *testing.T) {
	provider := NewClaudeProvider(nil)
	line := []byte(`{"type":"result","subtype":"success","duration_ms":1234,"is_error":false}`)

	event, ok := provider.ParseEvent(line)
	if !ok || event == nil {
		t.Fatalf("expected result event to parse")
	}
	if event.Type != AgentEventResult {
		t.Fatalf("expected result type, got %s", event.Type)
	}
	if event.DurationMs != 1234 {
		t.Fatalf("expected duration 1234, got %d", event.DurationMs)
	}
	if event.IsError {
		t.Fatalf("expected isError=false")
	}
}

// TestClaudeProvider_ParseEvent_Delta verifies delta parsing.
func TestClaudeProvider_ParseEvent_Delta(t *testing.T) {
	provider := NewClaudeProvider(nil)
	line := []byte(`{"delta":{"text":"partial"}}`)

	event, ok := provider.ParseEvent(line)
	if !ok || event == nil {
		t.Fatalf("expected delta event to parse")
	}
	if event.Type != AgentEventAssistantDelta {
		t.Fatalf("expected assistant delta type, got %s", event.Type)
	}
	if event.Text != "partial" {
		t.Fatalf("expected delta text, got %q", event.Text)
	}
}

// TestClaudeProvider_ParseStreamLine verifies text extraction.
func TestClaudeProvider_ParseStreamLine(t *testing.T) {
	provider := NewClaudeProvider(nil)

	display, ok := provider.ParseStreamLine([]byte(`{"delta":{"text":"hello"}}`))
	if !ok || display != "hello" {
		t.Fatalf("expected delta text, got %q (ok=%v)", display, ok)
	}

	display, ok = provider.ParseStreamLine([]byte(`{"message":{"content":[{"type":"text","text":"world"}]}}`))
	if !ok || display != "world" {
		t.Fatalf("expected message content, got %q (ok=%v)", display, ok)
	}

	_, ok = provider.ParseStreamLine([]byte("not-json"))
	if ok {
		t.Fatalf("expected non-json to return ok=false")
	}
}
