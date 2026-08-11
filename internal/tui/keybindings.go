package tui

import "github.com/zen-linear/zen-linear/internal/logger"

// uiActionScopes are the keybinding ids naming a UI action rather than a
// palette command, each with the panes it answers from. An action collides with
// a command the same way two commands do, so the scope has to be as honest here
// as it is on a Command: the favorite keys reach the navigation tree only, and
// the tab and column keys the issue panes only.
// TestUIActionScopesCoverEveryActionKeyCallSite keeps this in step with the code.
var uiActionScopes = map[string]CommandScope{
	"quit":               ScopeGlobal,
	"open_palette":       ScopeGlobal,
	"search":             ScopeGlobal,
	"focus_navigation":   ScopeGlobal,
	"focus_issues":       ScopeGlobal,
	"focus_details":      ScopeGlobal,
	"tab_next":           ScopeIssue,
	"tab_prev":           ScopeIssue,
	"columns_left":       ScopeIssue,
	"columns_right":      ScopeIssue,
	"favorite_move_up":   ScopeNavigation,
	"favorite_move_down": ScopeNavigation,
	"favorite_nest":      ScopeNavigation,
	"favorite_unnest":    ScopeNavigation,
}

// bindingRune returns the rune a keybinding value resolves to, and whether it
// resolves at all. This is the one place a binding turns into a key, so it is
// where the two rules that reject one live: a value has to be a single rune,
// and a movement rune belongs to the widget under the cursor. actionKey applies
// the same test, so neither a command nor an action can take one.
func bindingRune(key string) (rune, bool) {
	runes := []rune(key)
	if len(runes) != 1 || isMovementRune(runes[0]) {
		return 0, false
	}
	return runes[0], true
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
		r, ok := bindingRune(key)
		if !ok {
			continue
		}
		scope, isCommand := scopeByID[id]
		if !isCommand {
			// An id naming neither a command nor an action binds nothing: a
			// typo, or a command a release removed. Letting it claim the rune
			// would take that key from a command that still exists, and the
			// user would have one dead key explained by another.
			scope, isCommand = uiActionScopes[id]
			if !isCommand {
				continue
			}
		}
		claimed[r] = append(claimed[r], scope)
	}

	for i := range commands {
		if key, ok := bindings[commands[i].ID]; ok {
			if r, valid := bindingRune(key); valid {
				commands[i].ShortcutRune = r
				commands[i].ShortcutDisplay = ""
				continue
			}
			// A rejected binding leaves the default in place rather than
			// stranding the command with no key at all.
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

// warnRejectedKeybindings logs the bindings that resolve to nothing, so a
// config that quietly does less than it says has one place that admits it.
// knownCommands is every command this build registers, including any the
// session filtered out, or a binding for one would read as a typo.
func warnRejectedKeybindings(bindings map[string]string, knownCommands map[string]bool) {
	for id, key := range bindings {
		if _, ok := bindingRune(key); !ok {
			logger.Warning("tui.keybindings: binding is not a usable key, ignored id=%s key=%s", id, key)
			continue
		}
		if knownCommands[id] {
			continue
		}
		if _, isAction := uiActionScopes[id]; !isAction {
			logger.Warning("tui.keybindings: binding for unknown id, ignored id=%s key=%s", id, key)
		}
	}
}

// scopesOverlap reports whether two scopes share a pane, which is what makes
// one rune held by both a collision.
func scopesOverlap(a, b CommandScope) bool {
	return a == ScopeGlobal || b == ScopeGlobal || a == b
}

// commandBoundTo reports whether a command answering to this rune got it from
// the config by name. An explicit binding outranks a default action key: the
// user asked for the command by id, the action holds the rune only because
// nothing else claimed it. Reading the resolved shortcut rather than the raw
// config means a binding applyCommandKeybindings rejected is not honored here.
func (a *App) commandBoundTo(r rune) bool {
	if a.paletteCtrl == nil || len(a.config.Keybindings) == 0 {
		return false
	}
	for _, cmd := range a.paletteCtrl.commands {
		if cmd.ShortcutRune != r {
			continue
		}
		if _, bound := a.config.Keybindings[cmd.ID]; bound {
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

// actionKey returns the configured key for a UI action id, or the fallback when
// the config names none or names one bindingRune rejects.
func (a *App) actionKey(action string, fallback rune) rune {
	if key, ok := a.config.Keybindings[action]; ok {
		if r, valid := bindingRune(key); valid {
			return r
		}
	}
	return fallback
}
