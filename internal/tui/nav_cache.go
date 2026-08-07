package tui

import (
	"slices"

	"github.com/zen-linear/zen-linear/internal/cache"
	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// UseNavCache installs the navigation tree cached on disk: the path to write
// back to, and the copy this App paints its first tree from.
func (a *App) UseNavCache(path string, file cache.NavFile) {
	a.navCachePath = path
	a.navCache = file
}

// cachedNavData returns the tree to paint before the network answers. UI
// thread only.
func (a *App) cachedNavData() (cache.NavData, bool) {
	if a.navCachePath == "" {
		return cache.NavData{}, false
	}
	return a.navCache.DataFor(a.activeWorkspaceName)
}

// recordNavCache writes the current tree for the active workspace. Call it off
// the UI thread; the write is a file write. A failure is logged and swallowed:
// the cache only buys a faster first paint, so losing it costs nothing the user
// can act on.
func (a *App) recordNavCache(workspace string, teams []linearapi.Team, favorites []linearapi.Favorite) {
	if a.navCachePath == "" {
		return
	}
	data := cache.NavData{Teams: teams, Favorites: favorites}
	if err := cache.RecordNav(a.navCachePath, workspace, data); err != nil {
		logger.Warning("tui.nav_cache: failed to record navigation cache path=%s error=%v", a.navCachePath, err)
	}
}

// recordNavCacheAsync writes the tree from a goroutine, for the UI-thread
// callers that must not block on a file write.
func (a *App) recordNavCacheAsync() {
	if a.navCachePath == "" {
		return
	}
	workspace := a.activeWorkspaceName
	teams := slices.Clone(a.navTeams)
	favorites := slices.Clone(a.favorites)
	go a.recordNavCache(workspace, teams, favorites)
}

// notifyNavigationSettled reports that the launch fetch has been applied.
func (a *App) notifyNavigationSettled() {
	if a.navigationSettled != nil {
		a.navigationSettled()
	}
}

// navDataUnchanged reports whether a freshly fetched tree matches the cached
// copy already on screen. When it does there is nothing to rebuild, which is
// what keeps a refetch from moving the cursor out from under the user.
func navDataUnchanged(cached cache.NavData, teams []linearapi.Team, favorites []linearapi.Favorite) bool {
	return slices.Equal(cached.Teams, teams) && slices.Equal(cached.Favorites, favorites)
}
