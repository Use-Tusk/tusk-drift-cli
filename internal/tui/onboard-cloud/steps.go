package onboardcloud

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

type IntroStep struct{ BaseStep }

func (IntroStep) ID() onboardStep       { return stepIntro }
func (IntroStep) InputIndex() int       { return -1 }
func (IntroStep) Heading(*Model) string { return "Welcome to Tusk Drift Cloud!" }
func (IntroStep) Description(*Model) string {
	return `This wizard will help you:
  • Connect your GitHub/GitLab repository
  • Register your service for Tusk Drift Cloud
  • Create an API key for CI/CD
  • Configure recording parameters

Facing any issues along the way? Contact us at
` + styles.LinkStyle.Render("support@usetusk.ai") + ` and we'll be right with you.

Press [enter] to continue.`
}
func (IntroStep) Default(*Model) string { return "" }

type ValidateConfigStep struct{ BaseStep }

func (ValidateConfigStep) ID() onboardStep       { return stepValidateConfig }
func (ValidateConfigStep) InputIndex() int       { return -1 }
func (ValidateConfigStep) Heading(*Model) string { return "Checking prerequisites..." }
func (ValidateConfigStep) Description(m *Model) string {
	if !configExists() {
		return `❌ No .tusk/config.yaml found.

Please run 'tusk init' first to create your base configuration.
Then come back and run 'tusk init-cloud' to set up cloud integration.`
	}
	return "✓ Configuration file found"
}
func (ValidateConfigStep) Default(*Model) string { return "" }
func (ValidateConfigStep) Validate(m *Model, input string) error {
	if !configExists() {
		return fmt.Errorf("config file not found - please run 'tusk init' first")
	}
	return nil
}

func (ValidateConfigStep) Apply(m *Model, input string) {
	if err := loadExistingConfig(m); err != nil {
		m.Err = fmt.Errorf("failed to load config: %w", err)
	}
}

func (ValidateConfigStep) ShouldAutoProcess(m *Model) bool {
	return configExists() && m.Err == nil && m.ValidationErr == nil
}

type VerifyGitRepoStep struct{ BaseStep }

func (VerifyGitRepoStep) ID() onboardStep       { return stepVerifyGitRepo }
func (VerifyGitRepoStep) InputIndex() int       { return -1 }
func (VerifyGitRepoStep) Heading(*Model) string { return "Detecting Git repository..." }
func (VerifyGitRepoStep) Description(m *Model) string {
	if m.GitRepoOwner != "" && m.GitRepoName != "" {
		var repoType string
		switch m.CodeHostingResourceType {
		case CodeHostingResourceTypeGitHub:
			repoType = "GitHub repository"
		case CodeHostingResourceTypeGitLab:
			repoType = "GitLab repository"
		default:
			repoType = "repository"
		}
		return fmt.Sprintf("✓ Found %s: %s/%s", repoType, m.GitRepoOwner, m.GitRepoName)
	}
	if len(m.AvailableRemotes) > 1 && m.Err == nil {
		return fmt.Sprintf("Found %d git remotes. Please select which one to use.", len(m.AvailableRemotes))
	}
	if m.Err != nil {
		return fmt.Sprintf("Error detecting repository: %s", m.Err.Error())
	}
	return "Checking current directory..."
}
func (VerifyGitRepoStep) Default(*Model) string { return "" }
func (VerifyGitRepoStep) Help(m *Model) string {
	if m.GitRepoOwner != "" && m.GitRepoName != "" {
		return "Validating..."
	}
	if m.Err != nil {
		return "esc: quit"
	}
	return "Detecting..."
}

func (VerifyGitRepoStep) ShouldAutoProcess(m *Model) bool {
	// Don't auto-process if we need remote selection
	if len(m.AvailableRemotes) > 1 && m.GitRepoOwner == "" {
		return true
	}
	return m.Err == nil
}

func (VerifyGitRepoStep) Execute(m *Model) tea.Cmd {
	if m.GitRepoOwner == "" {
		if err := detectGitRepo(m); err != nil {
			m.Err = err
			return nil
		}

		return func() tea.Msg { return stepCompleteMsg{} }
	}
	return nil
}

type SelectRemoteStep struct{ BaseStep }

func (SelectRemoteStep) ID() onboardStep       { return stepSelectRemote }
func (SelectRemoteStep) InputIndex() int       { return 0 }
func (SelectRemoteStep) Heading(*Model) string { return "Select Git Remote" }

