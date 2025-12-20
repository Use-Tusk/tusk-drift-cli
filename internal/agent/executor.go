package agent

import (
	"encoding/json"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/agent/tools"
)

// ToolName is a type-safe identifier for tools
type ToolName string

// Tool name constants - use these instead of raw strings for type safety
const (
	ToolReadFile               ToolName = "read_file"
	ToolWriteFile              ToolName = "write_file"
	ToolListDirectory          ToolName = "list_directory"
	ToolGrep                   ToolName = "grep"
	ToolPatchFile              ToolName = "patch_file"
	ToolRunCommand             ToolName = "run_command"
	ToolStartBackgroundProcess ToolName = "start_background_process"
	ToolStopBackgroundProcess  ToolName = "stop_background_process"
	ToolGetProcessLogs         ToolName = "get_process_logs"
	ToolWaitForReady           ToolName = "wait_for_ready"
	ToolHTTPRequest            ToolName = "http_request"
	ToolAskUser                ToolName = "ask_user"
	ToolTuskList               ToolName = "tusk_list"
	ToolTuskRun                ToolName = "tusk_run"
	ToolTransitionPhase        ToolName = "transition_phase"
	ToolAbortSetup             ToolName = "abort_setup"
	ToolFetchSDKManifest       ToolName = "fetch_sdk_manifest"
)

// ToolDefinition is the single source of truth for a tool's metadata and implementation
type ToolDefinition struct {
	Name                 ToolName
	Description          string
	InputSchema          json.RawMessage
	Executor             ToolExecutor // Set at runtime via RegisterTools
	RequiresConfirmation bool         // Whether this tool requires user confirmation by default
}

// ToolRegistry holds all tool definitions, keyed by name
type ToolRegistry struct {
	tools map[ToolName]*ToolDefinition
}

// Global registry instance (populated by RegisterTools)
var globalRegistry *ToolRegistry

// Get returns a tool definition by name
func (r *ToolRegistry) Get(name ToolName) *ToolDefinition {
	if r == nil || r.tools == nil {
		return nil
	}
	return r.tools[name]
}

// All returns all tool definitions
func (r *ToolRegistry) All() []*ToolDefinition {
	if r == nil || r.tools == nil {
		return nil
	}
	result := make([]*ToolDefinition, 0, len(r.tools))
	for _, def := range r.tools {
		result = append(result, def)
	}
	return result
}

// PhaseTool represents a tool available in a phase with optional configuration
type PhaseTool struct {
	Name                 ToolName
	RequiresConfirmation bool          // Require user confirmation before executing
	CustomTimeout        time.Duration // Override default timeout (0 = use default)
}

// T creates a PhaseTool with default configuration (short name for use with builder pattern)
func T(name ToolName) PhaseTool {
	return PhaseTool{Name: name}
}

// WithConfirmation returns a copy of the PhaseTool that requires user confirmation
func (pt PhaseTool) WithConfirmation() PhaseTool {
	pt.RequiresConfirmation = true
	return pt
}

// WithTimeout returns a copy of the PhaseTool with a custom timeout
func (pt PhaseTool) WithTimeout(d time.Duration) PhaseTool {
	pt.CustomTimeout = d
	return pt
}

// Tools is a helper to create []PhaseTool from ToolName constants (simple case, no config)
func Tools(names ...ToolName) []PhaseTool {
	out := make([]PhaseTool, 0, len(names))
	for _, n := range names {
		out = append(out, PhaseTool{Name: n})
	}
	return out
}

