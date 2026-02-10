package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/components"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Friendly tool names
var toolDisplayNames = map[string]string{
	"read_file":                "Read file",
	"write_file":               "Write file",
	"list_directory":           "List directory",
	"grep":                     "Search files",
	"patch_file":               "Edit file",
	"run_command":              "Run command",
	"start_background_process": "Start service",
	"stop_background_process":  "Stop service",
	"get_process_logs":         "Get logs",
	"wait_for_ready":           "Wait for ready",
	"http_request":             "HTTP request",
	"ask_user":                 "Ask user",
	"tusk_validate_config":     "Validate config",
	"tusk_list":                "List traces",
	"tusk_run":                 "Run tests",
	"transition_phase":         "Complete phase",
}

// Plural forms for "allow all X" context
var toolDisplayNamesPlural = map[string]string{
	"read_file":                "file reads",
	"write_file":               "file writes",
	"list_directory":           "directory listings",
	"grep":                     "file searches",
	"patch_file":               "file edits",
	"run_command":              "commands",
	"start_background_process": "service starts",
	"stop_background_process":  "service stops",
	"get_process_logs":         "log retrievals",
	"wait_for_ready":           "ready checks",
	"http_request":             "HTTP requests",
	"tusk_validate_config":     "config validations",
	"tusk_list":                "trace listings",
	"tusk_run":                 "test runs",
}

func getToolDisplayName(name string) string {
	if friendly, ok := toolDisplayNames[name]; ok {
		return friendly
	}
	return name
}

func getToolDisplayNamePlural(name string) string {
	if plural, ok := toolDisplayNamesPlural[name]; ok {
		return plural
	}
	return getToolDisplayName(name)
}

// TUI Messages
type (
	phaseChangedMsg struct {
		phaseName   string
		phaseDesc   string
		phaseNumber int
		totalPhases int
	}

	toolStartMsg struct {
		toolName string
		input    string
	}

	toolCompleteMsg struct {
		toolName string
		success  bool
		output   string
	}

	agentTextMsg struct {
		text      string
		streaming bool
	}

	thinkingMsg struct {
		thinking bool
	}

	errorMsg struct {
		err error
	}

	completedMsg struct {
		removableFiles []string // Files that can be safely removed from .tusk/
	}

	eligibilityCompletedMsg struct{}

	abortedMsg struct {
		reason string
	}

	tickMsg time.Time

	userInputRequestMsg struct {
		question   string
		responseCh chan string
	}

	portConflictMsg struct {
		port       int
		responseCh chan bool
	}

	rerunConfirmMsg struct {
		responseCh chan bool
	}

	cloudSetupPromptMsg struct {
		responseCh chan bool
	}

	userSelectRequestMsg struct {
		question   string
		options    []SelectOption
		responseCh chan string // Returns the selected option ID
	}

	permissionRequestMsg struct {
		toolName        string
		preview         string
		commandPrefixes []string    // For run_command granularity
		responseCh      chan string // "approve", "approve_tool_type", "approve_commands:<csv>", "approve_all", "deny"
	}

	// For graceful shutdown
	shutdownStartedMsg struct{}
	autoQuitMsg        struct{}
	fatalErrorMsg      struct{}

	// Sidebar updates
	sidebarUpdateMsg struct {
		key   string
		value string
	}
)

// SelectOption represents a selectable option for ask_user_select
type SelectOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// TUIModel is the bubbletea model for the agent TUI
type TUIModel struct {
	// Agent state
	currentPhase     string
	phaseDesc        string
	phaseNumber      int
	totalPhases      int
	thinking         bool
	currentTool      string
	completed        bool
	hasError         bool
	streamingText    string
	lastAgentMessage string
	lastToolComplete bool // Track if we just completed a tool (for showing thinking)
	hideProgressBar  bool // Hide progress bar (e.g., for eligibility-only mode)

	// Log state
	logs       []logEntry
	logMutex   sync.RWMutex
	maxLogs    int
	autoScroll bool

	// Sidebar state
	sidebarInfo  map[string]string
	sidebarOrder []string
	todoItems    []todoItem
	todoMutex    sync.RWMutex

	// User input
	userInputMode             bool
	userInputPrompt           string
	userInputTextarea         textarea.Model
	userInputCh               chan string
	portConflictMode          bool
	portConflictPort          int
	portConflictCh            chan bool
	rerunConfirmMode          bool
	rerunConfirmCh            chan bool
	cloudSetupPromptMode      bool
	cloudSetupPromptCh        chan bool
	userSelectMode            bool
	userSelectPrompt          string
	userSelectOptions         []SelectOption
	userSelectIndex           int
	userSelectCh              chan string
	permissionMode            bool
	permissionTool            string
	permissionPreview         string
	permissionCommandPrefixes []string // For run_command granularity
	permissionCh              chan string
	permissionDenyMode        bool
	permissionDenyBuffer      string

	// UI state
	viewport      viewport.Model
	spinner       spinner.Model
	progress      progress.Model
	width         int
	height        int
	quitting      bool
	viewportReady bool

	// Shutdown state
	shutdownRequested bool
	shutdownTime      time.Time

	// Animation
	lastTickTime time.Time
	pulsePhase   int

	// Mouse drag hint state
	mouseDragging     bool      // True while user is dragging (left button held + motion)
	dragHintVisible   bool      // True when hint should be shown
	dragHintExpiresAt time.Time // When to hide the hint after release

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

type logEntry struct {
	timestamp time.Time
	level     string // "phase", "tool-start", "tool-complete", "agent", "error", "warning", "success", "dim", "spacing", "plain"
	message   string
	toolName  string // for tool entries
}

type todoItem struct {
	text   string
	done   bool
	active bool
}

// NewTUIModel creates a new TUI model
func NewTUIModel(ctx context.Context, cancel context.CancelFunc, phaseNames []string, hideProgressBar bool) *TUIModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(styles.PrimaryColor))

	opts := []progress.Option{}
	if styles.NoColor() {
		opts = append(opts, progress.WithColorProfile(termenv.Ascii))
	} else {
		opts = append(opts, progress.WithDefaultGradient())
	}
	p := progress.New(opts...)

	// Initialize textarea for user input (supports paste and multiline)
	ta := textarea.New()
	ta.Placeholder = "Type your response..."
	ta.ShowLineNumbers = false
	ta.SetWidth(80)
	ta.SetHeight(1)                                  // Start with single line, grows as needed
	ta.CharLimit = 0                                 // No limit
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle() // No highlight on cursor line
	ta.FocusedStyle.Base = lipgloss.NewStyle()       // Clean base style

	todoItems := make([]todoItem, len(phaseNames))
	for i, name := range phaseNames {
		todoItems[i] = todoItem{
			text:   name,
			done:   false,
			active: i == 0, // First phase active
		}
	}

	return &TUIModel{
		spinner:           s,
		progress:          p,
		maxLogs:           5000,
		logs:              make([]logEntry, 0),
		totalPhases:       len(phaseNames),
		ctx:               ctx,
		cancel:            cancel,
		autoScroll:        true,
		lastTickTime:      time.Now(),
		sidebarInfo:       make(map[string]string),
		sidebarOrder:      []string{},
		todoItems:         todoItems,
		userInputTextarea: ta,
		hideProgressBar:   hideProgressBar,
	}
}

