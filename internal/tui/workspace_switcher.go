package tui

import (
	"fmt"

	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
)

func workspaceNameForKey(workspaces []config.Workspace, token string) string {
	if token == "" {
		return ""
	}
	for _, workspace := range workspaces {
		if workspace.APIKey() == token {
			return workspace.Name
		}
	}
	return ""
}

func workspacePickerItems(workspaces []config.Workspace, active string) []PickerItem {
	items := make([]PickerItem, 0, len(workspaces))
	for _, workspace := range workspaces {
		label := workspace.Name
		if workspace.Name == active {
			label += " (active)"
		}
		items = append(items, PickerItem{ID: workspace.Name, Label: label})
	}
	return items
}

func (a *App) showWorkspacePicker() {
	if len(a.config.Workspaces) == 0 {
		a.flashError("No workspaces configured — add a workspaces list to config.json")
		return
	}
	items := workspacePickerItems(a.config.Workspaces, a.activeWorkspaceName)
	a.pickerModal.Show("Switch Workspace", items, func(item PickerItem) {
		a.switchWorkspace(item.ID)
	})
}

func (a *App) switchWorkspace(name string) {
	if name == a.activeWorkspaceName {
		a.flashStatus(fmt.Sprintf("Already on %s", name))
		return
	}

	var workspace config.Workspace
	found := false
	for _, candidate := range a.config.Workspaces {
		if candidate.Name == name {
			workspace = candidate
			found = true
			break
		}
	}
	if !found {
		a.flashError(fmt.Sprintf("Unknown workspace %q", name))
		return
	}

	key := workspace.APIKey()
	if key == "" {
		a.flashError(fmt.Sprintf("%s is not set — cannot switch to %s", workspace.APIKeyEnv, workspace.Name))
		return
	}

	logger.Info("tui.workspace: switching workspace name=%s", workspace.Name)
	a.persistSession()
	a.clearComposeDrafts()
	a.activeWorkspaceName = workspace.Name
	a.markSessionWorkspace()
	a.apiUseBearer = false
	a.apiOnUnauthorized = nil
	a.config.LinearAPIKey = key
	a.reloadWorkspace()
	a.updateAllPaneTitles()
	a.flashSuccess(fmt.Sprintf("Switched to %s", workspace.Name))
}
