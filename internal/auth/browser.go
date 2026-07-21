package auth

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/roeyazroel/linear-tui/internal/config"
)

// OpenURL opens url in the user's default browser.
func OpenURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}

// ClientID returns LINEAR_CLIENT_ID if set, otherwise the embedded default.
func ClientID(defaultID string) string {
	if id := os.Getenv(config.LinearClientIDEnv); id != "" {
		return id
	}
	return defaultID
}
