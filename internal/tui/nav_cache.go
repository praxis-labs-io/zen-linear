package tui

import (
	"slices"

	"github.com/praxis-labs-io/zen-linear/internal/cache"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
)

// UseNavCache installs the navigation tree cached on disk: the path to write
// back to, and the copy this App paints its first tree from.
func (a *App) UseNavCache(path string, file cache.NavFile) {
	a.navCachePath = path
	a.navCache = file
}

// navCacheEnabled reports whether this session's tree can be cached. It needs a
// configured workspace name to file the entry under. A bare API key or an OAuth
// session has none, and nothing else on disk tells two Linear workspaces apart:
// sharing one entry between them would paint the wrong teams, and fingerprinting
// the token to tell them apart would put a derivative of a live credential in a
// cache file. Those sessions launch the way every session did before the cache.
func (a *App) navCacheEnabled() bool {
	return a.navCachePath != "" && a.activeWorkspaceName != ""
}

// cachedNavData returns the tree to paint before the network answers. UI
// thread only.
func (a *App) cachedNavData() (cache.NavData, bool) {
	if !a.navCacheEnabled() {
		return cache.NavData{}, false
	}
	return a.navCache.DataFor(a.activeWorkspaceName)
}

// recordNavCache writes the tree for a cache key and installs it in the copy
// held in memory, so a second loadInitialData in the same session (a settings
// save, a workspace switch) paints what is on disk rather than what was there
// at startup. Call it off the UI thread; the write is a file write. A failure
// is logged and swallowed: the cache only buys a faster first paint, so losing
// it costs nothing the user can act on.
func (a *App) recordNavCache(key string, teams []linearapi.Team, favorites []linearapi.Favorite) {
	if a.navCachePath == "" || key == "" {
		return
	}
	data := cache.NavData{Teams: teams, Favorites: favorites}
	if err := cache.RecordNav(a.navCachePath, key, data); err != nil {
		logger.Warning("tui.nav_cache: failed to record navigation cache path=%s error=%v", a.navCachePath, err)
		return
	}
	a.QueueUpdateDraw(func() {
		a.navCache.Set(key, data)
	})
}

// recordNavCacheAsync writes the tree from a goroutine, for the UI-thread
// callers that must not block on a file write.
func (a *App) recordNavCacheAsync() {
	if !a.navCacheEnabled() {
		return
	}
	// A workspace switch clears navTeams while the outgoing workspace's tree is
	// still on screen and its favorites are still actionable. Recording in that
	// window would file the old favorites under the incoming workspace with no
	// teams at all, which is a sidebar the next launch cannot use.
	if len(a.navTeams) == 0 {
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
