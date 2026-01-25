package agents

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/roeyazroel/linear-tui/internal/logger"
)

// ClaudeProvider invokes the Claude Code CLI.
type ClaudeProvider struct {
	lookPath  func(string) (string, error)
	toolUseMu sync.Mutex
	toolUses  map[string]claudeToolUseInfo
}

// NewClaudeProvider creates a Claude provider with an optional lookPath override.
func NewClaudeProvider(lookPath func(string) (string, error)) *ClaudeProvider {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	return &ClaudeProvider{
		lookPath: lookPath,
		toolUses: make(map[string]claudeToolUseInfo),
	}
}

// Name returns the display name for this provider.
func (p *ClaudeProvider) Name() string {
	return "Claude"
}

// ResolveBinary finds the Claude CLI binary.
func (p *ClaudeProvider) ResolveBinary() (string, bool) {
	path, err := p.lookPath("claude")
	if err != nil {
		return "", false
	}
	return path, true
}

// BuildArgs builds argv for a non-interactive Claude run.
func (p *ClaudeProvider) BuildArgs(prompt string, issueContext string, options AgentRunOptions) []string {
	fullPrompt := buildAgentPrompt(prompt, issueContext)
	args := []string{
		"-p",
		"--verbose",
		"--output-format", "stream-json",
		"--include-partial-messages",
	}
	if options.Model != "" {
		args = append(args, "--model", options.Model)
	}
	if options.Workspace != "" {
		args = append(args, "--add-dir", options.Workspace)
	}
	if mode, ok := claudePermissionMode(options.Sandbox); ok {
		args = append(args, "--permission-mode", mode)
	}
	args = append(args, fullPrompt)
	return args
}

// ParseEvent parses a stream-json line into an AgentEvent.
func (p *ClaudeProvider) ParseEvent(line []byte) (*AgentEvent, bool) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return nil, false
	}

	var event claudeStreamEvent
	if err := json.Unmarshal([]byte(trimmed), &event); err != nil {
		logger.ErrorWithErr(err, "agents.claude: failed to parse stream event")
		return nil, false
	}

	if delta := strings.TrimSpace(event.Delta.Text); delta != "" {
		return &AgentEvent{
			Type: AgentEventAssistantDelta,
			Text: delta,
		}, true
	}

	switch event.Type {
	case "system":
		return &AgentEvent{
			Type:          AgentEventSystem,
			Subtype:       event.Subtype,
			Model:         event.Model,
			SessionID:     event.SessionID,
			ResumeCommand: buildClaudeResumeCommand(event.SessionID),
		}, true
	case "assistant":
		if toolEvent := p.parseClaudeToolUse(event.Message.Content); toolEvent != nil {
			return toolEvent, true
		}
		if text := extractClaudeMessageText(event.Message.Content); text != "" {
			return &AgentEvent{
				Type: AgentEventAssistant,
				Text: text,
			}, true
		}
	case "user":
		if toolEvent := p.parseClaudeToolResult(event); toolEvent != nil {
			return toolEvent, true
		}
		if text := extractClaudeMessageText(event.Message.Content); text != "" {
			return &AgentEvent{
				Type: AgentEventUser,
				Text: text,
			}, true
		}
	case "result":
		if event.IsError {
			logger.Error("agents.claude: result error subtype=%s duration_ms=%d", event.Subtype, event.DurationMs)
		}
		return &AgentEvent{
			Type:       AgentEventResult,
			Subtype:    event.Subtype,
			DurationMs: event.DurationMs,
			IsError:    event.IsError,
		}, true
	}

	if text := extractClaudeEventText(event); text != "" {
		return &AgentEvent{
			Type: AgentEventUnknown,
			Text: text,
		}, true
	}

	return nil, false
}

// ParseStreamLine attempts to extract display text from Claude stream-json.
func (p *ClaudeProvider) ParseStreamLine(line []byte) (string, bool) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return "", false
	}

	var event claudeStreamEvent
	if err := json.Unmarshal([]byte(trimmed), &event); err != nil {
		return "", false
	}
	text := extractClaudeEventText(event)
	if text == "" {
		return "", false
	}
	return text, true
}

// buildClaudeResumeCommand returns a resume command when a session id is available.
func buildClaudeResumeCommand(sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return ""
	}
	return fmt.Sprintf("claude --resume %s", sessionID)
}

// claudePermissionMode maps sandbox settings to Claude permission modes.
func claudePermissionMode(sandbox string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(sandbox)) {
	case "enabled":
		return "default", true
	case "disabled":
		return "bypassPermissions", true
	default:
		return "", false
	}
}

// parseClaudeToolUse converts a tool_use message into a tool call event.
func (p *ClaudeProvider) parseClaudeToolUse(items []claudeMessageContent) *AgentEvent {
	for _, item := range items {
		if item.Type != "tool_use" {
			continue
		}
		detail := summarizeClaudeToolInput(item.Input)
		p.rememberToolUse(item.ID, item.Name, detail)
		return &AgentEvent{
			Type:    AgentEventToolCall,
			Subtype: "started",
			Tool: &AgentToolCall{
				Name:   strings.TrimSpace(item.Name),
				Path:   detail,
				Status: "started",
			},
		}
	}
	return nil
}

// parseClaudeToolResult converts a tool_result message into a tool call event.
func (p *ClaudeProvider) parseClaudeToolResult(event claudeStreamEvent) *AgentEvent {
	for _, item := range event.Message.Content {
		if item.Type != "tool_result" {
			continue
		}
		toolUseID := strings.TrimSpace(item.ToolUseID)
		if toolUseID == "" {
			return nil
		}
		info, _ := p.popToolUse(toolUseID)
		toolName := strings.TrimSpace(info.Name)
		if toolName == "" {
			toolName = "tool"
		}
		return &AgentEvent{
			Type:    AgentEventToolCall,
			Subtype: "completed",
			Tool: &AgentToolCall{
				Name:    toolName,
				Path:    info.Detail,
				Status:  "completed",
				Summary: summarizeClaudeToolResult(item.Content, event.ToolUseResult),
			},
		}
	}
	return nil
}

