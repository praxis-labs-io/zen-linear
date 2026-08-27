package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogger(t *testing.T) {
	// Reset global state
	resetLogger()

	// Create temporary directory for test logs
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Initialize logger
	err := Init(logPath, LevelDebug)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	defer func() {
		_ = Close()
	}()

	// Write test logs
	Debug("Debug message: %s", "test debug")
	Info("Info message: %s", "test info")
	Warning("Warning message: %s", "test warning")
	Error("Error message: %s", "test error")

	// Close to ensure all writes are flushed
	if err := Close(); err != nil {
		t.Fatalf("Failed to close logger: %v", err)
	}

	// Read log file
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	// Verify session markers
	if !strings.Contains(logContent, "=== Session started ===") {
		t.Error("Log file should contain session start marker")
	}
	if !strings.Contains(logContent, "=== Session ended ===") {
		t.Error("Log file should contain session end marker")
	}

	// Verify log levels
	if !strings.Contains(logContent, "DEBUG: Debug message: test debug") {
		t.Error("Log file should contain debug message")
	}
	if !strings.Contains(logContent, "INFO: Info message: test info") {
		t.Error("Log file should contain info message")
	}
	if !strings.Contains(logContent, "WARN: Warning message: test warning") {
		t.Error("Log file should contain warning message")
	}
	if !strings.Contains(logContent, "ERROR: Error message: test error") {
		t.Error("Log file should contain error message")
	}
}

func TestLoggerWithMinLevel(t *testing.T) {
	// Reset global state
	resetLogger()

	// Create temporary directory for test logs
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test-min-level.log")

	// Initialize logger with Warning minimum level
	err := Init(logPath, LevelWarning)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	defer func() {
		_ = Close()
	}()

	// Write test logs
	Debug("Should not appear")
	Info("Should not appear")
	Warning("Should appear")
	Error("Should appear")

	// Close to ensure all writes are flushed
	if err := Close(); err != nil {
		t.Fatalf("Failed to close logger: %v", err)
	}

	// Read log file
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	// Verify filtered messages
	if strings.Contains(logContent, "DEBUG:") {
		t.Error("Log file should not contain debug messages when min level is Warning")
	}
	if strings.Contains(logContent, "INFO:") && !strings.Contains(logContent, "Session") {
		t.Error("Log file should not contain info messages when min level is Warning")
	}
	if !strings.Contains(logContent, "WARN: Should appear") {
		t.Error("Log file should contain warning message")
	}
	if !strings.Contains(logContent, "ERROR: Should appear") {
		t.Error("Log file should contain error message")
	}
}

// TestReinitLogger verifies logging switches to the new file after reinit.
func TestReinitLogger(t *testing.T) {
	// Reset global state
	resetLogger()

	tmpDir := t.TempDir()
	firstPath := filepath.Join(tmpDir, "first.log")
	secondPath := filepath.Join(tmpDir, "second.log")

	if err := Init(firstPath, LevelInfo); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	Info("First log entry")

	if err := Reinit(secondPath, LevelError); err != nil {
		t.Fatalf("Reinit() error: %v", err)
	}

	Info("Should be filtered")
	Error("Second log entry")

	if err := Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	firstContent, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("read first log file: %v", err)
	}

	secondContent, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatalf("read second log file: %v", err)
	}

	firstLog := string(firstContent)
	if !strings.Contains(firstLog, "INFO: First log entry") {
		t.Error("First log file should contain initial entry")
	}
	if strings.Contains(firstLog, "Second log entry") {
		t.Error("First log file should not contain entries after reinit")
	}

	secondLog := string(secondContent)
	if !strings.Contains(secondLog, "ERROR: Second log entry") {
		t.Error("Second log file should contain new entry")
	}
	if strings.Contains(secondLog, "INFO: Should be filtered") {
		t.Error("Second log file should honor the new minimum level")
	}
}

