package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/agent/tools"
)

// Phase represents a distinct phase in the agent's workflow
type Phase struct {
	ID            string
	Name          string
	Description   string
	Instructions  string
	Tools         []PhaseTool // Which tools are available in this phase
	Required      bool        // Must complete, or can skip?
	MaxIterations int         // Max iterations for this phase (0 = use default)
	// OnEnter is called when entering this phase, returns additional context to append to instructions
	OnEnter func(state *State) string
}

// PhaseManager manages the agent's progress through phases
type PhaseManager struct {
	phases           []*Phase
	currentIdx       int
	state            *State
	transitioned     bool
	previousProgress string // Progress from a previous run (if any)
}

// NewPhaseManager creates a new PhaseManager with default phases
func NewPhaseManager() *PhaseManager {
	return &PhaseManager{
		phases:     defaultPhases(),
		currentIdx: 0,
		state:      &State{},
	}
}

// CurrentPhase returns the current phase
func (pm *PhaseManager) CurrentPhase() *Phase {
	if pm.currentIdx >= len(pm.phases) {
		return nil
	}
	return pm.phases[pm.currentIdx]
}

// AdvancePhase moves to the next phase
func (pm *PhaseManager) AdvancePhase() (*Phase, error) {
	pm.currentIdx++
	pm.transitioned = true
	if pm.currentIdx >= len(pm.phases) {
		return nil, nil
	}
	return pm.phases[pm.currentIdx], nil
}

// IsComplete returns true if all phases are done
func (pm *PhaseManager) IsComplete() bool {
	return pm.currentIdx >= len(pm.phases)
}

// HasTransitioned returns true if a transition occurred this iteration
func (pm *PhaseManager) HasTransitioned() bool {
	return pm.transitioned
}

// ResetTransitionFlag resets the transition flag
func (pm *PhaseManager) ResetTransitionFlag() {
	pm.transitioned = false
}