func (SelectRemoteStep) Description(m *Model) string {
	if len(m.AvailableRemotes) == 0 {
		return "No remotes found."
	}

	desc := "Multiple git remotes found. Please select which one to use:\n\n"
	names := getSortedRemoteNames(m.AvailableRemotes)
	for i, name := range names {
		url := m.AvailableRemotes[name]
		desc += fmt.Sprintf("  %d. %s → %s\n", i+1, name, url)
	}
	desc += "\nEnter the number of the remote to use:"
	return desc
}

func (SelectRemoteStep) Default(*Model) string { return "1" }

func (SelectRemoteStep) Help(*Model) string {
	return "enter: select • esc: quit"
}

func (SelectRemoteStep) ShouldSkip(m *Model) bool {
	// Skip if:
	// 1. We already have repo info (origin was found or only one remote)
	// 2. There are no remotes (error case handled by VerifyGitRepoStep)
	return m.GitRepoOwner != "" || len(m.AvailableRemotes) <= 1
}

func (SelectRemoteStep) SkipReason(m *Model) string {
	if m.GitRepoOwner != "" {
		return fmt.Sprintf("Using remote '%s'", m.SelectedRemoteName)
	}
	return ""
}

func (SelectRemoteStep) Validate(m *Model, input string) error {
	num, err := strconv.Atoi(input)
	if err != nil {
		return fmt.Errorf("please enter a valid number")
	}

	if num < 1 || num > len(m.AvailableRemotes) {
		return fmt.Errorf("please enter a number between 1 and %d", len(m.AvailableRemotes))
	}

	return nil
}

func (SelectRemoteStep) Apply(m *Model, input string) {
	num, _ := strconv.Atoi(input)
	names := getSortedRemoteNames(m.AvailableRemotes)
	selectedName := names[num-1]
	m.SelectedRemoteName = selectedName
}

func (SelectRemoteStep) Execute(m *Model) tea.Cmd {
	if m.SelectedRemoteName != "" && m.GitRepoOwner == "" {
		url := m.AvailableRemotes[m.SelectedRemoteName]
		if err := detectRepoFromURL(m, url); err != nil {
			m.Err = err
			return nil
		}
		return func() tea.Msg { return stepCompleteMsg{} }
	}
	return nil
}

func (SelectRemoteStep) Clear(m *Model) {
	m.SelectedRemoteName = ""
	m.GitRepoOwner = ""
	m.GitRepoName = ""
}

type SelectClientStep struct{ BaseStep }

func (SelectClientStep) ID() onboardStep       { return stepSelectClient }
func (SelectClientStep) InputIndex() int       { return 0 }
func (SelectClientStep) Heading(*Model) string { return "Select organization" }
func (SelectClientStep) Description(m *Model) string {
	if len(m.AvailableClients) == 0 {
		return "No organizations found"
	}
	if len(m.AvailableClients) == 1 {
		return fmt.Sprintf("Using: %s", m.AvailableClients[0].Name)
	}
	desc := "You're a member of the following organizations:\n"
	for i, c := range m.AvailableClients {
		desc += fmt.Sprintf("  %d. %s\n", i+1, c.Name)
	}
	return desc + "\nChoose an organization to set up Tusk Drift Cloud for (enter number):"
}

func (SelectClientStep) Default(m *Model) string {
	if m.DefaultClientIndex > 0 {
		return fmt.Sprintf("%d", m.DefaultClientIndex)
	}
	return "1"
}
func (SelectClientStep) ShouldSkip(m *Model) bool { return len(m.AvailableClients) == 1 }
func (SelectClientStep) Help(*Model) string {
	return "enter: use default • esc: quit"
}

func (SelectClientStep) Validate(m *Model, input string) error {
	num, err := strconv.Atoi(input)
	if err != nil {
		return fmt.Errorf("please enter a valid number")
	}

	if num < 1 || num > len(m.AvailableClients) {
		return fmt.Errorf("please enter a number between 1 and %d", len(m.AvailableClients))
	}

	return nil
}

func (SelectClientStep) Apply(m *Model, input string) {
	num, _ := strconv.Atoi(input)
	m.SelectedClient = &m.AvailableClients[num-1]

	// Save selected client to CLI config
	saveSelectedClientToCLIConfig(m.SelectedClient.ID, m.SelectedClient.Name)
}

type VerifyRepoAccessStep struct{ BaseStep }

func (VerifyRepoAccessStep) ID() onboardStep       { return stepVerifyRepoAccess }
func (VerifyRepoAccessStep) InputIndex() int       { return -1 }
func (VerifyRepoAccessStep) Heading(*Model) string { return "Verifying repository access..." }

