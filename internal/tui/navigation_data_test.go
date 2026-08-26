package tui

import (
	"context"
	"slices"
	"testing"
	"time"

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

// currentNavText names the row under the cursor, off its own node rather than
// the rendered label, which carries a fold marker.
func currentNavText(app *App) string {
	nav, ok := app.navigationTree.GetCurrentNode().GetReference().(*NavigationNode)
	if !ok {
		return ""
	}
	return nav.Text
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
		nav, ok := child.GetReference().(*NavigationNode)
		if !ok {
			t.Fatalf("row %q under the heading is not a navigation node", child.GetText())
		}
		names = append(names, nav.Text)
	}
	if want := []string{"Engineering", "Design"}; !slices.Equal(names, want) {
		t.Errorf("teams under the heading = %q, want %q", names, want)
	}

	// The blank row and the heading are rows nothing can be done on, so the
	// cursor has to step over both.
	app.navigationTree.SetCurrentNode(app.allIssuesNode)
	pressDownOnNavigation(app)
	if got := currentNavText(app); got != "Alpha" {
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
			if got := currentNavText(app); got != "Engineering" {
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

// teamGroupLabels reads a team's headings and whether each is open.
func teamGroupLabels(teamNode *tview.TreeNode) []string {
	labels := []string{}
	for _, child := range teamNode.GetChildren() {
		labels = append(labels, child.GetText())
	}
	return labels
}

func openTestTeam(t *testing.T, app *App) *tview.TreeNode {
	t.Helper()
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}}, nil)
	teamNode := app.findTeamTreeNode("team-1")
	app.populateTeamNodeChildren(teamNode, "team-1",
		[]linearapi.Project{{ID: "project-1", Name: "Website", TeamID: "team-1"}},
		[]linearapi.WorkflowState{{ID: "state-1", Name: "Todo"}},
		[]linearapi.Cycle{{ID: "cycle-1", Number: 12}},
	)
	setNavFold(teamNode, true)
	return teamNode
}

func TestATeamOpensOntoThreeFoldedHeadings(t *testing.T) {
	app := newUXTestApp(t)
	teamNode := openTestTeam(t, app)

	want := []string{"All Issues", "▸ Cycles", "▸ Status", "▸ Projects"}
	if got := teamGroupLabels(teamNode); !slices.Equal(got, want) {
		t.Fatalf("team children = %q, want %q", got, want)
	}
	for _, group := range teamNode.GetChildren()[1:] {
		if group.IsExpanded() {
			t.Errorf("%q opened expanded, which is the wall of rows this replaced", group.GetText())
		}
	}
	if got := teamNode.GetText(); got != "▾ Engineering" {
		t.Errorf("open team reads %q, want it marked open", got)
	}
}

func TestOpeningAHeadingShowsWhatIsInside(t *testing.T) {
	app := newUXTestApp(t)
	app.fetchIssuesPage = func(context.Context, linearapi.FetchIssuesParams, *string) (linearapi.IssuePage, error) {
		t.Error("opening a heading must not refresh the issue list")
		return linearapi.IssuePage{}, nil
	}
	teamNode := openTestTeam(t, app)

	cycles := teamNode.GetChildren()[1]
	app.navigationTree.SetCurrentNode(cycles)
	pressEnterOnNavigation(app)

	if !cycles.IsExpanded() {
		t.Fatal("Enter did not open the heading")
	}
	if got := cycles.GetText(); got != "▾ Cycles" {
		t.Errorf("opened heading reads %q, want it marked open", got)
	}

	pressEnterOnNavigation(app)
	if cycles.IsExpanded() {
		t.Fatal("Enter did not close the heading again")
	}
	if got := cycles.GetText(); got != "▸ Cycles" {
		t.Errorf("closed heading reads %q, want it marked closed", got)
	}
}

func TestAHeadingCannotBeFavorited(t *testing.T) {
	app := newFavoritesTestApp(t)
	teamNode := openTestTeam(t, app)
	app.navigationTree.SetCurrentNode(teamNode.GetChildren()[2])

	// The stubs fail the test if any mutation fires. A heading stands for
	// nothing Linear can hold a favorite on.
	handleToggleFavorite(app)

	if len(app.favorites) != 0 {
		t.Fatalf("favorites = %+v, want the heading refused", app.favorites)
	}
}

func TestUpAtTheTopOfTheTreeStaysOnTheTree(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}}, nil)
	app.navigationTree.SetCurrentNode(app.allIssuesNode)

	for _, event := range []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyRune, 'k', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone),
	} {
		if app.handleNavigationKey(event) == nil {
			t.Errorf("%v was swallowed at the top of the tree", event.Key())
		}
		if app.navSearchFocused {
			t.Fatalf("%v walked into the query box", event.Key())
		}
	}
}

