package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/config"
)

// cacheVersion is the schema this build reads and writes. A file at any other
// version is discarded rather than migrated, the way the navigation cache is:
// throwing it away costs one request.
const cacheVersion = 1

// cacheFileName is the update check's record under the application directory.
const cacheFileName = "update-check.json"

// cacheFile is what the last check found and when. It holds no more than that:
// whether an update is available is derived from the running version, which
// changes under the file every time the user upgrades.
type cacheFile struct {
	Version   int       `json:"version"`
	CheckedAt time.Time `json:"checked_at"`
	LatestTag string    `json:"latest_tag"`
}

// fresh reports whether a record still answers for a check made now.
func (f cacheFile) fresh(now time.Time, ttl time.Duration) bool {
	if f.LatestTag == "" || f.CheckedAt.IsZero() {
		return false
	}
	// A timestamp in the future is a clock that moved, not a fresh check. Left
	// alone it would hold the cache shut until the clock caught up.
	if f.CheckedAt.After(now) {
		return false
	}
	return now.Sub(f.CheckedAt) < ttl
}

// Path returns the update check's cache path under the application directory.
func Path() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, cacheFileName), nil
}

// loadCache reads the last check. A missing, unreadable, or stale-schema file
// is not an error: it means this launch asks GitHub, which is what every
// launch would do without the cache.
func loadCache(path string) cacheFile {
	if path == "" {
		return cacheFile{}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cacheFile{}
	}

	var file cacheFile
	if err := json.Unmarshal(data, &file); err != nil {
		return cacheFile{}
	}
	if file.Version != cacheVersion {
		return cacheFile{}
	}

	return file
}

// recordCache stores what the check found. A path that cannot be written is
// reported rather than swallowed, though the caller treats it as nothing worth
// telling the user: the cost is one request next launch.
func recordCache(path, tag string, at time.Time) error {
	if path == "" {
		return errors.New("update cache path is empty")
	}

	encoded, err := json.MarshalIndent(cacheFile{
		Version:   cacheVersion,
		CheckedAt: at,
		LatestTag: tag,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal update cache: %w", err)
	}
	encoded = append(encoded, '\n')

	// 0600 matches the session and navigation files beside it, though this one
	// holds nothing about the user at all.
	if err := config.WriteFileAtomic(path, encoded, 0o600); err != nil {
		return fmt.Errorf("write update cache: %w", err)
	}

	return nil
}
