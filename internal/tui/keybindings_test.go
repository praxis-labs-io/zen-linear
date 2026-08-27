package tui

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

// applyBindings resolves a config's keybindings against the given registry and
// applies them, which is the pair DefaultCommands runs.
func applyBindings(commands []Command, bindings map[string]string) {
	applyCommandKeybindings(commands, resolveKeybindings(bindings, commandScopes(commands)))
}

// TestApplyCommandKeybindings verifies overrides apply and claimed default
// shortcuts are cleared.
func TestApplyCommandKeybindings(t *testing.T) {
	commands := []Command{
		{ID: "copy_id", ShortcutRune: 'y'},
		{ID: "copy_url", ShortcutRune: 'w'},
		{ID: "set_parent", ShortcutRune: 'i'},
	}
	applyBindings(commands, map[string]string{
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

// TestKeybindingStealSparesACommandInAnotherScope verifies the steal reads
// scope. Two commands that never answer from the same pane can hold one rune.
func TestKeybindingStealSparesACommandInAnotherScope(t *testing.T) {
	commands := []Command{
		{ID: "toggle_favorite", Scope: ScopeNavigation, ShortcutRune: 'f'},
		{ID: "archive", Scope: ScopeIssue, ShortcutRune: 'x'},
		{ID: "refresh", ShortcutRune: 'r'},
	}
	applyBindings(commands, map[string]string{"toggle_favorite": "x"})

	if commands[0].ShortcutRune != 'x' {
		t.Errorf("toggle_favorite = %q, want x", commands[0].ShortcutRune)
	}
	if commands[1].ShortcutRune != 'x' {
		t.Errorf("archive lost x to a navigation command, want it kept")
	}
	if commands[2].ShortcutRune != 'r' {
		t.Errorf("refresh = %q, want r", commands[2].ShortcutRune)
	}
}

// TestKeybindingStealTakesTheRuneFromAnOverlappingScope verifies the other
// half: a global command reaches every pane, so it does collide.
func TestKeybindingStealTakesTheRuneFromAnOverlappingScope(t *testing.T) {
	commands := []Command{
		{ID: "refresh", ShortcutRune: 'r'},
		{ID: "archive", Scope: ScopeIssue, ShortcutRune: 'x'},
	}
	applyBindings(commands, map[string]string{"refresh": "x"})

	if commands[1].ShortcutRune != 0 {
		t.Errorf("archive kept %q, want cleared by the global command", commands[1].ShortcutRune)
	}
}

// TestBindingForAnUnknownIDTakesNoRune covers the upgrade case. A config
// naming a command a release removed used to leave two dead keys: the one it
// named, and the one it quietly took from a command that still exists.
func TestBindingForAnUnknownIDTakesNoRune(t *testing.T) {
	commands := []Command{
		{ID: "edit_issue", Scope: ScopeIssue, ShortcutRune: 'e'},
		{ID: "refresh", ShortcutRune: 'r'},
	}
	applyBindings(commands, map[string]string{"toggle_expand_all": "e"})

	if commands[0].ShortcutRune != 'e' {
		t.Errorf("edit_issue lost e to a command that no longer exists, want it kept")
	}
	if commands[1].ShortcutRune != 'r' {
		t.Errorf("refresh = %q, want r", commands[1].ShortcutRune)
	}
}

// TestBindingForAUIActionTakesTheRune pins the other half. An action is matched
// before the pane dispatch, so the command it shadows must stop advertising the
// key rather than print one that does nothing.
func TestBindingForAUIActionTakesTheRune(t *testing.T) {
	commands := []Command{{ID: "edit_issue", Scope: ScopeIssue, ShortcutRune: 'e'}}
	applyBindings(commands, map[string]string{"quit": "e"})

	if commands[0].ShortcutRune != 0 {
		t.Errorf("edit_issue kept %q, want it cleared by the action binding", commands[0].ShortcutRune)
	}
}

// TestActionStealRespectsItsScope covers an action that answers from one pane.
// The favorite keys reach the navigation tree only, so binding one to an issue
// command's rune takes nothing: the two can never both answer.
func TestActionStealRespectsItsScope(t *testing.T) {
	commands := []Command{{ID: "archive", Scope: ScopeIssue, ShortcutRune: 'x'}}
	applyBindings(commands, map[string]string{"favorite_move_up": "x"})

	if commands[0].ShortcutRune != 'x' {
		t.Errorf("archive lost x to a navigation action, want it kept")
	}
}

// TestARejectedBindingLeavesItsRuneToBeClaimed covers the half-applied case. A
// rejected binding used to keep its command's default and skip the claim check,
// so a second command explicitly bound to that rune collided with it and lost
// on registry order.
func TestARejectedBindingLeavesItsRuneToBeClaimed(t *testing.T) {
	commands := []Command{
		{ID: "archive", Scope: ScopeIssue, ShortcutRune: 'x'},
		{ID: "edit_issue", Scope: ScopeIssue, ShortcutRune: 'e'},
	}
	// The first is rejected as a movement rune, the second claims archive's
	// default.
	applyBindings(commands, map[string]string{"archive": "j", "edit_issue": "x"})

	if commands[0].ShortcutRune != 0 {
		t.Errorf("archive kept %q, want it given up to the command bound to x", commands[0].ShortcutRune)
	}
	if commands[1].ShortcutRune != 'x' {
		t.Errorf("edit_issue = %q, want x", commands[1].ShortcutRune)
	}
}

// TestARejectedBindingDoesNotOutrankAnAction covers the same half-applied
// binding reaching the dispatcher: its id was still in the config, so
// commandBoundTo counted it and the command's untouched default beat the action
// the user did bind to that rune.
func TestARejectedBindingDoesNotOutrankAnAction(t *testing.T) {
	// zoom_details keeps its default v because ctrl+v is not a single rune.
	app := bindingApp(t, map[string]string{"zoom_details": "ctrl+v", "focus_details": "v"})
	app.detailsHidden = false
	app.focusedPane = FocusIssues

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, 'v', tcell.ModNone))

	if app.focusedPane != FocusDetails {
		t.Errorf("v left focus on %v, want the action the user bound to it", app.focusedPane)
	}
	if app.detailsZoomed {
		t.Error("v zoomed, so a rejected binding still outranked the action")
	}
}

// TestAnActionBindingBeatsAnotherActionsDefault covers the last silent-dead
// case: the switch takes the first matching case, so an action holding a rune
// by default swallowed the one the user moved onto it.
func TestAnActionBindingBeatsAnotherActionsDefault(t *testing.T) {
	// search holds / by default; the user asks for focus_details there.
	app := bindingApp(t, map[string]string{"focus_details": "/"})
	app.detailsHidden = false
	app.focusedPane = FocusIssues
	section := app.activeIssuesSection

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, '/', tcell.ModNone))

	if app.focusedPane != FocusDetails {
		t.Errorf("/ left focus on %v, want the details pane", app.focusedPane)
	}
	if app.activeIssuesSection != section {
		t.Error("/ opened the Search tab, so the default still owned the rune")
	}
}

