package logging

import (
	"sync"
)

// TestLogger interface for logging to specific tests
type TestLogger interface {
	LogToCurrentTest(testID, message string)
	LogToService(message string)
}

// Global registry for the current test logger
var (
	currentLogger TestLogger
	loggerMu      sync.RWMutex
)

// SetTestLogger sets the global test logger (called by TUI)
func SetTestLogger(logger TestLogger) {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	currentLogger = logger
}

// LogToCurrentTest logs to the currently active test
func LogToCurrentTest(testID, message string) {
	loggerMu.RLock()
	defer loggerMu.RUnlock()

	if currentLogger != nil {
		currentLogger.LogToCurrentTest(testID, message)
	}
}

// LogToService logs to service logs
func LogToService(message string) {
	loggerMu.RLock()
	defer loggerMu.RUnlock()

	if currentLogger != nil {
		currentLogger.LogToService(message)
	}
}

// LogToCurrentTestOrService tries to log to current test, falls back to service
func LogToCurrentTestOrService(testID, message string) {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	if currentLogger != nil {
		if testID != "" {
			currentLogger.LogToCurrentTest(testID, message)
		} else {
			currentLogger.LogToService(message)
		}
	}
}
