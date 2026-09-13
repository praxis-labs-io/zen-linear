package tui

import (
	"slices"

	"github.com/praxis-labs-io/zen-linear/internal/cache"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
)

// UseNavCache sets the cache file to write back to and the copy the first tree paints from.
func (a *App) UseNavCache(path string, file cache.NavFile) {
	a.navCachePath = path
	a.navCache = file
}

// A bare API key or OAuth session has no workspace name, and fingerprinting the token would cache a credential derivative.
func (a *App) navCacheEnabled() bool {
	return a.navCachePath != "" && a.activeWorkspaceName != ""
}

func (a *App) cachedNavData() (cache.NavData, bool) {
	if !a.navCacheEnabled() {
		return cache.NavData{}, false
	}
	return a.navCache.DataFor(a.activeWorkspaceName)
}

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

func (a *App) recordNavCacheAsync() {
	if !a.navCacheEnabled() {
		return
	}
	if len(a.navTeams) == 0 {
		return
	}
	workspace := a.activeWorkspaceName
	teams := slices.Clone(a.navTeams)
	favorites := slices.Clone(a.favorites)
	go a.recordNavCache(workspace, teams, favorites)
}

func (a *App) notifyNavigationSettled() {
	if a.navigationSettled != nil {
		a.navigationSettled()
	}
}

func navDataUnchanged(cached cache.NavData, teams []linearapi.Team, favorites []linearapi.Favorite) bool {
	return slices.Equal(cached.Teams, teams) && slices.Equal(cached.Favorites, favorites)
}