// A log path the user cannot write used to take the rest of the session down
// with it: the old file was already closed, so nothing landed anywhere, not
// even the report of the failure.
func TestReinitToAnUnwritablePathKeepsLogging(t *testing.T) {
	resetLogger()

	tmpDir := t.TempDir()
	firstPath := filepath.Join(tmpDir, "first.log")
	if err := Init(firstPath, LevelInfo); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// A regular file where the new path wants a directory.
	blocker := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blocker, nil, 0644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	if err := Reinit(filepath.Join(blocker, "nested", "app.log"), LevelInfo); err == nil {
		t.Fatal("Reinit() to an unwritable path returned no error")
	}

	Info("Still logging")
	if err := Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	content, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("read first log file: %v", err)
	}
	if !strings.Contains(string(content), "INFO: Still logging") {
		t.Errorf("entries after a failed reinit went nowhere: %q", string(content))
	}
}

// A settings save or a workspace switch reinitializes the logger from the UI
// thread while background fetches are still logging. The race detector is the
// assertion here.
func TestReinitWhileOtherGoroutinesLog(t *testing.T) {
	resetLogger()

	tmpDir := t.TempDir()
	if err := Init(filepath.Join(tmpDir, "first.log"), LevelDebug); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	var wg sync.WaitGroup
	for writer := 0; writer < 8; writer++ {
		wg.Add(1)
		go func(writer int) {
			defer wg.Done()
			for entry := 0; entry < 50; entry++ {
				Debug("writer %d entry %d", writer, entry)
			}
		}(writer)
	}

	for reinit := 0; reinit < 5; reinit++ {
		path := filepath.Join(tmpDir, fmt.Sprintf("reinit-%d.log", reinit))
		if err := Reinit(path, LevelDebug); err != nil {
			t.Errorf("Reinit() error: %v", err)
		}
	}
	wg.Wait()

	if err := Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
}

func TestLoggerDisabled(t *testing.T) {
	// Reset global state
	resetLogger()

	// Initialize logger with empty path (disabled)
	err := Init("", LevelDebug)
	if err != nil {
		t.Fatalf("Failed to initialize disabled logger: %v", err)
	}

	// These should not panic or error
	Debug("Test debug")
	Info("Test info")
	Warning("Test warning")
	Error("Test error")

	// Close should not error
	if err := Close(); err != nil {
		t.Errorf("Close should not error for disabled logger: %v", err)
	}
}

func TestErrorWithErr(t *testing.T) {
	// Reset global state
	resetLogger()

	// Create temporary directory for test logs
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test-error.log")

	// Initialize logger
	err := Init(logPath, LevelDebug)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	defer func() {
		_ = Close()
	}()

	// Write error with context
	testErr := os.ErrNotExist
	ErrorWithErr(testErr, "Failed to open file")

	// Give it a moment to write
	time.Sleep(10 * time.Millisecond)

	// Close to ensure all writes are flushed
	if err := Close(); err != nil {
		t.Fatalf("Failed to close logger: %v", err)
	}

	// Read log file
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	// Verify error message with context
	if !strings.Contains(logContent, "ERROR: Failed to open file") {
		t.Error("Log file should contain error message")
	}
	if !strings.Contains(logContent, "file does not exist") {
		t.Error("Log file should contain error details")
	}
}