func (VerifyRepoAccessStep) Description(m *Model) string {
	if m.RepoAccessVerified {
		return fmt.Sprintf("✓ Access verified for %s/%s (Repo ID: %d)",
			m.GitRepoOwner, m.GitRepoName, m.RepoID)
	}

	if m.Err != nil {
		// Check if it's a "no GitHub connection" error - show GitHub auth instructions
		if m.NeedsCodeHostingAuth {
			return m.buildCodeHostingAuthMessage()
		}

		instructions := fmt.Sprintf("Repository: %s/%s\n\nPlease check:\n  1. The Tusk GitHub app has access to this repository",
			m.GitRepoOwner, m.GitRepoName)

		if len(m.AvailableClients) > 1 {
			instructions += "\n  2. If you belong to multiple organizations, ensure this repository belongs to the correct organization"
		}

		return instructions
	}

	return fmt.Sprintf("Checking if Tusk can access %s/%s...",
		m.GitRepoOwner, m.GitRepoName)
}

func (VerifyRepoAccessStep) Default(*Model) string { return "" }

func (VerifyRepoAccessStep) Help(m *Model) string {
	if m.RepoAccessVerified {
		return "Continuing..."
	}
	if m.Err != nil {
		if m.NeedsCodeHostingAuth {
			return "enter: open browser and retry • esc: quit"
		}
		return "enter: retry • ctrl+b/←: back • esc: quit"
	}
	return "Verifying..."
}

func (VerifyRepoAccessStep) ShouldAutoProcess(m *Model) bool {
	// Auto-process in two cases:
	// 1. Success - move to next step
	// 2. Haven't tried yet - start verification
	return m.RepoAccessVerified || (m.Err == nil && !m.RepoAccessVerified)
}

func (VerifyRepoAccessStep) Execute(m *Model) tea.Cmd {
	if !m.RepoAccessVerified {
		// If needs code hosting auth and user pressed Enter, open browser first
		if m.NeedsCodeHostingAuth && m.Err != nil {
			openCodeHostingAuthBrowser(m)
			// Clear the flag so next Enter press will verify instead of re-opening
			m.NeedsCodeHostingAuth = false
			return nil
		}

		return verifyRepoAccess(m)
	}
	return nil
}

func (VerifyRepoAccessStep) Clear(m *Model) {
	m.RepoAccessVerified = false
	m.RepoID = 0
	m.NeedsCodeHostingAuth = false
}

type CreateObservableServiceStep struct{ BaseStep }

func (CreateObservableServiceStep) ID() onboardStep       { return stepCreateObservableService }
func (CreateObservableServiceStep) InputIndex() int       { return -1 }
func (CreateObservableServiceStep) Heading(*Model) string { return "Creating observable service..." }

func (CreateObservableServiceStep) Description(m *Model) string {
	if m.ServiceCreated && m.ServiceID != "" {
		return fmt.Sprintf("✓ Service created successfully\n  Service ID: %s", m.ServiceID)
	}
	if m.ServiceID != "" && !m.ServiceCreated {
		return fmt.Sprintf("✓ Service already configured (ID: %s)", m.ServiceID)
	}
	if m.Err != nil {
		return fmt.Sprintf("Repository: %s/%s\n\nFailed to create service",
			m.GitRepoOwner, m.GitRepoName)
	}
	return fmt.Sprintf("Creating observable service for %s/%s...",
		m.GitRepoOwner, m.GitRepoName)
}

func (CreateObservableServiceStep) Default(*Model) string { return "" }

func (CreateObservableServiceStep) Help(m *Model) string {
	if m.ServiceID != "" {
		return "Continuing..."
	}
	if m.Err != nil {
		return "enter: retry • ctrl+b/←: back • esc: quit"
	}
	return "Creating service..."
}

func (CreateObservableServiceStep) ShouldSkip(m *Model) bool {
	return m.ServiceID != ""
}

func (CreateObservableServiceStep) SkipReason(m *Model) string {
	if m.ServiceID != "" {
		return fmt.Sprintf("Service already configured with ID: %s\n\nThis was loaded from your existing .tusk/config.yaml file.", m.ServiceID)
	}
	return ""
}

func (CreateObservableServiceStep) ShouldAutoProcess(m *Model) bool {
	return m.ServiceID != "" || m.Err == nil
}

func (CreateObservableServiceStep) Execute(m *Model) tea.Cmd {
	if m.ServiceID != "" {
		return func() tea.Msg { return stepCompleteMsg{} }
	}

	return createObservableService(m)
}

