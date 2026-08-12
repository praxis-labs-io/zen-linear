package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// Drives the whole app the way the running one is: zoomed details, an issue
// with no comments, focus into the box, then type.
func TestReproFullApp(t *testing.T) {
	app := newDetailsTestApp(t)
	app.selectedIssue = detailsFixture()
	app.updateDetailsView()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(160, 44)
	draw := func() {
		app.pages.SetRect(0, 0, 160, 44)
		app.pages.Draw(screen)
		screen.Show()
	}

	app.detailsHidden = false
	app.focusedPane = FocusDetails
	app.focusedDetailsView = true
	app.detailsZoomed = true
	app.rebuildContentLayout()
	app.updateFocus()
	draw()

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModNone))
	draw()
	x, y, w, h := app.detailsComposeArea.GetRect()
	t.Logf("after c: focus=%T commentsFocus=%v active=%v area rect=%d,%d %dx%d slots=%d",
		app.app.GetFocus(), app.commentsFocus, app.composeBoxActive(), x, y, w, h, len(app.detailsCommentsPage.slots))

	typeRunes(t, app, "hello")
	draw()
	t.Logf("box holds %q", app.detailsComposeArea.GetText())
}