// TestUIActionScopesCoverEveryActionKeyCallSite keeps the action list in step
// with the handlers. An id missing from it is treated as unknown, so a binding
// on it would be dropped instead of taking the key the action answers to.
func TestUIActionScopesCoverEveryActionKeyCallSite(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("globbing the package sources: %v", err)
	}
	callSite := regexp.MustCompile(`actionKey\("([a-z_]+)"`)

	found := 0
	for _, source := range sources {
		body, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("reading %s: %v", source, err)
		}
		for _, match := range callSite.FindAllStringSubmatch(string(body), -1) {
			found++
			if _, ok := uiActionScopes[match[1]]; !ok {
				t.Errorf("%s calls actionKey(%q), which is missing from uiActionScopes", source, match[1])
			}
		}
	}
	if found == 0 {
		t.Fatal("found no actionKey call sites, the pattern has drifted from the code")
	}
}

// bindingApp returns an app whose commands were built from the given
// keybindings, so the config path under test is the one NewApp runs.
func bindingApp(t *testing.T, bindings map[string]string) *App {
	t.Helper()
	app := NewApp(linearapi.ClientConfig{}, config.Config{
		PageSize:    1,
		CacheTTL:    time.Minute,
		Keybindings: bindings,
	}, nil)
	stopBackgroundWorkOnCleanup(t, app)
	app.queueUpdateDraw = func(f func()) { f() }
	// A rendered page, or the ring these tests read has no stop to move to and
	// they would agree with each other over an empty pane.
	app.selectedIssue = detailsFixture()
	app.selectedIssue.Comments = threadedComments()
	app.updateDetailsView()
	focusCommentCards(app)
	drawComments(t, app, 80)
	return app
}

