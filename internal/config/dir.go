package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// dirName is the application's directory under $HOME.
	dirName = ".zen-linear"
	// legacyDirName is the pre-rename directory, migrated on startup.
	legacyDirName = ".linear-tui"
)

// Dir returns the application directory: $HOME/.zen-linear.
func Dir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(homeDir, dirName), nil
}

// MigrateLegacyDir moves $HOME/.linear-tui to $HOME/.zen-linear when the old
// directory exists and the new one does not. A rename keeps symlinked config
// files pointing at their original targets, which a copy would not.
func MigrateLegacyDir() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	legacy := filepath.Join(homeDir, legacyDirName)
	current := filepath.Join(homeDir, dirName)

	if _, err := os.Stat(legacy); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat legacy config dir: %w", err)
	}

	// A present target means the user already runs the new layout; leaving the
	// legacy directory alone is safer than merging two config trees.
	if _, err := os.Stat(current); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat config dir: %w", err)
	}

	if err := os.Rename(legacy, current); err != nil {
		return fmt.Errorf("migrate config dir %s to %s: %w", legacy, current, err)
	}
	return nil
}

// rewriteLegacyPath maps a path still rooted in the pre-rename directory onto
// the current one. The rename moved the files but left absolute paths inside
// the config behind, so a stale log_file silently recreates ~/.linear-tui and
// writes there. Applied on load rather than during the migration, because the
// migration has already run for anyone carrying a stale path.
func rewriteLegacyPath(path string) string {
	if path == "" {
		return path
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	legacy := filepath.Join(homeDir, legacyDirName)
	cleaned := filepath.Clean(path)
	if cleaned != legacy && !strings.HasPrefix(cleaned, legacy+string(filepath.Separator)) {
		return path
	}
	return filepath.Join(homeDir, dirName, strings.TrimPrefix(cleaned, legacy))
}
