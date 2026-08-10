package tui

// applyCommandKeybindings overrides palette command shortcut runes from the
// keybindings config, keyed by command id. Explicit mappings own their keys:
// a command whose default shortcut is claimed by a mapping loses it.
func applyCommandKeybindings(commands []Command, bindings map[string]string) {
	if len(bindings) == 0 {
		return
	}

	claimed := make(map[rune]string, len(bindings))
	for id, key := range bindings {
		if runes := []rune(key); len(runes) == 1 {
			claimed[runes[0]] = id
		}
	}

	for i := range commands {
		if key, ok := bindings[commands[i].ID]; ok {
			if runes := []rune(key); len(runes) == 1 {
				commands[i].ShortcutRune = runes[0]
				commands[i].ShortcutDisplay = ""
			}
			continue
		}
		if owner, taken := claimed[commands[i].ShortcutRune]; taken && owner != commands[i].ID {
			commands[i].ShortcutRune = 0
		}
	}
}

// commandShortcutLabel returns the key a palette command answers to, for help
// text that would otherwise state a default the user has remapped. It renders
// the key the same way the palette does. A command left without one has no key
// to name, so the caller is told to leave it out rather than print a stand-in.
func (a *App) commandShortcutLabel(id string) (label string, ok bool) {
	if a.paletteCtrl == nil {
		return "", false
	}
	for _, cmd := range a.paletteCtrl.commands {
		if cmd.ID != id {
			continue
		}
		if cmd.ShortcutDisplay != "" {
			return cmd.ShortcutDisplay, true
		}
		if shortcut := FormatShortcut(cmd.ShortcutRune); shortcut != "" {
			return shortcut, true
		}
		return "", false
	}
	return "", false
}

// actionKey returns the configured key for a UI action id, or the fallback.

func (a *App) actionKey(action string, fallback rune) rune {
	if key, ok := a.config.Keybindings[action]; ok {
		if runes := []rune(key); len(runes) == 1 {
			return runes[0]
		}
	}
	return fallback
}
