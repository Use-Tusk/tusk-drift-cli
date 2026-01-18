// Package log provides centralized logging for the Tusk CLI.
package log

import (
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
)

// OutputMode determines how user-facing output is rendered
type OutputMode int

const (
	// ModeTUI indicates the TUI is active - user output goes to TUI panels
	ModeTUI OutputMode = iota
	// ModeHeadless indicates headless/print mode - user output goes to stdout
	ModeHeadless
)

// TUILogger interface for logging to TUI panels
type TUILogger interface {
	LogToCurrentTest(testID, message string)
	LogToService(message string)
}

// logMessage represents a message to be logged
type logMessage struct {
	msgType logType
	testID  string
	message string
}

type logType int

const (
	logTypeService logType = iota
	logTypeTest
)

type Logger struct {
	mode      atomic.Int32
	tuiLogger atomic.Pointer[TUILogger]
	logChan   chan logMessage
	stopChan  chan struct{}
	wg        sync.WaitGroup
	level     slog.Level
}

var (
	instance *Logger
	once     sync.Once
)

// Get returns the singleton logger instance
func Get() *Logger {
	once.Do(func() {
		instance = &Logger{
			logChan:  make(chan logMessage, 1000),
			stopChan: make(chan struct{}),
			level:    slog.LevelInfo,
		}
		instance.mode.Store(int32(ModeHeadless))
		instance.wg.Add(1)
		go instance.process()
	})
	return instance
}

func (l *Logger) process() {
	defer l.wg.Done()
	for {
		select {
		case msg := <-l.logChan:
			l.handleLogMessage(msg)
		case <-l.stopChan:
			// Drain remaining messages
			for {
				select {
				case msg := <-l.logChan:
					l.handleLogMessage(msg)
				default:
					return
				}
			}
		}
	}
}

func (l *Logger) handleLogMessage(msg logMessage) {
	tuiPtr := l.tuiLogger.Load()
	if tuiPtr == nil {
		return
	}
	tui := *tuiPtr
	switch msg.msgType {
	case logTypeService:
		tui.LogToService(msg.message)
	case logTypeTest:
		tui.LogToCurrentTest(msg.testID, msg.message)
	}
}

// Setup configures the singleton logger (call once at startup)
func Setup(debug bool, mode OutputMode) {
	l := Get()
	l.mode.Store(int32(mode))

	if debug {
		l.level = slog.LevelDebug
	} else {
		l.level = slog.LevelInfo
	}

	// Configure slog with our custom handler
	handler := NewHandler(os.Stderr, &slog.HandlerOptions{
		Level: l.level,
	})
	slog.SetDefault(slog.New(handler))
}

// SetTUILogger sets the TUI logger (called when TUI starts)
func SetTUILogger(tui TUILogger) {
	l := Get()
	if tui == nil {
		l.tuiLogger.Store(nil)
	} else {
		l.tuiLogger.Store(&tui)
	}
}

// SetMode changes the output mode
func SetMode(mode OutputMode) {
	Get().mode.Store(int32(mode))
}

// GetMode returns the current output mode
func GetMode() OutputMode {
	return OutputMode(Get().mode.Load())
}

// Shutdown gracefully stops the logger, draining pending messages
func Shutdown() {
	l := Get()
	close(l.stopChan)
	l.wg.Wait()
}

// --- Developer Logging (wraps slog) ---

// Debug logs a debug-level message
func Debug(msg string, args ...any) {
	slog.Debug(msg, args...)
}

// Info logs an info-level message
func Info(msg string, args ...any) {
	slog.Info(msg, args...)
}

// Warn logs a warning-level message
func Warn(msg string, args ...any) {
	slog.Warn(msg, args...)
}

// Error logs an error-level message
func Error(msg string, args ...any) {
	slog.Error(msg, args...)
}

// --- User-Facing Output (styled, mode-aware) ---

// UserError prints a styled error message to the user
func UserError(msg string) {
	printStyled(renderError(msg))
}

// UserWarn prints a styled warning message to the user
func UserWarn(msg string) {
	printStyled(renderWarning(msg))
}

// UserSuccess prints a styled success message to the user
func UserSuccess(msg string) {
	printStyled(renderSuccess(msg))
}

// UserInfo prints an informational message to the user
func UserInfo(msg string) {
	printStyled(msg)
}

// UserProgress prints a progress/dim message to the user
func UserProgress(msg string) {
	printStyled(renderDim(msg))
}

// UserDeviation prints a deviation/warning-level message (orange color)
func UserDeviation(msg string) {
	printStyled(renderDeviation(msg))
}

// Print prints a message without styling or newline
func Print(msg string) {
	if GetMode() == ModeHeadless {
		io.WriteString(os.Stdout, msg)
	}
}

// Println prints a message with newline but no styling
func Println(msg string) {
	if GetMode() == ModeHeadless {
		io.WriteString(os.Stdout, msg+"\n")
	}
}

// Stderr prints a message to stderr without newline
func Stderr(msg string) {
	if GetMode() == ModeHeadless {
		io.WriteString(os.Stderr, msg)
	}
}

// Stderrln prints a message to stderr with newline
func Stderrln(msg string) {
	if GetMode() == ModeHeadless {
		io.WriteString(os.Stderr, msg+"\n")
	}
}

func printStyled(msg string) {
	// User output always goes to stdout for headless mode
	// TUI mode handles its own display
	if GetMode() == ModeHeadless {
		io.WriteString(os.Stdout, msg+"\n")
	}
}

// ServiceLog logs a message to the TUI service panel
// Non-blocking: returns immediately, message is queued for processing
func ServiceLog(msg string) {
	l := Get()
	select {
	case l.logChan <- logMessage{msgType: logTypeService, message: msg}:
	default:
		// Queue full - log to stderr as fallback
		slog.Debug("TUI log queue full, dropping message", "message", msg)
	}
}

// TestLog logs a message to a specific test's log panel
// Non-blocking: returns immediately, message is queued for processing
func TestLog(testID, msg string) {
	l := Get()
	select {
	case l.logChan <- logMessage{msgType: logTypeTest, testID: testID, message: msg}:
	default:
		// Queue full - log to stderr as fallback
		slog.Debug("TUI log queue full, dropping message", "testID", testID, "message", msg)
	}
}

// TestOrServiceLog tries to log to test, falls back to service if testID is empty
func TestOrServiceLog(testID, msg string) {
	if testID != "" {
		TestLog(testID, msg)
	} else {
		ServiceLog(msg)
	}
}
