package tui

import "testing"

// TestSpinnerLifecycle verifies start/stop and frame cycling.
func TestSpinnerLifecycle(t *testing.T) {
	spinner := newSpinner(spinnerFramesASCII)
	if spinner.Running() {
		t.Fatalf("expected spinner to be stopped initially")
	}

	spinner.Start()
	if !spinner.Running() {
		t.Fatalf("expected spinner to be running after Start")
	}

	frame1 := spinner.NextFrame()
	frame2 := spinner.NextFrame()
	if frame1 == "" || frame2 == "" {
		t.Fatalf("expected frames while running")
	}

	spinner.Stop()
	if spinner.Running() {
		t.Fatalf("expected spinner to be stopped after Stop")
	}

	if frame := spinner.NextFrame(); frame != "" {
		t.Fatalf("expected empty frame when stopped, got %q", frame)
	}
}

// TestSpinnerCyclesEveryFrame verifies the frame set wraps rather than
// stopping at the end, so a slow fetch keeps spinning.
func TestSpinnerCyclesEveryFrame(t *testing.T) {
	spinner := newSpinner(spinnerFramesDots)
	spinner.Start()

	for i := range spinnerFramesDots {
		if frame := spinner.NextFrame(); frame != spinnerFramesDots[i] {
			t.Fatalf("frame %d = %q, want %q", i, frame, spinnerFramesDots[i])
		}
	}
	if frame := spinner.NextFrame(); frame != spinnerFramesDots[0] {
		t.Fatalf("frame after the last = %q, want %q", frame, spinnerFramesDots[0])
	}
}
