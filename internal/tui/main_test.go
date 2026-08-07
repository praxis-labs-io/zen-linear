package tui

import (
	"os"
	"testing"
	"time"
)

// TestMain parks the spinner's frame loop for the whole package. Tests stub
// queueUpdateDraw to run inline, so a tick lands on the ticker's goroutine and
// writes App state while the test reads it. At an hour no tick ever fires, and
// the panes still paint their message when a load starts, which is what the
// assertions read.
func TestMain(m *testing.M) {
	defaultLoadingFrameInterval = time.Hour
	os.Exit(m.Run())
}
