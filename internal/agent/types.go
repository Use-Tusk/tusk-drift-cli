package agent

import "encoding/json"

// Message represents a conversation message
type Message struct {
	Role    string    `json:"role"` // "user", "assistant"
	Content []Content `json:"content"`
}

// Content represents a content block in a message
type Content struct {
	Type      string          `json:"type"` // "text", "tool_use", "tool_result"
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`          // For tool_use
	Name      string          `json:"name,omitempty"`        // For tool_use
	Input     json.RawMessage `json:"input,omitempty"`       // For tool_use
	ToolUseID string          `json:"tool_use_id,omitempty"` // For tool_result
	Content   string          `json:"content,omitempty"`     // For tool_result (as string)
	IsError   bool            `json:"is_error,omitempty"`    // For tool_result
}

// Tool defines a tool the agent can use
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ToolExecutor executes a tool and returns the result
type ToolExecutor func(input json.RawMessage) (string, error)

// APIResponse from Claude API
type APIResponse struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Role         string    `json:"role"`
	Content      []Content `json:"content"`
	Model        string    `json:"model"`
	StopReason   string    `json:"stop_reason"` // "end_turn", "tool_use", "max_tokens"
	StopSequence *string   `json:"stop_sequence"`
	Usage        Usage     `json:"usage"`
}

// Usage tracks token usage
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// APIError represents an error from the Claude API
type APIError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// State tracks what the agent has learned and done
type State struct {
	// Discovery results
	ProjectType      string `json:"project_type"`       // "nodejs", "python", "go", etc.
	PackageManager   string `json:"package_manager"`    // "npm", "yarn", "pnpm" (Node.js); "pip", "poetry", "uv", "pipenv" (Python)
	ModuleSystem     string `json:"module_system"`      // "esm", "cjs" (Node.js specific)
	Framework        string `json:"framework"`          // "fastapi", "flask", "django" (Python specific); "express", "fastify", "hono" (Node.js)
	EntryPoint       string `json:"entry_point"`        // e.g., "src/server.ts", "main.py", "app/main.py"
	StartCommand     string `json:"start_command"`      // e.g., "npm run start", "uvicorn app.main:app"
	Port             string `json:"port"`               // e.g., "3000", "8000"
	HealthEndpoint   string `json:"health_endpoint"`    // e.g., "/health"
	DockerType       string `json:"docker_type"`        // "none", "dockerfile", "compose"
	ServiceName      string `json:"service_name"`       // e.g., "my-service"
	HasExternalCalls bool   `json:"has_external_calls"` // Does it make outbound HTTP/DB calls?

	// Compatibility check results
	CompatibilityWarnings []string `json:"compatibility_warnings"` // e.g., ["mongodb@6.3.0 not instrumented"]

	// Progress tracking
	AppStartsWithoutSDK bool `json:"app_starts_without_sdk"`
	SDKInstalled        bool `json:"sdk_installed"`
	SDKInstrumented     bool `json:"sdk_instrumented"`
	ConfigCreated       bool `json:"config_created"`
	SimpleTestPassed    bool `json:"simple_test_passed"`
	ComplexTestPassed   bool `json:"complex_test_passed"`

	// Error tracking
	Errors   []PhaseError `json:"errors"`
	Warnings []string     `json:"warnings"`

	// Eligibility mode
	EligibilityReport string `json:"eligibility_report,omitempty"` // Raw JSON report

	// Cloud setup state
	IsAuthenticated       bool    `json:"is_authenticated"`
	UserEmail             string  `json:"user_email"`
	UserId                string  `json:"user_id"`
	BearerToken           string  `json:"bearer_token,omitempty"`
	SelectedClientID      string  `json:"selected_client_id"`
	SelectedClientName    string  `json:"selected_client_name"`
	GitRepoOwner          string  `json:"git_repo_owner"`
	GitRepoName           string  `json:"git_repo_name"`
	CodeHostingType       string  `json:"code_hosting_type"` // "github" or "gitlab"
	CloudServiceID        string  `json:"cloud_service_id"`
	ApiKeyCreated         bool    `json:"api_key_created"`
	ApiKeyValue           string  `json:"api_key_value,omitempty"` // Only set during session, not persisted
	SamplingRate          float64 `json:"sampling_rate"`
	ExportSpans           bool    `json:"export_spans"`
	EnableEnvVarRecording bool    `json:"enable_env_var_recording"`
}

// PhaseError represents an error that occurred during a phase
type PhaseError struct {
	Phase   string `json:"phase"`
	Message string `json:"message"`
	Fatal   bool   `json:"fatal"`
}

// Config holds agent configuration
type Config struct {
	APIKey          string
	Model           string
	SystemPrompt    string
	MaxTokens       int
	WorkDir         string
	SkipPermissions bool // Skip permission prompts for consequential actions
	DisableProgress bool // Don't save or resume from PROGRESS.md
	SkipToCloud     bool // Skip local setup and go directly to cloud setup (for testing)
	PrintMode       bool // Headless mode - no TUI, stream to stdout
	OutputLogs      bool // Output all logs to a file in .tusk/logs/
	EligibilityOnly bool // Only run eligibility check, output JSON and exit
}
