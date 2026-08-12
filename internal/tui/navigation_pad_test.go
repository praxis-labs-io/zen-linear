package tui

import (
	"fmt"
	"slices"
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
	// The root is hidden and never fitted, so it is not one of the labels.
	for _, child := range app.navigationTree.GetRoot().GetChildren() {
		walk(child)
	}
	return labels
}

// navTreeLabelsByLevel walks the tree returning each label with its depth, so
// assertions can pin the width a node was fitted to against its own level
// rather than accepting any level's width.
func navTreeLabelsByLevel(app *App) []struct {
	label string
	level int
} {
	var out []struct {
		label string
		level int
	}
	var walk func(*tview.TreeNode, int)
	walk = func(node *tview.TreeNode, level int) {
		out = append(out, struct {
			label string
			level int
		}{node.GetText(), level})
		for _, child := range node.GetChildren() {
			walk(child, level+1)
		}
	}
	for _, child := range app.navigationTree.GetRoot().GetChildren() {
		walk(child, 0)
	}
	return out
}

func TestPadNavigationTree_FitsEveryNodeToItsLevelWidth(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree(navTestTeams(3), nil)

	const width = 30
	app.padNavigationTree(width)

	truncated := false
	for _, node := range navTreeLabelsByLevel(app) {
		want := width - 2*node.level
		if got := runeCellWidth(node.label); got != want {
			t.Fatalf("level %d label %q fitted to width %d, want %d", node.level, node.label, got, want)
		}
		if strings.Contains(node.label, "…") {
			truncated = true
		}
	}
	if !truncated {
		t.Fatal("no label was truncated, so the test is not exercising a fit")
	}
}

// TestPadNavigationTree_IsIdempotentAtAnUnchangedWidth covers the skip path
// from the outside: redrawing at the same width must leave every label exactly
// as it was. The saving itself is measured by BenchmarkPadNavigationTree, not
// asserted here, because a unit test cannot see that the work was skipped
// without pinning an invariant the code should not have.
func TestPadNavigationTree_IsIdempotentAtAnUnchangedWidth(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree(navTestTeams(3), nil)

	app.padNavigationTree(30)
	before := navTreeLabels(app)
	app.padNavigationTree(30)
	after := navTreeLabels(app)

	if !slices.Equal(before, after) {
		t.Fatalf("labels changed on a redraw at the same width:\n%v\n%v", before, after)
	}
}

// TestPadNavigationTree_PicksUpALabelChangedElsewhere guards the trap the cache
// could set: if a fitted width alone counted as proof the rendered text is
// current, anything that relabels a node outside padNavigationNode would be
// silently discarded and the pane would show stale text until a resize.
func TestPadNavigationTree_PicksUpALabelChangedElsewhere(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree(navTestTeams(3), nil)
	app.padNavigationTree(30)

	teamNode := app.navigationTree.GetRoot().GetChildren()[1]
	teamNode.SetText("Renamed elsewhere")

	app.padNavigationTree(30)

	got := teamNode.GetText()
	if !strings.HasPrefix(got, "Renamed elsewhere") {
		t.Fatalf("node text = %q, want the relabel to survive the redraw", got)
	}
	if width := runeCellWidth(got); width != 30 {
		t.Fatalf("relabelled node fitted to width %d, want 30 for its level", width)
	}
}

func TestPadNavigationTree_RefitsOnWidthChange(t *testing.T) {
	app := newUXTestApp(t)
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
	app := newUXTestApp(t)
	app.rebuildNavigationTree(navTestTeams(1), nil)
	app.padNavigationTree(30)

	teamNode := app.navigationTree.GetRoot().GetChildren()[1]
	added := tview.NewTreeNode("A child added long after the first draw completed")
	teamNode.AddChild(added)

	app.padNavigationTree(30)

	if got := added.GetText(); got == "A child added long after the first draw completed" {
		t.Fatal("a node added after the first draw was never fitted")
	}
	if width := runeCellWidth(added.GetText()); width != 28 {
		t.Fatalf("added node fitted to width %d, want 28 for its level", width)
	}
}

func TestForgetNavNodeLabels_DropsTheSubtree(t *testing.T) {
	app := newUXTestApp(t)
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
	app := newUXTestApp(b)
	benchNavTree(app, 20, 19)
	app.padNavigationTree(30)

	b.ResetTimer()
	for b.Loop() {
		app.padNavigationTree(30)
	}
}

func BenchmarkPadNavigationTree_AfterResize(b *testing.B) {
	app := newUXTestApp(b)
	benchNavTree(app, 20, 19)

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		app.padNavigationTree(30 + i%2)
	}
}
