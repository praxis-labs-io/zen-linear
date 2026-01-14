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
	// defaultLogger is the global logger instance.
	defaultLogger *Logger
	// once ensures the default logger is initialized only once.
	once sync.Once
)

// Init initializes the global logger with the specified log file path.
// If logPath is empty, logging is disabled.
// Returns an error if the log file cannot be created.
func Init(logPath string, minLevel LogLevel) error {
	var initErr error
	once.Do(func() {
		if logPath == "" {
			// Logging disabled
			defaultLogger = &Logger{
				enabled: false,
			}
			return
		}

		// Create log directory if it doesn't exist
		logDir := filepath.Dir(logPath)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			initErr = fmt.Errorf("create log directory: %w", err)
			return
		}

		// Open log file for appending
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			initErr = fmt.Errorf("open log file: %w", err)
			return
		}

		defaultLogger = &Logger{
			file:     file,
			minLevel: minLevel,
			enabled:  true,
		}

		// Write session start marker
		defaultLogger.log(LevelInfo, "=== Session started ===")
	})

	return initErr
}

// Close closes the log file. Should be called when the application exits.
func Close() error {
	if defaultLogger != nil && defaultLogger.enabled && defaultLogger.file != nil && !defaultLogger.closed {
		defaultLogger.log(LevelInfo, "=== Session ended ===")
		defaultLogger.closed = true
		return defaultLogger.file.Close()
	}
	return nil
}

// log writes a log message with the specified level and message.
func (l *Logger) log(level LogLevel, message string) {
	if !l.enabled || level < l.minLevel || l.closed {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logLine := fmt.Sprintf("[%s] %s: %s\n", timestamp, level.String(), message)

	if l.file != nil {
		io.WriteString(l.file, logLine)
	}
}

// Debug logs a debug-level message.
func Debug(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.log(LevelDebug, fmt.Sprintf(format, args...))
	}
}

// Info logs an info-level message.
func Info(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.log(LevelInfo, fmt.Sprintf(format, args...))
	}
}

// Warning logs a warning-level message.
func Warning(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.log(LevelWarning, fmt.Sprintf(format, args...))
	}
}

// Error logs an error-level message.
func Error(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.log(LevelError, fmt.Sprintf(format, args...))
	}
}

// ErrorWithErr logs an error with additional error context.
func ErrorWithErr(err error, format string, args ...interface{}) {
	if defaultLogger != nil && err != nil {
		message := fmt.Sprintf(format, args...)
		defaultLogger.log(LevelError, fmt.Sprintf("%s: %v", message, err))
	}
}