func TestFoldingATeamCostsNoFetch(t *testing.T) {
	app := newUXTestApp(t)
	app.fetchIssuesPage = func(context.Context, linearapi.FetchIssuesParams, *string) (linearapi.IssuePage, error) {
		t.Error("folding a team refetched the issue list")
		return linearapi.IssuePage{}, nil
	}
	teamNode := openTestTeam(t, app)
	app.navigationTree.SetCurrentNode(teamNode)

	pressEnterOnNavigation(app)
	if teamNode.IsExpanded() {
		t.Fatal("Enter did not close the team")
	}
	pressEnterOnNavigation(app)
	if !teamNode.IsExpanded() {
		t.Fatal("Enter did not open the team again")
	}
}

func TestATeamsOwnAllIssuesScopesToTheTeam(t *testing.T) {
	app := newUXTestApp(t)
	scoped := make(chan linearapi.FetchIssuesParams, 1)
	app.fetchIssuesPage = func(_ context.Context, params linearapi.FetchIssuesParams, _ *string) (linearapi.IssuePage, error) {
		select {
		case scoped <- params:
		default:
		}
		return linearapi.IssuePage{}, nil
	}
	teamNode := openTestTeam(t, app)

	app.navigationTree.SetCurrentNode(teamNode.GetChildren()[0])
	pressEnterOnNavigation(app)

	select {
	case params := <-scoped:
		if params.TeamID != "team-1" {
			t.Fatalf("fetch params = %+v, want the whole of team-1", params)
		}
		if params.ProjectID != "" || params.StateID != "" || params.CycleID != "" {
			t.Fatalf("fetch params = %+v, want nothing narrowing the team", params)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("selecting a team's All Issues never fetched")
	}
}

func TestATeamThatLoadedNothingGoesBackForIt(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}}, nil)
	teamNode := app.findTeamTreeNode("team-1")

	// A load that answered without answering. Every Linear team has states,
	// so this is a failure wearing the shape of an empty team.
	app.populateTeamNodeChildren(teamNode, "team-1", nil, nil, nil)
	if teamChildrenLoaded(teamNode) {
		t.Fatal("a team with no states counted as loaded, so nothing will go back for it")
	}

	app.populateTeamNodeChildren(teamNode, "team-1",
		[]linearapi.Project{{ID: "project-1", Name: "Website", TeamID: "team-1"}},
		[]linearapi.WorkflowState{{ID: "state-1", Name: "Todo"}},
		nil,
	)

	if !teamChildrenLoaded(teamNode) {
		t.Fatal("a real load did not count as loaded")
	}
	want := []string{"All Issues", "▸ Status", "▸ Projects"}
	if got := teamGroupLabels(teamNode); !slices.Equal(got, want) {
		t.Fatalf("team rows after the retry = %q, want %q", got, want)
	}
}

func TestAnOpenTeamClosesWithoutGoingBackForItsRows(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}}, nil)
	teamNode := app.findTeamTreeNode("team-1")

	// The shape a load that answered with nothing leaves behind.
	app.populateTeamNodeChildren(teamNode, "team-1", nil, nil, nil)
	setNavFold(teamNode, true)

	app.navigationTree.SetCurrentNode(teamNode)
	pressEnterOnNavigation(app)

	if teamNode.IsExpanded() {
		t.Fatal("an open team sprang back open instead of closing")
	}
}

func TestAFavoritedTeamStillScopesTheList(t *testing.T) {
	app := newUXTestApp(t)
	scoped := make(chan linearapi.FetchIssuesParams, 1)
	app.fetchIssuesPage = func(_ context.Context, params linearapi.FetchIssuesParams, _ *string) (linearapi.IssuePage, error) {
		select {
		case scoped <- params:
		default:
		}
		return linearapi.IssuePage{}, nil
	}
	app.rebuildNavigationTree(
		[]linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}},
		[]linearapi.Favorite{{ID: "fav-a", Type: "team", TeamID: "team-1", TeamName: "Engineering", SortOrder: 1}},
	)

	favorite := app.favoritesGroup.GetChildren()[0]
	if got := favorite.GetText(); got != "▸ Engineering" {
		t.Fatalf("favorited team reads %q, want it marked closed", got)
	}

	app.navigationTree.SetCurrentNode(favorite)
	pressEnterOnNavigation(app)

	select {
	case params := <-scoped:
		if params.TeamID != "team-1" {
			t.Fatalf("fetch params = %+v, want the whole of team-1", params)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a favorited team no longer scopes the issue list")
	}
}

func TestRebuildingATeamsRowsForgetsTheOnesItDropped(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}}, nil)
	teamNode := app.findTeamTreeNode("team-1")

	app.populateTeamNodeChildren(teamNode, "team-1", nil, nil, nil)
	app.padNavigationTree(30)
	dropped := teamNode.GetChildren()[0]

	app.populateTeamNodeChildren(teamNode, "team-1", nil,
		[]linearapi.WorkflowState{{ID: "state-1", Name: "Todo"}}, nil)

	if _, held := app.navNodeLabels[dropped]; held {
		t.Fatal("the label cache still holds a row the rebuild dropped")
	}
}
