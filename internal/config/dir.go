package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// dirName is the application's directory under $HOME.
	dirName = ".zen-linear"
	// xdgDirName is the application's directory under $XDG_CONFIG_HOME.
	xdgDirName       = "zen-linear"
	xdgConfigHomeEnv = "XDG_CONFIG_HOME"
)

// Dir returns the application directory: $HOME/.zen-linear.
func Dir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(homeDir, dirName), nil
}

// xdgDir returns $XDG_CONFIG_HOME/zen-linear, defaulting the base to
// $HOME/.config.
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

// DirMode is the app directory's mode. It holds credentials.json beside the
// log and the session, so it is the owner's alone.
const DirMode = 0o700

// EnsureDirFor creates the directory holding path and returns it. A directory
// under Dir() is created at DirMode and tightened to it, since MkdirAll no-ops
// on one that exists and four writers used to race to create this one at 0755.
//
// Anywhere else is created and left alone: the settings file is often written
// to $XDG_CONFIG_HOME/zen-linear, which is not ours to narrow and is usually a
// dotfiles checkout.
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
	// MkdirAll no-ops on a directory that exists, so a 0755 one left by an
	// older build is only tightened here.
	//
	// A refusal is not fatal to the write this was guarding. Not every
	// filesystem a home directory can live on implements chmod, and breaking
	// every settings, credentials and log write on one of them would cost more
	// than the hardening buys.
	_ = os.Chmod(dir, DirMode)
	return dir, nil
}

// ownedDir reports whether dir is the app directory, the only one whose mode
// is ours to enforce.
func ownedDir(dir string) bool {
	appDir, err := Dir()
	if err != nil {
		return false
	}
	return filepath.Clean(dir) == filepath.Clean(appDir)
}
