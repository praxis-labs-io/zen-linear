package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/config"
)

func TestPaneHintsNameWhatTheKeyboardDoesHere(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pane    FocusTarget
		want    []string
		wantNot []string
	}{
		{
			name:    "navigation",
			pane:    FocusNavigation,
			want:    []string{": palette", "↑↓ move", "⏎ open", "Tab search", "l issues", "< hide nav"},
			wantNot: []string{"panes", "hide details"},
		},
		{
			name:    "issues",
			pane:    FocusIssues,
			want:    []string{": palette", "j/k move", "⏎ preview", "v view", "/ search", "h/l panes"},
			wantNot: []string{"hide nav", "comments"},
		},
		{
			name:    "details",
			pane:    FocusDetails,
			want:    []string{": palette", "j/k scroll", "{/} comments", "v view", "> hide details", "h back"},
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

func TestZoomedHintsDropTheHideKey(t *testing.T) {
	app := newThreadedTestApp(t)
	app.toggleDetailsZoom()

	for _, tc := range []struct {
		name string
		lit  string
	}{
		{"nothing lit", ""},
		{"a card lit", app.detailsCommentsSource[0].ID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app.focusedCommentID = tc.lit
			app.updateStatusBar()

			got := app.statusBar.GetText(true)
			if strings.Contains(got, "hide details") {
				t.Errorf("zoomed hints = %q, want no hide key while it is inert", got)
			}
			if !strings.Contains(got, "v close") {
				t.Errorf("zoomed hints = %q, want the zoom key to read as closing the view", got)
			}
			if strings.Contains(got, "v view") {
				t.Errorf("zoomed hints = %q, want no offer to open a view already open", got)
			}
			if !strings.Contains(got, "←/h navigation") {
				t.Errorf("zoomed hints = %q, want the way off the zoomed pane", got)
			}
			wantEscape := "Esc back to list"
			if tc.lit != "" {
				wantEscape = "Esc let go"
			}
			if !strings.Contains(got, wantEscape) {
				t.Errorf("zoomed hints = %q, want %q", got, wantEscape)
			}
			if tc.lit != "" && strings.Contains(got, "back to list") {
				t.Errorf("zoomed hints = %q, want Esc to name only the card it lets go of", got)
			}
		})
	}
}

func TestWritingDropsTheKeyHints(t *testing.T) {
	app := newUXTestApp(t)
	app.focusedPane = FocusDetails
	app.detailsFocus = detailsFocusText

	app.updateStatusBar()

	if got := app.statusBar.GetText(true); got != "Writing a comment" {
		t.Errorf("hints = %q while writing, want only %q", got, "Writing a comment")
	}
}

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

	app.flashSuccess("Archived ZNL-1")
	app.statusMessage = ""
	if got := app.toastTag(); got != app.themeTags.Accent {
		t.Errorf("loading tag = %q, want the accent", got)
	}
}

func TestEveryThemeHasASuccessColor(t *testing.T) {
	themes := map[string]Theme{config.ThemeTerminal: TerminalTheme()}
	for name, theme := range ThemeRegistry {
		themes[name] = theme
	}
	for name, theme := range themes {
		if got := theme.SuccessColor(); !got.Valid() {
			t.Errorf("%s has no success color: %v", name, got)
		}
	}
	legacy := Theme{StatusReview: tcell.NewRGBColor(1, 2, 3)}
	if got := legacy.SuccessColor(); got != legacy.StatusReview {
		t.Errorf("a theme with no Success falls back to %v, want the review color", got)
	}
}

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

func TestErrorTextIsNotReadAsColorTags(t *testing.T) {
	app := newUXTestApp(t)

	app.updateStatusBarWithError(errors.New("field [teamId] is required"))

	if got := app.statusBar.GetText(true); !strings.Contains(got, "[teamId]") {
		t.Errorf("error reads %q, want the bracketed field kept", got)
	}
}

func statusText(app *App) string {
	app.uiUpdateMu.Lock()
	defer app.uiUpdateMu.Unlock()
	return app.statusBar.GetText(true) + " " + app.statusToast.GetText(true)
}

const flashTestDuration = 5 * time.Millisecond

func shortenFlash(t *testing.T) {
	t.Helper()
	previous := flashDuration
	flashDuration = flashTestDuration
	t.Cleanup(func() { flashDuration = previous })
}

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

func TestALongFlashLeavesTheHintsRoom(t *testing.T) {
	app := newUXTestApp(t)
	app.focusedPane = FocusIssues
	app.flashStatus("Opened GitHub: https://github.com/praxis-labs-io/zen-linear/pull/1234 and then some")

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
