package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/config"
)

// AgentPromptTemplatesModal manages editing of agent prompt templates.
type AgentPromptTemplatesModal struct {
	app           *App
	modal         *tview.Flex
	list          *tview.List
	fm            *FormModal
	nameField     *tview.InputField
	focused       tview.Primitive
	promptField   *tview.TextArea
	helpView      *tview.TextView
	templates     []config.AgentPromptTemplate
	selectedIndex int
	onSave        func([]config.AgentPromptTemplate) error
}

const (
	promptTemplatesModalHeight = 24
	promptTemplatesModalWidth  = 110
)

// NewAgentPromptTemplatesModal creates a new prompt templates modal.
func NewAgentPromptTemplatesModal(app *App) *AgentPromptTemplatesModal {
	pm := &AgentPromptTemplatesModal{
		app:           app,
		selectedIndex: -1,
	}

	pm.list = tview.NewList().
		ShowSecondaryText(false).
		SetMainTextStyle(tcell.StyleDefault.Foreground(app.theme.Foreground).Background(app.theme.ModalBackground())).
		SetSelectedStyle(app.listSelectionStyle()).
		SetHighlightFullLine(true)
	pm.list.SetBackgroundColor(app.theme.ModalBackground())
	pm.list.SetChangedFunc(func(index int, _ string, _ string, _ rune) {
		pm.selectTemplate(index)
	})

	// The list lives in its own bordered box, lit like a field on focus.
	listFrame := tview.NewFlex().SetDirection(tview.FlexRow)
	listFrame.Box = tview.NewBox() // restore the background fill (see NewFormModal)
	listFrame.AddItem(pm.list, 0, 1, true)
	listFrame.SetBackgroundColor(app.theme.ModalBackground()).
		SetBorder(true).
		SetBorderColor(app.theme.Border)
	pm.list.SetFocusFunc(func() {
		// Focus can leave the form for the list; without this the last
		// form field keeps its focused border.
		pm.fm.BlurFrames()
		listFrame.SetBorderColor(app.theme.BorderFocus)
	})
	pm.list.SetBlurFunc(func() {
		listFrame.SetBorderColor(app.theme.Border)
	})

	pm.fm = NewFormModal(app, "Agent Prompts")
	pm.nameField = pm.fm.AddInput("Name", "")
	pm.promptField = pm.fm.AddTextArea("Prompt", "", 8)
	pm.fm.AddButtons(
		FormButton{Label: "Add", OnPress: pm.addTemplate},
		FormButton{Label: "Delete", OnPress: pm.deleteSelected},
		FormButton{Label: "Save", OnPress: pm.saveTemplates},
		FormButton{Label: "Cancel", OnPress: pm.Hide},
	)
	pm.fm.SetOnSubmit(pm.saveTemplates)
	pm.fm.SetOnCancel(pm.Hide)

	pm.helpView = tview.NewTextView()
	pm.helpView.SetText("⏎ edit · a add · d delete\n⇧Tab list · ⌃S save · Esc cancel")
	pm.helpView.SetTextColor(app.theme.SecondaryText)
	pm.helpView.SetBackgroundColor(app.theme.ModalBackground())
	pm.helpView.SetTextAlign(tview.AlignCenter)

	// Left column: the list with its hints beneath. Right column: the form
	// with its buttons at the bottom.
	leftColumn := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(listFrame, 0, 1, true).
		AddItem(nil, 1, 0, false).
		AddItem(pm.helpView, 2, 0, false)

	rightColumn := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(pm.fm.ContentBody(), 0, 1, false).
		AddItem(nil, 1, 0, false).
		AddItem(pm.fm.ButtonsRow(), 1, 0, false).
		AddItem(nil, 1, 0, false)

	modalContent := tview.NewFlex().
		AddItem(leftColumn, 0, 1, true).
		AddItem(nil, 2, 0, false).
		AddItem(rightColumn, 0, 2, false)
	modalContent.Box = tview.NewBox().SetBackgroundColor(app.theme.ModalBackground())
	modalContent.SetBackgroundColor(app.theme.ModalBackground()).
		SetBorder(true).
		SetBorderColor(app.theme.BorderFocus).
		SetTitle(" Agent Prompts ").
		SetTitleColor(app.theme.Foreground)
	padding := app.density.ModalPadding
	modalContent.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)

	pm.modal = tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(modalContent, promptTemplatesModalHeight, 0, true).
			AddItem(nil, 0, 1, false), promptTemplatesModalWidth, 0, true).
		AddItem(nil, 0, 1, false)
	pm.modal.SetBackgroundColor(app.theme.ModalBackground())

	return pm
}

