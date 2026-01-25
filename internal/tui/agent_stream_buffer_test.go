package tui

import (
	"strings"
	"testing"

	"github.com/roeyazroel/linear-tui/internal/agents"
)

// TestAgentStreamBuffer_ThinkingStreams verifies thinking lines stream in chunks.
func TestAgentStreamBuffer_ThinkingStreams(t *testing.T) {
	buffer := NewAgentStreamBuffer()

	chunk := strings.Repeat("a", thinkingFlushChars+1)
	update := buffer.Append(agents.AgentEvent{
		Type: agents.AgentEventThinking,
		Text: chunk,
	})

	if len(update.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(update.Lines))
	}
	if update.Lines[0].Kind != StreamLineThinking {
		t.Fatalf("expected thinking line, got %s", update.Lines[0].Kind)
	}
	if update.Lines[0].Text == "" {
		t.Fatalf("expected thinking text to be present")
	}
	if update.Done {
		t.Fatalf("did not expect done=true")
	}
}

// TestAgentStreamBuffer_ThinkingCoalesces verifies thinking flushes before other events.
func TestAgentStreamBuffer_ThinkingCoalesces(t *testing.T) {
	buffer := NewAgentStreamBuffer()

	update := buffer.Append(agents.AgentEvent{
		Type: agents.AgentEventThinking,
		Text: "first",
	})
	if len(update.Lines) != 0 {
		t.Fatalf("expected no flush yet, got %+v", update.Lines)
	}

	update = buffer.Append(agents.AgentEvent{
		Type: agents.AgentEventThinking,
		Text: "second",
	})
	if len(update.Lines) != 0 {
		t.Fatalf("expected no flush yet, got %+v", update.Lines)
	}

	update = buffer.Append(agents.AgentEvent{
		Type: agents.AgentEventUser,
		Text: "prompt",
	})
	if len(update.Lines) != 1 {
		t.Fatalf("expected thinking line only, got %+v", update.Lines)
	}
	if update.Lines[0].Kind != StreamLineThinking {
		t.Fatalf("expected first line to be thinking, got %s", update.Lines[0].Kind)
	}
	if !strings.Contains(update.Lines[0].Text, "first") || !strings.Contains(update.Lines[0].Text, "second") {
		t.Fatalf("expected coalesced thinking text, got %q", update.Lines[0].Text)
	}
}

// TestAgentStreamBuffer_AssistantAndResult verifies assistant accumulation and final text emission.
func TestAgentStreamBuffer_AssistantAndResult(t *testing.T) {
	buffer := NewAgentStreamBuffer()

	update := buffer.Append(agents.AgentEvent{
		Type: agents.AgentEventAssistant,
		Text: "first response",
	})

	if len(update.Lines) != 0 {
		t.Fatalf("expected no stream lines for assistant, got %+v", update.Lines)
	}

	update = buffer.Append(agents.AgentEvent{
		Type:       agents.AgentEventResult,
		Subtype:    "success",
		DurationMs: 1200,
	})

	if !update.Done {
		t.Fatalf("expected done=true on result")
	}
	if update.FinalText != "first response" {
		t.Fatalf("unexpected final text: %q", update.FinalText)
	}
	if len(update.Lines) != 0 {
		t.Fatalf("expected no stream lines for result, got %+v", update.Lines)
	}
}
