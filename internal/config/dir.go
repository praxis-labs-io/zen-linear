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