// TestCommandBindingBeatsADefaultActionKey covers ZNL-31: a command bound to a
// rune an action holds by default never fired, with nothing said about it.
func TestCommandBindingBeatsADefaultActionKey(t *testing.T) {
	app := bindingApp(t, map[string]string{"toggle_navigation_pane": "}"})
	lit := app.focusedCommentID

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, '}', tcell.ModNone))

	if !app.navigationHidden {
		t.Error("} did not run the command bound to it")
	}
	if got := app.focusedCommentID; got != lit {
		t.Errorf("} also stepped the ring to %q, want the command to own the key", got)
	}
}

// TestActionKeyStandsWhenTheBoundCommandIsOutOfScope pins the limit of that
// precedence. A navigation command holds the rune nowhere else, so the action
// still answers in the details pane.
func TestActionKeyStandsWhenTheBoundCommandIsOutOfScope(t *testing.T) {
	app := bindingApp(t, map[string]string{"toggle_favorite": "}"})
	lit := app.focusedCommentID

	app.handleGlobalKey(tcell.NewEventKey(tcell.KeyRune, '}', tcell.ModNone))

	if got := app.focusedCommentID; got == lit {
		t.Error("} did not step the ring, want the action to answer out of the command's scope")
	}
}

// TestMovementRunesStayWithTheWidgets verifies a binding cannot take one, from
// either side. A command on j would strand the cursor in the list, and g is the
// sharper case: nothing else claims it, so only this rule keeps go-to-top.
func TestMovementRunesStayWithTheWidgets(t *testing.T) {
	for _, tt := range []struct {
		name    string
		binding map[string]string
		key     rune
	}{
		{"command on j", map[string]string{"archive": "j"}, 'j'},
		{"command on g", map[string]string{"archive": "g"}, 'g'},
		{"action on j", map[string]string{"quit": "j"}, 'j'},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := bindingApp(t, tt.binding)
			app.focusedPane = FocusIssues

			event := tcell.NewEventKey(tcell.KeyRune, tt.key, tcell.ModNone)
			if got := app.handleGlobalKey(event); got != event {
				t.Errorf("%q was claimed by a keybinding instead of reaching the table", tt.key)
			}
			if app.pages.HasPage("confirmation") {
				t.Errorf("%q opened the archive confirmation", tt.key)
			}
		})
	}
}

// TestAMovementBindingLeavesTheDefaultAlone pins what a rejected binding does
// to the command that asked for it: nothing. Clearing the rune instead would
// answer one unusable key with a second one.
func TestAMovementBindingLeavesTheDefaultAlone(t *testing.T) {
	commands := []Command{{ID: "archive", Scope: ScopeIssue, ShortcutRune: 'x'}}
	applyBindings(commands, map[string]string{"archive": "j"})

	if commands[0].ShortcutRune != 'x' {
		t.Errorf("archive = %q, want its default x kept", commands[0].ShortcutRune)
	}
}

// TestToggleFavoriteHonoursKeybindingOverride verifies the favorite command is
// remappable like any other palette command.
func TestToggleFavoriteHonoursKeybindingOverride(t *testing.T) {
	commands := DefaultCommands(nil)
	applyBindings(commands, map[string]string{"toggle_favorite": "F"})

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
	h := &reorderHarness{app: newUXTestApp(t), settled: make(chan struct{}, 1)}
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
// the keybindings config instead of the hardcoded defaults. The config is read
// once, when the registry is built, so the change has to go through the rebuild
// a settings save runs.
func TestFavoriteMoveKeysResolveThroughActionKey(t *testing.T) {
	h := newReorderHarness(t)
	h.app.config.Keybindings = map[string]string{
		"favorite_move_up":   "U",
		"favorite_move_down": "D",
	}
	h.app.rebuildCommands()

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

// TestFavoriteMoveKeysAreSwallowedOnNonFavorites verifies the keys never reach
// tview, whose jump-to-child and jump-to-parent they would otherwise fire.
func TestFavoriteMoveKeysAreSwallowedOnNonFavorites(t *testing.T) {
	app := newUXTestApp(t)
	app.updateFavoriteSortFunc = func(context.Context, string, float64) error {
		t.Error("reorder must not reach the API for a non-favorite")
		return nil
	}
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Name: "Engineering"}}, nil)
	app.navigationTree.SetCurrentNode(app.findTeamTreeNode("team-1"))

	for _, r := range []rune{'J', 'K'} {
		if app.handleNavigationKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)) != nil {
			t.Errorf("%q reached the tree on a team node, want it swallowed", r)
		}
	}
}
