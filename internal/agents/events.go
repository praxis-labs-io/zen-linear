package agents

// AgentEventType represents a high-level streaming event type.
type AgentEventType string

const (
	AgentEventSystem         AgentEventType = "system"
	AgentEventUser           AgentEventType = "user"
	AgentEventAssistant      AgentEventType = "assistant"
	AgentEventAssistantDelta AgentEventType = "assistant_delta"
	AgentEventThinking       AgentEventType = "thinking"
	AgentEventToolCall       AgentEventType = "tool_call"
	AgentEventResult         AgentEventType = "result"
	AgentEventUnknown        AgentEventType = "unknown"
)

// AgentEvent captures a parsed stream event for UI rendering.
type AgentEvent struct {
	Type          AgentEventType
	Subtype       string
	Text          string
	Model         string
	SessionID     string
	ResumeCommand string
	DurationMs    int64
	IsError       bool
	Tool          *AgentToolCall
}

// AgentToolCall captures tool call details for display.
type AgentToolCall struct {
	Name    string
	Path    string
	Status  string
	Summary string
}

// EventParser allows providers to emit structured events.
type EventParser interface {
	// ParseEvent attempts to decode a raw stream line into an AgentEvent.
	ParseEvent(line []byte) (*AgentEvent, bool)
}
