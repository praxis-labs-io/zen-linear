package agents

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

const SubtypeCompleted = "completed"

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

type AgentToolCall struct {
	Name    string
	Path    string
	Status  string
	Summary string
}

type EventParser interface {
	ParseEvent(line []byte) (*AgentEvent, bool)
}
