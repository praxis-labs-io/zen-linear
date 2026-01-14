package logger

import (
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
		_ = Close() //nolint:errcheck // test cleanup
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
		_ = Close() //nolint:errcheck // test cleanup
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
		_ = Close() //nolint:errcheck // test cleanup
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
	if defaultLogger != nil && defaultLogger.file != nil && !defaultLogger.closed {
		_ = defaultLogger.file.Close()
	}
	defaultLogger = nil
	once = sync.Once{}
}
