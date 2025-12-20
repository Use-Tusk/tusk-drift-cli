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
	ProjectType      string `json:"project_type"`       // "nodejs", "unknown"
	PackageManager   string `json:"package_manager"`    // "npm", "yarn", "pnpm"
	ModuleSystem     string `json:"module_system"`      // "esm", "cjs"
	EntryPoint       string `json:"entry_point"`        // e.g., "src/server.ts"
	StartCommand     string `json:"start_command"`      // e.g., "npm run start"
	Port             string `json:"port"`               // e.g., "3000"
	HealthEndpoint   string `json:"health_endpoint"`    // e.g., "/health"
	DockerType       string `json:"docker_type"`        // "none", "dockerfile", "compose"
	ServiceName      string `json:"service_name"`       // e.g., "my-service"
	HasExternalCalls bool   `json:"has_external_calls"` // Does it make outbound HTTP/DB calls?

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
	DisableSandbox  bool // Disable fence sandboxing for commands
}
