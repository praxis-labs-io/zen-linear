package tui

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/cache"
	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

// newNavCacheTestApp builds an App whose navigation fetch is held open, so a
// test can watch what paints before the network answers. Releasing the returned
// channel lets the fetch return teams and favorites.
func newNavCacheTestApp(t *testing.T, cfg config.Config, teams []linearapi.Team, favorites []linearapi.Favorite) (*App, chan struct{}, string) {
	t.Helper()

	app := newDefaultNavTestApp(t, cfg)
	// The cache only files entries under a configured workspace name.
	app.activeWorkspaceName = navCacheTestWorkspace
	stopBackgroundWorkOnCleanup(t, app)
	release := make(chan struct{})
	app.fetchTeamsFunc = func(context.Context) ([]linearapi.Team, error) {
		<-release
		return teams, nil
	}
	app.fetchFavoritesFunc = func(context.Context) ([]linearapi.Favorite, error) {
		<-release
		return favorites, nil
	}
	app.fetchCurrentUserFunc = func(context.Context) (linearapi.User, error) {
		return linearapi.User{ID: "user-1", DisplayName: "Drew"}, nil
	}

	path := filepath.Join(t.TempDir(), "nav-cache.json")
	return app, release, path
}

// navCacheTestWorkspace is the configured workspace the test apps run as.
const navCacheTestWorkspace = "Praxis"

// installNavCache seeds the disk copy the app paints its first tree from.
func installNavCache(t *testing.T, app *App, path, workspace string, data cache.NavData) {
	t.Helper()
	if err := cache.RecordNav(path, workspace, data); err != nil {
		t.Fatalf("RecordNav: %v", err)
	}
	file, err := cache.LoadNav(path)
	if err != nil {
		t.Fatalf("LoadNav: %v", err)
	}
	app.UseNavCache(path, file)
}

// installNavSettledHook reports when a launch has finished applying its fetch,
// including the launches that trigger no refresh of their own.
func installNavSettledHook(app *App) <-chan struct{} {
	done := make(chan struct{}, 4)
	app.navigationSettled = func() {
		select {
		case done <- struct{}{}:
		default:
		}
	}
	return done
}

func waitForNavSettled(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the navigation fetch to settle")
	}
}

// treeTeamNames lists the team nodes on screen, in order.
func treeTeamNames(app *App) []string {
	root := app.navigationTree.GetRoot()
	if root == nil {
		return nil
	}
	var names []string
	for _, child := range root.GetChildren() {
		if nav, ok := child.GetReference().(*NavigationNode); ok && nav.IsTeam {
			names = append(names, nav.Text)
		}
	}
	return names
}

// findTreeTeamNode returns the tree node for a team id.
func findTreeTeamNode(app *App, teamID string) *tview.TreeNode {
	root := app.navigationTree.GetRoot()
	if root == nil {
		return nil
	}
	for _, child := range root.GetChildren() {
		if nav, ok := child.GetReference().(*NavigationNode); ok && nav.IsTeam && nav.TeamID == teamID {
			return child
		}
	}
	return nil
}

// TestLoadInitialDataPaintsCachedTreeBeforeTheFetch is the point of the cache:
// the sidebar and the issue list are up while the network request is still out.
func TestLoadInitialDataPaintsCachedTreeBeforeTheFetch(t *testing.T) {
	app, release, path := newNavCacheTestApp(t, config.Config{}, defaultNavTeams(), nil)
	installNavCache(t, app, path, navCacheTestWorkspace, cache.NavData{Teams: defaultNavTeams()})
	refreshDone := installRefreshCompletionHook(app)
	settled := installNavSettledHook(app)

	app.loadInitialData()
	// The cached tree drives the startup refresh, so this returns while the
	// teams fetch above is still blocked.
	waitForRefreshCompletion(t, refreshDone)

	if names := treeTeamNames(app); len(names) != 2 || names[0] != "Engineering" {
		t.Fatalf("team nodes = %v, want the two cached teams", names)
	}

	close(release)
	waitForNavSettled(t, settled)
}

