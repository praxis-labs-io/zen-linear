package tui

import (
	"os"
	"testing"
	"time"
)

// Tests stub queueUpdateDraw inline, so a spinner tick would write App state on the ticker's goroutine mid-assertion.
func TestMain(m *testing.M) {
	defaultLoadingFrameInterval = time.Hour
	os.Exit(m.Run())
}