func (CreateObservableServiceStep) Clear(m *Model) {
	m.ServiceCreated = false
	// Don't clear ServiceID - it's persistent from config
}

type CreateApiKeyStep struct{ BaseStep }

func (CreateApiKeyStep) ID() onboardStep       { return stepCreateApiKey }
func (CreateApiKeyStep) InputIndex() int       { return 1 }
func (CreateApiKeyStep) Heading(*Model) string { return "Create API key" }

func (CreateApiKeyStep) Description(m *Model) string {
	if m.HasApiKey {
		return "✓ API key already configured (TUSK_API_KEY environment variable is set)"
	}
	if m.ApiKey != "" {
		shellConfig := detectShellConfig()
		return fmt.Sprintf(`✓ API key created successfully!

⚠️  SAVE THIS API KEY - It won't be shown again!

  API Key: %s

Next steps:

1. For local development, add to your shell config:
   echo 'export TUSK_API_KEY=%s' >> ~/%s
   source ~/%s

2. For current terminal session only:
   export TUSK_API_KEY=%s

3. For CI/CD, add as a secret environment variable:
   TUSK_API_KEY=%s

Tip: Hold Option/Alt while clicking and dragging to select text in most terminals.

Press [enter] to continue...`,
			m.ApiKey,
			m.ApiKey,
			filepath.Base(shellConfig),
			filepath.Base(shellConfig),
			m.ApiKey,
			m.ApiKey)
	}

	if m.Err != nil {
		return "Failed to create API key.\n\nAPI keys are needed for CI/CD workflows to authenticate with Tusk Cloud."
	}
	return fmt.Sprintf(`API keys are needed for CI/CD workflows to authenticate with Tusk Cloud.

Alternatively, you can also create an API key here: %s

Enter a name for the new API key or press [enter] to skip:`,
		styles.LinkStyle.Render("https://app.usetusk.ai/app/settings/api-keys"))
}

func (CreateApiKeyStep) Default(*Model) string { return "" }

func (CreateApiKeyStep) Help(m *Model) string {
	if m.HasApiKey {
		return "Continuing..."
	}
	if m.ApiKey != "" {
		return "enter: continue"
	}
	if m.Err != nil {
		return "enter: retry • ctrl+b/←: back • esc: quit"
	}
	return "enter: skip • ctrl+b/←: back • esc: quit"
}

func (CreateApiKeyStep) ShouldSkip(m *Model) bool {
	return m.HasApiKey
}

func (CreateApiKeyStep) SkipReason(m *Model) string {
	if m.HasApiKey {
		return "API key already configured via TUSK_API_KEY environment variable.\n\nNo need to create a new one."
	}
	return ""
}

func (CreateApiKeyStep) ShouldAutoProcess(m *Model) bool {
	// Auto-process if we just created the key (so user can copy it)
	// or if skipping entirely
	return m.HasApiKey || (m.ApiKey != "" && m.Err == nil)
}

func (CreateApiKeyStep) Validate(m *Model, input string) error {
	if strings.TrimSpace(input) == "" {
		// Empty input means skip - that's ok
		return nil
	}

	if len(input) > 100 {
		return fmt.Errorf("API key name is too long (max 100 characters)")
	}
	return nil
}

func (CreateApiKeyStep) Apply(m *Model, input string) {
	m.ApiKeyName = strings.TrimSpace(input)
}

func (CreateApiKeyStep) Execute(m *Model) tea.Cmd {
	if m.ApiKeyName != "" && m.ApiKey == "" {
		return createApiKey(m)
	}

	return nil
}

func (CreateApiKeyStep) Clear(m *Model) {
	m.ApiKey = ""
	m.ApiKeyID = ""
	m.ApiKeyName = ""
	m.CreateApiKeyChoice = false
}

type RecordingConfigStep struct{ BaseStep }

func (RecordingConfigStep) ID() onboardStep       { return stepRecordingConfig }
func (RecordingConfigStep) InputIndex() int       { return -1 }
func (RecordingConfigStep) Heading(*Model) string { return "Configure recording parameters" }

func (RecordingConfigStep) Description(m *Model) string {
	return `Configure how Tusk records execution traces from your application:

• Sampling Rate: Percentage of requests to record (0.01 = 1%, 0.1 = 10%)
  Lower rates reduce performance overhead. We recommend starting 10% for
  dev/staging, and 1% for production environments.

• Export Spans: Upload trace data to Tusk Drift Cloud (required for cloud features)
  Disable only if using Tusk Drift locally without cloud integration.

• Record Environment Variables: Record and replay environment variables for accurate
  replay behavior. Recommended if your application's business logic depends on 
  environment variables, as this ensures the most accurate replay behavior.`
}