// toolDefinitions returns the static tool definitions (schema only, no executors)
func toolDefinitions() map[ToolName]*ToolDefinition {
	return map[ToolName]*ToolDefinition{
		ToolReadFile: {
			Name:        ToolReadFile,
			Description: "Read the contents of a file at the given path. Use this to examine source code, configuration files, etc. Do not read binary files.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {
						"type": "string",
						"description": "Path to the file to read (relative to project root)"
					}
				},
				"required": ["path"]
			}`),
		},
		ToolWriteFile: {
			Name:        ToolWriteFile,
			Description: "Write content to a file. Creates the file and any parent directories if they don't exist. Use this to create new files like tuskDriftInit.ts or config.yaml.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {
						"type": "string",
						"description": "Path to the file to write"
					},
					"content": {
						"type": "string",
						"description": "Content to write to the file"
					}
				},
				"required": ["path", "content"]
			}`),
			RequiresConfirmation: true,
		},
		ToolListDirectory: {
			Name:        ToolListDirectory,
			Description: "List files and directories in the given path. Use this to explore the project structure.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {
						"type": "string",
						"description": "Directory path to list (default: current directory)"
					}
				}
			}`),
		},
		ToolGrep: {
			Name:        ToolGrep,
			Description: "Search for a pattern in files. Use this to find specific code patterns, imports, or configurations.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"pattern": {
						"type": "string",
						"description": "Regex pattern to search for"
					},
					"path": {
						"type": "string",
						"description": "File or directory to search in (default: current directory)"
					},
					"include": {
						"type": "string",
						"description": "Glob pattern for files to include (e.g., '*.ts', '*.json')"
					}
				},
				"required": ["pattern"]
			}`),
		},
		ToolPatchFile: {
			Name:        ToolPatchFile,
			Description: "Apply a targeted edit to an existing file by replacing a specific string. The search string must be unique in the file.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {
						"type": "string",
						"description": "Path to the file to modify"
					},
					"search": {
						"type": "string",
						"description": "Exact text to find in the file (must be unique)"
					},
					"replace": {
						"type": "string",
						"description": "Text to replace it with"
					}
				},
				"required": ["path", "search", "replace"]
			}`),
			RequiresConfirmation: true,
		},
		ToolRunCommand: {
			Name:        ToolRunCommand,
			Description: "Run a shell command and wait for it to complete. Use ONLY for one-shot commands like 'npm install', 'sleep 2', 'cat file.txt'. DO NOT use for servers or long-running processes like 'npm run dev' or 'npm start' - use start_background_process instead.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"command": {
						"type": "string",
						"description": "Command to execute (runs in sh -c)"
					},
					"timeout_seconds": {
						"type": "integer",
						"description": "Timeout in seconds (default: 120)"
					}
				},
				"required": ["command"]
			}`),
			RequiresConfirmation: true,
		},
		ToolStartBackgroundProcess: {
			Name:        ToolStartBackgroundProcess,
			Description: "Start a long-running process in the background (e.g., 'npm run dev', 'npm start', 'node server.js'). Returns a handle to reference it later. ALWAYS use this for starting servers instead of run_command.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"command": {
						"type": "string",
						"description": "Command to execute"
					},
					"env": {
						"type": "object",
						"description": "Additional environment variables to set (e.g., {\"TUSK_DRIFT_MODE\": \"RECORD\"})",
						"additionalProperties": {"type": "string"}
					}
				},
				"required": ["command"]
			}`),
			RequiresConfirmation: true,
		},
		ToolStopBackgroundProcess: {
			Name:        ToolStopBackgroundProcess,
			Description: "Stop a background process started with start_background_process.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"handle": {
						"type": "string",
						"description": "Process handle returned by start_background_process"
					}
				},
				"required": ["handle"]
			}`),
		},
		ToolGetProcessLogs: {
			Name:        ToolGetProcessLogs,
			Description: "Get recent stdout/stderr from a background process. Use this to check for errors or verify the service is running correctly.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"handle": {
						"type": "string",
						"description": "Process handle"
					},
					"lines": {
						"type": "integer",
						"description": "Number of recent lines to return (default: 100)"
					}
				},
				"required": ["handle"]
			}`),
		},
		ToolWaitForReady: {
			Name:        ToolWaitForReady,
			Description: "Wait for a service to be ready by polling an HTTP endpoint. Returns when the endpoint responds with 2xx/3xx or times out.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {
						"type": "string",
						"description": "URL to poll (e.g., http://localhost:3000/health)"
					},
					"timeout_seconds": {
						"type": "integer",
						"description": "Maximum time to wait (default: 30)"
					},
					"interval_seconds": {
						"type": "integer",
						"description": "Polling interval (default: 1)"
					}
				},
				"required": ["url"]
			}`),
		},
		ToolHTTPRequest: {
			Name:        ToolHTTPRequest,
			Description: "Make an HTTP request and return the response. Use this to test endpoints.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"method": {
						"type": "string",
						"description": "HTTP method (GET, POST, PUT, DELETE, etc.)"
					},
					"url": {
						"type": "string",
						"description": "URL to request"
					},
					"headers": {
						"type": "object",
						"description": "Request headers",
						"additionalProperties": {"type": "string"}
					},
					"body": {
						"type": "string",
						"description": "Request body"
					}
				},
				"required": ["method", "url"]
			}`),
			RequiresConfirmation: true,
		},
		ToolAskUser: {
			Name:        ToolAskUser,
			Description: "Ask the user a question and wait for their response. Use when you need clarification, confirmation, or information you can't determine from the codebase.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"question": {
						"type": "string",
						"description": "Question to ask the user. Be clear and specific."
					}
				},
				"required": ["question"]
			}`),
		},
		ToolTuskList: {
			Name:        ToolTuskList,
			Description: "Run 'tusk list' to show available recorded traces. Use after recording to verify traces were captured.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
		ToolTuskRun: {
			Name:        ToolTuskRun,
			Description: "Run 'tusk run' to replay recorded traces and verify the service behaves consistently. Returns test results.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"filter": {
						"type": "string",
						"description": "Optional filter pattern for traces (regex)"
					},
					"debug": {
						"type": "boolean",
						"description": "Enable debug logging for troubleshooting"
					}
				}
			}`),
			RequiresConfirmation: true,
		},
		ToolTransitionPhase: {
			Name:        ToolTransitionPhase,
			Description: "Complete the current phase and move to the next one. You MUST call this to progress through phases. Include results from the current phase.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"results": {
						"type": "object",
						"description": "Results/findings from this phase to save (e.g., {\"port\": \"3000\", \"entry_point\": \"src/server.ts\"})"
					},
					"notes": {
						"type": "string",
						"description": "Any additional notes or observations"
					}
				}
			}`),
		},
		ToolAbortSetup: {
			Name:        ToolAbortSetup,
			Description: "Abort the setup process gracefully. Use when you detect an unsupported project type (e.g., not Node.js) or when setup cannot proceed for a valid reason. Provide a clear reason for the user.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"reason": {
						"type": "string",
						"description": "Clear explanation of why setup cannot proceed (e.g., 'Tusk Drift currently only supports Node.js projects. Detected: Python project (found requirements.txt)')"
					}
				},
				"required": ["reason"]
			}`),
		},
		ToolFetchSDKManifest: {
			Name:        ToolFetchSDKManifest,
			Description: "Fetch an SDK instrumentation manifest from a trusted CDN (unpkg.com, jsdelivr.net, npmjs.org). Use this to discover what packages are instrumented by a Tusk Drift SDK. Returns JSON with sdkVersion, language, and instrumentations array.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {
						"type": "string",
						"description": "URL of the SDK manifest (must be from unpkg.com, cdn.jsdelivr.net, or registry.npmjs.org)"
					}
				},
				"required": ["url"]
			}`),
		},
	}
}

