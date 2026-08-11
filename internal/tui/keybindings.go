package tui

import "github.com/zen-linear/zen-linear/internal/logger"

// uiActionIDs are the keybinding ids naming a UI action rather than a palette
// command. They resolve through actionKey, which is matched before the pane
// dispatch, so a binding on one reaches every pane.
// TestUIActionIDsCoverEveryActionKeyCallSite keeps this in step with the code.
var uiActionIDs = map[string]bool{
	"quit":               true,
	"open_palette":       true,
	"search":             true,
	"focus_navigation":   true,
	"focus_issues":       true,
	"focus_details":      true,
	"tab_next":           true,
	"tab_prev":           true,
	"columns_left":       true,
	"columns_right":      true,
	"favorite_move_up":   true,
	"favorite_move_down": true,
	"favorite_nest":      true,
	"favorite_unnest":    true,
}

// applyCommandKeybindings overrides palette command shortcut runes from the
// keybindings config, keyed by command id. Explicit mappings own their keys: a
// command whose default shortcut is claimed by a mapping loses it, but only
// when the two can be reached from the same pane. A navigation command and an
// issue command never answer together, so they can share a rune.
func applyCommandKeybindings(commands []Command, bindings map[string]string) {
	if len(bindings) == 0 {
		return
	}

	scopeByID := make(map[string]CommandScope, len(commands))
	for _, cmd := range commands {
		scopeByID[cmd.ID] = cmd.Scope
	}

	claimed := make(map[rune][]CommandScope, len(bindings))
	for id, key := range bindings {
		runes := []rune(key)
		if len(runes) != 1 {
			continue
		}
		scope, isCommand := scopeByID[id]
		if !isCommand {
			// An id naming neither a command nor an action binds nothing: a
			// typo, or a command a release removed. Letting it claim the rune
			// would take that key from a command that still exists, and the
			// user would have one dead key explained by another.
			if !uiActionIDs[id] {
				logger.Warning("tui.keybindings: binding for unknown id, ignored id=%s key=%s", id, key)
				continue
			}
			// An action is matched before the pane dispatch, so it reaches
			// every pane and collides with anything.
			scope = ScopeGlobal
		}
		claimed[runes[0]] = append(claimed[runes[0]], scope)
	}

	for i := range commands {
		if key, ok := bindings[commands[i].ID]; ok {
			if runes := []rune(key); len(runes) == 1 {
				commands[i].ShortcutRune = runes[0]
				commands[i].ShortcutDisplay = ""
			}
			continue
		}
		for _, scope := range claimed[commands[i].ShortcutRune] {
			if scopesOverlap(scope, commands[i].Scope) {
				commands[i].ShortcutRune = 0
				break
			}
		}
	}
}

// scopesOverlap reports whether two scopes share a pane, which is what makes
// one rune held by both a collision.
func scopesOverlap(a, b CommandScope) bool {
	return a == ScopeGlobal || b == ScopeGlobal || a == b
}

// commandBoundTo reports whether the keybindings config names a palette command
// on this rune. An explicit binding outranks a default action key: the user
// asked for the command by id, the action holds the rune only because nothing
// else claimed it.
func (a *App) commandBoundTo(r rune) bool {
	if a.paletteCtrl == nil || len(a.config.Keybindings) == 0 {
		return false
	}
	for _, cmd := range a.paletteCtrl.commands {
		key, ok := a.config.Keybindings[cmd.ID]
		if !ok {
			continue
		}
		if runes := []rune(key); len(runes) == 1 && runes[0] == r {
			return true
		}
	}
	return false
}

// isMovementRune reports whether the rune belongs to a widget's own scrolling.
// A keybinding never takes one: losing j to a command would strand the cursor.
func isMovementRune(r rune) bool {
	switch r {
	case 'h', 'j', 'k', 'l', 'g', 'G':
		return true
	}
	return false
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
