package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/config"
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarning
	LevelError
)

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
	maxLogSize    = 5 << 20
	rotatedSuffix = ".1"
)

var (
	globalMu      sync.RWMutex
	defaultLogger *Logger
)

// Init opens the global log at logPath. An empty path disables logging, and a
// second call is a no-op.
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
	defaultLogger.log(LevelInfo, "=== Session started ===")
	return nil
}

func newLogger(logPath string, minLevel LogLevel) (*Logger, error) {
	if logPath == "" {
		return &Logger{enabled: false}, nil
	}

	if _, err := config.EnsureDirFor(logPath); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

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

// Reinit swaps the global log for logPath, opening the new file before closing
// the old one. An error means the new path would not open.
func Reinit(logPath string, minLevel LogLevel) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	replacement, err := newLogger(logPath, minLevel)
	if err != nil {
		return err
	}

	previous := defaultLogger
	defaultLogger = replacement
	defaultLogger.log(LevelInfo, "=== Session started ===")

	if previous != nil {
		if err := previous.close(); err != nil {
			defaultLogger.log(LevelWarning, fmt.Sprintf("closing previous log: %v", err))
		}
	}
	return nil
}

func Close() error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if defaultLogger == nil {
		return nil
	}
	return defaultLogger.close()
}

func current() *Logger {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return defaultLogger
}

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

func (l *Logger) log(level LogLevel, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rotateIfFull()
	l.write(level, message)
}

func (l *Logger) rotateIfFull() {
	if !l.enabled || l.closed || l.file == nil || l.size < maxLogSize || l.path == "" {
		return
	}

	_ = l.file.Close()
	if err := os.Rename(l.path, l.path+rotatedSuffix); err != nil {
		l.reopen()
		return
	}
	l.reopen()
}

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

func (l *Logger) write(level LogLevel, message string) {
	if !l.enabled || level < l.minLevel || l.closed || l.file == nil {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logLine := fmt.Sprintf("[%s] %s: %s\n", timestamp, level.String(), message)
	n, _ := io.WriteString(l.file, logLine)
	l.size += int64(n)
}

func Debug(format string, args ...interface{}) {
	if l := current(); l != nil {
		l.log(LevelDebug, fmt.Sprintf(format, args...))
	}
}

func Info(format string, args ...interface{}) {
	if l := current(); l != nil {
		l.log(LevelInfo, fmt.Sprintf(format, args...))
	}
}

func Warning(format string, args ...interface{}) {
	if l := current(); l != nil {
		l.log(LevelWarning, fmt.Sprintf(format, args...))
	}
}

func Error(format string, args ...interface{}) {
	if l := current(); l != nil {
		l.log(LevelError, fmt.Sprintf(format, args...))
	}
}

func ErrorWithErr(err error, format string, args ...interface{}) {
	if l := current(); l != nil && err != nil {
		message := fmt.Sprintf(format, args...)
		l.log(LevelError, fmt.Sprintf("%s: %v", message, err))
	}
}

// Start opens path, else fallback, else turns logging off. It returns the path
// opened, empty when logging is off, and a warning that is empty on a clean open.
func Start(path, fallback string, minLevel LogLevel) (opened, warning string) {
	globalMu.Lock()
	previous := defaultLogger
	defaultLogger = nil
	globalMu.Unlock()
	if previous != nil {
		_ = previous.close()
	}

	return openWithFallback(Init, path, fallback, minLevel)
}

// Restart is Start for a running logger. The current log stays open until a
// replacement does.
func Restart(path, fallback string, minLevel LogLevel) (opened, warning string) {
	return openWithFallback(Reinit, path, fallback, minLevel)
}

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

	_ = open("", minLevel)
	return "", fmt.Sprintf("cannot log to %s (%v); logging is off", path, err)
}
