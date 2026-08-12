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
// resolves at all. A value has to be a single rune, and a movement rune belongs
// to the widget under the cursor.
func bindingRune(key string) (rune, bool) {
	runes := []rune(key)
	if len(runes) != 1 || isMovementRune(runes[0]) {
		return 0, false
	}
	return runes[0], true
}

// keyClaim is an id the user named on a rune, and the panes it answers from.
type keyClaim struct {
	id    string
	scope CommandScope
}

// resolvedKeybindings is the config's keybindings after validation. A binding
// that cannot become a key is absent rather than partly applied: half-honoring
// one is how a rejected entry used to keep a command's default alive at one
// call site while taking a rune at another.
type resolvedKeybindings struct {
	byID   map[string]rune
	claims map[rune][]keyClaim
}

// commandScopes indexes a registry by id, including commands a session filters
// out. A binding for one of those names a real command, not a typo.
func commandScopes(commands []Command) map[string]CommandScope {
	scopes := make(map[string]CommandScope, len(commands))
	for _, cmd := range commands {
		scopes[cmd.ID] = cmd.Scope
	}
	return scopes
}

// resolveKeybindings drops every binding that cannot become a key, saying so in
// the log, and indexes what survives by id and by rune.
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
			// A typo, or a command a release removed. Letting it claim the
			// rune would take that key from something that still exists, and
			// the user would have one dead key explained by another.
			logger.Warning("tui.keybindings: binding for unknown id, ignored id=%s key=%s", id, key)
			continue
		}
		resolved.byID[id] = r
		resolved.claims[r] = append(resolved.claims[r], keyClaim{id: id, scope: scope})
	}
	return resolved
}

// rebuildCommands rebuilds the palette registry against the current config,
// which is what re-resolves App.bindings. Both a command's shortcut and a UI
// action's key are read from that resolved set, so nothing in the keyboard
// follows a config change until this runs.
func (a *App) rebuildCommands() {
	commands := DefaultCommands(a)
	if a.paletteCtrl == nil {
		a.paletteCtrl = NewPaletteController(commands)
		return
	}
	a.paletteCtrl.SetCommands(commands)
}

// key returns the rune the config bound to an id, when it named one that
// resolved.
func (r *resolvedKeybindings) key(id string) (rune, bool) {
	if r == nil {
		return 0, false
	}
	k, ok := r.byID[id]
	return k, ok
}

// takenFrom reports whether some other id was explicitly bound to this rune
// somewhere the caller's own scope reaches. A default gives way to a binding
// the user wrote by name; two scopes that never answer together do not collide.
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

// applyCommandKeybindings moves palette command shortcuts onto the runes the
// config named. A command whose default is claimed by a binding loses it, but
// only when the two can be reached from the same pane.
func applyCommandKeybindings(commands []Command, resolved *resolvedKeybindings) {
	for i := range commands {
		if key, ok := resolved.key(commands[i].ID); ok {
			commands[i].ShortcutRune = key
			commands[i].ShortcutDisplay = ""
			continue
		}
		// Reached by commands the config never named and by ones whose binding
		// was rejected: both keep their default only if nothing else took it.
		if resolved.takenFrom(commands[i].ID, commands[i].ShortcutRune, commands[i].Scope) {
			commands[i].ShortcutRune = 0
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
// nothing else claimed it. Both halves read the resolved set, so a rejected
// binding neither names a command here nor keeps a rune anywhere else.
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

// isMovementRune reports whether the rune is reserved for moving: j, k, g and
// G walk a list, h and l step between panes. A keybinding never takes one, or
// the cursor loses a way to go.
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

// actionKey returns the key a UI action answers to: the one the config named,
// or its fallback unless something else was bound to that rune. It returns 0
// when the fallback is taken, which no key event carries, so the action's case
// stops matching and the id that claimed the rune gets it. Without that, an
// action earlier in handleGlobalKey's switch keeps a rune the user moved and
// the binding is silently dead.
func (a *App) actionKey(action string, fallback rune) rune {
	if key, ok := a.bindings.key(action); ok {
		return key
	}
	if a.bindings.takenFrom(action, fallback, uiActionScopes[action]) {
		return 0
	}
	return fallback
}
