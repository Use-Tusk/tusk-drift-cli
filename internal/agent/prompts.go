package agent

import (
	_ "embed"
)

// Embedded prompt files - loaded at compile time

//go:embed prompts/system.md
var SystemPrompt string

//go:embed prompts/phase_detect_language.md
var PhaseDetectLanguagePrompt string

// Node.js specific prompts
//
//go:embed prompts/phase_check_compatibility.md
var PhaseCheckCompatibilityNodejsPrompt string

//go:embed prompts/phase_gather_info_nodejs.md
var PhaseGatherInfoNodejsPrompt string

//go:embed prompts/phase_instrument_sdk.md
var PhaseInstrumentSDKNodejsPrompt string

// Python specific prompts
//
//go:embed prompts/phase_check_compatibility_python.md
var PhaseCheckCompatibilityPythonPrompt string

//go:embed prompts/phase_gather_info_python.md
var PhaseGatherInfoPythonPrompt string

//go:embed prompts/phase_instrument_sdk_python.md
var PhaseInstrumentSDKPythonPrompt string

// Shared prompts (language-agnostic)
//
//go:embed prompts/phase_confirm_app_starts.md
var PhaseConfirmAppStartsPrompt string

//go:embed prompts/phase_create_config.md
var PhaseCreateConfigPrompt string

//go:embed prompts/phase_simple_test.md
var PhaseSimpleTestPrompt string

//go:embed prompts/phase_complex_test.md
var PhaseComplexTestPrompt string

//go:embed prompts/phase_summary.md
var PhaseSummaryPrompt string

// Cloud setup prompts
//
//go:embed prompts/phase_cloud_auth.md
var PhaseCloudAuthPrompt string

//go:embed prompts/phase_cloud_detect_repo.md
var PhaseCloudDetectRepoPrompt string

//go:embed prompts/phase_cloud_verify_access.md
var PhaseCloudVerifyAccessPrompt string

//go:embed prompts/phase_cloud_create_service.md
var PhaseCloudCreateServicePrompt string

//go:embed prompts/phase_cloud_create_api_key.md
var PhaseCloudCreateApiKeyPrompt string

//go:embed prompts/phase_cloud_configure_recording.md
var PhaseCloudConfigureRecordingPrompt string

//go:embed prompts/phase_cloud_summary.md
var PhaseCloudSummaryPrompt string

// GetGatherInfoPrompt returns the appropriate gather info prompt for the project type.
func GetGatherInfoPrompt(projectType string) string {
	switch projectType {
	case "python":
		return PhaseGatherInfoPythonPrompt
	case "nodejs":
		return PhaseGatherInfoNodejsPrompt
	default:
		return PhaseGatherInfoNodejsPrompt // Default to Node.js for backward compatibility
	}
}

// GetCheckCompatibilityPrompt returns the appropriate compatibility check prompt for the project type.
func GetCheckCompatibilityPrompt(projectType string) string {
	switch projectType {
	case "python":
		return PhaseCheckCompatibilityPythonPrompt
	case "nodejs":
		return PhaseCheckCompatibilityNodejsPrompt
	default:
		return PhaseCheckCompatibilityNodejsPrompt // Default to Node.js for backward compatibility
	}
}

// GetInstrumentSDKPrompt returns the appropriate SDK instrumentation prompt for the project type.
func GetInstrumentSDKPrompt(projectType string) string {
	switch projectType {
	case "python":
		return PhaseInstrumentSDKPythonPrompt
	case "nodejs":
		return PhaseInstrumentSDKNodejsPrompt
	default:
		return PhaseInstrumentSDKNodejsPrompt // Default to Node.js for backward compatibility
	}
}
