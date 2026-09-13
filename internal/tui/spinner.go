package tui

import "sync"

// The agent status line shares a line with streamed CLI output, where a braille glyph reads as noise.
var spinnerFramesASCII = []string{"-", "\\", "|", "/"}

var spinnerFramesDots = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

type spinner struct {
	mu      sync.Mutex
	frames  []string
	index   int
	running bool
}

func newSpinner(frames []string) *spinner {
	return &spinner{frames: frames}
}

func (s *spinner) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = true
	s.index = 0
}

func (s *spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
}

func (s *spinner) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

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
