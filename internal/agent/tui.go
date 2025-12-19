package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/components"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
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
	"tusk_list":                "List traces",
	"tusk_run":                 "Run tests",
	"transition_phase":         "Complete phase",
}

func getToolDisplayName(name string) string {
	if friendly, ok := toolDisplayNames[name]; ok {
		return friendly
	}
	return name
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

	completedMsg struct{}

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
	userInputMode    bool
	userInputPrompt  string
	userInputBuffer  string
	userInputCh      chan string
	portConflictMode bool
	portConflictPort int
	portConflictCh   chan bool
	rerunConfirmMode bool
	rerunConfirmCh   chan bool

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

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

type logEntry struct {
	timestamp time.Time
	level     string // "phase", "tool-start", "tool-complete", "agent", "error", "success", "dim", "spacing", "plain"
	message   string
	toolName  string // for tool entries
}

type todoItem struct {
	text   string
	done   bool
	active bool
}

// NewTUIModel creates a new TUI model
func NewTUIModel(ctx context.Context, cancel context.CancelFunc) *TUIModel {
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

	return &TUIModel{
		spinner:      s,
		progress:     p,
		maxLogs:      5000,
		logs:         make([]logEntry, 0),
		totalPhases:  7,
		ctx:          ctx,
		cancel:       cancel,
		autoScroll:   true,
		lastTickTime: time.Now(),
		sidebarInfo:  make(map[string]string),
		sidebarOrder: []string{},
		todoItems: []todoItem{
			{text: "Discovery", done: false, active: true},
			{text: "Verify app starts", done: false, active: false},
			{text: "Instrument SDK", done: false, active: false},
			{text: "Create config", done: false, active: false},
			{text: "Simple test", done: false, active: false},
			{text: "Complex test", done: false, active: false},
			{text: "Generate report", done: false, active: false},
		},
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

	case tickMsg:
		m.lastTickTime = time.Time(msg)
		m.pulsePhase = (m.pulsePhase + 1) % 20
		cmds = append(cmds, m.tickAnimation())

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
		// Don't add phase heading to log - it's shown in the header
		m.addLog("spacing", "", "")
		// Update progress bar
		if m.totalPhases > 0 {
			percent := float64(m.phaseNumber-1) / float64(m.totalPhases)
			cmds = append(cmds, m.progress.SetPercent(percent))
		}
		// Update todo items
		m.updateTodoForPhase(msg.phaseNumber)

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

			// Show the input on separate lines if it's meaningful
			if msg.input != "" && msg.input != "{}" {
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
		// Skip output for ask_user - we already showed the user's response when they pressed Enter
		if msg.output != "" && msg.toolName != "ask_user" {
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
		m.userInputBuffer = ""
		m.userInputCh = msg.responseCh
		m.addLog("spacing", "", "")
		m.addLog("agent", "🤖 "+msg.question, "")

	case portConflictMsg:
		m.portConflictMode = true
		m.portConflictPort = msg.port
		m.portConflictCh = msg.responseCh
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
		m.addLog("dim", "   Check .tusk/SETUP_REPORT.md for details.", "")
		m.addLog("spacing", "", "")
		m.addLog("plain", "───────────────────────────────────────────────────────────", "")
		m.addLog("spacing", "", "")
		m.addLog("plain", "You can safely remove these files from your .tusk directory:", "")
		m.addLog("plain", "  • PROGRESS.md - Setup progress tracking", "")
		m.addLog("plain", "  • SETUP_REPORT.md - Setup summary and test results", "")
		m.addLog("spacing", "", "")
		cmds = append(cmds, m.progress.SetPercent(1.0))
		// Mark all todos as done
		for i := range m.todoItems {
			m.todoItems[i].done = true
			m.todoItems[i].active = false
		}

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
			if m.userInputCh != nil {
				m.userInputCh <- m.userInputBuffer
			}
			m.userInputMode = false
			m.addLog("dim", "   > "+m.userInputBuffer, "")
			return m, nil
		case "backspace":
			if len(m.userInputBuffer) > 0 {
				m.userInputBuffer = m.userInputBuffer[:len(m.userInputBuffer)-1]
			}
			return m, nil
		case "ctrl+c":
			if m.userInputCh != nil {
				close(m.userInputCh)
			}
			m.userInputMode = false
			return m, m.initiateShutdown()
		default:
			if len(msg.String()) == 1 {
				m.userInputBuffer += msg.String()
			}
			return m, nil
		}
	}

	if m.portConflictMode {
		switch msg.String() {
		case "y", "Y":
			if m.portConflictCh != nil {
				m.portConflictCh <- true
			}
			m.portConflictMode = false
			m.addLog("dim", "   Killing process on port...", "")
			return m, nil
		case "n", "N", "esc":
			if m.portConflictCh != nil {
				m.portConflictCh <- false
			}
			m.portConflictMode = false
			return m, m.initiateShutdown()
		case "ctrl+c":
			if m.portConflictCh != nil {
				m.portConflictCh <- false
			}
			m.portConflictMode = false
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
	headerHeight := 5    // Title + empty line + status + progress + spacing
	infoPanelHeight := 3 // Info panel with border (1 content line + 2 border lines)
	spacerHeight := 5    // 5 lines of spacing between content and footer
	footerHeight := 1    // Help text line
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
	case "phase":
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(styles.PrimaryColor)).Render(entry.message)
	case "tool-start":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Render("🔧 " + entry.message)
	case "tool-complete":
		return styles.SuccessStyle.Render(entry.message)
	case "error":
		return styles.ErrorStyle.Render(entry.message)
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

	progressWidth := m.width - 2
	progressWidth = max(progressWidth, 20)
	m.progress.Width = progressWidth
	progressBar := m.progress.View()

	return lipgloss.JoinVertical(lipgloss.Left, title, "", statusLine, progressBar, "")
}

func (m *TUIModel) renderFooter() string {
	var helpText string

	switch {
	case m.userInputMode:
		inputStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)
		inputLine := inputStyle.Render("> " + m.userInputBuffer + "▌")
		helpText = components.Footer(m.width, "Enter: submit • Ctrl+C: cancel")
		return inputLine + "\n" + helpText
	case m.portConflictMode:
		prompt := styles.WarningStyle.Render(fmt.Sprintf("Kill process on port %d? (y/n)", m.portConflictPort))
		helpText = components.Footer(m.width, "y: yes • n: no • Ctrl+C: cancel")
		return prompt + "\n" + helpText
	case m.rerunConfirmMode:
		helpText = components.Footer(m.width, "y: rerun setup • q/Esc: exit")
		return helpText
	case m.completed:
		helpText = components.Footer(m.width, "q/Esc: quit")
	case m.shutdownRequested:
		helpText = components.Footer(m.width, "Exiting... (Ctrl+C to force)")
	default:
		// When agent is active, only Ctrl-C can stop
		helpText = components.Footer(m.width, "↑/↓: scroll • g/G: top/bottom • Ctrl+C: stop")
	}

	return helpText
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
func (m *TUIModel) GetFinalOutput() string {
	m.logMutex.RLock()
	defer m.logMutex.RUnlock()

	var lines []string
	lines = append(lines, "")
	lines = append(lines, components.Title(80, "TUSK DRIFT AI SETUP"))
	lines = append(lines, "")

	for _, entry := range m.logs {
		styled := m.styleLogEntry(entry)
		if styled != "" {
			lines = append(lines, styled)
		}
	}

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

func (m *TUIModel) SendCompleted(program *tea.Program) {
	program.Send(completedMsg{})
}

func (m *TUIModel) SendSidebarUpdate(program *tea.Program, key, value string) {
	program.Send(sidebarUpdateMsg{key: key, value: value})
}

func (m *TUIModel) SendRerunConfirm(program *tea.Program, responseCh chan bool) {
	program.Send(rerunConfirmMsg{responseCh: responseCh})
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