func (RecordingConfigStep) Default(m *Model) string { return "" }

func (RecordingConfigStep) Help(m *Model) string {
	if m.RecordingConfigTable != nil && m.RecordingConfigTable.EditMode {
		return "Type sampling rate (0.0-1.0) • tab/esc: done editing"
	}
	return "↑↓: navigate • tab/space: toggle/edit • enter: save"
}

func (RecordingConfigStep) Clear(m *Model) {
	m.RecordingConfigTable = nil
}

type ReviewStep struct{ BaseStep }

func (ReviewStep) ID() onboardStep       { return stepReview }
func (ReviewStep) InputIndex() int       { return -1 }
func (ReviewStep) Heading(*Model) string { return "Review" }
func (ReviewStep) Description(m *Model) string {
	var summary strings.Builder

	summary.WriteString("Here's what was configured:\n\n")

	summary.WriteString(fmt.Sprintf("📦 Repository: %s/%s\n\n", m.GitRepoOwner, m.GitRepoName))

	summary.WriteString(fmt.Sprintf("💻 Service ID: %s\n\n", m.ServiceID))

	summary.WriteString("🔑 API Key\n")
	switch {
	case m.HasApiKey:
		summary.WriteString("  ✓ Already configured (TUSK_API_KEY environment variable)\n\n")
	case m.ApiKey != "":
		summary.WriteString(fmt.Sprintf("  ✓ Created new API key (%s)\n", m.ApiKeyName))
		summary.WriteString("     Remember to set TUSK_API_KEY in your environment\n\n")
	default:
		summary.WriteString(fmt.Sprintf("  ⊘ Skipped (you can create one later at %s)\n\n",
			styles.LinkStyle.Render("https://app.usetusk.ai/app/settings/api-keys")))
	}

	summary.WriteString("⚙️  Recording Configuration\n")
	samplingRate, _ := strconv.ParseFloat(m.SamplingRate, 64)
	summary.WriteString(fmt.Sprintf("  • Sampling rate: %.2f (%.0f%% of requests)\n", samplingRate, samplingRate*100))
	summary.WriteString(fmt.Sprintf("  • Export spans: %t\n", m.ExportSpans))
	summary.WriteString(fmt.Sprintf("  • Record environment variables: %t\n\n", m.EnableEnvVarRecording))

	summary.WriteString("All settings have been saved to .tusk/config.yaml.\n")
	summary.WriteString("\nPress [enter] to continue...")

	return summary.String()
}

func (ReviewStep) Default(*Model) string         { return "" }
func (ReviewStep) Help(*Model) string            { return "enter: continue" }
func (ReviewStep) ShouldSkip(*Model) bool        { return false }
func (ReviewStep) ShouldAutoProcess(*Model) bool { return false }
func (ReviewStep) Validate(*Model, string) error { return nil }
func (ReviewStep) Apply(*Model, string)          {}
func (ReviewStep) Execute(*Model) tea.Cmd        { return nil }
func (ReviewStep) Clear(*Model)                  {}

type DoneStep struct{ BaseStep }

func (DoneStep) ID() onboardStep       { return stepDone }
func (DoneStep) InputIndex() int       { return -1 }
func (DoneStep) Heading(*Model) string { return "✓ Setup complete!" }
func (DoneStep) Description(*Model) string {
	return fmt.Sprintf(`Next steps:

  1. Push these changes for your Tusk Drift configuration and deploy the service to your
     desired environment to start automatically recording traces from live traffic

  2. Ensure you have trace recordings with Tusk Drift Cloud
     - Run 'tusk list --cloud' to see available tests
     - Run 'tusk run --cloud' to execute tests locally

  3. Create a GitHub/GitLab workflow to run trace recordings against code changes in your
     CI/CD pipeline

  4. Create a PR to test the CI workflow

For more information, see: %s`,
		styles.LinkStyle.Render("https://docs.usetusk.ai/api-tests/tusk-drift-cloud"))
}
func (DoneStep) Default(*Model) string { return "" }

func createFlow() *Flow {
	return NewFlow([]Step{
		IntroStep{},
		ValidateConfigStep{},
		VerifyGitRepoStep{},
		SelectRemoteStep{},
		SelectClientStep{},
		VerifyRepoAccessStep{},
		CreateObservableServiceStep{},
		CreateApiKeyStep{},
		RecordingConfigStep{},
		// CIWorkflowStep{},
		ReviewStep{},
		DoneStep{},
	})
}
