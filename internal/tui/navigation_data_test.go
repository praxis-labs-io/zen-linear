package tui

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/mattn/go-runewidth"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

func pressDownOnNavigation(app *App) {
	handler := app.navigationTree.InputHandler()
	handler(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), func(tview.Primitive) {})
}

func currentNavText(app *App) string {
	nav, ok := app.navigationTree.GetCurrentNode().GetReference().(*NavigationNode)
	if !ok {
		return ""
	}
	return nav.Text
}

func navRowStarts(app *App, node *tview.TreeNode, want string) bool {
	app.padNavigationTree(30)
	return strings.HasPrefix(node.GetText(), want)
}

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

	want := []string{"All Issues", "Cycles", "Status", "Projects"}
	if got := teamGroupLabels(teamNode); !slices.Equal(got, want) {
		t.Fatalf("team children = %q, want %q", got, want)
	}
	for _, group := range teamNode.GetChildren()[1:] {
		if group.IsExpanded() {
			t.Errorf("%q opened expanded, which is the wall of rows this replaced", group.GetText())
		}
		if !navRowStarts(app, group, "  "+navIconClosed) {
			t.Errorf("%q does not begin with an indent and a closed folder", group.GetText())
		}
	}
	if !navRowStarts(app, teamNode, navIconOpen) {
		t.Errorf("open team = %q, want it to begin with an open folder at the edge", teamNode.GetText())
	}
	if !navRowStarts(app, teamNode.GetChildren()[0], "  "+navIconBranch) {
		t.Errorf("All Issues = %q, want a branch in its column", teamNode.GetChildren()[0].GetText())
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
	if !navRowStarts(app, cycles, "  "+navIconOpen) {
		t.Errorf("opened heading = %q, want an open folder", cycles.GetText())
	}

	pressEnterOnNavigation(app)
	if cycles.IsExpanded() {
		t.Fatal("Enter did not close the heading again")
	}
	if !navRowStarts(app, cycles, "  "+navIconClosed) {
		t.Errorf("closed heading = %q, want a closed folder", cycles.GetText())
	}
}

func TestAHeadingCannotBeFavorited(t *testing.T) {
	app := newFavoritesTestApp(t)
	teamNode := openTestTeam(t, app)
	app.navigationTree.SetCurrentNode(teamNode.GetChildren()[2])

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
	want := []string{"All Issues", "Status", "Projects"}
	if got := teamGroupLabels(teamNode); !slices.Equal(got, want) {
		t.Fatalf("team rows after the retry = %q, want %q", got, want)
	}
}

func TestAnOpenTeamClosesWithoutGoingBackForItsRows(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}}, nil)
	teamNode := app.findTeamTreeNode("team-1")

	app.populateTeamNodeChildren(teamNode, "team-1", nil, nil, nil)
	setNavFold(teamNode, true)

	app.navigationTree.SetCurrentNode(teamNode)
	pressEnterOnNavigation(app)

	if teamNode.IsExpanded() {
		t.Fatal("an open team sprang back open instead of closing")
	}
}

