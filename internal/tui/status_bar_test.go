package tui

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestPaneHintsNameWhatTheKeyboardDoesHere pins each pane's hints. The bar is
// the only place a key is offered, so a pane naming one it does not answer to,
// or dropping one it does, is the failure worth catching.
func TestPaneHintsNameWhatTheKeyboardDoesHere(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pane    FocusTarget
		want    []string
		wantNot []string
	}{
		{
			name: "navigation",
			pane: FocusNavigation,
			want: []string{": palette", "↑↓ move", "⏎ open", "l issues", "{ hide nav"},
			// The tree is the leftmost pane; there is nothing to its left.
			wantNot: []string{"panes", "hide details"},
		},
		{
			name:    "issues",
			pane:    FocusIssues,
			want:    []string{": palette", "j/k move", "⏎ preview", "v view", "[/] tabs", "h/l panes"},
			wantNot: []string{"hide nav"},
		},
		{
			name:    "details",
			pane:    FocusDetails,
			want:    []string{": palette", "j/k scroll", "[/] tabs", "v view", "} hide details", "h back"},
			wantNot: []string{"pick a comment"},
		},
		{
			name: "palette",
			pane: FocusPalette,
			want: []string{"↑↓ move", "⏎ run", "Esc close"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newUXTestApp(t)
			app.focusedPane = tc.pane

			app.updateStatusBar()
			got := app.statusBar.GetText(true)

			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("hints = %q, want %q offered", got, want)
				}
			}
			for _, unwanted := range tc.wantNot {
				if strings.Contains(got, unwanted) {
					t.Errorf("hints = %q, want no %q", got, unwanted)
				}
			}
		})
	}
}

// The list's scope, ordering and filters belong to the issues pane's own
// footer. Saying them here as well is the same fact in two places.
func TestPaneHintsLeaveTheListContextToTheIssuesPane(t *testing.T) {
	app := newUXTestApp(t)
	app.selectedNavigation = &NavigationNode{ID: "all", Text: "All Issues"}
	app.focusedPane = FocusIssues

	app.updateStatusBar()

	got := app.statusBar.GetText(true)
	for _, unwanted := range []string{"All Issues", "Sort:", "0 issues", "No issues"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("hints = %q, want no %q", got, unwanted)
		}
	}
}

// TestPaletteHintLeadsEveryPane covers the one key that reaches everything the
// bar has no room to name holding the same place in every pane.
func TestPaletteHintLeadsEveryPane(t *testing.T) {
	app := newUXTestApp(t)

	for _, pane := range []FocusTarget{FocusNavigation, FocusIssues, FocusDetails} {
		app.focusedPane = pane
		app.updateStatusBar()
		if got := app.statusBar.GetText(true); !strings.HasPrefix(got, ": palette · ") {
			t.Errorf("hints = %q with %v focused, want the palette first", got, pane)
		}
	}
}

// Every key typed into a comment box goes into the text, the palette's
// included, so the line names none of them.
func TestWritingDropsTheKeyHints(t *testing.T) {
	app := newUXTestApp(t)
	app.focusedPane = FocusDetails
	app.detailsCommentsVisible = true
	app.focusedDetailsView = true
	app.commentsFocus = commentsFocusText

	app.updateStatusBar()

	if got := app.statusBar.GetText(true); got != "Writing a comment" {
		t.Errorf("hints = %q while writing, want only %q", got, "Writing a comment")
	}
}

// A flashed message takes the strip's right corner rather than the hints, so
// the keys stay readable while it is up.
func TestFlashedMessageTakesTheCorner(t *testing.T) {
	app := newUXTestApp(t)
	app.focusedPane = FocusIssues

	app.flashStatus("Copied ZNL-1")

	if got := app.statusToast.GetText(true); got != "Copied ZNL-1" {
		t.Errorf("toast = %q, want the message", got)
	}
	if got := app.statusBar.GetText(true); !strings.Contains(got, "j/k move") {
		t.Errorf("hints = %q, want the keys still named", got)
	}
}

// A one-off message used to sit in the bar for the rest of the session.
func TestFlashedMessageClearsItself(t *testing.T) {
	app := newUXTestApp(t)
	app.focusedPane = FocusIssues
	shortenFlash(t)
	queued := watchQueuedUpdates(app)

	app.flashStatus("Copied ZNL-1")
	waitForQueuedUpdate(t, queued)

	if got := app.statusToast.GetText(true); got != "" {
		t.Errorf("toast = %q after the flash expired, want it empty", got)
	}
}

// The last message wins: an older clear must not take a newer one down with it.
func TestASecondFlashKeepsItsOwnClock(t *testing.T) {
	app := newUXTestApp(t)
	app.focusedPane = FocusIssues
	shortenFlash(t)
	queued := watchQueuedUpdates(app)

	app.flashStatus("Copied ZNL-1")
	app.flashStatus("Copied ZNL-2")

	waitForQueuedUpdate(t, queued)
	if got := app.statusToast.GetText(true); got != "" {
		t.Errorf("toast = %q, want the second message cleared on its own timer", got)
	}
}

// An error holds the bar: a flash counting down behind it must not repaint over
// the failure a moment later.
func TestErrorSurvivesAPendingFlash(t *testing.T) {
	app := newUXTestApp(t)
	app.focusedPane = FocusIssues
	shortenFlash(t)

	app.flashStatus("Posting comment...")
	app.updateStatusBarWithError(errors.New("connection reset"))
	time.Sleep(20 * flashTestDuration)

	if got := app.statusBar.GetText(true); !strings.Contains(got, "connection reset") {
		t.Errorf("bar = %q, want the failure still on screen", got)
	}
	if got := app.statusToast.GetText(true); got != "" {
		t.Errorf("toast = %q, want the flash dropped for the failure", got)
	}
}

// Linear errors carry bracketed fragments, which a view reading color tags eats
// along with whatever names the failure.
func TestErrorTextIsNotReadAsColorTags(t *testing.T) {
	app := newUXTestApp(t)

	app.updateStatusBarWithError(errors.New("field [teamId] is required"))

	if got := app.statusBar.GetText(true); !strings.Contains(got, "[teamId]") {
		t.Errorf("error reads %q, want the bracketed field kept", got)
	}
}

// statusText is the whole strip, the hints and the message corner both, so a
// test asserting on what the app said does not have to know which half said it.
func statusText(app *App) string {
	return app.statusBar.GetText(true) + " " + app.statusToast.GetText(true)
}

const flashTestDuration = 5 * time.Millisecond

func shortenFlash(t *testing.T) {
	t.Helper()
	previous := flashDuration
	flashDuration = flashTestDuration
	t.Cleanup(func() { flashDuration = previous })
}

// watchQueuedUpdates reports every update the app queues, which for a flash is
// the clear firing off its timer.
func watchQueuedUpdates(app *App) <-chan struct{} {
	ran := make(chan struct{}, 1)
	app.queueUpdateDraw = func(f func()) {
		f()
		select {
		case ran <- struct{}{}:
		default:
		}
	}
	return ran
}

func waitForQueuedUpdate(t *testing.T, queued <-chan struct{}) {
	t.Helper()
	select {
	case <-queued:
	case <-time.After(2 * time.Second):
		t.Fatal("the flash never cleared")
	}
}