// TestLoadInitialDataLeavesTheCursorOnAnUnchangedRefetch is the other half of
// the promise: a user who has started navigating must not be moved when the
// fetch confirms what the cache already painted.
func TestLoadInitialDataLeavesTheCursorOnAnUnchangedRefetch(t *testing.T) {
	app, release, path := newNavCacheTestApp(t, config.Config{}, defaultNavTeams(), nil)
	installNavCache(t, app, path, navCacheTestWorkspace, cache.NavData{Teams: defaultNavTeams()})
	refreshDone := installRefreshCompletionHook(app)
	settled := installNavSettledHook(app)

	app.loadInitialData()
	waitForRefreshCompletion(t, refreshDone)

	// The user moves to a team while the fetch is still out.
	teamNode := findTreeTeamNode(app, "team-2")
	if teamNode == nil {
		t.Fatal("cached tree has no node for team-2")
	}
	app.navigationTree.SetCurrentNode(teamNode)

	close(release)
	waitForNavSettled(t, settled)

	nav := currentNavigationNode(t, app)
	if !nav.IsTeam || nav.TeamID != "team-2" {
		t.Fatalf("current node = %+v, want the team the user moved to", nav)
	}
}

// TestLoadInitialDataRebuildsWhenTheFetchDisagrees covers the launch after a
// team was added: the tree has to pick the new team up, and the user has to end
// up back on the list they were reading.
func TestLoadInitialDataRebuildsWhenTheFetchDisagrees(t *testing.T) {
	cached := []linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}}
	app, release, path := newNavCacheTestApp(t, config.Config{DefaultTeam: "ENG"}, defaultNavTeams(), nil)
	installNavCache(t, app, path, navCacheTestWorkspace, cache.NavData{Teams: cached})
	refreshDone := installRefreshCompletionHook(app)

	app.loadInitialData()
	waitForRefreshCompletion(t, refreshDone)
	if names := treeTeamNames(app); len(names) != 1 {
		t.Fatalf("team nodes = %v, want only the cached team", names)
	}

	close(release)
	// The rebuild re-resolves the place, which runs a second refresh.
	waitForRefreshCompletion(t, refreshDone)

	if names := treeTeamNames(app); len(names) != 2 || names[1] != "Nexa" {
		t.Fatalf("team nodes = %v, want both fetched teams", names)
	}
	nav := currentNavigationNode(t, app)
	if !nav.IsTeam || nav.TeamID != "team-1" {
		t.Fatalf("current node = %+v, want the team the user was on", nav)
	}
}

// TestLoadInitialDataRecordsTheFetchedTree verifies the next launch has
// something to paint from.
func TestLoadInitialDataRecordsTheFetchedTree(t *testing.T) {
	favorites := []linearapi.Favorite{{ID: "fav-1", Type: "project", ProjectID: "proj-1", ProjectName: "Website"}}
	app, release, path := newNavCacheTestApp(t, config.Config{}, defaultNavTeams(), favorites)
	app.UseNavCache(path, cache.NavFile{})
	settled := installNavSettledHook(app)

	app.loadInitialData()
	close(release)
	waitForNavSettled(t, settled)

	file, err := cache.LoadNav(path)
	if err != nil {
		t.Fatalf("LoadNav: %v", err)
	}
	data, ok := file.DataFor(navCacheTestWorkspace)
	if !ok {
		t.Fatal("nothing was recorded for the active workspace")
	}
	if len(data.Teams) != 2 || len(data.Favorites) != 1 {
		t.Fatalf("recorded %d teams and %d favorites, want 2 and 1", len(data.Teams), len(data.Favorites))
	}
}