// UpdateState updates the state with results from a phase
func (pm *PhaseManager) UpdateState(results map[string]interface{}) {
	// Update state fields based on results
	if v, ok := results["project_type"].(string); ok {
		pm.state.ProjectType = v
	}
	if v, ok := results["package_manager"].(string); ok {
		pm.state.PackageManager = v
	}
	if v, ok := results["module_system"].(string); ok {
		pm.state.ModuleSystem = v
	}
	if v, ok := results["framework"].(string); ok {
		pm.state.Framework = v
	}
	if v, ok := results["entry_point"].(string); ok {
		pm.state.EntryPoint = v
	}
	if v, ok := results["start_command"].(string); ok {
		pm.state.StartCommand = v
	}
	if v, ok := results["port"].(string); ok {
		pm.state.Port = v
	}
	if v, ok := results["health_endpoint"].(string); ok {
		pm.state.HealthEndpoint = v
	}
	if v, ok := results["docker_type"].(string); ok {
		pm.state.DockerType = v
	}
	if v, ok := results["service_name"].(string); ok {
		pm.state.ServiceName = v
	}
	if v, ok := results["has_external_calls"].(bool); ok {
		pm.state.HasExternalCalls = v
	}
	// Handle compatibility_warnings as []interface{} from JSON
	if v, ok := results["compatibility_warnings"].([]interface{}); ok {
		warnings := make([]string, 0, len(v))
		for _, w := range v {
			if s, ok := w.(string); ok {
				warnings = append(warnings, s)
			}
		}
		pm.state.CompatibilityWarnings = warnings
	}
	if v, ok := results["app_starts_without_sdk"].(bool); ok {
		pm.state.AppStartsWithoutSDK = v
	}
	if v, ok := results["sdk_installed"].(bool); ok {
		pm.state.SDKInstalled = v
	}
	if v, ok := results["sdk_instrumented"].(bool); ok {
		pm.state.SDKInstrumented = v
	}
	if v, ok := results["config_created"].(bool); ok {
		pm.state.ConfigCreated = v
	}
	if v, ok := results["simple_test_passed"].(bool); ok {
		pm.state.SimpleTestPassed = v
	}
	if v, ok := results["complex_test_passed"].(bool); ok {
		pm.state.ComplexTestPassed = v
	}

	// Cloud setup state
	if v, ok := results["is_authenticated"].(bool); ok {
		pm.state.IsAuthenticated = v
	}
	if v, ok := results["user_email"].(string); ok {
		pm.state.UserEmail = v
	}
	if v, ok := results["user_id"].(string); ok {
		pm.state.UserId = v
	}
	if v, ok := results["selected_client_id"].(string); ok {
		pm.state.SelectedClientID = v
	}
	if v, ok := results["selected_client_name"].(string); ok {
		pm.state.SelectedClientName = v
	}
	if v, ok := results["git_repo_owner"].(string); ok {
		pm.state.GitRepoOwner = v
	}
	if v, ok := results["git_repo_name"].(string); ok {
		pm.state.GitRepoName = v
	}
	if v, ok := results["code_hosting_type"].(string); ok {
		pm.state.CodeHostingType = v
	}
	if v, ok := results["cloud_service_id"].(string); ok {
		pm.state.CloudServiceID = v
	}
	if v, ok := results["api_key_created"].(bool); ok {
		pm.state.ApiKeyCreated = v
	}
	if v, ok := results["sampling_rate"].(float64); ok {
		pm.state.SamplingRate = v
	}
	if v, ok := results["export_spans"].(bool); ok {
		pm.state.ExportSpans = v
	}
	if v, ok := results["enable_env_var_recording"].(bool); ok {
		pm.state.EnableEnvVarRecording = v
	}

	// Trace upload state
	if v, ok := results["traces_uploaded"].(float64); ok {
		pm.state.TracesUploaded = int(v)
	}
	if v, ok := results["trace_upload_success"].(bool); ok {
		pm.state.TraceUploadSuccess = v
	}
	if v, ok := results["trace_upload_attempted"].(bool); ok {
		pm.state.TraceUploadAttempted = v
	}

	// Suite validation state
	if v, ok := results["suite_validation_success"].(bool); ok {
		pm.state.SuiteValidationSuccess = v
	}
	if v, ok := results["tests_in_suite"].(float64); ok {
		pm.state.TestsInSuite = int(v)
	}
	if v, ok := results["suite_validation_attempted"].(bool); ok {
		pm.state.SuiteValidationAttempted = v
	}

	// Verify mode state
	if v, ok := results["original_sampling_rate"].(float64); ok {
		pm.state.OriginalSamplingRate = v
	}
	if v, ok := results["original_export_spans"].(bool); ok {
		pm.state.OriginalExportSpans = v
	}
	if v, ok := results["original_enable_env_var_recording"].(bool); ok {
		pm.state.OriginalEnableEnvVarRecording = v
	}
	if v, ok := results["verify_simple_passed"].(bool); ok {
		pm.state.VerifySimplePassed = v
	}
	if v, ok := results["verify_complex_passed"].(bool); ok {
		pm.state.VerifyComplexPassed = v
	}
}

// StateAsContext returns the current state as a string for the prompt
func (pm *PhaseManager) StateAsContext() string {
	data, _ := json.MarshalIndent(pm.state, "", "  ")
	result := string(data)

	// Include previous progress if available
	if pm.previousProgress != "" {
		result += "\n\n## Previous Progress (from interrupted run)\n\n" + pm.previousProgress
	}

	return result
}

// SetPreviousProgress sets the progress from a previous interrupted run
func (pm *PhaseManager) SetPreviousProgress(progress string) {
	pm.previousProgress = progress
}

// SkipToPhase skips to a specific phase by name, returning true if found
func (pm *PhaseManager) SkipToPhase(phaseName string) bool {
	for i, phase := range pm.phases {
		if phase.Name == phaseName {
			pm.currentIdx = i
			return true
		}
	}
	return false
}

// GetPhaseNames returns all phase names in order
func (pm *PhaseManager) GetPhaseNames() []string {
	names := make([]string, len(pm.phases))
	for i, phase := range pm.phases {
		names[i] = phase.Name
	}
	return names
}

// RestoreDiscoveredInfo restores discovered information from parsed progress file
func (pm *PhaseManager) RestoreDiscoveredInfo(info map[string]string) {
	if v, ok := info["Service Name"]; ok {
		pm.state.ServiceName = v
	}
	if v, ok := info["Project Type"]; ok {
		pm.state.ProjectType = v
	}
	if v, ok := info["Package Manager"]; ok {
		pm.state.PackageManager = v
	}
	if v, ok := info["Module System"]; ok {
		pm.state.ModuleSystem = v
	}
	if v, ok := info["Framework"]; ok {
		pm.state.Framework = v
	}
	if v, ok := info["Entry Point"]; ok {
		pm.state.EntryPoint = v
	}
	if v, ok := info["Start Command"]; ok {
		pm.state.StartCommand = v
	}
	if v, ok := info["Port"]; ok {
		pm.state.Port = v
	}
	if v, ok := info["Health Endpoint"]; ok {
		pm.state.HealthEndpoint = v
	}
	if v, ok := info["Docker"]; ok {
		pm.state.DockerType = v
	}
}

