package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

const navVersion = 1

const navFileName = "nav-cache.json"

type NavData struct {
	Teams     []linearapi.Team     `json:"teams,omitempty"`
	Favorites []linearapi.Favorite `json:"favorites,omitempty"`
}

func (d NavData) Empty() bool {
	return len(d.Teams) == 0 && len(d.Favorites) == 0
}

type NavFile struct {
	Version    int                `json:"version"`
	Workspaces map[string]NavData `json:"workspaces,omitempty"`
}

func NavPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, navFileName), nil
}

func (f NavFile) DataFor(key string) (NavData, bool) {
	data, ok := f.Workspaces[navWorkspaceKey(key)]
	if !ok || data.Empty() {
		return NavData{}, false
	}
	return data, true
}

func (f *NavFile) Set(key string, data NavData) {
	if f.Workspaces == nil {
		f.Workspaces = make(map[string]NavData)
	}
	f.Version = navVersion
	f.Workspaces[navWorkspaceKey(key)] = data
}

// LoadNav reads the navigation cache. A missing file or one at another schema
// version returns an empty NavFile and no error.
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

// RecordNav stores one workspace's tree, keeping the others and replacing an
// unreadable file.
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

	if err := config.WriteFileAtomic(path, encoded, 0o600); err != nil {
		return fmt.Errorf("write navigation cache: %w", err)
	}

	return nil
}

func navWorkspaceKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
