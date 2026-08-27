package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
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

// Every row is fitted to the whole pane, whatever its depth, so the cursor line
// spans the width rather than starting where the text does. The depth is inside
// the label.
func TestPadNavigationTree_FitsEveryNodeToTheFullWidth(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree(navTestTeams(3), nil)

	const width = 30
	app.padNavigationTree(width)

	truncated := false
	for _, node := range navTreeLabelsByLevel(app) {
		if got := runeCellWidth(node.label); got != width {
			t.Fatalf("level %d label %q fitted to width %d, want the full %d", node.level, node.label, got, width)
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

	teamNode := app.teamsGroup.GetChildren()[0]
	teamNode.SetText("Renamed elsewhere")

	app.padNavigationTree(30)

	got := teamNode.GetText()
	if !strings.HasPrefix(got, navIconClosed+"Renamed elsewhere") {
		t.Fatalf("node text = %q, want the relabel to survive the redraw", got)
	}
	if width := runeCellWidth(got); width != 30 {
		t.Fatalf("relabelled node fitted to width %d, want the full 30", width)
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

	teamNode := app.teamsGroup.GetChildren()[0]
	added := tview.NewTreeNode("A child added long after the first draw completed").
		SetReference(&NavigationNode{ID: "p1", Text: "A child added long after the first draw completed", IsProject: true})
	teamNode.AddChild(added)

	app.padNavigationTree(30)

	if got := added.GetText(); got == "A child added long after the first draw completed" {
		t.Fatal("a node added after the first draw was never fitted")
	}
	if width := runeCellWidth(added.GetText()); width != 30 {
		t.Fatalf("added node fitted to width %d, want the full 30", width)
	}
	if !strings.HasPrefix(added.GetText(), "  "+navIconBranch+"A child") {
		t.Errorf("added node = %q, want it a level in on a branch", added.GetText())
	}
}

func TestForgetNavNodeLabels_DropsTheSubtree(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree(navTestTeams(2), nil)
	app.padNavigationTree(30)

	teamNode := app.teamsGroup.GetChildren()[0]
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

// Every row begins with a column, a folder where it opens and the same width
// blank where it does not, so siblings line up. The indent is a column a level,
// which is what puts a row under the title of the one it hangs off.
func TestEveryRowBeginsWithItsColumnAndStepsAColumnALevel(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree(
		[]linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}},
		[]linearapi.Favorite{
			{ID: "folder-1", Type: "folder", FolderName: "Current Projects", SortOrder: 1},
			{ID: "fav-a", Type: "project", ProjectID: "p1", ProjectName: "Alpha", ParentID: "folder-1", SortOrder: 2},
			{ID: "fav-b", Type: "cycle", CycleID: "c1", CycleName: "My Current Cycle", CycleTeamID: "team-1", SortOrder: 3},
		},
	)
	teamNode := app.findTeamTreeNode("team-1")
	app.populateTeamNodeChildren(teamNode, "team-1",
		[]linearapi.Project{{ID: "project-1", Name: "Website", TeamID: "team-1"}},
		[]linearapi.WorkflowState{{ID: "state-1", Name: "Todo"}},
		[]linearapi.Cycle{{ID: "cycle-1", Number: 12}},
	)
	setNavFold(teamNode, true)
	cycles := teamNode.GetChildren()[1]
	setNavFold(cycles, true)
	app.padNavigationTree(40)

	folder := app.favoritesGroup.GetChildren()[0]

	tests := []struct {
		name string
		node *tview.TreeNode
		want string
	}{
		{"a section's own row sits at the edge", folder, navIconOpen + "Current Projects"},
		{"a leaf beside it hangs off nothing", app.favoritesGroup.GetChildren()[1], navIconBlank + "My Current Cycle"},
		{"what it holds steps a column in", folder.GetChildren()[0], "  " + navIconBranch + "Alpha"},
		{"a team's leaf steps in the same", teamNode.GetChildren()[0], "  " + navIconBranch + "All Issues"},
		{"and lines up with its headings", cycles, "  " + navIconOpen + "Cycles"},
		{"a heading's own rows step in again", cycles.GetChildren()[0], "    " + navIconBranch + "Cycle 12"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.node.GetText(); !strings.HasPrefix(got, test.want) {
				t.Errorf("row = %q, want it to begin %q", got, test.want)
			}
		})
	}
}
