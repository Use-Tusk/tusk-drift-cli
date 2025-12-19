package agent

import (
	"encoding/json"
	"fmt"
	"strings"
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

		return fmt.Sprintf("✅ Transitioned to phase: %s\n\n%s\n\nCurrent state:\n%s",
			next.Name, next.Instructions, pm.StateAsContext()), nil
	}
}

func defaultPhases() []*Phase {
	return []*Phase{
		discoveryPhase(),
		confirmAppStartsPhase(),
		instrumentSDKPhase(),
		createConfigPhase(),
		simpleTestPhase(),
		complexTestPhase(),
		summaryPhase(),
	}
}

func discoveryPhase() *Phase {
	return &Phase{
		ID:           "discovery",
		Name:         "Discovery",
		Description:  "Analyze the project structure and dependencies",
		Instructions: PhaseDiscoveryPrompt,
		Tools: Tools(
			ToolReadFile,
			ToolListDirectory,
			ToolGrep,
			ToolAskUser,
			ToolTransitionPhase,
		),
		Required:      true,
		MaxIterations: 30,
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
		ID:           "instrument_sdk",
		Name:         "Instrument SDK",
		Description:  "Install and configure the Tusk Drift SDK",
		Instructions: PhaseInstrumentSDKPrompt,
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
