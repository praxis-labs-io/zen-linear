package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data through a temp file in the target's directory and
// renames it over the target, creating parent directories as needed. The files
// under the application directory are each rebuilt whole on every write, so an
// interrupted in-place write costs the entire record rather than the tail of
// it. Callers pass their own mode; a rename also narrows a file an earlier
// build left wide open, which os.WriteFile would not.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if path == "" {
		return fmt.Errorf("write path is empty")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tempName := temp.Name()
	defer func() {
		// Harmless once the rename succeeded; the temp file is gone by then.
		_ = os.Remove(tempName)
	}()

	if _, err := temp.Write(data); err != nil {
		// The write error is the one worth reporting; the temp file is removed
		// either way by the deferred cleanup.
		_ = temp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := temp.Chmod(perm); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set permissions on %s: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}

	return nil
}