// TestLoadInitialDataKeepsTheCachedTreeWhenTheFetchFails covers an offline
// launch: the tree stays up rather than emptying itself.
func TestLoadInitialDataKeepsTheCachedTreeWhenTheFetchFails(t *testing.T) {
	app, release, path := newNavCacheTestApp(t, config.Config{}, nil, nil)
	app.fetchTeamsFunc = func(context.Context) ([]linearapi.Team, error) {
		<-release
		return nil, context.DeadlineExceeded
	}
	installNavCache(t, app, path, navCacheTestWorkspace, cache.NavData{Teams: defaultNavTeams()})
	refreshDone := installRefreshCompletionHook(app)
	settled := installNavSettledHook(app)

	app.loadInitialData()
	waitForRefreshCompletion(t, refreshDone)
	close(release)
	waitForNavSettled(t, settled)

	if names := treeTeamNames(app); len(names) != 2 {
		t.Fatalf("team nodes = %v, want the cached teams to survive the failure", names)
	}
}

func TestNavDataUnchanged(t *testing.T) {
	teams := defaultNavTeams()
	favorites := []linearapi.Favorite{{ID: "fav-1", Type: "project", ProjectID: "proj-1"}}
	cached := cache.NavData{Teams: teams, Favorites: favorites}

	tests := []struct {
		name      string
		teams     []linearapi.Team
		favorites []linearapi.Favorite
		want      bool
	}{
		{name: "same tree", teams: teams, favorites: favorites, want: true},
		{name: "team renamed", teams: []linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Eng"}, teams[1]}, favorites: favorites},
		{name: "team added", teams: append(append([]linearapi.Team(nil), teams...), linearapi.Team{ID: "team-3"}), favorites: favorites},
		{name: "favorite removed", teams: teams},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := navDataUnchanged(cached, tc.teams, tc.favorites); got != tc.want {
				t.Fatalf("navDataUnchanged() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestLoadInitialDataKeepsTheCacheWhenFavoritesFail covers the half-failure the
// tree tolerates: favorites are not fatal to rendering, but a copy missing them
// must not reach disk or the next launch paints a Favorites-less sidebar.
func TestLoadInitialDataKeepsTheCacheWhenFavoritesFail(t *testing.T) {
	favorites := []linearapi.Favorite{{ID: "fav-1", Type: "project", ProjectID: "proj-1", ProjectName: "Website"}}
	app, release, path := newNavCacheTestApp(t, config.Config{}, defaultNavTeams(), nil)
	app.fetchFavoritesFunc = func(context.Context) ([]linearapi.Favorite, error) {
		<-release
		return nil, context.DeadlineExceeded
	}
	installNavCache(t, app, path, navCacheTestWorkspace, cache.NavData{Teams: defaultNavTeams(), Favorites: favorites})
	refreshDone := installRefreshCompletionHook(app)
	settled := installNavSettledHook(app)

	app.loadInitialData()
	waitForRefreshCompletion(t, refreshDone)
	close(release)
	waitForNavSettled(t, settled)

	file, err := cache.LoadNav(path)
	if err != nil {
		t.Fatalf("LoadNav: %v", err)
	}
	data, _ := file.DataFor(navCacheTestWorkspace)
	if len(data.Favorites) != 1 {
		t.Fatalf("cached favorites = %d, want the previous copy left alone", len(data.Favorites))
	}
	if app.favoritesGroup == nil {
		t.Fatal("the Favorites section was dropped from the tree")
	}
}

// TestLoadInitialDataUpdatesTheCacheItHolds covers the second launch inside one
// session: a settings save must not repaint the tree as it was at startup.
func TestLoadInitialDataUpdatesTheCacheItHolds(t *testing.T) {
	cached := []linearapi.Team{{ID: "team-1", Key: "ENG", Name: "Engineering"}}
	app, release, path := newNavCacheTestApp(t, config.Config{}, defaultNavTeams(), nil)
	installNavCache(t, app, path, navCacheTestWorkspace, cache.NavData{Teams: cached})
	refreshDone := installRefreshCompletionHook(app)

	app.loadInitialData()
	waitForRefreshCompletion(t, refreshDone)
	close(release)
	waitForRefreshCompletion(t, refreshDone)

	data, ok := app.cachedNavData()
	if !ok || len(data.Teams) != 2 {
		t.Fatalf("in-memory cache = %+v (ok=%v), want the two fetched teams", data, ok)
	}
}

// TestRebuildKeepsTheUserPutWhenTheirListIsGone covers the place that vanished
// server-side: the rebuild must not also throw the user at the default team.
func TestRebuildKeepsTheUserPutWhenTheirListIsGone(t *testing.T) {
	cached := []linearapi.Team{{ID: "team-9", Key: "OLD", Name: "Retired"}}
	app, release, path := newNavCacheTestApp(t, config.Config{DefaultTeam: "NEX"}, defaultNavTeams(), nil)
	installNavCache(t, app, path, navCacheTestWorkspace, cache.NavData{Teams: cached})
	refreshDone := installRefreshCompletionHook(app)

	app.loadInitialData()
	waitForRefreshCompletion(t, refreshDone)
	if node := findTreeTeamNode(app, "team-9"); node != nil {
		app.navigationTree.SetCurrentNode(node)
		app.onNavigationSelected(node.GetReference().(*NavigationNode), "")
		waitForRefreshCompletion(t, refreshDone)
	}

	close(release)
	waitForRefreshCompletion(t, refreshDone)

	if nav := currentNavigationNode(t, app); nav.IsTeam && nav.TeamID == "team-2" {
		t.Fatal("the rebuild jumped to the configured default team")
	}
}

// TestUnnamedSessionsAreNotCached covers a bare API key or an OAuth session:
// nothing on disk tells two Linear workspaces reached that way apart, so they
// get no entry rather than one they would share.
func TestUnnamedSessionsAreNotCached(t *testing.T) {
	app := newDefaultNavTestApp(t, config.Config{LinearAPIKey: "lin_api_anything"})
	path := filepath.Join(t.TempDir(), "nav-cache.json")
	if err := cache.RecordNav(path, "Praxis", cache.NavData{Teams: defaultNavTeams()}); err != nil {
		t.Fatalf("RecordNav: %v", err)
	}
	file, err := cache.LoadNav(path)
	if err != nil {
		t.Fatalf("LoadNav: %v", err)
	}
	app.UseNavCache(path, file)

	if _, ok := app.cachedNavData(); ok {
		t.Fatal("an unnamed session was handed a cached tree")
	}

	app.navTeams = defaultNavTeams()
	app.recordNavCacheAsync()
	after, err := cache.LoadNav(path)
	if err != nil {
		t.Fatalf("LoadNav: %v", err)
	}
	if len(after.Workspaces) != 1 {
		t.Fatalf("cache holds %d entries, want only the named one", len(after.Workspaces))
	}
}

// TestRecordNavCacheAsyncSkipsATeamlessTree covers the workspace-switch window
// where the tree on screen has outlived its teams: a favorites action there
// would file the old favorites under the new workspace with no teams.
func TestRecordNavCacheAsyncSkipsATeamlessTree(t *testing.T) {
	app := newDefaultNavTestApp(t, config.Config{})
	path := filepath.Join(t.TempDir(), "nav-cache.json")
	installNavCache(t, app, path, navCacheTestWorkspace, cache.NavData{Teams: defaultNavTeams()})

	app.navTeams = nil
	app.favorites = []linearapi.Favorite{{ID: "fav-1", Type: "project", ProjectID: "proj-1"}}
	app.recordNavCacheAsync()

	file, err := cache.LoadNav(path)
	if err != nil {
		t.Fatalf("LoadNav: %v", err)
	}
	data, _ := file.DataFor(navCacheTestWorkspace)
	if len(data.Teams) != 2 {
		t.Fatalf("cached teams = %d, want the recorded tree untouched", len(data.Teams))
	}
}

// TestResetNavigationTreeStopsShowingTheOldWorkspace covers the switch window:
// the sidebar must not keep offering teams the new key cannot resolve.
func TestResetNavigationTreeStopsShowingTheOldWorkspace(t *testing.T) {
	app := newDefaultNavTestApp(t, config.Config{})
	app.rebuildNavigationTree(defaultNavTeams(), nil)

	app.resetNavigationTree()

	if names := treeTeamNames(app); len(names) != 0 {
		t.Fatalf("team nodes = %v, want none after a reset", names)
	}
	if app.navLoadingNode == nil {
		t.Fatal("the tree has no waiting node after a reset")
	}
}
