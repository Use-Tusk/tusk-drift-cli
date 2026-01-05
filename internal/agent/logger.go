package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	"github.com/Use-Tusk/tusk-drift-cli/internal/version"
)

// CloseStatus represents the final status when closing the logger
type CloseStatus int

const (
	StatusCompleted CloseStatus = iota
	StatusCancelled
	StatusFailed
)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // "phase", "tool_start", "tool_complete", "message", "error", "thinking"
	Phase     string    `json:"phase,omitempty"`
	Tool      string    `json:"tool,omitempty"`
	Input     string    `json:"input,omitempty"`
	Output    string    `json:"output,omitempty"`
	Success   bool      `json:"success,omitempty"`
	Message   string    `json:"message,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// AgentLogger handles logging of agent activity to a file
type AgentLogger struct {
	file     *os.File
	mu       sync.Mutex
	filePath string
}

// NewAgentLogger creates a new logger that writes to .tusk/logs/setup-<datetime>.log
// mode should be "TUI" or "Headless"
func NewAgentLogger(workDir string, mode string) (*AgentLogger, error) {
	logsDir := utils.GetLogsDir()
	if err := os.MkdirAll(logsDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Use same datetime pattern as service logs: 20060102-150405
	timestamp := time.Now().Format("20060102-150405")
	logPath := filepath.Join(logsDir, fmt.Sprintf("setup-%s.log", timestamp))

	file, err := os.Create(logPath) //nolint:gosec // logPath is constructed from timestamp, not user input
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	// Write header with metadata
	header := fmt.Sprintf("# Tusk Setup Agent Log\n# Started: %s\n# Version: %s\n# Platform: %s/%s\n# Mode: %s\n# Log file: %s\n\n",
		time.Now().Format(time.RFC3339),
		version.Version,
		runtime.GOOS,
		runtime.GOARCH,
		mode,
		logPath)
	if _, err := file.WriteString(header); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("failed to write log header: %w", err)
	}

	return &AgentLogger{
		file:     file,
		filePath: logPath,
	}, nil
}

// Close closes the log file with the given status
func (l *AgentLogger) Close(status CloseStatus, err error) error {
	if l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	// Write footer based on status
	timestamp := time.Now().Format(time.RFC3339)
	var footer string
	switch status {
	case StatusCompleted:
		footer = fmt.Sprintf("\n# Completed: %s\n", timestamp)
	case StatusCancelled:
		footer = fmt.Sprintf("\n# Cancelled by user: %s\n", timestamp)
	case StatusFailed:
		footer = fmt.Sprintf("\n# Failed: %s\n", timestamp)
		if err != nil {
			footer += fmt.Sprintf("# Error: %s\n", err.Error())
		}
	}
	_, _ = l.file.WriteString(footer)

	return l.file.Close()
}

// FilePath returns the path to the log file
func (l *AgentLogger) FilePath() string {
	return l.filePath
}

// LogPhaseStart logs the start of a phase
func (l *AgentLogger) LogPhaseStart(phaseName, phaseDesc string, phaseNum, totalPhases int) {
	l.writeEntry(LogEntry{
		Timestamp: time.Now(),
		Type:      "phase_start",
		Phase:     phaseName,
		Message:   fmt.Sprintf("Phase %d/%d: %s - %s", phaseNum, totalPhases, phaseName, phaseDesc),
	})
}

// LogToolStart logs the start of a tool call
func (l *AgentLogger) LogToolStart(toolName, input string) {
	l.writeEntry(LogEntry{
		Timestamp: time.Now(),
		Type:      "tool_start",
		Tool:      toolName,
		Input:     input,
	})
}

// LogToolComplete logs the completion of a tool call
func (l *AgentLogger) LogToolComplete(toolName string, success bool, output string) {
	l.writeEntry(LogEntry{
		Timestamp: time.Now(),
		Type:      "tool_complete",
		Tool:      toolName,
		Success:   success,
		Output:    output,
	})
}

// LogMessage logs an agent message
func (l *AgentLogger) LogMessage(message string) {
	l.writeEntry(LogEntry{
		Timestamp: time.Now(),
		Type:      "message",
		Message:   message,
	})
}

// LogThinking logs when the agent is thinking
func (l *AgentLogger) LogThinking(thinking bool) {
	if thinking {
		l.writeEntry(LogEntry{
			Timestamp: time.Now(),
			Type:      "thinking",
			Message:   "Agent is thinking...",
		})
	}
}

// LogError logs an error
func (l *AgentLogger) LogError(err error) {
	l.writeEntry(LogEntry{
		Timestamp: time.Now(),
		Type:      "error",
		Error:     err.Error(),
	})
}

// LogUserInput logs a user input request and response
func (l *AgentLogger) LogUserInput(question, response string) {
	l.writeEntry(LogEntry{
		Timestamp: time.Now(),
		Type:      "user_input",
		Message:   question,
		Output:    response,
	})
}

// LogUserSelect logs a user selection request and response
func (l *AgentLogger) LogUserSelect(question, selectedID, selectedLabel string) {
	l.writeEntry(LogEntry{
		Timestamp: time.Now(),
		Type:      "user_select",
		Message:   question,
		Output:    fmt.Sprintf("%s (%s)", selectedLabel, selectedID),
	})
}

// writeEntry writes a log entry to the file
func (l *AgentLogger) writeEntry(entry LogEntry) {
	if l.file == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	_, _ = l.file.WriteString(string(data) + "\n")
	_ = l.file.Sync()
}