// Show displays the prompt templates modal with the current templates.
func (pm *AgentPromptTemplatesModal) Show(templates []config.AgentPromptTemplate, onSave func([]config.AgentPromptTemplate) error) {
	pm.templates = append([]config.AgentPromptTemplate(nil), templates...)
	pm.onSave = onSave
	pm.selectedIndex = -1

	pm.refreshList()
	if len(pm.templates) > 0 {
		pm.list.SetCurrentItem(0)
		pm.selectTemplate(0)
	} else {
		pm.clearFields()
	}

	pm.app.pages.AddPage("prompt_templates", pm.modal, true, true)
	pm.app.pages.SendToFront("prompt_templates")
	pm.focus(pm.list)
}

// Hide hides the prompt templates modal.
func (pm *AgentPromptTemplatesModal) Hide() {
	pm.app.pages.RemovePage("prompt_templates")
	pm.app.restoreModalFocus()
}

// focus records the target so Focus can put the user back where they were.
// UI thread only.
func (pm *AgentPromptTemplatesModal) focus(p tview.Primitive) {
	pm.focused = p
	pm.app.app.SetFocus(p)
}

// Focus returns keyboard focus for when an overlay above this modal closes.
// It restores the field the user was in: the list arms 'a' and 'd' as add and
// delete, so landing there mid-edit turns typing into a deletion.
func (pm *AgentPromptTemplatesModal) Focus() {
	if pm.focused == nil {
		pm.focused = pm.list
	}
	pm.app.app.SetFocus(pm.focused)
}

// HandleKey handles keyboard input for the prompt templates modal.
func (pm *AgentPromptTemplatesModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	listFocused := pm.app.app.GetFocus() == pm.list

	switch event.Key() {
	case tcell.KeyEscape:
		pm.Hide()
		return nil
	case tcell.KeyCtrlS:
		pm.saveTemplates()
		return nil
	case tcell.KeyEnter, tcell.KeyRight:
		// Enter (or right) on a template is edit mode: jump to its fields.
		if listFocused {
			pm.focus(pm.nameField)
			return nil
		}
	case tcell.KeyTab:
		if listFocused {
			pm.focus(pm.nameField)
			return nil
		}
	case tcell.KeyBacktab:
		if pm.app.app.GetFocus() == pm.nameField {
			pm.focus(pm.list)
			return nil
		}
	}

	// The add/delete shortcuts only fire from the list; in the form they
	// must stay typeable characters.
	if listFocused && event.Key() == tcell.KeyRune {
		switch event.Rune() {
		case 'a':
			pm.addTemplate()
			return nil
		case 'd':
			pm.deleteSelected()
			return nil
		}
	}
	if listFocused {
		return event
	}
	return pm.fm.HandleKey(event)
}

func (pm *AgentPromptTemplatesModal) refreshList() {
	pm.list.Clear()
	for _, template := range pm.templates {
		pm.list.AddItem(displayTemplateName(template.Name), "", 0, nil)
	}
}

func (pm *AgentPromptTemplatesModal) clearFields() {
	if pm.nameField != nil {
		pm.nameField.SetText("")
	}
	if pm.promptField != nil {
		pm.promptField.SetText("", true)
	}
}

