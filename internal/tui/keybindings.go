package tui

import "github.com/praxis-labs-io/zen-linear/internal/logger"

var uiActionScopes = map[string]CommandScope{
	"quit":               ScopeGlobal,
	"open_palette":       ScopeGlobal,
	"search":             ScopeGlobal,
	"focus_navigation":   ScopeGlobal,
	"focus_issues":       ScopeGlobal,
	"focus_details":      ScopeGlobal,
	"comment_next":       ScopeIssue,
	"comment_prev":       ScopeIssue,
	"columns_left":       ScopeIssue,
	"columns_right":      ScopeIssue,
	"favorite_move_up":   ScopeNavigation,
	"favorite_move_down": ScopeNavigation,
	"comment_reply":      ScopeComment,
	"comment_quote":      ScopeComment,
	"comment_copy_link":  ScopeComment,
	"comment_open":       ScopeComment,
	"comment_edit":       ScopeComment,
	"comment_delete":     ScopeComment,
}

func bindingRune(key string) (rune, bool) {
	runes := []rune(key)
	if len(runes) != 1 || isMovementRune(runes[0]) {
		return 0, false
	}
	return runes[0], true
}

type keyClaim struct {
	id    string
	scope CommandScope
}

type resolvedKeybindings struct {
	byID   map[string]rune
	claims map[rune][]keyClaim
}

func commandScopes(commands []Command) map[string]CommandScope {
	scopes := make(map[string]CommandScope, len(commands))
	for _, cmd := range commands {
		scopes[cmd.ID] = cmd.Scope
	}
	return scopes
}

func resolveKeybindings(bindings map[string]string, scopes map[string]CommandScope) *resolvedKeybindings {
	resolved := &resolvedKeybindings{
		byID:   make(map[string]rune, len(bindings)),
		claims: make(map[rune][]keyClaim, len(bindings)),
	}
	for id, key := range bindings {
		r, ok := bindingRune(key)
		if !ok {
			logger.Warning("tui.keybindings: binding is not a usable key, ignored id=%s key=%s", id, key)
			continue
		}
		scope, known := scopes[id]
		if !known {
			scope, known = uiActionScopes[id]
		}
		if !known {
			logger.Warning("tui.keybindings: binding for unknown id, ignored id=%s key=%s", id, key)
			continue
		}
		resolved.byID[id] = r
		resolved.claims[r] = append(resolved.claims[r], keyClaim{id: id, scope: scope})
	}
	return resolved
}

func (a *App) rebuildCommands() {
	commands := DefaultCommands(a)
	if a.paletteCtrl == nil {
		a.paletteCtrl = NewPaletteController(commands)
		return
	}
	a.paletteCtrl.SetCommands(commands)
}

func (r *resolvedKeybindings) key(id string) (rune, bool) {
	if r == nil {
		return 0, false
	}
	k, ok := r.byID[id]
	return k, ok
}

func (r *resolvedKeybindings) takenFrom(id string, key rune, scope CommandScope) bool {
	if r == nil || key == 0 {
		return false
	}
	for _, claim := range r.claims[key] {
		if claim.id != id && scopesOverlap(claim.scope, scope) {
			return true
		}
	}
	return false
}

func applyCommandKeybindings(commands []Command, resolved *resolvedKeybindings) {
	for i := range commands {
		if key, ok := resolved.key(commands[i].ID); ok {
			commands[i].ShortcutRune = key
			commands[i].ShortcutDisplay = ""
			continue
		}
		if resolved.takenFrom(commands[i].ID, commands[i].ShortcutRune, commands[i].Scope) {
			commands[i].ShortcutRune = 0
		}
	}
}

func scopesOverlap(a, b CommandScope) bool {
	return a == ScopeGlobal || b == ScopeGlobal || a == b
}

func (a *App) commandBoundTo(r rune) bool {
	if a.paletteCtrl == nil {
		return false
	}
	for _, cmd := range a.paletteCtrl.commands {
		if cmd.ShortcutRune != r {
			continue
		}
		if _, bound := a.bindings.key(cmd.ID); bound {
			return true
		}
	}
	return false
}

func isMovementRune(r rune) bool {
	switch r {
	case 'h', 'j', 'k', 'l', 'g', 'G':
		return true
	}
	return false
}

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

func (a *App) commandShortcutRune(id string) (rune, bool) {
	if a.paletteCtrl == nil {
		return 0, false
	}
	for _, cmd := range a.paletteCtrl.commands {
		if cmd.ID == id {
			return cmd.ShortcutRune, cmd.ShortcutRune != 0
		}
	}
	return 0, false
}

// Returns 0 when another id took the fallback, which no key event carries, so the claimant answers instead.
func (a *App) actionKey(action string, fallback rune) rune {
	if key, ok := a.bindings.key(action); ok {
		return key
	}
	if a.bindings.takenFrom(action, fallback, uiActionScopes[action]) {
		return 0
	}
	return fallback
}
