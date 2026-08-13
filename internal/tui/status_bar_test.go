package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
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
			want: []string{": palette", "↑↓ move", "⏎ open", "Tab search", "l issues", "{ hide nav"},
			// The tree is the leftmost pane; there is nothing to its left.
			wantNot: []string{"panes", "hide details"},
		},
		{
			name:    "issues",
			pane:    FocusIssues,
			want:    []string{": palette", "j/k move", "⏎ preview", "v view", "/ search", "h/l panes"},
			wantNot: []string{"hide nav", "tabs"},
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

// The corner says three things and colors them apart: plain text is a nudge,
// green is something that finished, red is something that failed. Plain is the
// default so the color means something when it shows up.
func TestTheToastColorsSaySuccessAndFailureApart(t *testing.T) {
	app := newUXTestApp(t)

	for _, tc := range []struct {
		name  string
		flash func(string)
		want  string
	}{
		{"info", app.flashStatus, app.themeTags.Foreground},
		{"success", app.flashSuccess, app.themeTags.Success},
		{"error", app.flashError, app.themeTags.Error},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.flash("Something happened")
			if got := app.toastTag(); got != tc.want {
				t.Errorf("%s toast tag = %q, want %q", tc.name, got, tc.want)
			}
		})
	}

	// Progress is not a result, so it stays plain whatever the last flash was.
	app.flashSuccess("Archived ZNL-1")
	app.statusMessage = ""
	if got := app.toastTag(); got != app.themeTags.Foreground {
		t.Errorf("loading tag = %q, want plain text", got)
	}
}

// Every theme needs a green to say success in, and the fallback has to give one
// to a theme that predates the field rather than an unpaintable default.
func TestEveryThemeHasASuccessColor(t *testing.T) {
	for name, theme := range ThemeRegistry {
		if got := theme.SuccessColor(); !got.Valid() {
			t.Errorf("%s has no success color: %v", name, got)
		}
	}
	legacy := Theme{StatusReview: tcell.NewRGBColor(1, 2, 3)}
	if got := legacy.SuccessColor(); got != legacy.StatusReview {
		t.Errorf("a theme with no Success falls back to %v, want the review color", got)
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

// A message longer than the strip used to take a fixed width wider than the
// row, which left the hints a negative width and drew neither half properly.
func TestALongFlashLeavesTheHintsRoom(t *testing.T) {
	app := newUXTestApp(t)
	app.focusedPane = FocusIssues
	app.flashStatus("Opened GitHub: https://github.com/zen-linear/zen-linear/pull/1234 and then some")

	lines := drawPrimitive(t, app.statusRow, 80)

	strip := lines[0]
	if !strings.Contains(strip, ": palette") {
		t.Errorf("strip = %q, want the hints still on it", strip)
	}
	if !strings.Contains(strip, "…") {
		t.Errorf("strip = %q, want the message truncated to its half", strip)
	}
	if width := runeCellWidth(strip); width > 80 {
		t.Errorf("strip is %d cells wide, want it inside 80", width)
	}
}

// Progress must not push a warning off the corner: a warning is said once, a
// fetch says "Loading..." on every refresh.
func TestLoadProgressWaitsBehindAWarning(t *testing.T) {
	app := newUXTestApp(t)
	app.focusedPane = FocusIssues

	app.flashStatus("Default project Missing not found")
	app.setLoadingMessage("Loading...")

	if got := app.statusToast.GetText(true); got != "Default project Missing not found" {
		t.Errorf("toast = %q, want the warning held", got)
	}

	app.statusMessage = ""
	app.fitStatusToast()
	if got := app.statusToast.GetText(true); got != "Loading..." {
		t.Errorf("toast = %q once the warning expired, want the progress", got)
	}
}
