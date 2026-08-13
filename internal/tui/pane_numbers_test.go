package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// paneTitles returns each pane's title with the color tags stripped.
func paneTitles(app *App) map[string]string {
	app.updateAllPaneTitles()
	return map[string]string{
		"navigation": stripTags(app.navigationPanel.GetTitle()),
		"issues":     stripTags(app.listIssuesTable.GetTitle()),
		"details":    stripTags(app.detailsDescriptionView.GetTitle()),
	}
}

// stripTags renders a title the way a view would and reads back the plain text.
func stripTags(title string) string {
	view := tview.NewTextView().SetDynamicColors(true)
	view.SetText(title)
	return view.GetText(true)
}

// TestPaneTitlesCarryTheirNumber covers the numbered, caret-free titles.
func TestPaneTitlesCarryTheirNumber(t *testing.T) {
	app := newUXTestApp(t)
	app.detailsHidden = false
	titles := paneTitles(app)

	for pane, want := range map[string]string{
		"navigation": "[1] Navigation",
		"issues":     "[2] Issues",
		"details":    "[3] Details",
	} {
		if got := strings.TrimSpace(titles[pane]); got != want {
			t.Errorf("%s title = %q, want %q", pane, got, want)
		}
	}
}

// TestPaneTitlesDropTheFocusCaret covers focus no longer being spelled with a
// caret: the border color and the active tab carry it instead.
func TestPaneTitlesDropTheFocusCaret(t *testing.T) {
	app := newUXTestApp(t)
	app.detailsHidden = false

	for _, pane := range []FocusTarget{FocusNavigation, FocusIssues, FocusDetails} {
		app.focusedPane = pane
		for name, title := range paneTitles(app) {
			if strings.Contains(title, "▶") {
				t.Errorf("%s title = %q while %v had focus, want no caret", name, title, pane)
			}
		}
	}
}

// TestNumberKeysFocusPanes covers typing a pane's number to reach it.
func TestNumberKeysFocusPanes(t *testing.T) {
	app := newUXTestApp(t)
	app.detailsHidden = false

	tests := []struct {
		key  rune
		want FocusTarget
	}{
		{'3', FocusDetails},
		{'1', FocusNavigation},
		{'2', FocusIssues},
		{'1', FocusNavigation},
	}

	for _, tt := range tests {
		pressKey(app, tt.key)
		if app.focusedPane != tt.want {
			t.Errorf("%q focused %v, want %v", tt.key, app.focusedPane, tt.want)
		}
	}
}

// TestNumberKeyRevealsAHiddenPane covers a number summoning a pane that was
// toggled off, rather than doing nothing.
func TestNumberKeyRevealsAHiddenPane(t *testing.T) {
	app := newUXTestApp(t)
	if !app.detailsHidden {
		t.Fatal("details pane starts open; this test covers the closed case")
	}

	pressKey(app, '3')

	if app.detailsHidden {
		t.Error("details pane still hidden after its number was typed")
	}
	if app.focusedPane != FocusDetails {
		t.Errorf("focused pane = %v, want Details", app.focusedPane)
	}

	app.navigationHidden = true
	pressKey(app, '1')
	if app.navigationHidden || app.focusedPane != FocusNavigation {
		t.Errorf("navigation hidden = %v, focused = %v; want shown and focused", app.navigationHidden, app.focusedPane)
	}
}

// TestNumberKeysRebind covers the numbers going through actionKey rather than
// being compared against a literal rune.
func TestNumberKeysRebind(t *testing.T) {
	app := NewApp(linearapi.ClientConfig{}, config.Config{
		PageSize: 1,
		CacheTTL: time.Minute,
		Keybindings: map[string]string{
			"focus_details": "d",
		},
	}, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	stopBackgroundWorkOnCleanup(t, app)
	app.detailsHidden = false

	pressKey(app, 'd')
	if app.focusedPane != FocusDetails {
		t.Errorf("rebound key focused %v, want Details", app.focusedPane)
	}

	app.focusedPane = FocusIssues
	pressKey(app, '3')
	if app.focusedPane == FocusDetails {
		t.Error("3 still focused Details after focus_details was rebound to d")
	}
}

// TestFocusedPaneNumberTakesTheAccent covers the number carrying focus along
// with the border: dim while the pane is idle, accent while it holds the keys.
func TestFocusedPaneNumberTakesTheAccent(t *testing.T) {
	app := newUXTestApp(t)
	app.detailsHidden = false

	titleFor := map[FocusTarget]func() string{
		FocusNavigation: app.navigationPanel.GetTitle,
		FocusIssues:     app.listIssuesTable.GetTitle,
		FocusDetails:    app.detailsDescriptionView.GetTitle,
	}

	for pane, title := range titleFor {
		app.focusedPane = pane
		app.updateAllPaneTitles()
		if got := title(); !strings.HasPrefix(strings.TrimSpace(got), app.themeTags.Accent) {
			t.Errorf("focused %v number = %q, want it to lead with the accent tag %q", pane, got, app.themeTags.Accent)
		}

		app.focusedPane = FocusPalette
		app.updateAllPaneTitles()
		if got := title(); !strings.HasPrefix(strings.TrimSpace(got), app.themeTags.SecondaryText) {
			t.Errorf("idle %v number = %q, want it to lead with the secondary tag %q", pane, got, app.themeTags.SecondaryText)
		}
	}
}

// TestNavigationTitleNamesTheWorkspace covers the workspace moving from a tree
// row to the pane border.
func TestNavigationTitleNamesTheWorkspace(t *testing.T) {
	app := newUXTestApp(t)
	app.activeWorkspaceName = "Praxis Labs"

	if got := strings.TrimSpace(paneTitles(app)["navigation"]); got != "[1] Praxis Labs" {
		t.Errorf("navigation title = %q, want %q", got, "[1] Praxis Labs")
	}
}

// The workspace name is the user's, and the title is built from color tags.
func TestNavigationTitleKeepsABracketedWorkspace(t *testing.T) {
	app := newUXTestApp(t)
	app.activeWorkspaceName = "[red] labs"

	if got := paneTitles(app)["navigation"]; !strings.Contains(got, "[red] labs") {
		t.Errorf("navigation title = %q, want the bracketed name kept", got)
	}
}