func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarning, "WARN"},
		{LevelError, "ERROR"},
		{LogLevel(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("LogLevel.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// resetLogger resets the global logger state for testing.
func resetLogger() {
	globalMu.Lock()
	defer globalMu.Unlock()

	if defaultLogger != nil {
		_ = defaultLogger.close()
	}
	defaultLogger = nil
}

// A log path that cannot be opened used to end the process: Init failed, main
// printed the error and returned 1, and the setting that caused it lived in a
// modal the app never got far enough to show. Logging is diagnostics, so it
// degrades instead.
func TestStartFallsBackWhenThePathCannotBeOpened(t *testing.T) {
	resetLogger()

	tmpDir := t.TempDir()
	// A regular file where the refused path wants a directory.
	blocker := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blocker, nil, 0644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	refused := filepath.Join(blocker, "nested", "app.log")
	fallback := filepath.Join(tmpDir, "fallback.log")

	opened, warning := Start(refused, fallback, LevelInfo)
	if opened != fallback {
		t.Errorf("Start() opened %q, want %q", opened, fallback)
	}
	if !strings.Contains(warning, refused) {
		t.Errorf("warning %q does not name the refused path %q", warning, refused)
	}

	Info("Logging after the fallback")
	if err := Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	content, err := os.ReadFile(fallback)
	if err != nil {
		t.Fatalf("read fallback log file: %v", err)
	}
	if !strings.Contains(string(content), "Logging after the fallback") {
		t.Errorf("fallback log missing the entry, got %q", content)
	}
}

func TestStartWithNowhereToLogLeavesLoggingOff(t *testing.T) {
	resetLogger()

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	refused := filepath.Join(blocker, "nested", "app.log")

	opened, warning := Start(refused, filepath.Join(blocker, "also", "app.log"), LevelInfo)
	if opened != "" {
		t.Errorf("Start() opened %q, want logging off", opened)
	}
	if !strings.Contains(warning, refused) {
		t.Errorf("warning %q does not name the refused path %q", warning, refused)
	}

	// Logging off is still a working logger, not a nil one.
	Info("Goes nowhere")
	if err := Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
}

func TestStartOnAWritablePathReportsNoWarning(t *testing.T) {
	resetLogger()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nested", "app.log")

	opened, warning := Start(path, filepath.Join(tmpDir, "fallback.log"), LevelInfo)
	if opened != path {
		t.Errorf("Start() opened %q, want %q", opened, path)
	}
	if warning != "" {
		t.Errorf("warning = %q, want none", warning)
	}
	if err := Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
}

// Restart must not fall back over a close failure on the file being left
// behind: the new path opened, which is the only thing the caller retries on.
func TestRestartMovesToTheNewPath(t *testing.T) {
	resetLogger()

	tmpDir := t.TempDir()
	firstPath := filepath.Join(tmpDir, "first.log")
	secondPath := filepath.Join(tmpDir, "second.log")
	if err := Init(firstPath, LevelInfo); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	opened, warning := Restart(secondPath, filepath.Join(tmpDir, "fallback.log"), LevelInfo)
	if opened != secondPath {
		t.Errorf("Restart() opened %q, want %q", opened, secondPath)
	}
	if warning != "" {
		t.Errorf("warning = %q, want none", warning)
	}

	Info("After the restart")
	if err := Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	content, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatalf("read second log file: %v", err)
	}
	if !strings.Contains(string(content), "After the restart") {
		t.Errorf("second log missing the entry, got %q", content)
	}
}

// Init is a no-op once a logger exists, so a naive Start would report a path it
// never opened and leave the session logging to the previous file.
func TestStartReplacesALoggerAlreadyRunning(t *testing.T) {
	resetLogger()

	tmpDir := t.TempDir()
	firstPath := filepath.Join(tmpDir, "first.log")
	secondPath := filepath.Join(tmpDir, "second.log")
	if err := Init(firstPath, LevelInfo); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	opened, warning := Start(secondPath, filepath.Join(tmpDir, "fallback.log"), LevelInfo)
	if opened != secondPath {
		t.Errorf("Start() opened %q, want %q", opened, secondPath)
	}
	if warning != "" {
		t.Errorf("warning = %q, want none", warning)
	}

	Info("Only the second log")
	if err := Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatalf("read second log file: %v", err)
	}
	if !strings.Contains(string(second), "Only the second log") {
		t.Errorf("second log missing the entry, got %q", second)
	}
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("read first log file: %v", err)
	}
	if strings.Contains(string(first), "Only the second log") {
		t.Error("Start() reported the new path but kept logging to the old one")
	}
}