// RestoreSetupProgress restores setup progress flags from parsed progress file
func (pm *PhaseManager) RestoreSetupProgress(progress map[string]bool) {
	if progress["app_starts_without_sdk"] {
		pm.state.AppStartsWithoutSDK = true
	}
	if progress["sdk_installed"] {
		pm.state.SDKInstalled = true
	}
	if progress["sdk_instrumented"] {
		pm.state.SDKInstrumented = true
	}
	if progress["config_created"] {
		pm.state.ConfigCreated = true
	}
	if progress["simple_test_passed"] {
		pm.state.SimpleTestPassed = true
	}
	if progress["complex_test_passed"] {
		pm.state.ComplexTestPassed = true
	}
}

// GetState returns the current state
func (pm *PhaseManager) GetState() *State {
	return pm.state
}

// HasCloudPhases returns true if cloud phases have been added
func (pm *PhaseManager) HasCloudPhases() bool {
	for _, phase := range pm.phases {
		if phase.ID == "cloud_auth" {
			return true
		}
	}
	return false
}

// AddCloudPhases adds the cloud setup phases after local setup is complete
func (pm *PhaseManager) AddCloudPhases() {
	cloudPhases := []*Phase{
		cloudAuthPhase(),
		cloudDetectRepoPhase(),
		cloudVerifyAccessPhase(),
		cloudCreateServicePhase(),
		cloudCreateApiKeyPhase(),
		cloudConfigureRecordingPhase(),
		cloudUploadTracesPhase(),
		cloudValidateSuitePhase(),
		cloudSummaryPhase(),
	}
	pm.phases = append(pm.phases, cloudPhases...)
}

// SetCloudOnlyMode replaces local phases with cloud-only phases (for --skip-to-cloud testing)
func (pm *PhaseManager) SetCloudOnlyMode() {
	pm.phases = []*Phase{
		cloudAuthPhase(),
		cloudDetectRepoPhase(),
		cloudVerifyAccessPhase(),
		cloudCreateServicePhase(),
		cloudCreateApiKeyPhase(),
		cloudConfigureRecordingPhase(),
		cloudUploadTracesPhase(),
		cloudValidateSuitePhase(),
		cloudSummaryPhase(),
	}
	pm.currentIdx = 0
}

// SetEligibilityOnlyMode replaces all phases with the eligibility check phase
func (pm *PhaseManager) SetEligibilityOnlyMode() {
	pm.phases = []*Phase{
		eligibilityCheckPhase(),
	}
	pm.currentIdx = 0
}

// SetVerifyMode replaces all phases with verify-only phases
func (pm *PhaseManager) SetVerifyMode() {
	pm.phases = []*Phase{
		verifySetupPhase(),
		verifySimpleTestPhase(),
		verifyComplexTestPhase(),
		verifyRestorePhase(),
	}
	pm.currentIdx = 0
}

