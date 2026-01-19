package logging

import (
	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
)

// TestLogger interface for logging to specific tests
// Kept for backwards compatibility with TUI implementations
type TestLogger = log.TUILogger

// SetTestLogger sets the global test logger (called by TUI)
// Delegates to the centralized log package
func SetTestLogger(logger TestLogger) {
	log.SetTUILogger(logger)
}

// LogToCurrentTest logs to the currently active test
// Delegates to the centralized log package
func LogToCurrentTest(testID, message string) {
	log.TestLog(testID, message)
}

// LogToService logs to service logs
// Delegates to the centralized log package
func LogToService(message string) {
	log.ServiceLog(message)
}

// LogToCurrentTestOrService tries to log to current test, falls back to service
// Delegates to the centralized log package
func LogToCurrentTestOrService(testID, message string) {
	log.TestOrServiceLog(testID, message)
}
