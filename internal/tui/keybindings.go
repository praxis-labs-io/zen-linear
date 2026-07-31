package tui

// applyCommandKeybindings overrides palette command shortcut runes from the
// keybindings config, keyed by command id.
func applyCommandKeybindings(commands []Command, bindings map[string]string) {
	if len(bindings) == 0 {
		return
	}
	for i := range commands {
		key, ok := bindings[commands[i].ID]
		if !ok {
			continue
		}
		if runes := []rune(key); len(runes) == 1 {
			commands[i].ShortcutRune = runes[0]
			commands[i].ShortcutDisplay = ""
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
