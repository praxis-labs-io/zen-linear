package tui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

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

	alive := make(chan struct{})
	go func() { app.app.QueueUpdateDraw(func() { close(alive) }) }()

	select {
	case <-alive:
	case <-time.After(4 * time.Second):
		t.Fatal("the event loop never came back: a draw func in the comments panel is reading live focus")
	}
}