func (m *TUIModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.tickAnimation(),
	)
}

func (m *TUIModel) tickAnimation() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// getNextPhaseName returns the name of the next phase
func (m *TUIModel) getNextPhaseName() string {
	m.todoMutex.RLock()
	defer m.todoMutex.RUnlock()

	if m.phaseNumber >= 0 && m.phaseNumber < len(m.todoItems) {
		return m.todoItems[m.phaseNumber].text // phaseNumber is 1-indexed, so this is next
	}
	return ""
}

func (m *TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateViewportSize()
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.MouseMsg:
		// Handle mouse wheel scrolling
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.autoScroll = false
			m.viewport.ScrollUp(3)
		case tea.MouseButtonWheelDown:
			m.viewport.ScrollDown(3)
			if m.viewport.AtBottom() {
				m.autoScroll = true
			}
		case tea.MouseButtonLeft:
			// Track drag state for showing "hold Option to select" hint
			switch msg.Action {
			case tea.MouseActionMotion:
				// User is dragging with left button held
				if !m.mouseDragging {
					m.mouseDragging = true
					m.dragHintVisible = true
				}
			case tea.MouseActionRelease:
				// User released the mouse button
				if m.mouseDragging {
					m.mouseDragging = false
					// Keep hint visible for 10 seconds after release
					m.dragHintExpiresAt = time.Now().Add(10 * time.Second)
				}
			}
		}
		return m, nil

	case tickMsg:
		m.lastTickTime = time.Time(msg)
		m.pulsePhase = (m.pulsePhase + 1) % 20
		cmds = append(cmds, m.tickAnimation())

		// Check if drag hint should expire
		if m.dragHintVisible && !m.mouseDragging && time.Now().After(m.dragHintExpiresAt) {
			m.dragHintVisible = false
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case progress.FrameMsg:
		var cmd tea.Cmd
		pm, cmd := m.progress.Update(msg)
		if p, ok := pm.(progress.Model); ok {
			m.progress = p
		}
		cmds = append(cmds, cmd)

	case shutdownStartedMsg:
		m.shutdownRequested = true
		m.addLog("spacing", "", "")
		m.addLog("dim", "⏹  Shutting down gracefully...", "")
		m.updateViewportSize()

	case autoQuitMsg:
		// Auto-quit after cleanup delay
		if m.shutdownRequested && !m.quitting {
			m.quitting = true
			return m, tea.Quit
		}

	case phaseChangedMsg:
		m.currentPhase = msg.phaseName
		m.phaseDesc = msg.phaseDesc
		m.phaseNumber = msg.phaseNumber
		m.totalPhases = msg.totalPhases
		// Add visual phase banner
		phaseBanner := fmt.Sprintf("Phase %d: %s", msg.phaseNumber, msg.phaseName)
		m.addLog("phase-banner", phaseBanner, "")
		// Update progress bar
		if m.totalPhases > 0 {
			percent := float64(m.phaseNumber-1) / float64(m.totalPhases)
			cmds = append(cmds, m.progress.SetPercent(percent))
		}
		// Update todo items
		m.updateTodoForPhase(msg.phaseNumber)
		// Recalculate viewport size since header height depends on phaseDesc
		m.updateViewportSize()

	case toolStartMsg:
		m.currentTool = msg.toolName
		m.thinking = false
		m.lastToolComplete = false

		// Skip logging transition_phase tool - it's internal
		if msg.toolName == "transition_phase" {
			break
		}

		// Add spacing before new tool
		m.addLog("spacing", "", "")

		displayName := getToolDisplayName(msg.toolName)

		// For file-related tools, show path inline
		if msg.toolName == "read_file" || msg.toolName == "list_directory" {
			path := extractPathFromInput(msg.input)
			if path != "" {
				m.addLog("tool-start", fmt.Sprintf("%s [%s]", displayName, path), msg.toolName)
			} else {
				m.addLog("tool-start", displayName, msg.toolName)
			}
		} else {
			m.addLog("tool-start", displayName, msg.toolName)

			// Skip showing input for tools where the UI will show it interactively
			skipInputTools := map[string]bool{
				"ask_user":        true,
				"ask_user_select": true,
			}

			// Show the input on separate lines if it's meaningful
			if msg.input != "" && msg.input != "{}" && !skipInputTools[msg.toolName] {
				formattedInput := formatToolInput(msg.input, m.width-10)
				for line := range strings.SplitSeq(formattedInput, "\n") {
					if strings.TrimSpace(line) != "" {
						m.addLog("dim", "     "+line, "")
					}
				}
			}
		}

	case toolCompleteMsg:
		m.currentTool = ""
		m.lastToolComplete = true

		// Skip logging transition_phase tool - it's internal
		if msg.toolName == "transition_phase" {
			break
		}

		displayName := getToolDisplayName(msg.toolName)
		if msg.success {
			m.addLog("tool-complete", fmt.Sprintf("   ✓ %s", displayName), msg.toolName)
		} else {
			m.addLog("error", fmt.Sprintf("   ✗ %s failed", displayName), msg.toolName)
		}
		// Show truncated output, preserving indentation
		// Skip output for certain tools
		skipOutputTools := map[string]bool{
			"ask_user":             true,
			"ask_user_select":      true,
			"cloud_check_auth":     true,
			"cloud_login":          true,
			"cloud_wait_for_login": true,
			"cloud_get_clients":    true,
		}
		if msg.output != "" && !skipOutputTools[msg.toolName] {
			outputLines := strings.Split(msg.output, "\n")
			maxLines := 4
			for i, line := range outputLines {
				if i >= maxLines {
					m.addLog("dim", fmt.Sprintf("     ... (%d more lines)", len(outputLines)-maxLines), "")
					break
				}
				// Skip completely empty lines but preserve lines with only whitespace/indentation
				if line == "" {
					continue
				}
				// Preserve indentation - just add our prefix
				maxWidth := max(m.width-12, 40)
				displayLine := line
				if len(displayLine) > maxWidth {
					displayLine = displayLine[:maxWidth-3] + "..."
				}
				m.addLog("dim", "     "+displayLine, "")
			}
		}

	case agentTextMsg:
		if msg.streaming {
			m.streamingText = msg.text
			m.lastToolComplete = false
		} else {
			m.streamingText = ""
			m.lastToolComplete = false
			if msg.text != "" && msg.text != m.lastAgentMessage {
				m.lastAgentMessage = msg.text
				m.addLog("spacing", "", "")
				// Format agent messages with proper markdown spacing
				// Wrap text at 120 chars max for readability
				maxWidth := min(max(m.width-8, 40), 120)
				lines := strings.Split(msg.text, "\n")
				for i, line := range lines {
					trimmed := strings.TrimSpace(line)
					switch {
					case trimmed == "":
						// Keep blank lines for markdown formatting
						m.addLog("spacing", "", "")
					case strings.HasPrefix(trimmed, "#"):
						if i > 0 {
							m.addLog("spacing", "", "")
						}
						m.addLog("agent-header", trimmed, "")
					default:
						// Wrap long lines for readability
						wrapped := wrapText(trimmed, maxWidth)
						for _, wrappedLine := range strings.Split(wrapped, "\n") {
							m.addLog("agent", wrappedLine, "")
						}
					}
				}
			}
		}

	case thinkingMsg:
		m.thinking = msg.thinking
		if msg.thinking {
			m.lastToolComplete = true
			cmds = append(cmds, m.spinner.Tick)
		}

	case userInputRequestMsg:
		m.userInputMode = true
		m.userInputPrompt = msg.question
		m.userInputTextarea.Reset()
		m.userInputTextarea.SetHeight(1) // Reset to single line
		m.userInputTextarea.Focus()
		m.userInputCh = msg.responseCh
		m.updateViewportSize() // Shrink content area to make room for textarea
		m.addLog("spacing", "", "")
		// Wrap the question text for readability
		maxWidth := min(max(m.width-8, 40), 120)
		wrapped := wrapText(msg.question, maxWidth)
		lines := strings.Split(wrapped, "\n")
		for i, line := range lines {
			if i == 0 {
				m.addLog("agent", "🤖 "+line, "")
			} else {
				m.addLog("agent", "   "+line, "")
			}
		}

	case userSelectRequestMsg:
		m.userSelectMode = true
		m.userSelectPrompt = msg.question
		m.userSelectOptions = msg.options
		m.userSelectIndex = 0
		m.userSelectCh = msg.responseCh
		m.updateViewportSize()
		m.addLog("spacing", "", "")
		// Wrap the question text for readability
		maxWidth := min(max(m.width-8, 40), 120)
		wrapped := wrapText(msg.question, maxWidth)
		lines := strings.Split(wrapped, "\n")
		for i, line := range lines {
			if i == 0 {
				m.addLog("agent", "🤖 "+line, "")
			} else {
				m.addLog("agent", "   "+line, "")
			}
		}

	case portConflictMsg:
		m.portConflictMode = true
		m.portConflictPort = msg.port
		m.portConflictCh = msg.responseCh
		m.updateViewportSize() // Shrink content area for port conflict prompt
		m.addLog("spacing", "", "")
		m.addLog("error", fmt.Sprintf("⚠️  Port %d is already in use", msg.port), "")

	case rerunConfirmMsg:
		m.rerunConfirmMode = true
		m.rerunConfirmCh = msg.responseCh
		m.currentPhase = "Setup Complete"
		m.phaseNumber = m.totalPhases
		m.addLog("spacing", "", "")
		m.addLog("success", "✅ Tusk Drift setup is already complete!", "")
		m.addLog("spacing", "", "")
		m.addLog("plain", "If you'd like to rerun the setup from scratch, press [y].", "")
		m.addLog("plain", "Otherwise, press [q] or [Esc] to exit.", "")
		cmds = append(cmds, m.progress.SetPercent(1.0))

	case cloudSetupPromptMsg:
		m.cloudSetupPromptMode = true
		m.cloudSetupPromptCh = msg.responseCh
		m.addLog("spacing", "", "")
		m.addLog("plain", "───────────────────────────────────────────────────────────", "")
		m.addLog("spacing", "", "")
		m.addLog("success", "✅ Local setup complete!", "")
		m.addLog("spacing", "", "")
		m.addLog("plain", "Would you like to continue with Tusk Drift Cloud setup?", "")
		m.addLog("plain", "This will connect your repository and enable cloud features.", "")
		m.addLog("spacing", "", "")
		m.addLog("dim", "Press [y] to continue with cloud setup, or [n] to finish.", "")

	case permissionRequestMsg:
		m.permissionMode = true
		m.permissionTool = msg.toolName
		m.permissionPreview = msg.preview
		m.permissionCommandPrefixes = msg.commandPrefixes
		m.permissionCh = msg.responseCh
		m.updateViewportSize() // Shrink content area for permission prompt
		m.addLog("spacing", "", "")
		displayName := getToolDisplayName(msg.toolName)
		m.addLog("dim", fmt.Sprintf("🔐 Permission required: %s", displayName), "")
		// Show the preview on a separate line for clarity
		if msg.preview != "" {
			for _, line := range strings.Split(msg.preview, "\n") {
				if strings.TrimSpace(line) != "" {
					m.addLog("dim", "   "+line, "")
				}
			}
		}

	case errorMsg:
		m.hasError = true
		m.addLog("spacing", "", "")
		m.addLog("error", "❌ "+msg.err.Error(), "")

	case completedMsg:
		m.completed = true
		m.thinking = false
		m.currentTool = ""
		m.addLog("spacing", "", "")
		m.addLog("success", "🎉 Setup complete!", "")
		m.addLog("dim", "   Check .tusk/setup/SETUP_REPORT.md for details.", "")
		m.addLog("spacing", "", "")
		m.addLog("plain", "───────────────────────────────────────────────────────────", "")
		m.addLog("spacing", "", "")
		if len(msg.removableFiles) > 0 {
			m.addLog("plain", "You can safely remove these files from your .tusk directory:", "")
			for _, file := range msg.removableFiles {
				m.addLog("plain", "  • "+file, "")
			}
			m.addLog("spacing", "", "")
		}
		cmds = append(cmds, m.progress.SetPercent(1.0))
		// Mark all todos as done
		for i := range m.todoItems {
			m.todoItems[i].done = true
			m.todoItems[i].active = false
		}

	case eligibilityCompletedMsg:
		m.completed = true
		m.thinking = false
		m.currentTool = ""
		m.addLog("spacing", "", "")
		m.addLog("success", "✅ Eligibility check complete!", "")
		m.addLog("dim", "   Check .tusk/eligibility-report.json for details.", "")
		cmds = append(cmds, m.progress.SetPercent(1.0))
		// Mark all todos as done
		for i := range m.todoItems {
			m.todoItems[i].done = true
			m.todoItems[i].active = false
		}

	case abortedMsg:
		m.completed = true
		m.thinking = false
		m.currentTool = ""
		// No "Setup complete!" or cleanup instructions for aborted setup

	case fatalErrorMsg:
		// Fatal error - auto-quit after a short delay to let user see the error
		m.hasError = true
		m.completed = true // Allow q/Esc to quit
		m.thinking = false
		m.currentTool = ""
		// Show recovery guidance
		m.addLog("spacing", "", "")
		m.addLog("plain", "───────────────────────────────────────────────────────────", "")
		m.addLog("spacing", "", "")
		m.addLog("plain", RecoveryGuidance(), "")
		m.addLog("spacing", "", "")
		cmds = append(cmds, tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
			return autoQuitMsg{}
		}))

	case sidebarUpdateMsg:
		m.updateSidebarInfo(msg.key, msg.value)
	}

	// Update viewport
	if m.viewportReady {
		m.updateViewportContent()
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *TUIModel) updateTodoForPhase(phaseNum int) {
	m.todoMutex.Lock()
	defer m.todoMutex.Unlock()

	for i := range m.todoItems {
		switch {
		case i < phaseNum-1:
			m.todoItems[i].done = true
			m.todoItems[i].active = false
		case i == phaseNum-1:
			m.todoItems[i].active = true
			m.todoItems[i].done = false
		default:
			m.todoItems[i].active = false
			m.todoItems[i].done = false
		}
	}
}

