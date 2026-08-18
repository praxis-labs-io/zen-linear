package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// keysPage is what the reference is showing, one line per row.
func keysPage(app *App) []string {
	return strings.Split(app.keysModal.view.GetText(true), "\n")
}

// openKeys presses the reference's own key through the dispatcher, the way a
// reader does, and returns what it drew.
func openKeys(t *testing.T, app *App) []string {
	t.Helper()
	pressKey(app, '?')
	if !app.pages.HasPage("keys") {
		t.Fatal("? left no keys page behind")
	}
	return keysPage(app)
}

// A section with no rows is a context the reader gets nothing for, which is
// worse than one the reference never mentions.
func TestEveryKeySectionHasRows(t *testing.T) {
	app := newUXTestApp(t)
	for _, section := range keySections {
		if len(app.keyRows(section.rows(app))) == 0 {
			t.Errorf("section %q lists no keys", section.title)
		}
	}
}

// keysTitle is the context the panel says it is for.
func keysTitle(app *App) string {
	return app.keysModal.panel.GetTitle()
}

// A legend, not a manual: the keys for here, and the ones that work anywhere.
func TestTheLegendShowsThisContextAndTheGlobalKeys(t *testing.T) {
	app := newUXTestApp(t)
	app.focusedPane = FocusIssues

	page := strings.Join(openKeys(t, app), "\n")

	if !strings.Contains(keysTitle(app), "Issue list") {
		t.Fatalf("panel title = %q, want the issue list named", keysTitle(app))
	}
	if !strings.Contains(page, "Anywhere") {
		t.Error("the global keys are missing: the pane numbers and quit work here too")
	}
	for _, section := range keySections {
		switch section.context {
		case keyContextIssues, keyContextGlobal:
			continue
		}
		if strings.Contains(page, section.title) {
			t.Errorf("%q is on the page, and the reader is not in it", section.title)
		}
	}
}

func TestTheLegendOpensOnEachContext(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*App)
		want  string
	}{
		{"the tree", func(a *App) { a.focusedPane = FocusNavigation }, "Navigation tree"},
		{"the issue list", func(a *App) { a.focusedPane = FocusIssues }, "Issue list"},
		{"the details page", func(a *App) { a.focusedPane = FocusDetails }, "Details page"},
		{"edit mode", func(a *App) {
			a.focusedPane = FocusDetails
			a.enterDetailsEdit()
		}, "Edit mode"},
		{"a picked comment", func(a *App) {
			a.selectedIssue.Comments = threadedComments()
			a.updateDetailsView()
			focusCommentCards(a)
			a.stepDetailsFocus(false)
		}, "A picked comment"},
		{"an open chooser", func(a *App) {
			a.focusedPane = FocusDetails
			a.enterDetailsEdit()
			a.detailsEdit.open = issueFieldState
		}, "Field chooser"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newDetailsTestApp(t)
			tc.setup(app)
			drawComments(t, app, 90)

			openKeys(t, app)
			if !strings.Contains(keysTitle(app), tc.want) {
				t.Fatalf("panel title = %q, want %q", keysTitle(app), tc.want)
			}
		})
	}
}

// A box is prose, so the key that opens the reference everywhere else has to
// land in the words instead.
func TestTheKeyTypesItselfInsideABox(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*testing.T, *App)
	}{
		{"the comment box", func(t *testing.T, a *App) {
			a.focusedPane = FocusDetails
			a.detailsHidden = false
			drawDetails(t, a, 90)
			// What a click into the box does: the widget takes the keyboard,
			// which is what composeBoxActive reads.
			a.app.SetFocus(a.detailsComposeArea)
		}},
		{"the navigation query box", func(_ *testing.T, a *App) {
			a.focusedPane = FocusNavigation
			a.navSearchFocused = true
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newDetailsTestApp(t)
			tc.setup(t, app)

			left := pressField(app, '?')

			if app.pages.HasPage("keys") {
				t.Fatal("? opened the reference from inside a box")
			}
			if left == nil || left.Rune() != '?' {
				t.Fatalf("the box was handed %v, want the ? to reach the text", left)
			}
		})
	}
}

// Every row resolves through the binding layer, so a moved key prints where it
// moved to rather than the default the code was written with.
func TestAMovedBindingPrintsItsNewKey(t *testing.T) {
	app := bindingApp(t, map[string]string{"comment_next": "n"})
	app.focusedPane = FocusDetails

	page := strings.Join(openKeys(t, app), "\n")

	if !strings.Contains(page, "{/n") {
		t.Fatalf("the comment row does not name the moved key, page:\n%s", page)
	}
}

// A key another binding took answers nothing, and a reference that printed it
// would be advertising a dead key.
func TestATakenKeyIsLeftOutRatherThanAdvertised(t *testing.T) {
	app := bindingApp(t, map[string]string{"toggle_favorite": "?"})
	app.focusedPane = FocusIssues

	app.ShowKeysModal()
	page := strings.Join(keysPage(app), "\n")

	if strings.Contains(page, "these keys") {
		t.Fatalf("the reference still names its own key after a binding took it, page:\n%s", page)
	}
}

// The palette is the way back to a command whose rune was taken, and the
// reference is the command most in need of it.
func TestThePaletteCommandOpensTheReference(t *testing.T) {
	app := bindingApp(t, map[string]string{"toggle_favorite": "?"})

	pressKey(app, '?')
	if app.pages.HasPage("keys") {
		t.Fatal("? still opened the reference after a binding took the key")
	}

	runPaletteCommand(t, app, "show_keys")
	if !app.pages.HasPage("keys") {
		t.Fatal("the palette command left no keys page behind")
	}
}

func TestTheReferenceClosesOnEscapeAndOnItsOwnKey(t *testing.T) {
	for _, tc := range []struct {
		name string
		send func(*App)
	}{
		{"escape", func(a *App) { a.handleGlobalKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)) }},
		{"its own key", func(a *App) { pressKey(a, '?') }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newUXTestApp(t)
			app.focusedPane = FocusIssues
			openKeys(t, app)

			tc.send(app)

			if app.pages.HasPage("keys") {
				t.Fatal("the reference stayed open")
			}
		})
	}
}