// PhaseTransitionTool creates the transition_phase tool executor
func (pm *PhaseManager) PhaseTransitionTool() ToolExecutor {
	return func(input json.RawMessage) (string, error) {
		var params struct {
			Results map[string]interface{} `json:"results"`
			Notes   string                 `json:"notes"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return "", fmt.Errorf("invalid input: %w", err)
		}

		// Special validation for eligibility_check phase
		currentPhase := pm.CurrentPhase()
		if currentPhase != nil && currentPhase.ID == "eligibility_check" {
			if reportData, ok := params.Results["eligibility_report"]; ok {
				// Convert to JSON string for validation
				reportJSON, err := json.Marshal(reportData)
				if err != nil {
					return "", fmt.Errorf("failed to serialize eligibility_report: %w", err)
				}

				// Parse and validate the report
				_, err = ParseEligibilityReport(string(reportJSON))
				if err != nil {
					return "", fmt.Errorf("eligibility report validation failed: %w. Please fix the report structure and try again", err)
				}

				// Store validated report in state for later saving
				pm.state.EligibilityReport = string(reportJSON)
			} else {
				return "", fmt.Errorf("eligibility_report is required in results for eligibility_check phase. Please provide the complete eligibility report with services and summary fields")
			}
		}

		// Update state with results
		if params.Results != nil {
			pm.UpdateState(params.Results)
		}

		// Move to next phase
		next, err := pm.AdvancePhase()
		if err != nil {
			return "", err
		}

		if next == nil {
			return "All phases complete! Generate the final summary report.", nil
		}

		// Build instructions, including any dynamic content from OnEnter
		instructions := next.Instructions
		if next.OnEnter != nil {
			if extra := next.OnEnter(pm.state); extra != "" {
				instructions = instructions + "\n\n" + extra
			}
		}

		return fmt.Sprintf("✅ Transitioned to phase: %s\n\n%s\n\nCurrent state:\n%s",
			next.Name, instructions, pm.StateAsContext()), nil
	}
}

func defaultPhases() []*Phase {
	return []*Phase{
		detectLanguagePhase(),
		checkCompatibilityPhase(),
		gatherInfoPhase(),
		confirmAppStartsPhase(),
		instrumentSDKPhase(),
		createConfigPhase(),
		simpleTestPhase(),
		complexTestPhase(),
		summaryPhase(),
	}
}

func detectLanguagePhase() *Phase {
	return &Phase{
		ID:           "detect_language",
		Name:         "Detect Language",
		Description:  "Identify the project's language/runtime",
		Instructions: PhaseDetectLanguagePrompt,
		Tools: Tools(
			ToolListDirectory,
			ToolReadFile,
			ToolAskUser,
			ToolTransitionPhase,
			ToolAbortSetup,
		),
		Required:      true,
		MaxIterations: 10,
	}
}

func checkCompatibilityPhase() *Phase {
	return &Phase{
		ID:          "check_compatibility",
		Name:        "Check Compatibility",
		Description: "Verify project dependencies are compatible with the SDK",
		// Instructions will be set dynamically in OnEnter based on project type
		Instructions: PhaseCheckCompatibilityNodejsPrompt, // Default to Node.js, overridden in OnEnter
		Tools: Tools(
			ToolReadFile,
			ToolGrep,
			ToolAskUser,
			ToolTransitionPhase,
			ToolAbortSetup,
		),
		Required:      true,
		MaxIterations: 15,
		OnEnter: func(state *State) string {
			// Fetch SDK manifest based on detected language
			manifestURL := tools.GetManifestURLForProjectType(state.ProjectType)
			if manifestURL == "" {
				return fmt.Sprintf("⚠️ No SDK manifest available for project type '%s'. Proceed with manual compatibility check.", state.ProjectType)
			}

			manifest, err := tools.FetchManifestFromURL(manifestURL)
			if err != nil {
				return fmt.Sprintf("❌ Failed to fetch SDK manifest: %s\n\nProceed with manual compatibility check.", err)
			}

			// Provide language-specific instructions
			instructions := GetCheckCompatibilityPrompt(state.ProjectType)
			return fmt.Sprintf("### Language-Specific Instructions\n\n%s\n\n### SDK Manifest (fetched automatically)\n\n```json\n%s\n```", instructions, manifest)
		},
	}
}

func gatherInfoPhase() *Phase {
	return &Phase{
		ID:          "gather_info",
		Name:        "Gather Project Info",
		Description: "Collect project details for SDK setup",
		// Instructions will be set dynamically in OnEnter based on project type
		Instructions: PhaseGatherInfoNodejsPrompt, // Default to Node.js, overridden in OnEnter
		Tools: Tools(
			ToolReadFile,
			ToolListDirectory,
			ToolGrep,
			ToolAskUser,
			ToolTransitionPhase,
		),
		Required:      true,
		MaxIterations: 50,
		OnEnter: func(state *State) string {
			// Return language-specific gather info instructions
			instructions := GetGatherInfoPrompt(state.ProjectType)
			if instructions != PhaseGatherInfoNodejsPrompt {
				// Override with language-specific instructions
				return fmt.Sprintf("### Language-Specific Instructions\n\n%s", instructions)
			}
			return ""
		},
	}
}

func confirmAppStartsPhase() *Phase {
	return &Phase{
		ID:           "confirm_app_starts",
		Name:         "Confirm App Starts",
		Description:  "Verify the service starts without Tusk Drift",
		Instructions: PhaseConfirmAppStartsPrompt,
		Tools: Tools(
			ToolRunCommand,
			ToolStartBackgroundProcess,
			ToolStopBackgroundProcess,
			ToolWaitForReady,
			ToolGetProcessLogs,
			ToolHTTPRequest,
			ToolAskUser,
			ToolReadFile,
			ToolTransitionPhase,
		),
		Required: true,
	}
}

func instrumentSDKPhase() *Phase {
	return &Phase{
		ID:          "instrument_sdk",
		Name:        "Instrument SDK",
		Description: "Install and configure the Tusk Drift SDK",
		// Instructions will be set dynamically in OnEnter based on project type
		Instructions: PhaseInstrumentSDKNodejsPrompt, // Default to Node.js, overridden in OnEnter
		Tools: Tools(
			ToolReadFile,
			ToolWriteFile,
			ToolPatchFile,
			ToolRunCommand,
			ToolListDirectory,
			ToolGrep,
			ToolAskUser,
			ToolTransitionPhase,
		),
		Required: true,
		OnEnter: func(state *State) string {
			// Return language-specific SDK instrumentation instructions
			instructions := GetInstrumentSDKPrompt(state.ProjectType)
			if instructions != PhaseInstrumentSDKNodejsPrompt {
				// Override with language-specific instructions
				return fmt.Sprintf("### Language-Specific Instructions\n\n%s", instructions)
			}
			return ""
		},
	}
}

func createConfigPhase() *Phase {
	return &Phase{
		ID:           "create_config",
		Name:         "Create Config",
		Description:  "Create the .tusk/config.yaml file",
		Instructions: PhaseCreateConfigPrompt,
		Tools: Tools(
			ToolWriteFile,
			ToolReadFile,
			ToolTuskValidateConfig,
			ToolAskUser,
			ToolTransitionPhase,
		),
		Required: true,
	}
}

func simpleTestPhase() *Phase {
	return &Phase{
		ID:           "simple_test",
		Name:         "Simple Test",
		Description:  "Record and replay a simple health check",
		Instructions: PhaseSimpleTestPrompt,
		Tools: Tools(
			ToolStartBackgroundProcess,
			ToolStopBackgroundProcess,
			ToolWaitForReady,
			ToolGetProcessLogs,
			ToolHTTPRequest,
			ToolTuskValidateConfig,
			ToolTuskList,
			ToolTuskRun,
			ToolReadFile,
			ToolWriteFile,
			ToolPatchFile,
			ToolRunCommand,
			ToolAskUser,
			ToolTransitionPhase,
		),
		Required:      true,
		MaxIterations: 100,
	}
}

func complexTestPhase() *Phase {
	return &Phase{
		ID:           "complex_test",
		Name:         "Complex Test",
		Description:  "Test an endpoint that makes external calls",
		Instructions: PhaseComplexTestPrompt,
		Tools: Tools(
			ToolStartBackgroundProcess,
			ToolStopBackgroundProcess,
			ToolWaitForReady,
			ToolGetProcessLogs,
			ToolHTTPRequest,
			ToolTuskList,
			ToolTuskRun,
			ToolReadFile,
			ToolGrep,
			ToolAskUser,
			ToolTransitionPhase,
			ToolWriteFile,
		),
		Required:      false,
		MaxIterations: 100,
	}
}

func summaryPhase() *Phase {
	return &Phase{
		ID:           "summary",
		Name:         "Summary",
		Description:  "Generate the setup report",
		Instructions: PhaseSummaryPrompt,
		Tools: Tools(
			ToolWriteFile,
			ToolReadFile,
			ToolTransitionPhase,
		),
		Required: true,
	}
}

// GetToolsForPhase returns the tool names available for a phase
func GetToolsForPhase(phase *Phase) []ToolName {
	out := make([]ToolName, 0, len(phase.Tools))
	for _, pt := range phase.Tools {
		out = append(out, pt.Name)
	}
	return out
}

// AllToolNames returns all possible tool names
func AllToolNames() []ToolName {
	seen := make(map[ToolName]bool)
	var all []ToolName
	for _, phase := range defaultPhases() {
		for _, pt := range phase.Tools {
			if !seen[pt.Name] {
				seen[pt.Name] = true
				all = append(all, pt.Name)
			}
		}
	}
	return all
}

// PhasesSummary returns a summary of all phases
func PhasesSummary() string {
	var lines []string
	for i, phase := range defaultPhases() {
		required := "required"
		if !phase.Required {
			required = "optional"
		}
		lines = append(lines, fmt.Sprintf("%d. %s (%s): %s", i+1, phase.Name, required, phase.Description))
	}
	return strings.Join(lines, "\n")
}

// Cloud Setup Phases

func cloudAuthPhase() *Phase {
	return &Phase{
		ID:           "cloud_auth",
		Name:         "Cloud Auth",
		Description:  "Authenticate with Tusk Cloud",
		Instructions: PhaseCloudAuthPrompt,
		Tools: Tools(
			ToolCloudCheckAuth,
			ToolCloudLogin,
			ToolCloudWaitForLogin,
			ToolCloudGetClients,
			ToolCloudSelectClient,
			ToolAskUser,
			ToolAskUserSelect,
			ToolTransitionPhase,
		),
		Required:      true,
		MaxIterations: 20,
	}
}

func cloudDetectRepoPhase() *Phase {
	return &Phase{
		ID:           "cloud_detect_repo",
		Name:         "Detect Repository",
		Description:  "Detect Git repository information",
		Instructions: PhaseCloudDetectRepoPrompt,
		Tools: Tools(
			ToolCloudDetectGitRepo,
			ToolAskUser,
			ToolTransitionPhase,
		),
		Required:      true,
		MaxIterations: 10,
	}
}

func cloudVerifyAccessPhase() *Phase {
	return &Phase{
		ID:           "cloud_verify_access",
		Name:         "Verify Access",
		Description:  "Verify Tusk has access to the repository",
		Instructions: PhaseCloudVerifyAccessPrompt,
		Tools: Tools(
			ToolCloudVerifyRepoAccess,
			ToolCloudGetAuthURL,
			ToolCloudOpenBrowser,
			ToolAskUser,
			ToolTransitionPhase,
		),
		Required:      true,
		MaxIterations: 30,
	}
}

func cloudCreateServicePhase() *Phase {
	return &Phase{
		ID:           "cloud_create_service",
		Name:         "Create Service",
		Description:  "Register a service in Tusk Cloud",
		Instructions: PhaseCloudCreateServicePrompt,
		Tools: Tools(
			ToolCloudCreateService,
			ToolTransitionPhase,
			ToolAskUser,
		),
		Required:      true,
		MaxIterations: 10,
	}
}

func cloudCreateApiKeyPhase() *Phase {
	return &Phase{
		ID:           "cloud_create_api_key",
		Name:         "Create API Key",
		Description:  "Generate an API key for CI/CD",
		Instructions: PhaseCloudCreateApiKeyPrompt,
		Tools: Tools(
			ToolCloudCheckApiKeyExists,
			ToolCloudCreateApiKey,
			ToolAskUser,
			ToolTransitionPhase,
		),
		Required:      false,
		MaxIterations: 15,
	}
}

func cloudConfigureRecordingPhase() *Phase {
	return &Phase{
		ID:           "cloud_configure_recording",
		Name:         "Configure Recording",
		Description:  "Configure recording parameters",
		Instructions: PhaseCloudConfigureRecordingPrompt,
		Tools: Tools(
			ToolCloudSaveConfig,
			ToolReadFile,
			ToolWriteFile,
			ToolAskUser,
			ToolTransitionPhase,
		),
		Required:      true,
		MaxIterations: 15,
	}
}

func cloudUploadTracesPhase() *Phase {
	return &Phase{
		ID:           "cloud_upload_traces",
		Name:         "Upload Traces",
		Description:  "Upload local traces to Tusk Cloud",
		Instructions: PhaseCloudUploadTracesPrompt,
		Tools: Tools(
			ToolTuskList,
			ToolStartBackgroundProcess,
			ToolStopBackgroundProcess,
			ToolWaitForReady,
			ToolGetProcessLogs,
			ToolHTTPRequest,
			ToolCloudUploadTraces,
			ToolCloudSaveConfig,
			ToolReadFile,
			ToolAskUser,
			ToolTransitionPhase,
			ToolAbortSetup,
			ToolResetCloudProgress,
			ToolTuskValidateConfig,
		),
		Required:      false,
		MaxIterations: 30,
	}
}

func cloudValidateSuitePhase() *Phase {
	return &Phase{
		ID:           "cloud_validate_suite",
		Name:         "Validate Suite",
		Description:  "Validate traces and add to test suite",
		Instructions: PhaseCloudValidateSuitePrompt,
		Tools: Tools(
			ToolCloudRunValidation,
			ToolAskUser,
			ToolTransitionPhase,
		),
		Required:      false,
		MaxIterations: 15,
	}
}

func cloudSummaryPhase() *Phase {
	return &Phase{
		ID:           "cloud_summary",
		Name:         "Cloud Summary",
		Description:  "Generate cloud setup report",
		Instructions: PhaseCloudSummaryPrompt,
		Tools: Tools(
			ToolWriteFile,
			ToolReadFile,
			ToolTransitionPhase,
		),
		Required: true,
	}
}

// Verify Mode Phases

func verifySetupPhase() *Phase {
	return &Phase{
		ID:           "verify_setup",
		Name:         "Verify Setup",
		Description:  "Validate config and prepare for verification",
		Instructions: PhaseVerifySetupPrompt,
		Tools: Tools(
			ToolTuskValidateConfig,
			ToolReadFile,
			ToolWriteFile,
			ToolCloudSaveConfig,
			ToolRunCommand,
			ToolAskUser,
			ToolTransitionPhase,
		),
		Required:      true,
		MaxIterations: 15,
	}
}

func verifySimpleTestPhase() *Phase {
	return &Phase{
		ID:           "verify_simple_test",
		Name:         "Verify Simple Test",
		Description:  "Record and replay a simple health check",
		Instructions: PhaseVerifySimpleTestPrompt,
		Tools: Tools(
			ToolStartBackgroundProcess,
			ToolStopBackgroundProcess,
			ToolWaitForReady,
			ToolGetProcessLogs,
			ToolHTTPRequest,
			ToolTuskList,
			ToolTuskRun,
			ToolReadFile,
			ToolWriteFile,
			ToolRunCommand,
			ToolTransitionPhase,
		),
		Required:      true,
		MaxIterations: 50,
	}
}

func verifyComplexTestPhase() *Phase {
	return &Phase{
		ID:           "verify_complex_test",
		Name:         "Verify Complex Test",
		Description:  "Record and replay a complex endpoint (optional)",
		Instructions: PhaseVerifyComplexTestPrompt,
		Tools: Tools(
			ToolStartBackgroundProcess,
			ToolStopBackgroundProcess,
			ToolWaitForReady,
			ToolGetProcessLogs,
			ToolHTTPRequest,
			ToolTuskList,
			ToolTuskRun,
			ToolReadFile,
			ToolWriteFile,
			ToolGrep,
			ToolTransitionPhase,
		),
		Required:      false,
		MaxIterations: 50,
	}
}

func verifyRestorePhase() *Phase {
	return &Phase{
		ID:           "verify_restore",
		Name:         "Verify Restore",
		Description:  "Restore config and report results",
		Instructions: PhaseVerifyRestorePrompt,
		Tools: Tools(
			ToolCloudSaveConfig,
			ToolReadFile,
			ToolWriteFile,
			ToolTransitionPhase,
		),
		Required:      true,
		MaxIterations: 10,
	}
}

// Eligibility Check Phase

func eligibilityCheckPhase() *Phase {
	return &Phase{
		ID:           "eligibility_check",
		Name:         "Eligibility Check",
		Description:  "Discover services and check SDK compatibility",
		Instructions: PhaseEligibilityCheckPrompt,
		Tools: Tools(
			ToolListDirectory,
			ToolReadFile,
			ToolGrep,
			ToolTransitionPhase,
		),
		Required:      true,
		MaxIterations: 100,
		OnEnter: func(state *State) string {
			// Fetch manifests for all supported languages
			var manifestInfo strings.Builder
			manifestInfo.WriteString("### SDK Manifests\n\n")

			for _, lang := range []string{"nodejs", "python"} {
				url := tools.GetManifestURLForProjectType(lang)
				if url == "" {
					continue
				}
				manifest, err := tools.FetchManifestFromURL(url)
				if err != nil {
					manifestInfo.WriteString(fmt.Sprintf("**%s**: Failed to fetch manifest - %s\n\n", lang, err))
					continue
				}
				manifestInfo.WriteString(fmt.Sprintf("**%s Manifest**:\n```json\n%s\n```\n\n", lang, manifest))
			}

			return manifestInfo.String()
		},
	}
}