func (m *TUIModel) updateSidebarInfo(key, value string) {
	// Check if key exists
	found := false
	for _, k := range m.sidebarOrder {
		if k == key {
			found = true
			break
		}
	}
	if !found {
		m.sidebarOrder = append(m.sidebarOrder, key)
	}
	m.sidebarInfo[key] = value
}

func (m *TUIModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.userInputMode {
		switch msg.String() {
		case "enter":
			// Enter always submits
			value := m.userInputTextarea.Value()
			if m.userInputCh != nil {
				m.userInputCh <- value
			}
			m.userInputMode = false
			m.userInputTextarea.Blur()
			m.updateViewportSize() // Reclaim space from textarea
			// Log the response (truncate if multiline for display)
			displayValue := value
			if strings.Contains(value, "\n") {
				lines := strings.Split(value, "\n")
				displayValue = fmt.Sprintf("%s... (%d lines)", lines[0], len(lines))
			}
			m.addLog("dim", "   > "+displayValue, "")
			return m, nil
		case "alt+enter", "ctrl+j":
			// Alt+Enter or Ctrl+J adds a newline
			m.userInputTextarea.InsertString("\n")
			// Grow textarea height to show multiline content (max 5 lines)
			lineCount := strings.Count(m.userInputTextarea.Value(), "\n") + 1
			m.userInputTextarea.SetHeight(min(lineCount, 5))
			// Update viewport size to account for taller footer
			m.updateViewportSize()
			return m, nil
		case "ctrl+c":
			if m.userInputCh != nil {
				close(m.userInputCh)
			}
			m.userInputMode = false
			m.userInputTextarea.Blur()
			return m, m.initiateShutdown()
		case "esc":
			// Cancel input without submitting
			if m.userInputCh != nil {
				m.userInputCh <- ""
			}
			m.userInputMode = false
			m.userInputTextarea.Blur()
			m.updateViewportSize() // Reclaim space from textarea
			m.addLog("dim", "   (cancelled)", "")
			return m, nil
		case "pgup":
			// Allow scrolling viewport while in input mode
			m.autoScroll = false
			m.viewport.HalfPageUp()
			return m, nil
		case "pgdown":
			// Allow scrolling viewport while in input mode
			m.viewport.HalfPageDown()
			if m.viewport.AtBottom() {
				m.autoScroll = true
			}
			return m, nil
		default:
			// Forward all other keys to textarea (handles paste, typing, backspace, etc.)
			var cmd tea.Cmd
			m.userInputTextarea, cmd = m.userInputTextarea.Update(msg)
			// Auto-grow height based on content (max 5 lines)
			lineCount := strings.Count(m.userInputTextarea.Value(), "\n") + 1
			newHeight := min(lineCount, 5)
			if newHeight != m.userInputTextarea.Height() {
				m.userInputTextarea.SetHeight(newHeight)
				m.updateViewportSize()
			}
			return m, cmd
		}
	}

	if m.portConflictMode {
		switch msg.String() {
		case "y", "Y":
			if m.portConflictCh != nil {
				m.portConflictCh <- true
			}
			m.portConflictMode = false
			m.updateViewportSize() // Reclaim space
			m.addLog("dim", "   Killing process on port...", "")
			return m, nil
		case "n", "N", "esc":
			if m.portConflictCh != nil {
				m.portConflictCh <- false
			}
			m.portConflictMode = false
			m.updateViewportSize() // Reclaim space
			return m, m.initiateShutdown()
		case "ctrl+c":
			if m.portConflictCh != nil {
				m.portConflictCh <- false
			}
			m.portConflictMode = false
			m.updateViewportSize() // Reclaim space
			return m, m.initiateShutdown()
		}
		return m, nil
	}

	if m.rerunConfirmMode {
		switch msg.String() {
		case "y", "Y":
			if m.rerunConfirmCh != nil {
				m.rerunConfirmCh <- true
			}
			m.rerunConfirmMode = false
			m.addLog("dim", "   Starting fresh setup...", "")
			return m, nil
		case "q", "esc":
			if m.rerunConfirmCh != nil {
				m.rerunConfirmCh <- false
			}
			m.rerunConfirmMode = false
			m.completed = true
			return m, m.initiateShutdown()
		case "ctrl+c":
			if m.rerunConfirmCh != nil {
				m.rerunConfirmCh <- false
			}
			m.rerunConfirmMode = false
			return m, m.initiateShutdown()
		}
		return m, nil
	}

	if m.cloudSetupPromptMode {
		switch msg.String() {
		case "y", "Y":
			if m.cloudSetupPromptCh != nil {
				m.cloudSetupPromptCh <- true
			}
			m.cloudSetupPromptMode = false
			m.addLog("spacing", "", "")
			m.addLog("dim", "   Continuing with cloud setup...", "")
			return m, nil
		case "n", "N", "q", "esc":
			if m.cloudSetupPromptCh != nil {
				m.cloudSetupPromptCh <- false
			}
			m.cloudSetupPromptMode = false
			m.addLog("spacing", "", "")
			m.addLog("dim", "   Skipping cloud setup.", "")
			return m, nil
		case "ctrl+c":
			if m.cloudSetupPromptCh != nil {
				m.cloudSetupPromptCh <- false
			}
			m.cloudSetupPromptMode = false
			return m, m.initiateShutdown()
		}
		return m, nil
	}

	if m.userSelectMode {
		switch msg.String() {
		case "up", "k":
			if m.userSelectIndex > 0 {
				m.userSelectIndex--
			}
			return m, nil
		case "down", "j":
			if m.userSelectIndex < len(m.userSelectOptions)-1 {
				m.userSelectIndex++
			}
			return m, nil
		case "enter":
			if len(m.userSelectOptions) > 0 && m.userSelectCh != nil {
				selected := m.userSelectOptions[m.userSelectIndex]
				m.userSelectCh <- selected.ID
				m.addLog("dim", "   > "+selected.Label, "")
			}
			m.userSelectMode = false
			m.updateViewportSize()
			return m, nil
		case "esc":
			if m.userSelectCh != nil {
				m.userSelectCh <- ""
			}
			m.userSelectMode = false
			m.updateViewportSize()
			m.addLog("dim", "   (cancelled)", "")
			return m, nil
		case "ctrl+c":
			if m.userSelectCh != nil {
				close(m.userSelectCh)
			}
			m.userSelectMode = false
			return m, m.initiateShutdown()
		case "pgup":
			m.autoScroll = false
			m.viewport.HalfPageUp()
			return m, nil
		case "pgdown":
			m.viewport.HalfPageDown()
			if m.viewport.AtBottom() {
				m.autoScroll = true
			}
			return m, nil
		}
		return m, nil
	}

	if m.permissionDenyMode {
		switch msg.String() {
		case "enter":
			alternative := strings.TrimSpace(m.permissionDenyBuffer)
			if alternative == "" {
				// Don't allow empty - must provide alternative
				return m, nil
			}
			if m.permissionCh != nil {
				m.permissionCh <- "deny:" + alternative
				m.addLog("dim", "   → "+alternative, "")
			}
			m.permissionDenyMode = false
			m.permissionMode = false
			m.permissionDenyBuffer = ""
			m.updateViewportSize() // Reclaim space
			return m, nil
		case "backspace":
			if len(m.permissionDenyBuffer) > 0 {
				m.permissionDenyBuffer = m.permissionDenyBuffer[:len(m.permissionDenyBuffer)-1]
			}
			return m, nil
		case "esc":
			// Go back to permission prompt (still has 2-line footer)
			m.permissionDenyMode = false
			m.permissionDenyBuffer = ""
			return m, nil
		case "ctrl+c":
			if m.permissionCh != nil {
				m.permissionCh <- "deny"
			}
			m.permissionDenyMode = false
			m.permissionMode = false
			m.permissionDenyBuffer = ""
			m.updateViewportSize() // Reclaim space
			return m, m.initiateShutdown()
		default:
			if len(msg.String()) == 1 {
				m.permissionDenyBuffer += msg.String()
			}
			return m, nil
		}
	}

	if m.permissionMode {
		switch msg.String() {
		case "y", "Y":
			if m.permissionCh != nil {
				m.permissionCh <- "approve"
			}
			m.permissionMode = false
			m.permissionCommandPrefixes = nil
			m.updateViewportSize() // Reclaim space
			m.addLog("dim", "   ✓ Allowed", "")
			return m, nil
		case "t", "T":
			if m.permissionCh != nil {
				if len(m.permissionCommandPrefixes) > 0 {
					// For commands, approve the specific command prefixes
					m.permissionCh <- "approve_commands:" + strings.Join(m.permissionCommandPrefixes, ",")
					m.addLog("dim", fmt.Sprintf("   ✓ Allowed (will auto-allow future `%s` commands)", strings.Join(m.permissionCommandPrefixes, "`, `")), "")
				} else {
					// Approve the tool type
					m.permissionCh <- "approve_tool_type"
					m.addLog("dim", fmt.Sprintf("   ✓ Allowed (will auto-allow future %s)", getToolDisplayNamePlural(m.permissionTool)), "")
				}
			}
			m.permissionMode = false
			m.permissionCommandPrefixes = nil
			m.updateViewportSize() // Reclaim space
			return m, nil
		case "a", "A":
			if m.permissionCh != nil {
				m.permissionCh <- "approve_all"
			}
			m.permissionMode = false
			m.permissionCommandPrefixes = nil
			m.updateViewportSize() // Reclaim space
			m.addLog("dim", "   ✓ Allowed (will auto-allow all future actions)", "")
			return m, nil
		case "n", "N":
			// Switch to deny mode with text input for alternative
			m.permissionDenyMode = true
			m.permissionDenyBuffer = ""
			return m, nil
		case "esc":
			if m.permissionCh != nil {
				m.permissionCh <- "deny"
			}
			m.permissionMode = false
			m.permissionCommandPrefixes = nil
			m.updateViewportSize() // Reclaim space
			m.addLog("dim", "   ✗ Denied", "")
			return m, nil
		case "ctrl+c":
			if m.permissionCh != nil {
				m.permissionCh <- "deny"
			}
			m.permissionMode = false
			m.permissionCommandPrefixes = nil
			m.updateViewportSize() // Reclaim space
			return m, m.initiateShutdown()
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "esc":
		// Only allow q/esc to quit when agent has completed
		if m.completed {
			return m, m.initiateShutdown()
		}
		// Otherwise ignore - only Ctrl-C can stop a running agent
		return m, nil
	case "ctrl+c":
		return m, m.initiateShutdown()
	case "up", "k":
		m.autoScroll = false
		m.viewport.ScrollUp(1)
	case "down", "j":
		m.viewport.ScrollDown(1)
		if m.viewport.AtBottom() {
			m.autoScroll = true
		}
	case "g":
		m.autoScroll = false
		m.viewport.GotoTop()
	case "G":
		m.autoScroll = true
		m.viewport.GotoBottom()
	}

	return m, nil
}

// initiateShutdown handles graceful shutdown with double Ctrl-C
func (m *TUIModel) initiateShutdown() tea.Cmd {
	if m.shutdownRequested {
		// Second Ctrl-C - force quit immediately
		m.quitting = true
		m.cancel()
		return tea.Quit
	}

	// First Ctrl-C - graceful shutdown
	m.shutdownRequested = true
	m.shutdownTime = time.Now()
	m.cancel() // Cancel the context to stop the agent

	return tea.Batch(
		func() tea.Msg {
			return shutdownStartedMsg{}
		},
		// Auto-quit after a short delay to allow cleanup
		tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return autoQuitMsg{}
		}),
	)
}