// A log that is already at the cap when it opens rotates on the first write
// rather than growing forever. Truncate makes the fixture sparse, so the test
// costs no real bytes.
func TestAFullLogRotatesAndKeepsWriting(t *testing.T) {
	resetLogger()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "app.log")
	if err := os.WriteFile(logPath, []byte("old contents\n"), 0644); err != nil {
		t.Fatalf("seed log: %v", err)
	}
	if err := os.Truncate(logPath, maxLogSize); err != nil {
		t.Fatalf("grow log to the cap: %v", err)
	}

	if err := Init(logPath, LevelInfo); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	Info("After the rotation")
	if err := Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	rotated, err := os.ReadFile(logPath + rotatedSuffix)
	if err != nil {
		t.Fatalf("read the rotated log: %v", err)
	}
	if !strings.HasPrefix(string(rotated), "old contents") {
		t.Errorf("rotated log lost what it held, got %q", rotated[:min(32, len(rotated))])
	}

	current, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read the current log: %v", err)
	}
	if !strings.Contains(string(current), "After the rotation") {
		t.Errorf("current log missing the entry, got %q", current)
	}
	if int64(len(current)) >= maxLogSize {
		t.Errorf("current log is %d bytes, want a fresh one", len(current))
	}
}

// One generation, not a chain: a second rotation replaces app.log.1 rather
// than shifting it to app.log.2.
func TestRotatingTwiceKeepsOneGeneration(t *testing.T) {
	resetLogger()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "app.log")

	for _, marker := range []string{"first round", "second round"} {
		if err := os.WriteFile(logPath, []byte(marker+"\n"), 0644); err != nil {
			t.Fatalf("seed log: %v", err)
		}
		if err := os.Truncate(logPath, maxLogSize); err != nil {
			t.Fatalf("grow log to the cap: %v", err)
		}
		resetLogger()
		if err := Init(logPath, LevelInfo); err != nil {
			t.Fatalf("Init() error: %v", err)
		}
		Info("rotated")
		if err := Close(); err != nil {
			t.Fatalf("Close() error: %v", err)
		}
	}

	rotated, err := os.ReadFile(logPath + rotatedSuffix)
	if err != nil {
		t.Fatalf("read the rotated log: %v", err)
	}
	if !strings.HasPrefix(string(rotated), "second round") {
		t.Error("app.log.1 was not replaced by the newer generation")
	}
	if _, err := os.Stat(logPath + rotatedSuffix + rotatedSuffix); !os.IsNotExist(err) {
		t.Error("rotation chained to app.log.1.1 instead of keeping one generation")
	}
}

func TestALogUnderTheCapDoesNotRotate(t *testing.T) {
	resetLogger()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "app.log")

	if err := Init(logPath, LevelInfo); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	Info("Well under the cap")
	if err := Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	if _, err := os.Stat(logPath + rotatedSuffix); !os.IsNotExist(err) {
		t.Errorf("a short log rotated: %v", err)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(content), "Well under the cap") {
		t.Errorf("log missing the entry, got %q", content)
	}
}

// A rotation whose rename fails reopens the same full log rather than starting
// a counter from nought against it, which would let the file grow another whole
// cap before the next attempt, and again after that.
func TestAFailedRotationDoesNotResetTheCounter(t *testing.T) {
	resetLogger()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "app.log")
	if err := os.WriteFile(logPath, nil, 0644); err != nil {
		t.Fatalf("seed log: %v", err)
	}
	if err := os.Truncate(logPath, maxLogSize); err != nil {
		t.Fatalf("grow log to the cap: %v", err)
	}

	// A directory where the rotation wants to put the file, so Rename fails.
	if err := os.Mkdir(logPath+rotatedSuffix, 0o755); err != nil {
		t.Fatalf("block the rotation target: %v", err)
	}

	if err := Init(logPath, LevelInfo); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	Info("Still logging after a refused rotation")

	current()
	globalMu.RLock()
	size := defaultLogger.size
	globalMu.RUnlock()
	if size < maxLogSize {
		t.Errorf("size = %d after a refused rotation, want the full log's %d", size, int64(maxLogSize))
	}

	if err := Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(content), "Still logging after a refused rotation") {
		t.Error("a refused rotation stopped the session logging")
	}
}
