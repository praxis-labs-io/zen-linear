package tui

import "sync"

// spinnerFramesASCII is the agent status line's set. It shares a line with
// streamed CLI output, where a braille glyph reads as noise.
var spinnerFramesASCII = []string{"-", "\\", "|", "/"}

// spinnerFramesDots is the glyph a waiting pane shows, matching zen-octo's.
var spinnerFramesDots = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// spinner advances a frame set for a status line or a waiting pane.
type spinner struct {
	mu      sync.Mutex
	frames  []string
	index   int
	running bool
}

// newSpinner constructs a stopped spinner over the given frames.
func newSpinner(frames []string) *spinner {
	return &spinner{frames: frames}
}

// Start enables the spinner.
func (s *spinner) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = true
	s.index = 0
}

// Stop disables the spinner.
func (s *spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
}

// Running reports whether the spinner is active.
func (s *spinner) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// NextFrame returns the next spinner frame or empty when stopped.
func (s *spinner) NextFrame() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || len(s.frames) == 0 {
		return ""
	}
	frame := s.frames[s.index%len(s.frames)]
	s.index++
	return frame
}