// rememberToolUse stores tool metadata for later tool_result correlation.
func (p *ClaudeProvider) rememberToolUse(id string, name string, detail string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	p.toolUseMu.Lock()
	p.toolUses[id] = claudeToolUseInfo{Name: name, Detail: detail}
	p.toolUseMu.Unlock()
}

// popToolUse returns stored tool metadata and removes it from the cache.
func (p *ClaudeProvider) popToolUse(id string) (claudeToolUseInfo, bool) {
	p.toolUseMu.Lock()
	defer p.toolUseMu.Unlock()
	if info, ok := p.toolUses[id]; ok {
		delete(p.toolUses, id)
		return info, true
	}
	return claudeToolUseInfo{}, false
}

// extractClaudeEventText returns the most relevant display text for a stream event.
func extractClaudeEventText(event claudeStreamEvent) string {
	if text := strings.TrimSpace(event.Delta.Text); text != "" {
		return text
	}
	if text := strings.TrimSpace(event.Text); text != "" {
		return text
	}
	if text := strings.TrimSpace(event.Content); text != "" {
		return text
	}
	if text := extractClaudeMessageText(event.Message.Content); text != "" {
		return text
	}
	return ""
}

// extractClaudeMessageText joins text content blocks from a Claude message.
func extractClaudeMessageText(items []claudeMessageContent) string {
	var builder strings.Builder
	for _, item := range items {
		if item.Text == "" {
			continue
		}
		builder.WriteString(item.Text)
	}
	return builder.String()
}

// summarizeClaudeToolInput extracts a concise detail string from tool input.
func summarizeClaudeToolInput(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	if value, ok := input["path"]; ok {
		return fmt.Sprintf("%v", value)
	}
	if value, ok := input["pattern"]; ok {
		return fmt.Sprintf("%v", value)
	}
	if value, ok := input["command"]; ok {
		return fmt.Sprintf("%v", value)
	}
	if value, ok := input["url"]; ok {
		return fmt.Sprintf("%v", value)
	}
	if value, ok := input["query"]; ok {
		return fmt.Sprintf("%v", value)
	}
	return ""
}

// summarizeClaudeToolResult converts tool result payloads into a short summary.
func summarizeClaudeToolResult(content any, result *claudeToolUseResultPayload) string {
	switch value := content.(type) {
	case string:
		return strings.TrimSpace(value)
	case []any:
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	case map[string]any:
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
	if result != nil {
		if result.Text != "" {
			return strings.TrimSpace(result.Text)
		}
		if result.Result != nil {
			if len(result.Result.Filenames) > 0 {
				return strings.TrimSpace(strings.Join(result.Result.Filenames, ", "))
			}
			if result.Result.NumFiles > 0 {
				return fmt.Sprintf("%d files", result.Result.NumFiles)
			}
		}
	}
	return ""
}

// buildAgentPrompt combines the user prompt with issue context.
func buildAgentPrompt(prompt string, issueContext string) string {
	return strings.TrimSpace(strings.Join([]string{
		"Use the issue context below to respond to the instruction.",
		"",
		"Instruction:",
		strings.TrimSpace(prompt),
		"",
		"Issue Context:",
		strings.TrimSpace(issueContext),
	}, "\n"))
}

// claudeToolUseInfo stores tool metadata for result correlation.
type claudeToolUseInfo struct {
	Name   string
	Detail string
}

// claudeStreamEvent captures common Claude stream-json fields.
type claudeStreamEvent struct {
	Type       string `json:"type"`
	Subtype    string `json:"subtype"`
	Text       string `json:"text"`
	Content    string `json:"content"`
	SessionID  string `json:"session_id"`
	Model      string `json:"model"`
	DurationMs int64  `json:"duration_ms"`
	IsError    bool   `json:"is_error"`
	Delta      struct {
		Text string `json:"text"`
	} `json:"delta"`
	Message struct {
		Role    string                 `json:"role"`
		Content []claudeMessageContent `json:"content"`
	} `json:"message"`
	ToolUseResult *claudeToolUseResultPayload `json:"tool_use_result"`
}

// claudeMessageContent captures message content blocks including tool calls.
type claudeMessageContent struct {
	Type      string         `json:"type"`
	Text      string         `json:"text"`
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Input     map[string]any `json:"input"`
	ToolUseID string         `json:"tool_use_id"`
	Content   any            `json:"content"`
}

// claudeToolUseResultPayload captures tool_use_result payloads that may be strings or objects.
type claudeToolUseResultPayload struct {
	Text   string
	Result *claudeToolUseResult
}

// UnmarshalJSON supports either string payloads or structured objects.
func (p *claudeToolUseResultPayload) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		p.Text = text
		return nil
	}
	var result claudeToolUseResult
	if err := json.Unmarshal(data, &result); err == nil {
		p.Result = &result
		return nil
	}
	var fallback any
	if err := json.Unmarshal(data, &fallback); err != nil {
		return err
	}
	p.Text = fmt.Sprintf("%v", fallback)
	return nil
}

// claudeToolUseResult captures tool result metadata for summary display.
type claudeToolUseResult struct {
	Filenames  []string `json:"filenames"`
	DurationMs int64    `json:"durationMs"`
	NumFiles   int      `json:"numFiles"`
	Truncated  bool     `json:"truncated"`
}
