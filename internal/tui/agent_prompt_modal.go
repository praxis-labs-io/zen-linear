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
	fm              *FormModal
	templateField   *tview.DropDown
	templateLabels  []string
	templatePrompts []string
	promptField     *tview.TextArea
	workspaceField  *tview.InputField
	onSubmit        func(prompt string, workspace string)
}

const agentPromptModalWidth = 90

// NewAgentPromptModal creates a new agent prompt modal.
func NewAgentPromptModal(app *App) *AgentPromptModal {
	am := &AgentPromptModal{app: app}
	am.fm = NewFormModal(app, "Ask Agent")
	am.fm.SetMaxWidth(agentPromptModalWidth)

	am.workspaceField = am.fm.AddInput("Workspace", "")

	if len(app.agentPromptTemplates) > 0 {
		labels := make([]string, 0, len(app.agentPromptTemplates))
		prompts := make([]string, 0, len(app.agentPromptTemplates))
		for _, template := range app.agentPromptTemplates {
			labels = append(labels, template.Name)
			prompts = append(prompts, template.Prompt)
		}
		am.templateLabels = labels
		am.templatePrompts = prompts
		am.templateField = am.fm.AddPicker("Template", am.templateLabels, 0, func(_ string, index int) {
			am.applyTemplatePrompt(index)
		})
	}

	am.promptField = am.fm.AddTextArea("Prompt (issue context included)", "", 5)

	am.fm.AddButtons(
		FormButton{Label: "Run", OnPress: am.submitPrompt},
		FormButton{Label: "Cancel", OnPress: am.Hide},
	)
	am.fm.SetOnSubmit(am.submitPrompt)
	am.fm.SetOnCancel(am.Hide)
	am.fm.SetHint("Esc cancel · ⌃⏎ run · template fills prompt · blank workspace uses CWD")

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

	am.fm.Show("agent_prompt")
}

// Hide hides the prompt modal.
func (am *AgentPromptModal) Hide() {
	am.fm.Hide("agent_prompt")
}

// HandleKey handles keyboard input for the prompt modal.
func (am *AgentPromptModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	return am.fm.HandleKey(event)
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
