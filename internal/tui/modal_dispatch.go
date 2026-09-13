package tui

import "github.com/gdamore/tcell/v2"

type modalController interface {
	HandleKey(*tcell.EventKey) *tcell.EventKey
	Focus()
}

type modalBinding struct {
	page string
	// Resolved per lookup, since rebuildModals replaces every modal pointer.
	controller func(*App) modalController
}

// Dispatch priority, not stack order: the first open page takes the key, whichever opened last.
var modalBindings = []modalBinding{
	{"confirmation", func(a *App) modalController { return a.confirmationModal }},
	{"picker", func(a *App) modalController { return a.pickerModal }},
	{"issue_form", func(a *App) modalController { return a.issueFormModal }},
	{"text_input", func(a *App) modalController { return a.textInputModal }},
	{"multi_select", func(a *App) modalController { return a.multiSelectModal }},
	{"settings", func(a *App) modalController { return a.settingsModal }},
	{"prompt_templates", func(a *App) modalController { return a.promptTemplatesModal }},
	{"agent_prompt", func(a *App) modalController { return a.agentPromptModal }},
	{"agent_output", func(a *App) modalController { return a.agentOutputModal }},
	{"keys", func(a *App) modalController { return a.keysModal }},
}

func (a *App) activeModal() modalController {
	for _, binding := range modalBindings {
		if a.pages.HasPage(binding.page) {
			return binding.controller(a)
		}
	}
	return nil
}

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

func (a *App) repairModalFocus() {
	for _, binding := range modalBindings {
		if !a.pages.HasPage(binding.page) {
			continue
		}
		if page := a.pages.GetPage(binding.page); page != nil && !page.HasFocus() {
			a.restoreModalFocus()
		}
		return
	}
}
