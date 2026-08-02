package tui

import (
	"context"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

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

// TestToggleFavoriteHonoursKeybindingOverride verifies the favorite command is
// remappable like any other palette command.
func TestToggleFavoriteHonoursKeybindingOverride(t *testing.T) {
	commands := DefaultCommands(nil)
	applyCommandKeybindings(commands, map[string]string{"toggle_favorite": "F"})

	for _, cmd := range commands {
		if cmd.ID != "toggle_favorite" {
			continue
		}
		if cmd.ShortcutRune != 'F' {
			t.Fatalf("toggle_favorite shortcut = %q, want F", cmd.ShortcutRune)
		}
		return
	}
	t.Fatal("toggle_favorite command not registered")
}

// reorderHarness drives the reorder keys and waits for the refresh to land, so
// the test never reads the tree while the mutation goroutine is writing it.
type reorderHarness struct {
	app     *App
	settled chan struct{}
	moved   []string
}

// newReorderHarness returns an app with two favorites and the cursor on the
// second, so a move up has somewhere to go.
func newReorderHarness(t *testing.T) *reorderHarness {
	t.Helper()
	h := &reorderHarness{app: newUXTestApp(), settled: make(chan struct{}, 1)}
	h.app.updateFavoriteSortFunc = func(_ context.Context, favoriteID string, _ float64) error {
		h.moved = append(h.moved, favoriteID)
		return nil
	}
	h.app.favoritesChanged = func() { h.settled <- struct{}{} }
	h.app.rebuildNavigationTree(nil, []linearapi.Favorite{
		{ID: "fav-a", Type: "project", ProjectID: "p1", ProjectName: "Alpha", SortOrder: 10},
		{ID: "fav-b", Type: "project", ProjectID: "p2", ProjectName: "Beta", SortOrder: 20},
	})
	h.app.navigationTree.SetCurrentNode(h.app.favoritesGroup.GetChildren()[1])
	return h
}

func (h *reorderHarness) press(r rune) *tcell.EventKey {
	return h.app.handleNavigationKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
}

// waitSettled blocks until the reorder goroutine has finished writing.
func (h *reorderHarness) waitSettled(t *testing.T) {
	t.Helper()
	select {
	case <-h.settled:
	case <-time.After(2 * time.Second):
		t.Fatal("reorder did not settle")
	}
}

// TestFavoriteMoveKeysResolveThroughActionKey verifies the reorder keys follow
// the keybindings config instead of the hardcoded defaults.
func TestFavoriteMoveKeysResolveThroughActionKey(t *testing.T) {
	h := newReorderHarness(t)
	h.app.config.Keybindings = map[string]string{
		"favorite_move_up":   "U",
		"favorite_move_down": "D",
	}

	if h.press('U') != nil {
		t.Fatal("remapped favorite_move_up was not handled")
	}
	h.waitSettled(t)

	// The default stops applying once remapped, so it falls through.
	if h.press('K') == nil {
		t.Error("default K still handled after the remap")
	}
}

// TestFavoriteMoveKeysDefaultToShiftJK verifies the unconfigured defaults.
func TestFavoriteMoveKeysDefaultToShiftJK(t *testing.T) {
	h := newReorderHarness(t)

	if h.press('K') != nil {
		t.Fatal("default K was not handled")
	}
	h.waitSettled(t)

	if len(h.moved) != 2 || h.moved[0] != "fav-b" {
		t.Errorf("reorder writes = %v, want fav-b first", h.moved)
	}
}

// TestFavoriteMoveKeysFallThroughOnNonFavorites verifies the keys stay with the
// tree when the cursor is not on a favorite, rather than raising an error.
func TestFavoriteMoveKeysFallThroughOnNonFavorites(t *testing.T) {
	app := newUXTestApp()
	app.updateFavoriteSortFunc = func(context.Context, string, float64) error {
		t.Error("reorder must not reach the API for a non-favorite")
		return nil
	}
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)
	app.navigationTree.SetCurrentNode(app.navigationTree.GetRoot().GetChildren()[1])

	for _, r := range []rune{'J', 'K'} {
		if app.handleNavigationKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)) == nil {
			t.Errorf("%q was consumed on a team node, want fall-through", r)
		}
	}
}
