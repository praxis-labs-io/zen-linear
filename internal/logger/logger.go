package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/config"
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
	path     string
	size     int64
	minLevel LogLevel
	enabled  bool
	closed   bool
}

const (
	// maxLogSize is where the log is moved aside. One previous generation is
	// what answers "what happened just before this" for a single-user TUI;
	// more would be an archive nobody reads.
	maxLogSize = 5 << 20
	// rotatedSuffix names that generation: app.log.1 beside app.log.
	rotatedSuffix = ".1"
)

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
	if _, err := config.EnsureDirFor(logPath); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	// Open log file for appending
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	// Seeded from what is already there, since the file is opened O_APPEND and
	// a relaunch would otherwise start counting from nought against a full log.
	var size int64
	if info, err := file.Stat(); err == nil {
		size = info.Size()
	}

	return &Logger{
		file:     file,
		path:     logPath,
		size:     size,
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

	previous := defaultLogger
	defaultLogger = replacement
	// Write session start marker
	defaultLogger.log(LevelInfo, "=== Session started ===")

	// A close failure is about the file being left behind and is not actionable,
	// so it is reported rather than returned: the error here means the new path
	// could not be opened, which is what a caller retries on.
	if previous != nil {
		if err := previous.close(); err != nil {
			defaultLogger.log(LevelWarning, fmt.Sprintf("closing previous log: %v", err))
		}
	}
	return nil
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
	l.rotateIfFull()
	l.write(level, message)
}

// rotateIfFull moves a full log aside and opens an empty one. Callers hold
// l.mu, which is the only thing guarding l.file: close is the sole other
// holder, and Reinit builds a separate Logger rather than touching this one's.
//
// A failed rotation keeps writing to the log it has. An oversized log is worth
// less than a session with nowhere to report why it has one.
func (l *Logger) rotateIfFull() {
	if !l.enabled || l.closed || l.file == nil || l.size < maxLogSize || l.path == "" {
		return
	}

	// Close releases the handle even when it reports an error, so there is no
	// branch here that can keep writing to l.file. Everything past this point
	// has to end in a reopen or the session logs nowhere for the rest of its
	// life, with write discarding the error that would have said so.
	_ = l.file.Close()
	if err := os.Rename(l.path, l.path+rotatedSuffix); err != nil {
		l.reopen()
		return
	}
	l.reopen()
}

// reopen replaces l.file after a rotation. Callers hold l.mu.
//
// The size is re-read rather than zeroed: a rotation whose rename failed
// reopens the same full file, and a counter starting from nought there would
// let it grow another whole cap before the next attempt, and again after that.
func (l *Logger) reopen() {
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		l.file = nil
		l.enabled = false
		return
	}
	l.file = file
	l.size = 0
	if info, err := file.Stat(); err == nil {
		l.size = info.Size()
	}
}

// write appends one line. Callers hold l.mu.
func (l *Logger) write(level LogLevel, message string) {
	if !l.enabled || level < l.minLevel || l.closed || l.file == nil {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logLine := fmt.Sprintf("[%s] %s: %s\n", timestamp, level.String(), message)
	n, _ := io.WriteString(l.file, logLine)
	l.size += int64(n)
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

// Start opens the log at path, falling back to fallback when path cannot be
// opened and to no logging when neither can. Logging is diagnostics: a path the
// app cannot write is worth reporting, never worth refusing to launch over.
//
// It returns the path actually opened, empty when logging ended up off, and a
// warning that is empty on a clean open. Callers should adopt the returned path
// as the effective one so the settings UI names where logs really go.
func Start(path, fallback string, minLevel LogLevel) (opened, warning string) {
	// Init is a no-op once a logger exists, and would report success without
	// opening anything. Starting means starting.
	globalMu.Lock()
	previous := defaultLogger
	defaultLogger = nil
	globalMu.Unlock()
	if previous != nil {
		// Nowhere to report a close failure to: this is the logger being torn
		// down, and the one replacing it is not open yet.
		_ = previous.close()
	}

	return openWithFallback(Init, path, fallback, minLevel)
}

// Restart is Start for a logger that is already running. A refused path never
// closes the log in hand until a replacement is open, so the session is never
// left with nowhere to report this failure.
func Restart(path, fallback string, minLevel LogLevel) (opened, warning string) {
	return openWithFallback(Reinit, path, fallback, minLevel)
}

// openWithFallback reads a nil error as "this path is now the log". Init breaks
// that on a second call, where nil means it left an existing logger alone, so
// Start clears the logger first rather than reporting a path it never opened.
func openWithFallback(open func(string, LogLevel) error, path, fallback string, minLevel LogLevel) (opened, warning string) {
	err := open(path, minLevel)
	if err == nil {
		return path, ""
	}

	if fallback != "" && fallback != path {
		if fallbackErr := open(fallback, minLevel); fallbackErr == nil {
			return fallback, fmt.Sprintf("cannot log to %s (%v); logging to %s instead", path, err, fallback)
		}
	}

	// Disabling cannot fail: newLogger returns a disabled logger for an empty
	// path without touching the filesystem.
	_ = open("", minLevel)
	return "", fmt.Sprintf("cannot log to %s (%v); logging is off", path, err)
}
