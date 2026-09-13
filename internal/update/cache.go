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

const cacheVersion = 1

const cacheFileName = "update-check.json"

type cacheFile struct {
	Version   int       `json:"version"`
	CheckedAt time.Time `json:"checked_at"`
	LatestTag string    `json:"latest_tag"`
}

func (f cacheFile) fresh(now time.Time, ttl time.Duration) bool {
	if f.LatestTag == "" || f.CheckedAt.IsZero() {
		return false
	}
	if f.CheckedAt.After(now) {
		return false
	}
	return now.Sub(f.CheckedAt) < ttl
}

func Path() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, cacheFileName), nil
}

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

	if err := config.WriteFileAtomic(path, encoded, 0o600); err != nil {
		return fmt.Errorf("write update cache: %w", err)
	}

	return nil
}