func (m *TUIModel) addLog(level, message, toolName string) {
	m.logMutex.Lock()
	defer m.logMutex.Unlock()

	m.logs = append(m.logs, logEntry{
		timestamp: time.Now(),
		level:     level,
		message:   message,
		toolName:  toolName,
	})

	if len(m.logs) > m.maxLogs {
		m.logs = m.logs[len(m.logs)-m.maxLogs:]
	}
}

func (m *TUIModel) updateViewportSize() {
	headerHeight := 5 // Title + empty line + status + progress + spacing
	if m.phaseDesc != "" {
		headerHeight = 6 // +1 for description subtitle
	}
	infoPanelHeight := 3 // Info panel with border (1 content line + 2 border lines)
	spacerHeight := 5    // 5 lines of spacing between content and footer
	footerHeight := 1    // Help text line

	// Account for multi-line footers in special modes
	switch {
	case m.userInputMode:
		textareaHeight := m.userInputTextarea.Height()
		footerHeight = textareaHeight + 1 // textarea + help text
	case m.userSelectMode:
		footerHeight = len(m.userSelectOptions) + 4 // header + options + empty line + help text
	case m.permissionMode:
		footerHeight = 1 // just help text (prompt is shown in content area)
	case m.permissionDenyMode:
		footerHeight = 2 // input line + help text
	case m.portConflictMode:
		footerHeight = 2 // prompt + help text
	}

	contentHeight := m.height - headerHeight - infoPanelHeight - spacerHeight - footerHeight

	contentHeight = max(contentHeight, 5)

	// Full width for main content (no sidebar)
	mainWidth := m.width - 2
	mainWidth = max(mainWidth, 40)

	if !m.viewportReady {
		m.viewport = viewport.New(mainWidth, contentHeight)
		m.viewport.Style = lipgloss.NewStyle()
		m.viewportReady = true
	} else {
		m.viewport.Width = mainWidth
		m.viewport.Height = contentHeight
	}

	m.updateViewportContent()
}

