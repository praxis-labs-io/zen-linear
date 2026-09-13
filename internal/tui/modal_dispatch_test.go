package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/rivo/tview"
)

var modalDispatchGolden = []string{
	"confirmation",
	"picker",
	"issue_form",
	"text_input",
	"multi_select",
	"settings",
	"prompt_templates",
	"agent_prompt",
	"agent_output",
	"keys",
}

func TestModalDispatchGoldenCoversTheRegistry(t *testing.T) {
	if len(modalBindings) != len(modalDispatchGolden) {
		t.Fatalf("registry has %d modals, golden list has %d", len(modalBindings), len(modalDispatchGolden))
	}
	for i, binding := range modalBindings {
		if binding.page != modalDispatchGolden[i] {
			t.Fatalf("registry[%d] is %q, golden list says %q", i, binding.page, modalDispatchGolden[i])
		}
	}
}

func openModal(t *testing.T, app *App, page string) {
	t.Helper()
	switch page {
	case "confirmation":
		app.confirmationModal.Show("Delete", "Sure?", "Delete", func() {})
	case "picker":
		app.showSortByPicker()
	case "issue_form":
		app.issueFormModal.Show(IssueFormOptions{TeamID: "team-1"})
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
	case "keys":
		app.ShowKeysModal()
	default:
		t.Fatalf("openModal: no opener for page %q", page)
	}
	if !app.pages.HasPage(page) {
		t.Fatalf("openModal(%q) left no page behind", page)
	}
}

func sendKey(app *App, key tcell.Key) {
	pressFieldKey(app, key)
}

func escape(app *App) {
	sendKey(app, tcell.KeyEscape)
}

func openPages(app *App) map[string]bool {
	open := make(map[string]bool, len(modalDispatchGolden))
	for _, page := range modalDispatchGolden {
		if app.pages.HasPage(page) {
			open[page] = true
		}
	}
	return open
}

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

var modalFocusTargets = []struct {
	page string
	want func(*App) tview.Primitive
}{
	{"picker", func(a *App) tview.Primitive { return a.pickerModal.list }},
	{"issue_form", func(a *App) tview.Primitive { return a.issueFormModal.fm.focusedPrimitive() }},
	{"text_input", func(a *App) tview.Primitive { return a.textInputModal.input }},
	{"multi_select", func(a *App) tview.Primitive { return a.multiSelectModal.list }},
	{"settings", func(a *App) tview.Primitive { return a.settingsModal.fm.focusedPrimitive() }},
	{"prompt_templates", func(a *App) tview.Primitive { return a.promptTemplatesModal.list }},
	{"agent_prompt", func(a *App) tview.Primitive { return a.agentPromptModal.fm.focusedPrimitive() }},
	{"agent_output", func(a *App) tview.Primitive { return a.agentOutputModal.streamView }},
}

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

func TestOverlayRestoresTheFieldTheUserWasIn(t *testing.T) {
	t.Run("prompt_templates", func(t *testing.T) {
		app := newUXTestApp(t)
		openModal(t, app, "prompt_templates")
		sendKey(app, tcell.KeyEnter)

		want := tview.Primitive(app.promptTemplatesModal.nameField)
		if app.app.GetFocus() != want {
			t.Fatal("Enter on a template did not move focus to the name field")
		}

		openModal(t, app, "picker")
		escape(app)

		if got := app.app.GetFocus(); got != want {
			t.Fatalf("focus after the overlay closed = %T, want the name field", got)
		}
	})

	t.Run("agent_output", func(t *testing.T) {
		app := newUXTestApp(t)
		openModal(t, app, "agent_output")
		sendKey(app, tcell.KeyTab)

		want := tview.Primitive(app.agentOutputModal.finalView)
		if app.app.GetFocus() != want {
			t.Fatal("Tab did not move focus to the final view")
		}

		openModal(t, app, "picker")
		escape(app)

		if got := app.app.GetFocus(); got != want {
			t.Fatalf("focus after the overlay closed = %T, want the final view", got)
		}
	})
}

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
