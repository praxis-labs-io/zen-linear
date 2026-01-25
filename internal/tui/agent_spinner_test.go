package tui

import "testing"

// TestAgentSpinner_Lifecycle verifies start/stop and frame cycling.
func TestAgentSpinner_Lifecycle(t *testing.T) {
	spinner := newAgentSpinner()
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
