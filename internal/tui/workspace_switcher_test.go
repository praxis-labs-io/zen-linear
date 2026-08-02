package tui

import (
	"testing"

	"github.com/zen-linear/zen-linear/internal/config"
)

func switcherTestWorkspaces() []config.Workspace {
	return []config.Workspace{
		{Name: "Acme", APIKeyEnv: "TEST_LINEAR_KEY_ACME"},
		{Name: "Side", APIKeyEnv: "TEST_LINEAR_KEY_SIDE"},
	}
}

// TestWorkspaceNameForKey verifies the active workspace is identified from the
// resolved token, and that OAuth/explicit tokens map to no workspace.
func TestWorkspaceNameForKey(t *testing.T) {
	t.Setenv("TEST_LINEAR_KEY_ACME", "k-acme")
	t.Setenv("TEST_LINEAR_KEY_SIDE", "k-side")
	workspaces := switcherTestWorkspaces()

	if got := workspaceNameForKey(workspaces, "k-side"); got != "Side" {
		t.Errorf("workspaceNameForKey(k-side) = %q, want Side", got)
	}
	if got := workspaceNameForKey(workspaces, "some-oauth-value"); got != "" {
		t.Errorf("workspaceNameForKey(unmatched) = %q, want empty", got)
	}
	if got := workspaceNameForKey(workspaces, ""); got != "" {
		t.Errorf("workspaceNameForKey(empty) = %q, want empty", got)
	}
}

// TestWorkspacePickerItemsMarksActive verifies picker labels flag the active
// workspace.
func TestWorkspacePickerItemsMarksActive(t *testing.T) {
	items := workspacePickerItems(switcherTestWorkspaces(), "Side")
	if len(items) != 2 {
		t.Fatalf("workspacePickerItems() returned %d items, want 2", len(items))
	}
	if items[0].Label != "Acme" || items[0].ID != "Acme" {
		t.Errorf("items[0] = %+v, want plain Acme entry", items[0])
	}
	if items[1].Label != "Side (active)" || items[1].ID != "Side" {
		t.Errorf("items[1] = %+v, want Side marked active", items[1])
	}
}

// TestSwitchWorkspaceMissingKeyKeepsCurrent verifies switching to a workspace
// whose env var is unset fails without touching the active workspace.
func TestSwitchWorkspaceMissingKeyKeepsCurrent(t *testing.T) {
	t.Setenv("TEST_LINEAR_KEY_ACME", "k-acme")
	// TEST_LINEAR_KEY_SIDE deliberately not set.

	cfg := config.Config{Workspaces: switcherTestWorkspaces()}
	cfg.LinearAPIKey = "k-acme"
	app := newDefaultNavTestApp(cfg)
	app.activeWorkspaceName = "Acme"

	app.switchWorkspace("Side")

	if app.activeWorkspaceName != "Acme" {
		t.Errorf("activeWorkspaceName = %q after failed switch, want Acme", app.activeWorkspaceName)
	}
	if app.config.LinearAPIKey != "k-acme" {
		t.Errorf("LinearAPIKey changed after failed switch")
	}
}

// TestSwitchWorkspaceUnknownNameKeepsCurrent verifies an unknown workspace
// name is rejected.
func TestSwitchWorkspaceUnknownNameKeepsCurrent(t *testing.T) {
	app := newDefaultNavTestApp(config.Config{Workspaces: switcherTestWorkspaces()})
	app.activeWorkspaceName = "Acme"

	app.switchWorkspace("Nonexistent")

	if app.activeWorkspaceName != "Acme" {
		t.Errorf("activeWorkspaceName = %q after unknown switch, want Acme", app.activeWorkspaceName)
	}
}
