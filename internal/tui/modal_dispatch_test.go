package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// modalDispatchGolden is the priority the global key handler gives modals,
// written out rather than read from the registry so a reordered registry fails
// here instead of agreeing with itself.
var modalDispatchGolden = []string{
	"confirmation",
	"picker",
	"create_issue",
	"create_comment",
	"edit_title",
	"edit_description",
	"edit_labels",
	"text_input",
	"multi_select",
	"settings",
	"prompt_templates",
	"agent_prompt",
	"agent_output",
}

// openModal opens one modal through the same call the app makes, so the test
// exercises whatever state that path sets up. The picker goes through a real
// command rather than PickerModal.Show, which no caller uses on its own.
func openModal(t *testing.T, app *App, page string) {
	t.Helper()
	switch page {
	case "confirmation":
		app.confirmationModal.Show("Delete", "Sure?", "Delete", func() {})
	case "picker":
		app.showSortByPicker()
	case "create_issue":
		app.createIssueModal.Show("team-1", "", func(title, description, teamID, projectID, assigneeID, cycleID string, priority int) {
		})
	case "create_comment":
		app.createCommentModal.Show("issue-1", "ZNL-1", func(issueID, body string) {})
	case "edit_title":
		app.editTitleModal.Show("issue-1", "Title", "ZNL-1", func(issueID, title string) {})
	case "edit_description":
		app.editDescriptionModal.Show("issue-1", "Body", "ZNL-1", func(issueID, description string) {})
	case "edit_labels":
		app.editLabelsModal.Show("issue-1", nil, []linearapi.IssueLabel{{ID: "label-1", Name: "Bug"}}, "ZNL-1", func(issueID string, labelIDs []string) {})
	case "text_input":
		app.textInputModal.Show("Related Issue", "Issue ID: ", "", func(string) {})
	case "multi_select":
		app.multiSelectModal.Show("Columns", []MultiSelectItem{{ID: "id", Label: "ID"}}, nil, func([]string) {})
	case "settings":
		app.settingsModal.Show()
	case "prompt_templates":
		app.promptTemplatesModal.Show([]config.AgentPromptTemplate{{Name: "Summarize", Prompt: "go"}}, func([]config.AgentPromptTemplate) error { return nil })
	case "agent_prompt":
		app.agentPromptModal.Show("ZNL-1", func(prompt, workspace string) {})
	case "agent_output":
		app.agentOutputModal.Show("Agent", func() {})
	default:
		t.Fatalf("openModal: no opener for page %q", page)
	}
	if !app.pages.HasPage(page) {
		t.Fatalf("openModal(%q) left no page behind", page)
	}
}

func escape(app *App) {
	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
}

// openPages reports which of the golden pages are currently up.
func openPages(app *App) map[string]bool {
	open := make(map[string]bool, len(modalDispatchGolden))
	for _, page := range modalDispatchGolden {
		if app.pages.HasPage(page) {
			open[page] = true
		}
	}
	return open
}

// TestModalDispatchOrder is the guard on priority. Every modal is up at once
// and each Escape has to reach the highest-priority one, so a chain that
// dispatched by stack position or by any other order fails here. Modals open
// in priority order, which puts the lowest-priority one on top of the page
// stack: the point is that dispatch ignores that.
func TestModalDispatchOrder(t *testing.T) {
	app := newUXTestApp(t)
	for _, page := range modalDispatchGolden {
		openModal(t, app, page)
	}

	for i, want := range modalDispatchGolden {
		before := openPages(app)
		escape(app)
		after := openPages(app)

		var gone []string
		for page := range before {
			if !after[page] {
				gone = append(gone, page)
			}
		}
		if len(gone) != 1 {
			t.Fatalf("Escape %d removed %v, want exactly %q", i+1, gone, want)
		}
		if gone[0] != want {
			t.Fatalf("Escape %d closed %q, want %q", i+1, gone[0], want)
		}
	}
}

