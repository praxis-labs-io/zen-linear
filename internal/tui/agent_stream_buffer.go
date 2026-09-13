package tui

import (
	"fmt"
	"strings"

	"github.com/praxis-labs-io/zen-linear/internal/agents"
)

type StreamLineKind string

const (
	StreamLineSystem    StreamLineKind = "system"
	StreamLineUser      StreamLineKind = "user"
	StreamLineAssistant StreamLineKind = "assistant"
	StreamLineThinking  StreamLineKind = "thinking"
	StreamLineTool      StreamLineKind = "tool"
	StreamLineResult    StreamLineKind = "result"
	StreamLineUnknown   StreamLineKind = "unknown"
)

type StreamLine struct {
	Kind StreamLineKind
	Text string
}

type StreamUpdate struct {
	Lines     []StreamLine
	FinalText string
	Done      bool
}

type AgentStreamBuffer struct {
	assistant        strings.Builder
	thinking         strings.Builder
	thinkingLastChar byte
	hasThinkingChar  bool
}

const thinkingFlushChars = 200

func NewAgentStreamBuffer() *AgentStreamBuffer {
	return &AgentStreamBuffer{}
}

// Append converts an event into stream lines, and into final text once the run completes.
func (b *AgentStreamBuffer) Append(event agents.AgentEvent) StreamUpdate {
	update := StreamUpdate{}

	switch event.Type {
	case agents.AgentEventThinking:
		b.appendThinkingText(event.Text)
		if shouldFlushThinking(event.Text, b.thinking.Len()) {
			b.flushThinkingLine(&update)
		}
		return update
	case agents.AgentEventSystem:
		b.flushThinkingLine(&update)
	case agents.AgentEventUser:
		b.flushThinkingLine(&update)
	case agents.AgentEventAssistant, agents.AgentEventAssistantDelta:
		b.flushThinkingLine(&update)
		if event.Text != "" {
			if b.assistant.Len() > 0 {
				b.assistant.WriteString("\n")
			}
			b.assistant.WriteString(event.Text)
		}
	case agents.AgentEventToolCall:
		b.flushThinkingLine(&update)
		update.Lines = append(update.Lines, StreamLine{
			Kind: StreamLineTool,
			Text: formatToolLine(event),
		})
	case agents.AgentEventResult:
		b.flushThinkingLine(&update)
		update.FinalText = strings.TrimSpace(b.assistant.String())
		update.Done = true
	default:
	}

	return update
}

func (b *AgentStreamBuffer) appendThinkingText(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}

	if b.thinking.Len() > 0 && needsThinkingSpace(b.thinkingLastChar, text) {
		b.thinking.WriteByte(' ')
		b.thinkingLastChar = ' '
		b.hasThinkingChar = true
	}

	b.thinking.WriteString(text)
	if len(text) > 0 {
		b.thinkingLastChar = text[len(text)-1]
		b.hasThinkingChar = true
	}
}

func (b *AgentStreamBuffer) flushThinkingLine(update *StreamUpdate) {
	content := strings.TrimSpace(b.thinking.String())
	if content == "" {
		b.resetThinkingBuffer()
		return
	}

	update.Lines = append(update.Lines, StreamLine{
		Kind: StreamLineThinking,
		Text: content,
	})
	b.resetThinkingBuffer()
}

func (b *AgentStreamBuffer) resetThinkingBuffer() {
	b.thinking.Reset()
	b.thinkingLastChar = 0
	b.hasThinkingChar = false
}

func shouldFlushThinking(latest string, currentLen int) bool {
	if currentLen >= thinkingFlushChars {
		return true
	}
	return strings.Contains(latest, "\n")
}

func needsThinkingSpace(lastChar byte, next string) bool {
	if next == "" {
		return false
	}
	if isSpaceByte(lastChar) {
		return false
	}
	return !isSpaceByte(next[0])
}

func isSpaceByte(value byte) bool {
	switch value {
	case ' ', '\n', '\r', '\t':
		return true
	default:
		return false
	}
}

func formatToolLine(event agents.AgentEvent) string {
	if event.Tool == nil || event.Tool.Name == "" {
		return "Tool call: (unknown)"
	}

	label := "Tool call"
	if event.Subtype != "" {
		label = fmt.Sprintf("Tool call %s", event.Subtype)
	}

	if event.Tool.Path != "" {
		if event.Tool.Summary != "" {
			return fmt.Sprintf("%s: %s (%s) %s", label, event.Tool.Name, event.Tool.Path, event.Tool.Summary)
		}
		return fmt.Sprintf("%s: %s (%s)", label, event.Tool.Name, event.Tool.Path)
	}

	if event.Tool.Summary != "" {
		return fmt.Sprintf("%s: %s %s", label, event.Tool.Name, event.Tool.Summary)
	}
	return fmt.Sprintf("%s: %s", label, event.Tool.Name)
}
