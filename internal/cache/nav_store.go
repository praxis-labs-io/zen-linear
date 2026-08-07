package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// navVersion is the schema version this build reads and writes. A file at any
// other version is discarded rather than migrated: it is a cache, so throwing
// it away costs one slow launch.
const navVersion = 1

// navFileName is the navigation cache under the application directory.
const navFileName = "nav-cache.json"

// NavData is what the navigation tree is built from. The linearapi structs go
// to disk as they are rather than through a mirror of their fields; Favorite
// alone carries two dozen, and navVersion covers the drift a hand-written
// mirror would have caught.
type NavData struct {
	Teams     []linearapi.Team     `json:"teams,omitempty"`
	Favorites []linearapi.Favorite `json:"favorites,omitempty"`
}

// Empty reports whether there is nothing worth painting a tree from.
func (d NavData) Empty() bool {
	return len(d.Teams) == 0 && len(d.Favorites) == 0
}

// NavFile is the on-disk cache. Entries are keyed by workspace because every id
// inside one is workspace-scoped, so a single record would paint the wrong
// tree on every switch away and back.
type NavFile struct {
	Version    int                `json:"version"`
	Workspaces map[string]NavData `json:"workspaces,omitempty"`
}

// NavPath returns the navigation cache path under the application directory.
func NavPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, navFileName), nil
}

// DataFor returns the cached tree for a key. The key is a workspace name where
// there is one; callers without one pass something else that identifies the
// Linear workspace, since two sessions sharing a key would paint each other's
// teams.
func (f NavFile) DataFor(key string) (NavData, bool) {
	data, ok := f.Workspaces[navWorkspaceKey(key)]
	if !ok || data.Empty() {
		return NavData{}, false
	}
	return data, true
}

// Set installs one entry in a loaded file without touching the others.
func (f *NavFile) Set(key string, data NavData) {
	if f.Workspaces == nil {
		f.Workspaces = make(map[string]NavData)
	}
	f.Version = navVersion
	f.Workspaces[navWorkspaceKey(key)] = data
}

// LoadNav reads the navigation cache. A missing, unreadable, or stale-schema
// file is not an error: it means the next launch paints the tree from the
// network, which is what every launch did before this cache existed.
func LoadNav(path string) (NavFile, error) {
	if path == "" {
		return NavFile{}, fmt.Errorf("navigation cache path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return NavFile{}, nil
		}
		return NavFile{}, fmt.Errorf("read navigation cache: %w", err)
	}

	var file NavFile
	if err := json.Unmarshal(data, &file); err != nil {
		return NavFile{}, fmt.Errorf("parse navigation cache: %w", err)
	}
	if file.Version != navVersion {
		return NavFile{}, nil
	}

	return file, nil
}

// RecordNav stores one entry's tree, keeping the others. An unreadable file is
// replaced rather than propagated: a corrupt cache must not block the next
// write.
func RecordNav(path, key string, data NavData) error {
	file, err := LoadNav(path)
	if err != nil {
		file = NavFile{}
	}
	file.Set(key, data)

	encoded, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal navigation cache: %w", err)
	}
	encoded = append(encoded, '\n')

	// Mode 0600 matches the session file: team and favorite names say what the
	// user works on.
	if err := config.WriteFileAtomic(path, encoded, 0o600); err != nil {
		return fmt.Errorf("write navigation cache: %w", err)
	}

	return nil
}

// navWorkspaceKey normalizes a workspace name into a map key so a rename of
// case or padding in the config still finds the cached tree.
func navWorkspaceKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