func (m *TUIModel) updateViewportContent() {
	m.logMutex.RLock()
	defer m.logMutex.RUnlock()

	var lines []string
	for _, entry := range m.logs {
		styled := m.styleLogEntry(entry)
		lines = append(lines, styled)
	}

	// Add streaming text with cursor
	if m.streamingText != "" {
		lines = append(lines, "")
		// Wrap streaming text - cap at 120 for readability on wide terminals
		maxWidth := min(max(m.width-8, 40), 120)
		wrapped := wrapText(m.streamingText, maxWidth)
		lines = append(lines, wrapped+"▌")
	}

	if m.currentTool != "" {
		lines = append(lines, "")
		execText := m.renderExecutingIndicator()
		lines = append(lines, execText)
	}

	// Show permission prompt after executing indicator for better visibility
	if m.permissionMode {
		lines = append(lines, "")
		displayName := getToolDisplayName(m.permissionTool)
		promptStyle := styles.WarningStyle
		lines = append(lines, promptStyle.Render(fmt.Sprintf("Allow %s?", displayName)))
	}

	if m.thinking && m.lastToolComplete && m.currentTool == "" {
		lines = append(lines, "")
		thinkingText := m.renderThinkingIndicator()
		lines = append(lines, thinkingText)
	}

	content := strings.Join(lines, "\n")
	m.viewport.SetContent(content)

	if m.autoScroll {
		m.viewport.GotoBottom()
	}
}