// RegisterTools initializes the tool registry with executors and returns API-compatible formats
func RegisterTools(workDir string, pm *ProcessManager, phaseMgr *PhaseManager) ([]Tool, map[string]ToolExecutor) {
	// Create tool implementations
	fs := tools.NewFilesystemTools(workDir)
	proc := tools.NewProcessTools(pm, workDir)
	http := tools.NewHTTPTools()
	user := tools.NewUserTools()
	tusk := tools.NewTuskTools(workDir)

	// Map tool names to their executors
	executorMap := map[ToolName]ToolExecutor{
		ToolReadFile:               fs.ReadFile,
		ToolWriteFile:              fs.WriteFile,
		ToolListDirectory:          fs.ListDirectory,
		ToolGrep:                   fs.Grep,
		ToolPatchFile:              fs.PatchFile,
		ToolRunCommand:             proc.RunCommand,
		ToolStartBackgroundProcess: proc.StartBackground,
		ToolStopBackgroundProcess:  proc.StopBackground,
		ToolGetProcessLogs:         proc.GetLogs,
		ToolWaitForReady:           proc.WaitForReady,
		ToolHTTPRequest:            http.Request,
		ToolAskUser:                user.Ask,
		ToolTuskList:               tusk.List,
		ToolTuskRun:                tusk.Run,
		ToolTransitionPhase:        phaseMgr.PhaseTransitionTool(),
		ToolAbortSetup:             tools.AbortSetup,
		ToolFetchSDKManifest:       tools.FetchSDKManifest,
	}

	// Build registry with definitions + executors
	defs := toolDefinitions()
	for name, executor := range executorMap {
		if def, ok := defs[name]; ok {
			def.Executor = executor
		}
	}

	globalRegistry = &ToolRegistry{tools: defs}

	// Convert to API formats for backward compatibility
	var apiTools []Tool
	apiExecutors := make(map[string]ToolExecutor)

	for _, def := range defs {
		apiTools = append(apiTools, Tool{
			Name:        string(def.Name),
			Description: def.Description,
			InputSchema: def.InputSchema,
		})
		if def.Executor != nil {
			apiExecutors[string(def.Name)] = def.Executor
		}
	}

	return apiTools, apiExecutors
}

// GetRegistry returns the global tool registry (available after RegisterTools is called)
func GetRegistry() *ToolRegistry {
	return globalRegistry
}

// FilterToolsForPhase filters tool definitions to only those available in the current phase
func FilterToolsForPhase(allTools []Tool, phase *Phase) []Tool {
	allowed := make(map[ToolName]bool)
	for _, pt := range phase.Tools {
		allowed[pt.Name] = true
	}

	var filtered []Tool
	for _, tool := range allTools {
		if allowed[ToolName(tool.Name)] {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

// GetPhaseToolConfig returns the PhaseTool config for a given tool in a phase (for checking confirmation, timeout, etc.)
func GetPhaseToolConfig(phase *Phase, name ToolName) *PhaseTool {
	for i := range phase.Tools {
		if phase.Tools[i].Name == name {
			return &phase.Tools[i]
		}
	}
	return nil
}

// ProcessManager wraps the tools.ProcessManager for external access
type ProcessManager = tools.ProcessManager

// NewProcessManager creates a new ProcessManager
func NewProcessManager(workDir string) *ProcessManager {
	return tools.NewProcessManager(workDir)
}

// NewProcessManagerWithOptions creates a new ProcessManager with options
func NewProcessManagerWithOptions(workDir string, disableSandbox bool) *ProcessManager {
	return tools.NewProcessManagerWithOptions(workDir, disableSandbox)
}
