package tui

import (
	"slices"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

// pressDownOnNavigation steps the tree's cursor through its own input handler,
// so a test sees the rows a user's Down key actually stops on.
func pressDownOnNavigation(app *App) {
	handler := app.navigationTree.InputHandler()
	handler(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), func(tview.Primitive) {})
}

// navRowLabels reads the tree's top-level rows, blank spacers included, so a
// test can assert both the sections and the gaps between them.
func navRowLabels(app *App) []string {
	labels := []string{}
	for _, child := range app.navigationTree.GetRoot().GetChildren() {
		labels = append(labels, child.GetText())
	}
	return labels
}

func TestTheTreeGroupsTeamsUnderAHeading(t *testing.T) {
	app := newUXTestApp(t)
	teams := []linearapi.Team{
		{ID: "team-1", Key: "ENG", Name: "Engineering"},
		{ID: "team-2", Key: "DES", Name: "Design"},
	}
	favorites := []linearapi.Favorite{
		{ID: "fav-a", Type: "project", ProjectID: "p1", ProjectName: "Alpha", SortOrder: 1},
	}
	app.rebuildNavigationTree(teams, favorites)

	want := []string{"All Issues", "", "Favorites", "", "Teams"}
	if got := navRowLabels(app); !slices.Equal(got, want) {
		t.Fatalf("navigation rows = %q, want %q", got, want)
	}

	group := app.teamsGroup
	if group == nil {
		t.Fatal("no Teams group")
	}
	if group.GetColor() != app.theme.Accent {
		t.Errorf("Teams heading color = %v, want the accent %v", group.GetColor(), app.theme.Accent)
	}

	names := []string{}
	for _, child := range group.GetChildren() {
		names = append(names, child.GetText())
	}
	if want := []string{"Engineering", "Design"}; !slices.Equal(names, want) {
		t.Errorf("teams under the heading = %q, want %q", names, want)
	}

	// The blank row and the heading are rows nothing can be done on, so the
	// cursor has to step over both.
	app.navigationTree.SetCurrentNode(app.allIssuesNode)
	pressDownOnNavigation(app)
	if got := app.navigationTree.GetCurrentNode().GetText(); got != "Alpha" {
		t.Errorf("Down from All Issues landed on %q, want the first favorite", got)
	}
}

func TestASectionMissingTakesItsBlankRowWithIt(t *testing.T) {
	teams := []linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}}
	favorites := []linearapi.Favorite{
		{ID: "fav-a", Type: "project", ProjectID: "p1", ProjectName: "Alpha", SortOrder: 1},
	}

	tests := []struct {
		name      string
		teams     []linearapi.Team
		favorites []linearapi.Favorite
		want      []string
	}{
		{"nothing favorited", teams, nil, []string{"All Issues", "", "Teams"}},
		{"no teams", nil, favorites, []string{"All Issues", "", "Favorites"}},
		{"neither", nil, nil, []string{"All Issues"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := newUXTestApp(t)
			app.rebuildNavigationTree(test.teams, test.favorites)
			if got := navRowLabels(app); !slices.Equal(got, test.want) {
				t.Fatalf("navigation rows = %q, want %q", got, test.want)
			}
			if test.name != "nothing favorited" {
				return
			}
			app.navigationTree.SetCurrentNode(app.allIssuesNode)
			pressDownOnNavigation(app)
			if got := app.navigationTree.GetCurrentNode().GetText(); got != "Engineering" {
				t.Errorf("Down from All Issues landed on %q, want the first team", got)
			}
		})
	}
}

func TestFavoritingLeavesTheTeamsSectionWhereItWas(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}}, nil)

	group := app.teamsGroup
	teamNode := group.GetChildren()[0]
	// The children a team expansion loaded: a rebuild that dropped them would
	// collapse the team the user had open.
	teamNode.AddChild(tview.NewTreeNode("Alpha").
		SetReference(&NavigationNode{ID: "p1", Text: "Alpha", IsProject: true}))
	teamNode.SetExpanded(true)

	app.favorites = []linearapi.Favorite{
		{ID: "fav-a", Type: "project", ProjectID: "p1", ProjectName: "Alpha", SortOrder: 1},
	}
	app.refreshFavoritesSection("")

	want := []string{"All Issues", "", "Favorites", "", "Teams"}
	if got := navRowLabels(app); !slices.Equal(got, want) {
		t.Fatalf("navigation rows = %q, want %q", got, want)
	}
	if app.teamsGroup != group {
		t.Fatal("the Teams section was rebuilt, so anything a team had loaded is gone")
	}
	if len(teamNode.GetChildren()) != 1 || !teamNode.IsExpanded() {
		t.Error("the expanded team lost what it had loaded")
	}
	if app.findTeamTreeNode("team-1") != teamNode {
		t.Error("findTeamTreeNode no longer reaches the team inside the heading")
	}
}

func TestUnfavoritingTheLastFavoriteTakesTheBlankRowWithIt(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree(
		[]linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}},
		[]linearapi.Favorite{{ID: "fav-a", Type: "project", ProjectID: "p1", ProjectName: "Alpha", SortOrder: 1}},
	)

	app.favorites = nil
	app.refreshFavoritesSection("")

	want := []string{"All Issues", "", "Teams"}
	if got := navRowLabels(app); !slices.Equal(got, want) {
		t.Fatalf("navigation rows = %q, want %q", got, want)
	}
}
