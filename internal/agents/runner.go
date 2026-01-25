package agents

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/roeyazroel/linear-tui/internal/logger"
)

const (
	maxStreamLineBytes = 1024 * 1024
)

// Runner executes provider CLIs and streams output.
type Runner struct {
	LookPath func(string) (string, error)
	ExecCmd  func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// NewRunner constructs a Runner with default exec behavior.
func NewRunner() *Runner {
	return &Runner{
		LookPath: exec.LookPath,
		ExecCmd:  exec.CommandContext,
	}
}

// Run starts the provider process and streams output lines to callbacks.
func (r *Runner) Run(ctx context.Context, p Provider, prompt string, issueContext string, options AgentRunOptions, onEvent func(AgentEvent), onLine func(string), onErr func(error)) error {
	if p == nil {
		return fmt.Errorf("provider is nil")
	}

	if onEvent == nil {
		onEvent = func(AgentEvent) {}
	}
	if onLine == nil {
		onLine = func(string) {}
	}
	if onErr == nil {
		onErr = func(error) {}
	}

	binary, ok := p.ResolveBinary()
	if !ok {
		return fmt.Errorf("agent binary not found for %s", p.Name())
	}

	logger.Debug("agents.runner: starting agent run provider=%s workspace=%s", p.Name(), options.Workspace)

	execCmd := r.ExecCmd
	if execCmd == nil {
		execCmd = exec.CommandContext
	}

	cmd := execCmd(ctx, binary, p.BuildArgs(prompt, issueContext, options)...)
	if options.Workspace != "" {
		cmd.Dir = options.Workspace
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open stdout: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("open stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start agent: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		streamLines(stdout, p, "", onEvent, onLine, onErr)
	}()
	go func() {
		defer wg.Done()
		streamLines(stderr, p, "stderr: ", onEvent, onLine, onErr)
	}()

	waitErr := cmd.Wait()
	wg.Wait()

	if waitErr != nil {
		logger.ErrorWithErr(waitErr, "agents.runner: agent exited with error provider=%s", p.Name())
		return fmt.Errorf("agent exited: %w", waitErr)
	}

	logger.Debug("agents.runner: agent run completed provider=%s", p.Name())
	return nil
}

// streamLines scans a stream line-by-line and forwards parsed output.
func streamLines(reader io.Reader, p Provider, prefix string, onEvent func(AgentEvent), onLine func(string), onErr func(error)) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), maxStreamLineBytes)
	for scanner.Scan() {
		raw := scanner.Bytes()
		if parser, ok := p.(EventParser); ok {
			if event, ok := parser.ParseEvent(raw); ok && event != nil {
				onEvent(*event)
				continue
			}
		}
		display, ok := p.ParseStreamLine(raw)
		if !ok {
			display = string(raw)
		}

		display = strings.TrimRight(display, "\r\n")
		if display == "" {
			continue
		}

		onLine(prefix + display)
	}

	if err := scanner.Err(); err != nil {
		if strings.Contains(err.Error(), "file already closed") {
			return
		}
		logger.ErrorWithErr(err, "agents.runner: stream read error prefix=%s", prefix)
		onErr(fmt.Errorf("read stream: %w", err))
	}
}
