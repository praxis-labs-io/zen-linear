package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunner_RunStreamsOutput(t *testing.T) {
	runner := NewRunner()
	runner.ExecCmd = helperExecCmd("success")

	provider := testProvider{binary: "helper"}
	var lines []string
	var linesMu sync.Mutex

	err := runner.Run(context.Background(), provider, "prompt", "context", AgentRunOptions{}, func(AgentEvent) {}, func(line string) {
		linesMu.Lock()
		lines = append(lines, line)
		linesMu.Unlock()
	}, func(error) {})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	linesMu.Lock()
	linesCopy := append([]string(nil), lines...)
	linesMu.Unlock()

	if !containsLine(linesCopy, "hello") {
		t.Fatalf("expected stdout line, got: %#v", linesCopy)
	}
	if !containsLine(linesCopy, "stderr: warn") {
		t.Fatalf("expected stderr line, got: %#v", linesCopy)
	}
}

func TestRunner_RunCancel(t *testing.T) {
	runner := NewRunner()
	runner.ExecCmd = helperExecCmd("sleep")

	provider := testProvider{binary: "helper"}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := runner.Run(ctx, provider, "prompt", "context", AgentRunOptions{}, func(AgentEvent) {}, func(string) {}, func(error) {})
	if err == nil {
		t.Fatalf("expected cancellation error")
	}
}

func TestRunner_RunUsesWorkspaceAsWorkingDir(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks() error: %v", err)
	}

	runner := NewRunner()
	runner.ExecCmd = helperExecCmd("cwd")

	var lines []string
	var linesMu sync.Mutex
	err = runner.Run(context.Background(), testProvider{binary: "helper"}, "prompt", "context",
		AgentRunOptions{Workspace: workspace}, func(AgentEvent) {}, func(line string) {
			linesMu.Lock()
			lines = append(lines, line)
			linesMu.Unlock()
		}, func(error) {})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	linesMu.Lock()
	linesCopy := append([]string(nil), lines...)
	linesMu.Unlock()

	if !containsLine(linesCopy, workspace) {
		t.Fatalf("agent ran in the wrong directory, want %q, got: %#v", workspace, linesCopy)
	}
}

func TestRunner_RunNonZero(t *testing.T) {
	runner := NewRunner()
	runner.ExecCmd = helperExecCmd("fail")

	provider := testProvider{binary: "helper"}
	err := runner.Run(context.Background(), provider, "prompt", "context", AgentRunOptions{}, func(AgentEvent) {}, func(string) {}, func(error) {})
	if err == nil {
		t.Fatalf("expected non-zero exit error")
	}
}

func TestRunnerHelperProcess(t *testing.T) {
	if os.Getenv("AGENT_TEST_HELPER") != "1" {
		return
	}

	mode := os.Getenv("AGENT_TEST_MODE")
	switch mode {
	case "success":
		_, _ = fmt.Fprintln(os.Stdout, `{"text":"hello"}`)
		_, _ = fmt.Fprintln(os.Stderr, "warn")
		os.Exit(0)
	case "fail":
		_, _ = fmt.Fprintln(os.Stderr, "boom")
		os.Exit(2)
	case "cwd":
		cwd, err := os.Getwd()
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		_, _ = fmt.Fprintf(os.Stdout, "{\"text\":%q}\n", cwd)
		os.Exit(0)
	case "sleep":
		time.Sleep(5 * time.Second)
		os.Exit(0)
	default:
		os.Exit(0)
	}
}

func helperExecCmd(mode string) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestRunnerHelperProcess")
		cmd.Env = append(os.Environ(),
			"AGENT_TEST_HELPER=1",
			fmt.Sprintf("AGENT_TEST_MODE=%s", mode),
		)
		return cmd
	}
}

func containsLine(lines []string, needle string) bool {
	for _, line := range lines {
		if strings.Contains(line, needle) {
			return true
		}
	}
	return false
}

type testProvider struct {
	binary string
}

func (p testProvider) Name() string {
	return "test"
}

func (p testProvider) ResolveBinary() (string, bool) {
	return p.binary, true
}

func (p testProvider) BuildArgs(string, string, AgentRunOptions) []string {
	return nil
}

func (p testProvider) ParseStreamLine(line []byte) (string, bool) {
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(line, &payload); err != nil {
		return "", false
	}
	if payload.Text == "" {
		return "", false
	}
	return payload.Text, true
}
