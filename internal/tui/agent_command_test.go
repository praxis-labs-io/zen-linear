package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/roeyazroel/linear-tui/internal/agents"
	"github.com/roeyazroel/linear-tui/internal/config"
	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

// TestAskAgentCommand_ShowsModalsAndStreams verifies the command flow.
func TestAskAgentCommand_ShowsModalsAndStreams(t *testing.T) {
	cfg := config.Config{
		PageSize:      1,
		CacheTTL:      time.Minute,
		AgentProvider: config.DefaultAgentProvider,
		AgentSandbox:  config.DefaultAgentSandbox,
		AgentModel:    "gpt-5.2",
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }

	selectedIssue := linearapi.Issue{ID: "issue-1", Title: "Test"}
	app.issuesMu.Lock()
	app.selectedIssue = &selectedIssue
	app.issuesMu.Unlock()

	app.fetchIssueByID = func(ctx context.Context, id string) (linearapi.Issue, error) {
		return linearapi.Issue{
			ID:          id,
			Title:       "Test",
			Description: "Desc",
			Comments: []linearapi.Comment{
				{Body: "Comment"},
			},
		}, nil
	}

	execPath, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error: %v", err)
	}

	workspaceDir := t.TempDir()
	var capturedArgs []string
	var capturedCmd *exec.Cmd
	var captureMu sync.Mutex

	app.agentRunner = &agents.Runner{
		LookPath: func(string) (string, error) {
			return "helper", nil
		},
		ExecCmd: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			captureMu.Lock()
			capturedArgs = append([]string(nil), args...)
			captureMu.Unlock()

			cmd := exec.CommandContext(ctx, execPath, "-test.run=TestAgentCommandHelperProcess")
			cmd.Env = append(os.Environ(),
				"AGENT_TUI_HELPER=1",
				"AGENT_TUI_MODE=success",
			)

			captureMu.Lock()
			capturedCmd = cmd
			captureMu.Unlock()
			return cmd
		},
	}

	command := findCommandByID(DefaultCommands(app), "ask_agent")
	if command == nil {
		t.Fatal("ask_agent command not found")
	}

	command.Run(app)
	if !app.pages.HasPage("agent_prompt") {
		t.Fatal("expected agent prompt modal to be visible")
	}

	app.agentPromptModal.promptField.SetText("Summarize", true)
	app.agentPromptModal.workspaceField.SetText(workspaceDir)
	app.agentPromptModal.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModCtrl))

	waitForCondition(t, time.Second, func() bool {
		return app.pages.HasPage("agent_output")
	})

	waitForCondition(t, time.Second, func() bool {
		captureMu.Lock()
		defer captureMu.Unlock()
		return capturedCmd != nil && capturedCmd.Dir == workspaceDir
	})

	captureMu.Lock()
	gotArgs := append([]string(nil), capturedArgs...)
	gotCmd := capturedCmd
	captureMu.Unlock()

	if gotCmd.Dir != workspaceDir {
		t.Fatalf("agent cmd dir = %q, want %q", gotCmd.Dir, workspaceDir)
	}

	joined := strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "--force") {
		t.Fatalf("expected --force in args: %s", joined)
	}
	if !strings.Contains(joined, "--sandbox") || !strings.Contains(joined, config.DefaultAgentSandbox) {
		t.Fatalf("expected sandbox option in args: %s", joined)
	}
	if !strings.Contains(joined, "--model") || !strings.Contains(joined, "gpt-5.2") {
		t.Fatalf("expected model option in args: %s", joined)
	}
	if !strings.Contains(joined, "--workspace") || !strings.Contains(joined, workspaceDir) {
		t.Fatalf("expected workspace option in args: %s", joined)
	}
}

// TestDefaultCommands_GatesAskAgent verifies command gating by availability.
func TestDefaultCommands_GatesAskAgent(t *testing.T) {
	cfg := config.Config{
		PageSize:      1,
		CacheTTL:      time.Minute,
		AgentProvider: config.DefaultAgentProvider,
		AgentSandbox:  config.DefaultAgentSandbox,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)

	app.agentRunner = &agents.Runner{
		LookPath: func(string) (string, error) {
			return "", exec.ErrNotFound
		},
	}

	commands := DefaultCommands(app)
	if findCommandByID(commands, "ask_agent") != nil {
		t.Fatal("expected ask_agent to be gated when no agents are available")
	}

	app.agentRunner = &agents.Runner{
		LookPath: func(string) (string, error) {
			return "agent", nil
		},
	}

	commands = DefaultCommands(app)
	if findCommandByID(commands, "ask_agent") == nil {
		t.Fatal("expected ask_agent when an agent is available")
	}
}

// findCommandByID locates a command by ID.
func findCommandByID(commands []Command, id string) *Command {
	for _, cmd := range commands {
		if cmd.ID == id {
			copyCmd := cmd
			return &copyCmd
		}
	}
	return nil
}

// TestAgentCommandHelperProcess is a helper process for command tests.
func TestAgentCommandHelperProcess(t *testing.T) {
	if os.Getenv("AGENT_TUI_HELPER") != "1" {
		return
	}

	mode := os.Getenv("AGENT_TUI_MODE")
	switch mode {
	case "success":
		_, _ = fmt.Fprintln(os.Stdout, `{"text":"hello"}`)
		os.Exit(0)
	default:
		os.Exit(0)
	}
}
