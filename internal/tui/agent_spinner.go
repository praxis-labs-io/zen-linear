package tui

import "sync"

// agentSpinner provides a simple ASCII spinner for status lines.
type agentSpinner struct {
	mu      sync.Mutex
	frames  []string
	index   int
	running bool
}

// newAgentSpinner constructs a spinner with default frames.
func newAgentSpinner() *agentSpinner {
	return &agentSpinner{
		frames: []string{"-", "\\", "|", "/"},
	}
}

// Start enables the spinner.
func (s *agentSpinner) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = true
	s.index = 0
}

// Stop disables the spinner.
func (s *agentSpinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
}

// Running reports whether the spinner is active.
func (s *agentSpinner) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// NextFrame returns the next spinner frame or empty when stopped.
func (s *agentSpinner) NextFrame() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || len(s.frames) == 0 {
		return ""
	}
	frame := s.frames[s.index%len(s.frames)]
	s.index++
	return frame
}
