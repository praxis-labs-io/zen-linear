package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

func runeCellWidth(text string) int {
	return runewidth.StringWidth(text)
}

// navTestTeams builds labels longer than any pane width under test, so a fit
// always truncates and a skipped node is distinguishable from a fitted one.
func navTestTeams(count int) []linearapi.Team {
	teams := make([]linearapi.Team, count)
	for i := range teams {
		teams[i] = linearapi.Team{
			ID:   fmt.Sprintf("team-%d", i),
			Name: fmt.Sprintf("Team %d with a name past any reasonable pane width", i),
		}
	}
	return teams
}

func navTreeLabels(app *App) []string {
	var labels []string
	var walk func(*tview.TreeNode)
	walk = func(node *tview.TreeNode) {
		labels = append(labels, node.GetText())
		for _, child := range node.GetChildren() {
			walk(child)
		}
	}
	walk(app.navigationTree.GetRoot())
	return labels
}

func TestPadNavigationTree_FitsEveryNodeToTheWidth(t *testing.T) {
	app := newUXTestApp()
	app.rebuildNavigationTree(navTestTeams(3), nil)

	app.padNavigationTree(30)

	for _, label := range navTreeLabels(app) {
		if width := runeCellWidth(label); width != 30 && width != 28 && width != 26 {
			t.Fatalf("label %q fitted to width %d, want the width for its level", label, width)
		}
	}
	if !strings.Contains(strings.Join(navTreeLabels(app), "\n"), "…") {
		t.Fatal("no label was truncated, so the test is not exercising a fit")
	}
}

// TestPadNavigationTree_SkipsNodesAlreadyFittedToTheWidth guards the reason this
// cache exists: padNavigationTree runs from SetDrawFunc, so it fires on every
// draw anywhere in the app, and re-deriving identical labels is pure waste.
func TestPadNavigationTree_SkipsNodesAlreadyFittedToTheWidth(t *testing.T) {
	app := newUXTestApp()
	app.rebuildNavigationTree(navTestTeams(3), nil)
	app.padNavigationTree(30)

	// Overwrite a fitted label. A redraw at the same width must not restore it,
	// which is only true if the node was skipped.
	teamNode := app.navigationTree.GetRoot().GetChildren()[1]
	teamNode.SetText("sentinel")

	app.padNavigationTree(30)

	if got := teamNode.GetText(); got != "sentinel" {
		t.Fatalf("node text = %q, want the redraw to skip an already-fitted node", got)
	}
}

func TestPadNavigationTree_RefitsOnWidthChange(t *testing.T) {
	app := newUXTestApp()
	app.rebuildNavigationTree(navTestTeams(3), nil)
	app.padNavigationTree(30)
	before := navTreeLabels(app)

	app.padNavigationTree(50)
	after := navTreeLabels(app)

	if len(before) != len(after) {
		t.Fatalf("node count changed from %d to %d", len(before), len(after))
	}
	for i := range after {
		if runeCellWidth(after[i]) <= runeCellWidth(before[i]) {
			t.Fatalf("label %d width %d did not grow from %d after a resize",
				i, runeCellWidth(after[i]), runeCellWidth(before[i]))
		}
	}
}

// TestPadNavigationTree_FitsNodesAddedAfterTheFirstDraw covers lazily added
// children: expanding a team inserts nodes the cache has never seen.
func TestPadNavigationTree_FitsNodesAddedAfterTheFirstDraw(t *testing.T) {
	app := newUXTestApp()
	app.rebuildNavigationTree(navTestTeams(1), nil)
	app.padNavigationTree(30)

	teamNode := app.navigationTree.GetRoot().GetChildren()[1]
	added := tview.NewTreeNode("A child added long after the first draw completed")
	teamNode.AddChild(added)

	app.padNavigationTree(30)

	if got := added.GetText(); got == "A child added long after the first draw completed" {
		t.Fatal("a node added after the first draw was never fitted")
	}
	if width := runeCellWidth(added.GetText()); width != 26 {
		t.Fatalf("added node fitted to width %d, want 26 for its level", width)
	}
}

func TestForgetNavNodeLabels_DropsTheSubtree(t *testing.T) {
	app := newUXTestApp()
	app.rebuildNavigationTree(navTestTeams(2), nil)
	app.padNavigationTree(30)

	teamNode := app.navigationTree.GetRoot().GetChildren()[1]
	child := tview.NewTreeNode("child")
	teamNode.AddChild(child)
	app.padNavigationTree(30)

	app.forgetNavNodeLabels(teamNode)

	if _, ok := app.navNodeLabels[teamNode]; ok {
		t.Fatal("team node still cached after forgetNavNodeLabels")
	}
	if _, ok := app.navNodeLabels[child]; ok {
		t.Fatal("child still cached after forgetNavNodeLabels")
	}
}

// benchNavTree builds a tree the size of a busy workspace: teams each expanded
// into cycles, statuses, and projects.
func benchNavTree(app *App, teams, childrenPerTeam int) {
	app.rebuildNavigationTree(navTestTeams(teams), nil)
	for _, teamNode := range app.navigationTree.GetRoot().GetChildren() {
		for i := range childrenPerTeam {
			teamNode.AddChild(tview.NewTreeNode(
				fmt.Sprintf("Child %d with a label longer than the pane is wide", i)))
		}
	}
}

func BenchmarkPadNavigationTree_SteadyState(b *testing.B) {
	app := newUXTestApp()
	benchNavTree(app, 20, 19)
	app.padNavigationTree(30)

	b.ResetTimer()
	for b.Loop() {
		app.padNavigationTree(30)
	}
}

func BenchmarkPadNavigationTree_AfterResize(b *testing.B) {
	app := newUXTestApp()
	benchNavTree(app, 20, 19)

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		app.padNavigationTree(30 + i%2)
	}
}
