package tui

import (
	"fmt"

	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
)

// workspaceNameForKey returns the name of the workspace whose API key matches
// the given token, or "" when none matches (explicit key or OAuth session).
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

// workspacePickerItems builds picker entries, marking the active workspace.
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

// showWorkspacePicker opens the workspace selection modal.
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

// switchWorkspace swaps the API client to the named workspace's key and
// reloads all data through the settings-apply path.
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
	// The switch clears every field the snapshot reads, so the outgoing
	// workspace's place has to go to disk before applySettings runs.
	a.persistSession()
	// Unposted comments belong to issues this workspace is leaving behind.
	// Dropped here rather than in resetCachedState, which a settings save also
	// runs: saving settings is no reason to lose what someone has written.
	a.clearComposeDrafts()
	a.activeWorkspaceName = workspace.Name
	a.markSessionWorkspace()
	// A workspace key is a personal API key, not an OAuth token, so drop any
	// bearer scheme and 401 refresh carried from an OAuth session first.
	a.apiUseBearer = false
	a.apiOnUnauthorized = nil
	newCfg := a.config
	newCfg.LinearAPIKey = key
	a.applySettings(newCfg)
	// The navigation pane's title is the workspace name.
	a.updateAllPaneTitles()
	a.flashSuccess(fmt.Sprintf("Switched to %s", workspace.Name))
}
