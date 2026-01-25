package agents

import "testing"

// TestCursorProvider_ParseEvent_System verifies system init parsing.
func TestCursorProvider_ParseEvent_System(t *testing.T) {
	provider := NewCursorProvider(nil)
	line := []byte(`{"type":"system","subtype":"init","model":"GPT-5.2","cwd":"/tmp","session_id":"abc","permissionMode":"default","apiKeySource":"login"}`)

	event, ok := provider.ParseEvent(line)
	if !ok || event == nil {
		t.Fatalf("expected system event to parse")
	}
	if event.Type != AgentEventSystem {
		t.Fatalf("expected system type, got %s", event.Type)
	}
	if event.Model != "GPT-5.2" {
		t.Fatalf("expected model to be parsed, got %q", event.Model)
	}
	if event.SessionID != "abc" {
		t.Fatalf("expected session id to be parsed, got %q", event.SessionID)
	}
	if event.ResumeCommand != "cursor-agent --resume abc" {
		t.Fatalf("expected resume command to be parsed, got %q", event.ResumeCommand)
	}
}

// TestCursorProvider_ParseEvent_User verifies user prompt parsing.
func TestCursorProvider_ParseEvent_User(t *testing.T) {
	provider := NewCursorProvider(nil)
	line := []byte(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"prompt text"}]}}`)

	event, ok := provider.ParseEvent(line)
	if !ok || event == nil {
		t.Fatalf("expected user event to parse")
	}
	if event.Type != AgentEventUser {
		t.Fatalf("expected user type, got %s", event.Type)
	}
	if event.Text != "prompt text" {
		t.Fatalf("expected prompt text, got %q", event.Text)
	}
}

// TestCursorProvider_ParseEvent_Assistant verifies assistant parsing.
func TestCursorProvider_ParseEvent_Assistant(t *testing.T) {
	provider := NewCursorProvider(nil)
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

// TestCursorProvider_ParseEvent_Thinking verifies thinking parsing.
func TestCursorProvider_ParseEvent_Thinking(t *testing.T) {
	provider := NewCursorProvider(nil)
	line := []byte(`{"type":"thinking","text":"reasoning chunk"}`)

	event, ok := provider.ParseEvent(line)
	if !ok || event == nil {
		t.Fatalf("expected thinking event to parse")
	}
	if event.Type != AgentEventThinking {
		t.Fatalf("expected thinking type, got %s", event.Type)
	}
	if event.Text != "reasoning chunk" {
		t.Fatalf("expected thinking text, got %q", event.Text)
	}
}

// TestCursorProvider_ParseEvent_ToolCall verifies tool call parsing.
func TestCursorProvider_ParseEvent_ToolCall(t *testing.T) {
	provider := NewCursorProvider(nil)
	line := []byte(`{"type":"tool_call","subtype":"started","tool_call":{"readToolCall":{"args":{"path":"README.md"}}}}`)

	event, ok := provider.ParseEvent(line)
	if !ok || event == nil {
		t.Fatalf("expected tool call event to parse")
	}
	if event.Type != AgentEventToolCall {
		t.Fatalf("expected tool call type, got %s", event.Type)
	}
	if event.Subtype != "started" {
		t.Fatalf("expected subtype started, got %q", event.Subtype)
	}
	if event.Tool == nil || event.Tool.Name != "read" || event.Tool.Path != "README.md" {
		t.Fatalf("unexpected tool details: %#v", event.Tool)
	}
}

// TestCursorProvider_ParseEvent_Result verifies result parsing.
func TestCursorProvider_ParseEvent_Result(t *testing.T) {
	provider := NewCursorProvider(nil)
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
}