func (m *TUIModel) renderThinkingIndicator() string {
	// Pulsating dot effect
	dots := []string{"○", "◔", "◑", "◕", "●", "◕", "◑", "◔"}
	dotIdx := (m.pulsePhase / 2) % len(dots)
	dot := dots[dotIdx]

	thinkingStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.PrimaryColor))
	return thinkingStyle.Render(dot + " Thinking...")
}

func (m *TUIModel) renderExecutingIndicator() string {
	dots := []string{"○", "◔", "◑", "◕", "●", "◕", "◑", "◔"}
	dotIdx := (m.pulsePhase / 2) % len(dots)
	dot := dots[dotIdx]

	execStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.PrimaryColor))

	if m.currentTool == "ask_user" {
		return execStyle.Render(fmt.Sprintf("%s Waiting for response...", dot))
	}

	displayName := getToolDisplayName(m.currentTool)
	return execStyle.Render(fmt.Sprintf("%s Executing [%s]...", dot, strings.ToLower(displayName)))
}

func (m *TUIModel) styleLogEntry(entry logEntry) string {
	switch entry.level {
	case "spacing":
		return ""
	case "phase-banner":
		style := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color(styles.SecondaryColor)).
			Padding(0, 1)
		return "\n" + style.Render(entry.message)
	case "phase":
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(styles.PrimaryColor)).Render(entry.message)
	case "tool-start":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Render("🔧 " + entry.message)
	case "tool-complete":
		return styles.SuccessStyle.Render(entry.message)
	case "error":
		return styles.ErrorStyle.Render(entry.message)
	case "warning":
		return styles.WarningStyle.Render(entry.message)
	case "success":
		return styles.SuccessStyle.Render(entry.message)
	case "agent":
		return entry.message
	case "agent-header":
		// Bold for markdown headers
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render(entry.message)
	case "dim":
		return styles.DimStyle.Render(entry.message)
	case "plain":
		return entry.message
	default:
		return entry.message
	}
}

func (m *TUIModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	// Top-down layout:
	// 1. Title + Info panel (narrow)
	// 2. Progress bar
	// 3. Main content (large)
	// 4. Spacer (5 lines)
	// 5. Footer

	header := m.renderHeader()
	infoPanel := m.renderInfoPanel()
	content := m.renderMainContent()
	spacer := strings.Repeat("\n", 4) // 5 lines of spacing (4 newlines = 5 lines)
	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, header, infoPanel, content, spacer, footer)
}

func (m *TUIModel) renderInfoPanel() string {
	// Info panel showing only detected info (no steps)
	// Build detected info items
	var detectedItems []string
	for _, key := range m.sidebarOrder {
		value := m.sidebarInfo[key]
		keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
		detectedItems = append(detectedItems, keyStyle.Render(key+": ")+value)
	}

	var content string
	if len(detectedItems) == 0 {
		content = styles.DimStyle.Render("Analyzing project...")
	} else {
		// Join with separator, truncate if too long
		separator := "  •  "
		content = strings.Join(detectedItems, separator)

		// Calculate available width for content (accounting for border and padding)
		contentWidth := m.width - 6
		contentWidth = max(contentWidth, 40)

		if lipgloss.Width(content) > contentWidth {
			runes := []rune(content)
			if len(runes) > contentWidth-3 {
				content = string(runes[:contentWidth-3]) + "..."
			}
		}
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("238")).
		Padding(0, 1).
		Width(m.width - 2)

	return boxStyle.Render(content)
}

func (m *TUIModel) renderMainContent() string {
	if m.viewportReady {
		content := m.viewport.View()
		contentStyle := lipgloss.NewStyle().Height(m.viewport.Height)
		return contentStyle.Render(content)
	}
	return ""
}

func (m *TUIModel) renderHeader() string {
	titleText := "• TUSK DRIFT AUTO SETUP •"
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(styles.PrimaryColor)).
		Width(m.width).
		Align(lipgloss.Center)
	title := titleStyle.Render(titleText)

	var statusText string
	if m.currentPhase != "" {
		statusText = fmt.Sprintf("Phase %d/%d: %s", m.phaseNumber, m.totalPhases, m.currentPhase)
		if m.phaseNumber < m.totalPhases {
			nextPhase := m.getNextPhaseName()
			if nextPhase != "" {
				dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
				statusText += dimStyle.Render(fmt.Sprintf("  →  Next: %s", nextPhase))
			}
		}
	} else {
		statusText = "Starting..."
	}

	statusWidth := lipgloss.Width(statusText)
	padding := max(m.width-statusWidth, 1)
	statusLine := statusText + strings.Repeat(" ", padding)

	var descLine string
	if m.phaseDesc != "" {
		descStyle := lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color("245"))
		descLine = descStyle.Render(m.phaseDesc)
	}

	if m.hideProgressBar {
		if descLine != "" {
			return lipgloss.JoinVertical(lipgloss.Left, title, "", statusLine, descLine, "")
		}
		return lipgloss.JoinVertical(lipgloss.Left, title, "", statusLine, "")
	}

	progressWidth := m.width - 2
	progressWidth = max(progressWidth, 20)
	m.progress.Width = progressWidth
	progressBar := m.progress.View()

	if descLine != "" {
		return lipgloss.JoinVertical(lipgloss.Left, title, "", statusLine, descLine, progressBar, "")
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, "", statusLine, progressBar, "")
}

