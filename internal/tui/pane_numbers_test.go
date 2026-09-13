package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

func paneTitles(app *App) map[string]string {
	app.updateAllPaneTitles()
	return map[string]string{
		"navigation": stripTags(app.navigationPanel.GetTitle()),
		"issues":     stripTags(app.listIssuesTable.GetTitle()),
		"details":    stripTags(app.detailsView.GetTitle()),
	}
}

func stripTags(title string) string {
	view := tview.NewTextView().SetDynamicColors(true)
	view.SetText(title)
	return view.GetText(true)
}

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

func TestASinglePaneLabelDimsWithItsNumber(t *testing.T) {
	app := newUXTestApp(t)
	app.detailsHidden = false

	titleFor := map[FocusTarget]func() string{
		FocusNavigation: app.navigationPanel.GetTitle,
		FocusIssues:     app.listIssuesTable.GetTitle,
	}

	for pane, title := range titleFor {
		app.focusedPane = pane
		app.updateAllPaneTitles()
		if got := title(); strings.Count(got, app.themeTags.Accent) != 2 {
			t.Errorf("focused %v title = %q, want the number and the label both accented", pane, got)
		}

		app.focusedPane = FocusPalette
		app.updateAllPaneTitles()
		if got := title(); strings.Contains(got, app.themeTags.Foreground) {
			t.Errorf("idle %v title = %q, want the label dimmed rather than lit", pane, got)
		}
		if got := title(); strings.Count(got, app.themeTags.SecondaryText) != 2 {
			t.Errorf("idle %v title = %q, want the number and the label both dim", pane, got)
		}
	}
}

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

func TestFocusedPaneNumberTakesTheAccent(t *testing.T) {
	app := newUXTestApp(t)
	app.detailsHidden = false

	titleFor := map[FocusTarget]func() string{
		FocusNavigation: app.navigationPanel.GetTitle,
		FocusIssues:     app.listIssuesTable.GetTitle,
		FocusDetails:    app.detailsView.GetTitle,
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

func TestNavigationTitleNamesTheWorkspace(t *testing.T) {
	app := newUXTestApp(t)
	app.activeWorkspaceName = "Praxis Labs"

	if got := strings.TrimSpace(paneTitles(app)["navigation"]); got != "[1] Praxis Labs" {
		t.Errorf("navigation title = %q, want %q", got, "[1] Praxis Labs")
	}
}

func TestNavigationTitleKeepsABracketedWorkspace(t *testing.T) {
	app := newUXTestApp(t)
	app.activeWorkspaceName = "[red] labs"

	if got := paneTitles(app)["navigation"]; !strings.Contains(got, "[red] labs") {
		t.Errorf("navigation title = %q, want the bracketed name kept", got)
	}
}
