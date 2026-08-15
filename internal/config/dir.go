package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// dirName is the application's directory under $HOME.
const dirName = ".zen-linear"

// Dir returns the application directory: $HOME/.zen-linear.
func Dir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(homeDir, dirName), nil
}
