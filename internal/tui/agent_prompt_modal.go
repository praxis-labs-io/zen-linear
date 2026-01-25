package tui

import (
	"os"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// AgentPromptModal manages the prompt input for agent runs.
type AgentPromptModal struct {
	app             *App
	modal           *tview.Flex
	modalContent    *tview.Flex
	modalWidth      int
	form            *tview.Form
	templateField   *tview.DropDown
	templateLabels  []string
	templatePrompts []string
	promptField     *tview.TextArea
	workspaceField  *tview.InputField
	onSubmit        func(prompt string, workspace string)
}

const (
	agentPromptLabel    = "Prompt (issue context included)"
	minPromptModalWidth = 80
	maxPromptModalWidth = 140
	promptModalHeight   = 20
)

// NewAgentPromptModal creates a new agent prompt modal.
func NewAgentPromptModal(app *App) *AgentPromptModal {
	am := &AgentPromptModal{
		app: app,
	}

	am.form = tview.NewForm()
	am.form.SetBackgroundColor(app.theme.HeaderBg)
	am.form.SetFieldBackgroundColor(app.theme.InputBg)
	am.form.SetFieldTextColor(app.theme.Foreground)
	am.form.SetButtonBackgroundColor(app.theme.Accent)
	am.form.SetButtonTextColor(app.theme.SelectionText)
	am.form.SetLabelColor(app.theme.Foreground)

	am.workspaceField = tview.NewInputField().
		SetLabel("Workspace").
		SetFieldWidth(0)
	am.form.AddFormItem(am.workspaceField)

	if len(app.agentPromptTemplates) > 0 {
		labels := make([]string, 0, len(app.agentPromptTemplates))
		prompts := make([]string, 0, len(app.agentPromptTemplates))
		for _, template := range app.agentPromptTemplates {
			labels = append(labels, template.Name)
			prompts = append(prompts, template.Prompt)
		}
		am.templateLabels = labels
		am.templatePrompts = prompts

		am.templateField = tview.NewDropDown().
			SetLabel("Template").
			SetOptions(am.templateLabels, func(_ string, index int) {
				am.applyTemplatePrompt(index)
			})
		am.templateField.SetFieldWidth(40)
		am.templateField.SetListStyles(
			tcell.StyleDefault.Background(app.theme.HeaderBg).Foreground(app.theme.Foreground),
			tcell.StyleDefault.Background(app.theme.Accent).Foreground(app.theme.SelectionText),
		)
		am.form.AddFormItem(am.templateField)
	}

	am.form.AddTextArea(agentPromptLabel, "", 70, 5, 0, nil)
	if item := am.form.GetFormItemByLabel(agentPromptLabel); item != nil {
		if textArea, ok := item.(*tview.TextArea); ok {
			am.promptField = textArea
		}
	}

	am.form.AddButton("Run", func() {
		am.submitPrompt()
	})
	am.form.AddButton("Cancel", func() {
		am.Hide()
	})

	headerView := tview.NewTextView()
	headerView.SetText("Ask Agent")
	headerView.SetTextColor(app.theme.Accent)
	headerView.SetBackgroundColor(app.theme.HeaderBg)

	helpView := tview.NewTextView()
	helpView.SetText("Esc: cancel • Ctrl+Enter / Cmd+Enter: run • Template fills prompt • Workspace blank uses CWD • Includes title, description, comments")
	helpView.SetTextColor(app.theme.SecondaryText)
	helpView.SetBackgroundColor(app.theme.HeaderBg)
	helpView.SetTextAlign(tview.AlignCenter)

	am.modalContent = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(headerView, 1, 0, false).
		AddItem(am.form, 0, 1, true).
		AddItem(helpView, 1, 0, false)
	am.modalContent.Box = tview.NewBox().SetBackgroundColor(app.theme.HeaderBg)
	am.modalContent.SetBackgroundColor(app.theme.HeaderBg).
		SetBorder(true).
		SetBorderColor(app.theme.Accent).
		SetTitle(" Agent Prompt ").
		SetTitleColor(app.theme.Foreground)
	padding := app.density.ModalPadding
	am.modalContent.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)

	am.modalWidth = minPromptModalWidth
	am.modal = am.buildModal(am.modalWidth)

	return am
}

// Show displays the prompt modal.
func (am *AgentPromptModal) Show(onSubmit func(prompt string, workspace string)) {
	am.onSubmit = onSubmit
	defaultPrompt := ""
	if am.templateField != nil && len(am.templatePrompts) > 0 {
		am.templateField.SetCurrentOption(0)
		defaultPrompt = am.templatePrompts[0]
	}
	if am.promptField != nil {
		am.promptField.SetText(defaultPrompt, true)
	}
	if am.workspaceField != nil {
		defaultWorkspace := strings.TrimSpace(am.app.config.AgentWorkspace)
		if defaultWorkspace == "" {
			if cwd, err := os.Getwd(); err == nil {
				defaultWorkspace = cwd
			}
		}
		am.workspaceField.SetText(defaultWorkspace)
	}

	am.updateModalWidth()

	am.app.pages.AddPage("agent_prompt", am.modal, true, true)
	am.app.pages.SendToFront("agent_prompt")
	am.app.app.SetFocus(am.form)
}

// Hide hides the prompt modal.
func (am *AgentPromptModal) Hide() {
	am.app.pages.RemovePage("agent_prompt")
	am.app.updateFocus()
}

// HandleKey handles keyboard input for the prompt modal.
func (am *AgentPromptModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		am.Hide()
		return nil
	case tcell.KeyEnter:
		mod := event.Modifiers()
		if mod&tcell.ModCtrl != 0 || mod&tcell.ModMeta != 0 {
			am.submitPrompt()
			return nil
		}
	}
	return event
}

// submitPrompt validates and submits the prompt text.
func (am *AgentPromptModal) submitPrompt() {
	if am.promptField == nil {
		return
	}

	prompt := strings.TrimSpace(am.promptField.GetText())
	if prompt == "" {
		return
	}

	workspace := ""
	if am.workspaceField != nil {
		workspace = strings.TrimSpace(am.workspaceField.GetText())
	}

	am.Hide()
	if am.onSubmit != nil {
		am.onSubmit(prompt, workspace)
	}
}

// buildModal builds the centered modal container with the given width.
func (am *AgentPromptModal) buildModal(width int) *tview.Flex {
	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(am.modalContent, promptModalHeight, 0, true).
			AddItem(nil, 0, 1, false), width, 0, true).
		AddItem(nil, 0, 1, false)
	modal.SetBackgroundColor(am.app.theme.Background)
	return modal
}

// updateModalWidth adjusts the modal width to fit the workspace path.
func (am *AgentPromptModal) updateModalWidth() {
	workspace := ""
	if am.workspaceField != nil {
		workspace = strings.TrimSpace(am.workspaceField.GetText())
	}

	desiredWidth := minPromptModalWidth
	if workspace != "" {
		desiredWidth = len(workspace) + 20
		if desiredWidth < minPromptModalWidth {
			desiredWidth = minPromptModalWidth
		}
		if desiredWidth > maxPromptModalWidth {
			desiredWidth = maxPromptModalWidth
		}
	}

	if desiredWidth != am.modalWidth {
		am.modalWidth = desiredWidth
		am.modal = am.buildModal(am.modalWidth)
	}
}

// applyTemplatePrompt updates the prompt field from the selected template.
func (am *AgentPromptModal) applyTemplatePrompt(index int) {
	if am.promptField == nil {
		return
	}
	if index < 0 || index >= len(am.templatePrompts) {
		return
	}
	am.promptField.SetText(am.templatePrompts[index], true)
}
