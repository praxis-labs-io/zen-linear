package tui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// TestDrawingTheCommentsPanelKeepsTheAppAlive is the regression test for a
// freeze that took the whole app: Application.draw holds the app's lock for the
// entire frame, so a draw func that calls GetFocus blocks on a mutex its own
// goroutine holds. Nothing draws after that and no key is read, Ctrl+C
// included. Anything reachable from a draw reads state, never live focus.
func TestDrawingTheCommentsPanelKeepsTheAppAlive(t *testing.T) {
	app, _ := newComposeTestApp(t)
	app.app.SetRoot(app.detailsView, true)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init screen: %v", err)
	}
	screen.SetSize(120, 40)
	app.app.SetScreen(screen)

	go func() { _ = app.app.Run() }()
	t.Cleanup(func() { app.app.Stop() })

	// A queued update only runs between frames, so it never lands while a draw
	// func is holding the application.
	alive := make(chan struct{})
	go func() { app.app.QueueUpdateDraw(func() { close(alive) }) }()

	select {
	case <-alive:
	case <-time.After(4 * time.Second):
		t.Fatal("the event loop never came back: a draw func in the comments panel is reading live focus")
	}
}
