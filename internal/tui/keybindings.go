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

// actionKey returns the configured key for a UI action id, or the fallback.
func (a *App) actionKey(action string, fallback rune) rune {
	if key, ok := a.config.Keybindings[action]; ok {
		if runes := []rune(key); len(runes) == 1 {
			return runes[0]
		}
	}
	return fallback
}
