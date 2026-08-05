package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/agents"
	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
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
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopDetailTimersOnCleanup(t, app)

	// Use a mutex to synchronize access to pages and other shared state
	var pagesMu sync.Mutex
	app.queueUpdateDraw = func(f func()) {
		pagesMu.Lock()
		f()
		pagesMu.Unlock()
	}

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
	var cmdReady bool
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
			cmdReady = true
			captureMu.Unlock()
			return cmd
		},
	}

	command := findCommandByID(DefaultCommands(app), "ask_agent")
	if command == nil {
		t.Fatal("ask_agent command not found")
	}

	pagesMu.Lock()
	command.Run(app)
	hasPrompt := app.pages.HasPage("agent_prompt")
	pagesMu.Unlock()
	if !hasPrompt {
		t.Fatal("expected agent prompt modal to be visible")
	}

	pagesMu.Lock()
	app.agentPromptModal.promptField.SetText("Summarize", true)
	app.agentPromptModal.workspaceField.SetText(workspaceDir)
	app.agentPromptModal.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModCtrl))
	pagesMu.Unlock()

	waitForCondition(t, time.Second, func() bool {
		pagesMu.Lock()
		defer pagesMu.Unlock()
		return app.pages.HasPage("agent_output")
	})

	waitForCondition(t, time.Second, func() bool {
		captureMu.Lock()
		defer captureMu.Unlock()
		return cmdReady
	})

	captureMu.Lock()
	gotArgs := append([]string(nil), capturedArgs...)
	captureMu.Unlock()

	// Every flag the app sends, pinned. The default sandbox is enabled, so
	// --force stays off. The workspace reaches the agent as the working
	// directory, covered by TestRunner_RunUsesWorkspaceAsWorkingDir.
	wantFlags := []string{"--print", "--output-format", "stream-json", "--model", "gpt-5.2", "-p"}
	if len(gotArgs) != len(wantFlags)+1 {
		t.Fatalf("agent args = %q, want %q plus one prompt", gotArgs, wantFlags)
	}
	if !slices.Equal(gotArgs[:len(wantFlags)], wantFlags) {
		t.Fatalf("agent flags = %q, want %q", gotArgs[:len(wantFlags)], wantFlags)
	}
	if !strings.Contains(gotArgs[len(wantFlags)], "Summarize") {
		t.Fatalf("agent prompt = %q, want it to carry the typed prompt", gotArgs[len(wantFlags)])
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
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopDetailTimersOnCleanup(t, app)

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