// TestModalDispatchRoutesToTheOpenModal covers each modal on its own, so a
// registry entry pointing at the wrong page or the wrong modal shows up even
// where the priority test would mask it.
func TestModalDispatchRoutesToTheOpenModal(t *testing.T) {
	for _, page := range modalDispatchGolden {
		t.Run(page, func(t *testing.T) {
			app := newUXTestApp(t)
			focused := app.focusedPane

			openModal(t, app, page)
			escape(app)

			if app.pages.HasPage(page) {
				t.Fatalf("Escape left %q open; the key went somewhere else", page)
			}
			if app.focusedPane != focused {
				t.Fatalf("Escape moved focus to %v, want %v untouched", app.focusedPane, focused)
			}
		})
	}
}

// modalFocusTargets names where keyboard focus belongs once an overlay above a
// modal closes. Form-backed modals answer with their own current field, which
// still fails if the registry hands the overlay's Focus call to the wrong
// modal: the field belongs to a different form.
var modalFocusTargets = []struct {
	page string
	want func(*App) tview.Primitive
}{
	{"picker", func(a *App) tview.Primitive { return a.pickerModal.list }},
	{"create_issue", func(a *App) tview.Primitive { return a.createIssueModal.fm.focusedPrimitive() }},
	{"create_comment", func(a *App) tview.Primitive { return a.createCommentModal.fm.focusedPrimitive() }},
	{"edit_title", func(a *App) tview.Primitive { return a.editTitleModal.fm.focusedPrimitive() }},
	{"edit_description", func(a *App) tview.Primitive { return a.editDescriptionModal.fm.focusedPrimitive() }},
	{"edit_labels", func(a *App) tview.Primitive { return a.editLabelsModal.list }},
	{"text_input", func(a *App) tview.Primitive { return a.textInputModal.input }},
	{"multi_select", func(a *App) tview.Primitive { return a.multiSelectModal.list }},
	{"settings", func(a *App) tview.Primitive { return a.settingsModal.fm.focusedPrimitive() }},
	{"prompt_templates", func(a *App) tview.Primitive { return a.promptTemplatesModal.list }},
	{"agent_prompt", func(a *App) tview.Primitive { return a.agentPromptModal.fm.focusedPrimitive() }},
	{"agent_output", func(a *App) tview.Primitive { return a.agentOutputModal.streamView }},
}

// TestOverlayRestoresFocusToTheModalBeneath covers what the picker's own
// focus-restore missed: it named create_issue and edit_title, so closing a
// picker over any other modal handed focus to a pane instead.
//
// The overlay is a picker, which is how this happens for real: one opened
// behind a fetch lands on whatever the user opened while it waited. The picker
// itself needs an overlay that outranks it, and confirmation is the only one.
// Confirmation has no such overlay and so has no row here.
func TestOverlayRestoresFocusToTheModalBeneath(t *testing.T) {
	for _, target := range modalFocusTargets {
		t.Run(target.page, func(t *testing.T) {
			app := newUXTestApp(t)
			openModal(t, app, target.page)
			want := target.want(app)

			overlay := "picker"
			if target.page == "picker" {
				overlay = "confirmation"
			}
			openModal(t, app, overlay)
			escape(app)

			if app.pages.HasPage(overlay) {
				t.Fatalf("Escape did not close the %s overlay", overlay)
			}
			if got := app.app.GetFocus(); got != want {
				t.Fatalf("focus after closing the overlay = %T, want %T from %s", got, want, target.page)
			}
		})
	}
}

// TestGlobalKeyCaptureIsBound drives the capture tview actually installed,
// rather than handleGlobalKey directly, so an unbound handler fails somewhere.
func TestGlobalKeyCaptureIsBound(t *testing.T) {
	app := newUXTestApp(t)
	capture := app.app.GetInputCapture()
	if capture == nil {
		t.Fatal("no global input capture installed")
	}

	openModal(t, app, "confirmation")
	capture(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))

	if app.pages.HasPage("confirmation") {
		t.Fatal("Escape through the installed capture did not reach the modal")
	}
}
