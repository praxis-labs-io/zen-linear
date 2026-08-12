package tui

import (
	"strings"
	"testing"
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

// A flashed message shares the strip rather than replacing the hints, so the
// keys stay readable while it is up.
func TestFlashedMessageJoinsTheHints(t *testing.T) {
	app := newUXTestApp(t)
	app.focusedPane = FocusIssues
	app.statusMessage = "Copied ZNL-1"

	app.updateStatusBar()

	got := app.statusBar.GetText(true)
	if !strings.Contains(got, "Copied ZNL-1") || !strings.Contains(got, "j/k move") {
		t.Errorf("hints = %q, want the message beside the keys", got)
	}
}