func (m *TUIModel) renderFooter() string {
	var helpText string

	switch {
	case m.userInputMode:
		m.userInputTextarea.SetWidth(min(m.width-4, 120))
		textareaView := m.userInputTextarea.View()
		helpText = m.applyDragHint("Enter: submit • Shift+Enter/Ctrl+J: newline • Esc: cancel")
		return textareaView + "\n" + components.Footer(m.width, helpText)
	case m.portConflictMode:
		prompt := styles.WarningStyle.Render(fmt.Sprintf("Kill process on port %d? (y/n)", m.portConflictPort))
		helpText = m.applyDragHint("y: yes • n: no • Ctrl+C: cancel")
		return prompt + "\n" + components.Footer(m.width, helpText)
	case m.rerunConfirmMode:
		helpText = m.applyDragHint("y: rerun setup • q/Esc: exit")
		return components.Footer(m.width, helpText)
	case m.cloudSetupPromptMode:
		helpText = m.applyDragHint("y: continue with cloud setup • n: skip")
		return components.Footer(m.width, helpText)
	case m.userSelectMode:
		color := lipgloss.Color(styles.PrimaryColor)
		headerStyle := lipgloss.NewStyle().
			Foreground(color).
			Bold(true)
		selectedStyle := lipgloss.NewStyle().
			Foreground(color).
			Bold(true)
		normalStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("250"))

		var optionLines []string
		optionLines = append(optionLines, headerStyle.Render("Select an option:\n"))
		for i, opt := range m.userSelectOptions {
			if i == m.userSelectIndex {
				optionLines = append(optionLines, selectedStyle.Render("  › "+opt.Label))
			} else {
				optionLines = append(optionLines, normalStyle.Render("    "+opt.Label))
			}
		}
		helpText = m.applyDragHint("↑/↓: navigate • Enter: select • Esc: cancel")
		return strings.Join(optionLines, "\n") + "\n\n" + components.Footer(m.width, helpText)
	case m.permissionDenyMode:
		inputStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)
		prompt := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("Suggest alternative: ")
		inputLine := prompt + inputStyle.Render(m.permissionDenyBuffer+"▌")
		helpText = m.applyDragHint("Enter: submit • Esc: back")
		return inputLine + "\n" + components.Footer(m.width, helpText)
	case m.permissionMode:
		var tOption string
		if len(m.permissionCommandPrefixes) > 0 {
			display := "`" + strings.Join(m.permissionCommandPrefixes, "`, `") + "`"
			if len(display) > 25 {
				display = display[:22] + "..."
			}
			tOption = fmt.Sprintf("t: allow all %s commands", display)
		} else {
			tOption = "t: allow all " + getToolDisplayNamePlural(m.permissionTool)
		}
		helpText = m.applyDragHint(fmt.Sprintf("y: allow once • %s • a: allow all actions • n: deny & suggest", tOption))
		return components.Footer(m.width, helpText)
	case m.completed:
		helpText = "q/Esc: quit"
	case m.shutdownRequested:
		helpText = "Exiting... (Ctrl+C to force)"
	default:
		// When agent is active, only Ctrl-C can stop
		helpText = "↑/↓: scroll • g/G: top/bottom • Ctrl+C: stop"
	}

	return components.Footer(m.width, m.applyDragHint(helpText))
}

// applyDragHint appends a platform-appropriate text selection hint to the right side of the footer text
func (m *TUIModel) applyDragHint(text string) string {
	if !m.dragHintVisible {
		return text
	}

	hint := "Hold Shift to select text"
	if runtime.GOOS == "darwin" {
		hint = "Hold Option to select text"
	}
	hintWidth := lipgloss.Width(hint)
	textWidth := lipgloss.Width(text)
	// Calculate padding to right-align hint flush to the right edge
	padding := m.width - textWidth - hintWidth
	if padding >= 2 {
		return text + strings.Repeat(" ", padding) + hint
	} else if m.width > hintWidth {
		// Not enough space for both, just show the hint right-aligned
		return strings.Repeat(" ", m.width-hintWidth) + hint
	}
	return text
}

// Helper functions

// formatToolInput formats tool input JSON for display with bullet points
func formatToolInput(input string, maxWidth int) string {
	// Try to parse as JSON
	input = strings.TrimSpace(input)
	if input == "" || input == "{}" {
		return ""
	}

	// Parse JSON into a map to get key-value pairs
	var data map[string]any
	if err := json.Unmarshal([]byte(input), &data); err != nil {
		// Fallback: just return cleaned input
		return input
	}

	// Format each key-value pair as a bullet point
	var result []string
	for key, value := range data {
		// Capitalize key and convert underscores to spaces
		displayKey := strings.ToUpper(key[:1]) + key[1:]
		displayKey = strings.ReplaceAll(displayKey, "_", " ")

		var displayValue string
		switch v := value.(type) {
		case string:
			displayValue = v
		case map[string]any, []any:
			// For complex values, marshal back to JSON
			if b, err := json.Marshal(v); err == nil {
				displayValue = string(b)
			} else {
				displayValue = fmt.Sprintf("%v", v)
			}
		default:
			displayValue = fmt.Sprintf("%v", v)
		}

		bulletPrefix := "• " + displayKey + ": "
		availableWidth := maxWidth - len(bulletPrefix)
		availableWidth = max(availableWidth, 20)
		if len(displayValue) > availableWidth {
			displayValue = displayValue[:availableWidth-3] + "..."
		}

		result = append(result, bulletPrefix+displayValue)
	}

	return strings.Join(result, "\n")
}

// wrapText wraps text to fit within maxWidth
func wrapText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = 80
	}

	var lines []string
	for line := range strings.SplitSeq(text, "\n") {
		if len(line) <= maxWidth {
			lines = append(lines, line)
			continue
		}

		// Wrap long lines, find a good break point
		for len(line) > maxWidth {
			breakAt := maxWidth
			for i := maxWidth - 1; i > maxWidth/2; i-- {
				if line[i] == ' ' {
					breakAt = i
					break
				}
			}
			lines = append(lines, line[:breakAt])
			line = strings.TrimSpace(line[breakAt:])
		}
		if line != "" {
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, "\n")
}

// extractPathFromInput extracts the "path" field from tool input JSON
func extractPathFromInput(input string) string {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return ""
	}
	return params.Path
}

