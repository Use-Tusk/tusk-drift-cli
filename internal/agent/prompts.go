package agent

import (
	_ "embed"
)

// Embedded prompt files - loaded at compile time

//go:embed prompts/system.md
var SystemPrompt string

//go:embed prompts/phase_detect_language.md
var PhaseDetectLanguagePrompt string

//go:embed prompts/phase_check_compatibility.md
var PhaseCheckCompatibilityPrompt string

//go:embed prompts/phase_gather_info_nodejs.md
var PhaseGatherInfoNodejsPrompt string

//go:embed prompts/phase_confirm_app_starts.md
var PhaseConfirmAppStartsPrompt string

//go:embed prompts/phase_instrument_sdk.md
var PhaseInstrumentSDKPrompt string

//go:embed prompts/phase_create_config.md
var PhaseCreateConfigPrompt string

//go:embed prompts/phase_simple_test.md
var PhaseSimpleTestPrompt string

//go:embed prompts/phase_complex_test.md
var PhaseComplexTestPrompt string

//go:embed prompts/phase_summary.md
var PhaseSummaryPrompt string
