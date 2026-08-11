package tui

import "github.com/gdamore/tcell/v2"

// modalController is what a modal implements to take part in global key
// dispatch. Focus is part of it, not an optional assertion, so a new modal
// cannot register without saying where keyboard focus belongs.
type modalController interface {
	HandleKey(*tcell.EventKey) *tcell.EventKey
	Focus()
}

// modalBinding ties a page name to the modal that owns keys while that page is
// up.
type modalBinding struct {
	page string
	// Resolved on every lookup: rebuildModals replaces every modal pointer on
	// a theme or settings change.
	controller func(*App) modalController
}

// modalBindings is dispatch priority, not stack order. The first page found
// open takes the key, whichever modal opened last.
var modalBindings = []modalBinding{
	{"confirmation", func(a *App) modalController { return a.confirmationModal }},
	{"picker", func(a *App) modalController { return a.pickerModal }},
	{"issue_form", func(a *App) modalController { return a.issueFormModal }},
	{"edit_description", func(a *App) modalController { return a.editDescriptionModal }},
	{"edit_labels", func(a *App) modalController { return a.editLabelsModal }},
	{"text_input", func(a *App) modalController { return a.textInputModal }},
	{"multi_select", func(a *App) modalController { return a.multiSelectModal }},
	{"settings", func(a *App) modalController { return a.settingsModal }},
	{"prompt_templates", func(a *App) modalController { return a.promptTemplatesModal }},
	{"agent_prompt", func(a *App) modalController { return a.agentPromptModal }},
	{"agent_output", func(a *App) modalController { return a.agentOutputModal }},
}

// activeModal returns the modal that owns keys right now, or nil when none is
// open. A page is only ever added by the modal that owns it, so a page present
// means its pointer is set.
func (a *App) activeModal() modalController {
	for _, binding := range modalBindings {
		if a.pages.HasPage(binding.page) {
			return binding.controller(a)
		}
	}
	return nil
}

// restoreModalFocus hands keys back to whatever an overlay covered. It raises
// the same modal activeModal would pick, so what is on top is what types.
func (a *App) restoreModalFocus() {
	for _, binding := range modalBindings {
		if !a.pages.HasPage(binding.page) {
			continue
		}
		a.pages.SendToFront(binding.page)
		binding.controller(a).Focus()
		return
	}
	a.updateFocus()
}
