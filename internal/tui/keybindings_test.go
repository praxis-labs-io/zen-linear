package tui

import "testing"

// TestApplyCommandKeybindings verifies overrides apply and claimed default
// shortcuts are cleared.
func TestApplyCommandKeybindings(t *testing.T) {
	commands := []Command{
		{ID: "copy_id", ShortcutRune: 'y'},
		{ID: "copy_url", ShortcutRune: 'w'},
		{ID: "set_parent", ShortcutRune: 'i'},
	}
	applyCommandKeybindings(commands, map[string]string{
		"copy_url": "y",
		"copy_id":  "i",
	})
	if commands[1].ShortcutRune != 'y' {
		t.Errorf("copy_url = %q, want y", commands[1].ShortcutRune)
	}
	if commands[0].ShortcutRune != 'i' {
		t.Errorf("copy_id = %q, want i", commands[0].ShortcutRune)
	}
	if commands[2].ShortcutRune != 0 {
		t.Errorf("set_parent kept %q, want cleared (key claimed)", commands[2].ShortcutRune)
	}
}
