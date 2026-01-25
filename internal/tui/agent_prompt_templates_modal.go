package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/roeyazroel/linear-tui/internal/config"
)

// AgentPromptTemplatesModal manages editing of agent prompt templates.
type AgentPromptTemplatesModal struct {
	app           *App
	modal         *tview.Flex
	list          *tview.List
	form          *tview.Form
	nameField     *tview.InputField
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
		SetMainTextColor(app.theme.Foreground).
		SetSelectedBackgroundColor(app.theme.Accent).
		SetSelectedTextColor(app.theme.SelectionText).
		SetHighlightFullLine(true)
	pm.list.SetBackgroundColor(app.theme.HeaderBg)
	pm.list.SetChangedFunc(func(index int, _ string, _ string, _ rune) {
		pm.selectTemplate(index)
	})

	pm.form = tview.NewForm()
	pm.form.SetBackgroundColor(app.theme.HeaderBg)
	pm.form.SetFieldBackgroundColor(app.theme.InputBg)
	pm.form.SetFieldTextColor(app.theme.Foreground)
	pm.form.SetButtonBackgroundColor(app.theme.Accent)
	pm.form.SetButtonTextColor(app.theme.SelectionText)
	pm.form.SetLabelColor(app.theme.Foreground)
	pm.form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			pm.Hide()
			return nil
		}
		return event
	})

	pm.nameField = tview.NewInputField().
		SetLabel("Name").
		SetFieldWidth(40)
	pm.form.AddFormItem(pm.nameField)

	pm.form.AddTextArea("Prompt", "", 70, 8, 0, nil)
	if item := pm.form.GetFormItemByLabel("Prompt"); item != nil {
		if textArea, ok := item.(*tview.TextArea); ok {
			pm.promptField = textArea
		}
	}

	pm.form.AddButton("Add", func() {
		pm.addTemplate()
	})
	pm.form.AddButton("Delete", func() {
		pm.deleteSelected()
	})
	pm.form.AddButton("Save", func() {
		pm.saveTemplates()
	})
	pm.form.AddButton("Cancel", func() {
		pm.Hide()
	})

	titleView := tview.NewTextView()
	titleView.SetText("Edit Agent Prompts")
	titleView.SetTextColor(app.theme.Accent)
	titleView.SetBackgroundColor(app.theme.HeaderBg)

	pm.helpView = tview.NewTextView()
	pm.helpView.SetText("a: add | d: delete | Ctrl+S: save | Esc: cancel")
	pm.helpView.SetTextColor(app.theme.SecondaryText)
	pm.helpView.SetBackgroundColor(app.theme.HeaderBg)
	pm.helpView.SetTextAlign(tview.AlignCenter)

	body := tview.NewFlex().
		AddItem(pm.list, 0, 1, true).
		AddItem(pm.form, 0, 2, false)

	modalContent := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(titleView, 1, 0, false).
		AddItem(body, 0, 1, true).
		AddItem(pm.helpView, 1, 0, false)
	modalContent.Box = tview.NewBox().SetBackgroundColor(app.theme.HeaderBg)
	modalContent.SetBackgroundColor(app.theme.HeaderBg).
		SetBorder(true).
		SetBorderColor(app.theme.Accent).
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
	pm.modal.SetBackgroundColor(app.theme.Background)

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
	pm.app.app.SetFocus(pm.list)
}

// Hide hides the prompt templates modal.
func (pm *AgentPromptTemplatesModal) Hide() {
	pm.app.pages.RemovePage("prompt_templates")
	pm.app.updateFocus()
}

// HandleKey handles keyboard input for the prompt templates modal.
func (pm *AgentPromptTemplatesModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		pm.Hide()
		return nil
	case tcell.KeyCtrlS:
		pm.saveTemplates()
		return nil
	case tcell.KeyTab:
		if pm.app.app.GetFocus() == pm.list {
			pm.app.app.SetFocus(pm.form)
			return nil
		}
	case tcell.KeyBacktab:
		if pm.app.app.GetFocus() == pm.nameField {
			pm.app.app.SetFocus(pm.list)
			return nil
		}
	}

	if event.Key() == tcell.KeyRune {
		switch event.Rune() {
		case 'a':
			pm.addTemplate()
			return nil
		case 'd':
			pm.deleteSelected()
			return nil
		}
	}

	return event
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
	pm.app.app.SetFocus(pm.nameField)
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
