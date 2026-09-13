package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	dirName          = ".zen-linear"
	xdgDirName       = "zen-linear"
	xdgConfigHomeEnv = "XDG_CONFIG_HOME"
)

func Dir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(homeDir, dirName), nil
}

func xdgDir() (string, error) {
	if base := os.Getenv(xdgConfigHomeEnv); base != "" {
		return filepath.Join(base, xdgDirName), nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", xdgDirName), nil
}

const DirMode = 0o700

// EnsureDirFor creates the directory holding path and returns it. Under Dir()
// it is also tightened to DirMode; anywhere else is left alone.
func EnsureDirFor(path string) (string, error) {
	dir := filepath.Dir(path)
	owned := ownedDir(dir)

	mode := os.FileMode(0o755)
	if owned {
		mode = DirMode
	}
	if err := os.MkdirAll(dir, mode); err != nil {
		return "", fmt.Errorf("create directory %s: %w", dir, err)
	}
	if !owned {
		return dir, nil
	}
	_ = os.Chmod(dir, DirMode)
	return dir, nil
}

func ownedDir(dir string) bool {
	appDir, err := Dir()
	if err != nil {
		return false
	}
	return filepath.Clean(dir) == filepath.Clean(appDir)
}