func (pm *AgentPromptTemplatesModal) selectTemplate(index int) {
	pm.applyFieldsToSelected()
	if index < 0 || index >= len(pm.templates) {
		pm.selectedIndex = -1
		pm.clearFields()
		return
	}

	pm.selectedIndex = index
	template := pm.templates[index]
	if pm.nameField != nil {
		pm.nameField.SetText(template.Name)
	}
	if pm.promptField != nil {
		pm.promptField.SetText(template.Prompt, true)
	}
}

func (pm *AgentPromptTemplatesModal) applyFieldsToSelected() {
	if pm.selectedIndex < 0 || pm.selectedIndex >= len(pm.templates) {
		return
	}
	if pm.nameField != nil {
		pm.templates[pm.selectedIndex].Name = pm.nameField.GetText()
	}
	if pm.promptField != nil {
		pm.templates[pm.selectedIndex].Prompt = pm.promptField.GetText()
	}

	name := displayTemplateName(pm.templates[pm.selectedIndex].Name)
	pm.list.SetItemText(pm.selectedIndex, name, "")
}

func (pm *AgentPromptTemplatesModal) addTemplate() {
	pm.applyFieldsToSelected()
	name := pm.nextTemplateName()
	pm.templates = append(pm.templates, config.AgentPromptTemplate{
		Name:   name,
		Prompt: "",
	})
	pm.refreshList()
	newIndex := len(pm.templates) - 1
	pm.list.SetCurrentItem(newIndex)
	pm.selectTemplate(newIndex)
	pm.focus(pm.nameField)
}

func (pm *AgentPromptTemplatesModal) deleteSelected() {
	if pm.selectedIndex < 0 || pm.selectedIndex >= len(pm.templates) {
		pm.app.updateStatusBarWithError(fmt.Errorf("no template selected"))
		return
	}
	pm.templates = append(pm.templates[:pm.selectedIndex], pm.templates[pm.selectedIndex+1:]...)
	pm.selectedIndex = -1
	pm.refreshList()
	if len(pm.templates) == 0 {
		pm.clearFields()
		return
	}

	nextIndex := pm.list.GetCurrentItem()
	if nextIndex < 0 || nextIndex >= len(pm.templates) {
		nextIndex = len(pm.templates) - 1
	}
	pm.list.SetCurrentItem(nextIndex)
	pm.selectTemplate(nextIndex)
}

func (pm *AgentPromptTemplatesModal) saveTemplates() {
	pm.applyFieldsToSelected()
	templates, err := pm.validateTemplates()
	if err != nil {
		pm.app.updateStatusBarWithError(err)
		return
	}
	if pm.onSave != nil {
		if err := pm.onSave(templates); err != nil {
			pm.app.updateStatusBarWithError(err)
			return
		}
	}
	pm.Hide()
}

func (pm *AgentPromptTemplatesModal) validateTemplates() ([]config.AgentPromptTemplate, error) {
	if len(pm.templates) == 0 {
		return nil, fmt.Errorf("at least one template is required")
	}

	valid := make([]config.AgentPromptTemplate, 0, len(pm.templates))
	for i, template := range pm.templates {
		name := strings.TrimSpace(template.Name)
		prompt := strings.TrimSpace(template.Prompt)
		if name == "" || prompt == "" {
			return nil, fmt.Errorf("template %d must include a name and prompt", i+1)
		}
		valid = append(valid, config.AgentPromptTemplate{
			Name:   name,
			Prompt: prompt,
		})
	}

	return valid, nil
}

func (pm *AgentPromptTemplatesModal) nextTemplateName() string {
	base := "New template"
	if !pm.templateNameExists(base) {
		return base
	}
	for i := 2; i < 1000; i++ {
		name := fmt.Sprintf("%s %d", base, i)
		if !pm.templateNameExists(name) {
			return name
		}
	}
	return fmt.Sprintf("%s %d", base, len(pm.templates)+1)
}

func (pm *AgentPromptTemplatesModal) templateNameExists(name string) bool {
	for _, template := range pm.templates {
		if strings.EqualFold(strings.TrimSpace(template.Name), strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

func displayTemplateName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "(untitled)"
	}
	return trimmed
}
