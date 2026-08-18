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

// The whole of "context aware": the reader's own section leads, and the rest
// stay reachable under it.
func TestTheReferenceLeadsWithTheContextItWasOpenedFrom(t *testing.T) {
	app := newUXTestApp(t)
	app.focusedPane = FocusIssues

	page := openKeys(t, app)

	if !strings.HasPrefix(page[0], "Issue list") {
		t.Fatalf("first line = %q, want the issue list's own section", page[0])
	}
	if !strings.Contains(page[0], "you are here") {
		t.Errorf("first line = %q, want it marked as the reader's context", page[0])
	}
	for _, section := range keySections {
		if !strings.Contains(strings.Join(page, "\n"), section.title) {
			t.Errorf("section %q is missing: every context stays reachable", section.title)
		}
	}
	if marked := strings.Count(strings.Join(page, "\n"), "you are here"); marked != 1 {
		t.Errorf("%d sections marked, want exactly one", marked)
	}
}

func TestTheReferenceOpensOnEachContext(t *testing.T) {
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

			page := openKeys(t, app)
			if !strings.HasPrefix(page[0], tc.want) {
				t.Fatalf("first line = %q, want %q", page[0], tc.want)
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

// keysRows is the page the reference lays out for a screen of that width.
func keysRows(t *testing.T, app *App, width int) []string {
	t.Helper()
	app.pages.SetRect(0, 0, width, 44)
	lines, _ := app.keysPage(app.keyBlocks(keyContextIssues))
	return lines
}

// Ninety rows in a narrow panel is a lot of scrolling beside a lot of empty
// terminal, so the sections column up into whatever width there is.
func TestTheReferenceColumnsUpOnAWiderScreen(t *testing.T) {
	app := newUXTestApp(t)

	narrow := len(keysRows(t, app, 80))
	medium := len(keysRows(t, app, 120))
	wide := len(keysRows(t, app, 170))

	if wide >= medium || medium >= narrow {
		t.Fatalf("rows were %d wide, %d medium, %d narrow, want each wider screen shorter", wide, medium, narrow)
	}
	if wide*keysMaxColumns < narrow-len(keySections) {
		t.Errorf("the widest layout is %d rows against %d, want about a third of them", wide, narrow)
	}
}

// A heading in one column with half its keys in the next is worse than the
// scrolling it saved.
func TestNoSectionIsSplitAcrossAColumn(t *testing.T) {
	app := newUXTestApp(t)
	blocks := app.keyBlocks(keyContextIssues)

	// Past keysMaxColumns as well: a break that lands on a block boundary by
	// luck at one count will not at six.
	for count := 1; count <= 6; count++ {
		columns := packKeyBlocks(blocks, count)
		if len(columns) > count {
			t.Fatalf("packed into %d columns, want at most %d", len(columns), count)
		}
		joined := make([]string, len(columns))
		for i, column := range columns {
			joined[i] = strings.Join(column, "\n")
		}
		for _, block := range blocks {
			whole := strings.Join(block, "\n")
			held := 0
			for _, column := range joined {
				if strings.Contains(column, whole) {
					held++
				}
			}
			if held != 1 {
				t.Errorf("in %d columns, %q sits whole in %d of them, want exactly one",
					count, block[0], held)
			}
		}
	}
}