func TestAFavoritedTeamOnlyFolds(t *testing.T) {
	app := newUXTestApp(t)
	app.fetchIssuesPage = func(context.Context, linearapi.FetchIssuesParams, *string) (linearapi.IssuePage, error) {
		t.Error("opening a favorited team refetched the issue list")
		return linearapi.IssuePage{}, nil
	}
	app.rebuildNavigationTree(
		[]linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}},
		[]linearapi.Favorite{{ID: "fav-a", Type: "team", TeamID: "team-1", TeamName: "Engineering", SortOrder: 1}},
	)

	favorite := app.favoritesGroup.GetChildren()[0]
	if !navRowStarts(app, favorite, navIconClosed) {
		t.Fatalf("favorited team = %q, want a closed folder at the edge", favorite.GetText())
	}
	app.populateTeamNodeChildren(favorite, "team-1", nil,
		[]linearapi.WorkflowState{{ID: "state-1", Name: "Todo"}}, nil)

	app.navigationTree.SetCurrentNode(favorite)
	pressEnterOnNavigation(app)
	if !favorite.IsExpanded() {
		t.Fatal("Enter did not open the favorited team")
	}
	if got := favorite.GetChildren()[0].GetText(); !strings.Contains(got, "All Issues") {
		t.Errorf("favorited team opens onto %q, want its own All Issues", got)
	}

	pressEnterOnNavigation(app)
	if favorite.IsExpanded() {
		t.Fatal("Enter did not close it again")
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

func TestExpandableRowsAreMutedAndSelectableOnesAreNot(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree(
		[]linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}},
		[]linearapi.Favorite{
			{ID: "folder-1", Type: "folder", FolderName: "Current Projects", SortOrder: 1},
			{ID: "fav-a", Type: "project", ProjectID: "p1", ProjectName: "Alpha", ParentID: "folder-1", SortOrder: 2},
		},
	)
	teamNode := app.findTeamTreeNode("team-1")
	app.populateTeamNodeChildren(teamNode, "team-1",
		[]linearapi.Project{{ID: "project-1", Name: "Website", TeamID: "team-1"}},
		[]linearapi.WorkflowState{{ID: "state-1", Name: "Todo"}},
		nil,
	)

	tests := []struct {
		name  string
		node  *tview.TreeNode
		muted bool
	}{
		{"a team", teamNode, true},
		{"one of its headings", teamNode.GetChildren()[1], true},
		{"a favorites folder", app.favoritesGroup.GetChildren()[0], true},
		{"the team's All Issues", teamNode.GetChildren()[0], false},
		{"a status", teamNode.GetChildren()[1].GetChildren()[0], false},
		{"a project", teamNode.GetChildren()[2].GetChildren()[0], false},
		{"a favorite", app.favoritesGroup.GetChildren()[0].GetChildren()[0], false},
		{"All Issues", app.allIssuesNode, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want := app.theme.Foreground
			if test.muted {
				want = app.theme.SecondaryText
			}
			if got := test.node.GetColor(); got != want {
				t.Errorf("%q color = %v, want %v", test.node.GetText(), got, want)
			}
		})
	}
}

func TestAFavoritedTeamReadsAsStructure(t *testing.T) {
	app := newUXTestApp(t)
	app.rebuildNavigationTree(
		[]linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}},
		[]linearapi.Favorite{{ID: "fav-a", Type: "team", TeamID: "team-1", TeamName: "Engineering", SortOrder: 1}},
	)

	favorite := app.favoritesGroup.GetChildren()[0]
	if got := favorite.GetColor(); got != app.theme.SecondaryText {
		t.Errorf("favorited team color = %v, want it muted %v", got, app.theme.SecondaryText)
	}
	if got := app.findTeamTreeNode("team-1").GetColor(); got != app.theme.SecondaryText {
		t.Errorf("the team's own row = %v, want it muted %v", got, app.theme.SecondaryText)
	}
}

func TestTheRowMeasureIgnoresTheLocale(t *testing.T) {
	for _, prefix := range []string{navIconOpen, navIconClosed, navIconBranch, navIconBlank} {
		if got := navCellWidth.StringWidth(prefix); got != 2 {
			t.Errorf("prefix %q measured %d cells, want 2", prefix, got)
		}
	}
	wide := (&runewidth.Condition{EastAsianWidth: true}).StringWidth(navIconBranch)
	if wide == 2 {
		t.Skip("the branch is not ambiguous in this build, so there is nothing to pin")
	}
	if got := fitToWidth(navIconBranch+"Alpha", 20); navCellWidth.StringWidth(got) != 20 {
		t.Errorf("fitted row measured %d cells, want the full 20", navCellWidth.StringWidth(got))
	}
}