// GetFinalOutput returns the log content for printing after exit
// Renders a complete view similar to the TUI layout
func (m *TUIModel) GetFinalOutput() string {
	m.logMutex.RLock()
	defer m.logMutex.RUnlock()

	width := m.width
	if width == 0 {
		width = 80
	}

	var lines []string

	titleText := "• TUSK DRIFT AUTO SETUP •"
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(styles.PrimaryColor)).
		Width(width).
		Align(lipgloss.Center)
	lines = append(lines, "")
	lines = append(lines, titleStyle.Render(titleText))

	var statusText string
	switch {
	case m.hasError:
		statusText = fmt.Sprintf("✗ Failed at phase %d/%d: %s", m.phaseNumber, m.totalPhases, m.currentPhase)
	case m.completed:
		statusText = fmt.Sprintf("✓ Completed %d/%d phases", m.phaseNumber, m.totalPhases)
	case m.shutdownRequested:
		statusText = fmt.Sprintf("⏹ Stopped at phase %d/%d: %s", m.phaseNumber, m.totalPhases, m.currentPhase)
	default:
		statusText = fmt.Sprintf("Phase %d/%d: %s", m.phaseNumber, m.totalPhases, m.currentPhase)
	}
	statusStyle := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
	lines = append(lines, statusStyle.Render(statusText))

	// Progress bar (static representation)
	var progressPercent float64
	if m.totalPhases > 0 {
		if m.completed {
			progressPercent = 1.0
		} else {
			progressPercent = float64(m.phaseNumber-1) / float64(m.totalPhases)
		}
	}
	progressWidth := width - 4
	filledWidth := int(float64(progressWidth) * progressPercent)
	emptyWidth := progressWidth - filledWidth
	progressBar := lipgloss.NewStyle().Foreground(lipgloss.Color(styles.PrimaryColor)).Render(strings.Repeat("█", filledWidth))
	progressBar += lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(strings.Repeat("░", emptyWidth))
	lines = append(lines, "  "+progressBar)
	lines = append(lines, "")

	// Info panel (detected info)
	if len(m.sidebarOrder) > 0 {
		var detectedItems []string
		for _, key := range m.sidebarOrder {
			value := m.sidebarInfo[key]
			keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
			detectedItems = append(detectedItems, keyStyle.Render(key+": ")+value)
		}
		separator := "  •  "
		infoContent := strings.Join(detectedItems, separator)

		boxStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1).
			Width(width - 2)
		lines = append(lines, boxStyle.Render(infoContent))
		lines = append(lines, "")
	}

	lines = append(lines, styles.DimStyle.Render(strings.Repeat("─", width)))
	lines = append(lines, "")

	// Log entries
	for _, entry := range m.logs {
		if entry.level == "spacing" {
			lines = append(lines, "") // Preserve spacing as empty lines
			continue
		}
		styled := m.styleLogEntry(entry)
		if styled != "" {
			lines = append(lines, styled)
		}
	}

	lines = append(lines, "")

	return strings.Join(lines, "\n")
}

// Public methods for sending messages to the TUI

func (m *TUIModel) SendPhaseChange(program *tea.Program, name, desc string, num, total int) {
	program.Send(phaseChangedMsg{
		phaseName:   name,
		phaseDesc:   desc,
		phaseNumber: num,
		totalPhases: total,
	})
}

func (m *TUIModel) SendToolStart(program *tea.Program, name, input string) {
	program.Send(toolStartMsg{toolName: name, input: input})
}

func (m *TUIModel) SendToolComplete(program *tea.Program, name string, success bool, output string) {
	program.Send(toolCompleteMsg{toolName: name, success: success, output: output})
}

func (m *TUIModel) SendAgentText(program *tea.Program, text string, streaming bool) {
	program.Send(agentTextMsg{text: text, streaming: streaming})
}

func (m *TUIModel) SendThinking(program *tea.Program, thinking bool) {
	program.Send(thinkingMsg{thinking: thinking})
}

func (m *TUIModel) SendError(program *tea.Program, err error) {
	program.Send(errorMsg{err: err})
}

func (m *TUIModel) SendFatalError(program *tea.Program, err error) {
	program.Send(errorMsg{err: err})
	program.Send(fatalErrorMsg{})
}

func (m *TUIModel) SendCompleted(program *tea.Program, workDir string) {
	// Check which cleanup files exist
	var removableFiles []string

	// Check if the entire .tusk/setup/ directory exists and has files
	setupDir := filepath.Join(workDir, ".tusk", "setup")
	if _, err := os.Stat(setupDir); err == nil {
		removableFiles = append(removableFiles, ".tusk/setup/ - Setup progress, reports, and cache")
	}

	program.Send(completedMsg{removableFiles: removableFiles})
}

func (m *TUIModel) SendAborted(program *tea.Program, reason string) {
	program.Send(abortedMsg{reason: reason})
}

func (m *TUIModel) SendEligibilityCompleted(program *tea.Program, workDir string) {
	program.Send(eligibilityCompletedMsg{})
}

func (m *TUIModel) SendSidebarUpdate(program *tea.Program, key, value string) {
	program.Send(sidebarUpdateMsg{key: key, value: value})
}

func (m *TUIModel) SendRerunConfirm(program *tea.Program, responseCh chan bool) {
	program.Send(rerunConfirmMsg{responseCh: responseCh})
}

// SendCloudSetupPrompt prompts the user whether to continue with cloud setup
func (m *TUIModel) SendCloudSetupPrompt(program *tea.Program, responseCh chan bool) {
	program.Send(cloudSetupPromptMsg{responseCh: responseCh})
}

// UpdateTodoItems updates the todo list with new phase names (used when adding cloud phases)
func (m *TUIModel) UpdateTodoItems(program *tea.Program, phaseNames []string) {
	m.todoMutex.Lock()
	defer m.todoMutex.Unlock()

	// Find the current active phase
	currentActiveIdx := -1
	for i, item := range m.todoItems {
		if item.active {
			currentActiveIdx = i
			break
		}
	}

	// Create new todo items
	newItems := make([]todoItem, len(phaseNames))
	for i, name := range phaseNames {
		done := false
		active := false
		// Preserve done status for existing items
		if i < len(m.todoItems) {
			done = m.todoItems[i].done
		}
		// Keep current active item active
		if i == currentActiveIdx {
			active = true
		}
		newItems[i] = todoItem{
			text:   name,
			done:   done,
			active: active,
		}
	}
	m.todoItems = newItems
	m.totalPhases = len(phaseNames)
}

func (m *TUIModel) RequestUserInput(program *tea.Program, question string) string {
	responseCh := make(chan string, 1)
	program.Send(userInputRequestMsg{question: question, responseCh: responseCh})

	select {
	case response := <-responseCh:
		return response
	case <-m.ctx.Done():
		return ""
	}
}

// RequestUserSelect asks the user to select from a list of options.
// Returns the ID of the selected option, or empty string if cancelled.
func (m *TUIModel) RequestUserSelect(program *tea.Program, question string, options []SelectOption) string {
	responseCh := make(chan string, 1)
	program.Send(userSelectRequestMsg{question: question, options: options, responseCh: responseCh})

	select {
	case response := <-responseCh:
		return response
	case <-m.ctx.Done():
		return ""
	}
}

// RequestPermission asks the user for permission to execute a tool.
// commandPrefixes is non-empty for run_command/start_background_process to enable command-level granularity.
// Returns "approve", "approve_tool_type", "approve_commands:<csv>", "approve_all", "deny", or "deny:<alternative>".
func (m *TUIModel) RequestPermission(program *tea.Program, toolName, preview string, commandPrefixes []string) string {
	responseCh := make(chan string, 1)
	program.Send(permissionRequestMsg{toolName: toolName, preview: preview, commandPrefixes: commandPrefixes, responseCh: responseCh})

	select {
	case response := <-responseCh:
		return response
	case <-m.ctx.Done():
		return "deny"
	}
}

func (m *TUIModel) RequestPortConflict(program *tea.Program, port int) bool {
	responseCh := make(chan bool, 1)
	program.Send(portConflictMsg{port: port, responseCh: responseCh})

	select {
	case response := <-responseCh:
		return response
	case <-m.ctx.Done():
		return false
	}
}
