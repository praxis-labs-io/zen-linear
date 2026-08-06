package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogLevel represents the severity level of a log message.
type LogLevel int

const (
	// LevelDebug is for detailed debugging information.
	LevelDebug LogLevel = iota
	// LevelInfo is for general informational messages.
	LevelInfo
	// LevelWarning is for warning messages.
	LevelWarning
	// LevelError is for error messages.
	LevelError
)

// String returns the string representation of a log level.
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarning:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger provides thread-safe logging to a file.
type Logger struct {
	mu       sync.Mutex
	file     *os.File
	minLevel LogLevel
	enabled  bool
	closed   bool
}

var (
	// globalMu guards defaultLogger. A settings save reinitializes the logger
	// from the UI thread while background fetches are still writing to it.
	globalMu sync.RWMutex
	// defaultLogger is the global logger instance.
	defaultLogger *Logger
)

// Init initializes the global logger with the specified log file path.
// If logPath is empty, logging is disabled. Initializing twice is a no-op.
// Returns an error if the log file cannot be created.
func Init(logPath string, minLevel LogLevel) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if defaultLogger != nil {
		return nil
	}

	replacement, err := newLogger(logPath, minLevel)
	if err != nil {
		return err
	}
	defaultLogger = replacement
	// Write session start marker
	defaultLogger.log(LevelInfo, "=== Session started ===")
	return nil
}

// newLogger opens the log file. An empty path disables logging.
func newLogger(logPath string, minLevel LogLevel) (*Logger, error) {
	if logPath == "" {
		// Logging disabled
		return &Logger{enabled: false}, nil
	}

	// Create log directory if it doesn't exist
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	// Open log file for appending
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	return &Logger{
		file:     file,
		minLevel: minLevel,
		enabled:  true,
	}, nil
}

// Reinit closes the current logger and reinitializes it with new settings.
// The new file opens before the old one closes: a path the user cannot write
// would otherwise leave the session with nowhere to log, including the report
// of this failure.
func Reinit(logPath string, minLevel LogLevel) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	replacement, err := newLogger(logPath, minLevel)
	if err != nil {
		return err
	}

	var closeErr error
	if defaultLogger != nil {
		closeErr = defaultLogger.close()
	}
	defaultLogger = replacement
	// Write session start marker
	defaultLogger.log(LevelInfo, "=== Session started ===")
	return closeErr
}

// Close closes the log file. Should be called when the application exits.
func Close() error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if defaultLogger == nil {
		return nil
	}
	return defaultLogger.close()
}

// current returns the logger to write to, or nil when there is none.
func current() *Logger {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return defaultLogger
}

// close writes the session end marker and closes the file. A writer that took
// this logger before the swap either finishes its line first or sees closed.
func (l *Logger) close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.enabled || l.file == nil || l.closed {
		return nil
	}
	l.write(LevelInfo, "=== Session ended ===")
	l.closed = true
	return l.file.Close()
}

// log writes a log message with the specified level and message.
func (l *Logger) log(level LogLevel, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.write(level, message)
}

// write appends one line. Callers hold l.mu.
func (l *Logger) write(level LogLevel, message string) {
	if !l.enabled || level < l.minLevel || l.closed || l.file == nil {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logLine := fmt.Sprintf("[%s] %s: %s\n", timestamp, level.String(), message)
	_, _ = io.WriteString(l.file, logLine)
}

// Debug logs a debug-level message.
func Debug(format string, args ...interface{}) {
	if l := current(); l != nil {
		l.log(LevelDebug, fmt.Sprintf(format, args...))
	}
}

// Info logs an info-level message.
func Info(format string, args ...interface{}) {
	if l := current(); l != nil {
		l.log(LevelInfo, fmt.Sprintf(format, args...))
	}
}

// Warning logs a warning-level message.
func Warning(format string, args ...interface{}) {
	if l := current(); l != nil {
		l.log(LevelWarning, fmt.Sprintf(format, args...))
	}
}

// Error logs an error-level message.
func Error(format string, args ...interface{}) {
	if l := current(); l != nil {
		l.log(LevelError, fmt.Sprintf(format, args...))
	}
}

// ErrorWithErr logs an error with additional error context.
func ErrorWithErr(err error, format string, args ...interface{}) {
	if l := current(); l != nil && err != nil {
		message := fmt.Sprintf(format, args...)
		l.log(LevelError, fmt.Sprintf("%s: %v", message, err))
	}
}
