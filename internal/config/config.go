package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

var (
	k = koanf.New(".")

	cachedConfig    *Config
	cachedConfigErr error
	loadMutex       sync.Mutex
	hasLoaded       bool
)

type Config struct {
	Service       ServiceConfig       `koanf:"service"`
	TuskAPI       TuskAPIConfig       `koanf:"tusk_api"`
	Comparison    ComparisonConfig    `koanf:"comparison"`
	TestExecution TestExecutionConfig `koanf:"test_execution"`
	Recording     RecordingConfig     `koanf:"recording"`
	Replay        ReplayConfig        `koanf:"replay"`
	Traces        TracesConfig        `koanf:"traces"`
	Results       ResultsConfig       `koanf:"results"`
}

type ServiceConfig struct {
	ID            string              `koanf:"id"`
	Name          string              `koanf:"name"`
	Port          int                 `koanf:"port"`
	Start         StartConfig         `koanf:"start"`
	Stop          StopConfig          `koanf:"stop"`
	Readiness     ReadinessConfig     `koanf:"readiness_check"`
	Communication CommunicationConfig `koanf:"communication"`
}

type StartConfig struct {
	Command string `koanf:"command"`
}

type StopConfig struct {
	Command string `koanf:"command"`
}

type CommunicationConfig struct {
	Type    string `koanf:"type"`     // "auto", "unix", "tcp"
	TCPPort int    `koanf:"tcp_port"` // Default: 9001
}

type ReadinessConfig struct {
	Command  string `koanf:"command"`
	Timeout  string `koanf:"timeout"`
	Interval string `koanf:"interval"`
}

type TuskAPIConfig struct {
	URL           string `koanf:"url"`
	Auth0Domain   string `koanf:"auth0_domain"`
	Auth0ClientID string `koanf:"auth0_client_id"`
}

type TestExecutionConfig struct {
	Concurrency int    `koanf:"concurrency"`
	Timeout     string `koanf:"timeout"`
}

type ComparisonConfig struct {
	IgnoreFields     []string `koanf:"ignore_fields"`
	IgnorePatterns   []string `koanf:"ignore_patterns"`
	IgnoreUUIDs      *bool    `koanf:"ignore_uuids"`
	IgnoreTimestamps *bool    `koanf:"ignore_timestamps"`
	IgnoreDates      *bool    `koanf:"ignore_dates"`
}

type RecordingConfig struct {
	SamplingRate          float64 `koanf:"sampling_rate"`
	ExportSpans           *bool   `koanf:"export_spans"`
	EnableEnvVarRecording *bool   `koanf:"enable_env_var_recording"`
}

type ReplayConfig struct {
	// Reserved for future replay-specific options
}

type TracesConfig struct {
	Dir string `koanf:"dir"`
}

type ResultsConfig struct {
	Dir string `koanf:"dir"`
}

// Load loads the config file and applies environment overrides.
// This function is idempotent - calling it multiple times will only load once.
func Load(configFile string) error {
	loadMutex.Lock()
	defer loadMutex.Unlock()

	if hasLoaded {
		log.Debug("Config already loaded, skipping reload")
		return nil
	}

	if configFile == "" {
		configFile = findConfigFile()
	}

	if configFile != "" {
		if err := k.Load(file.Provider(configFile), yaml.Parser()); err != nil {
			log.ServiceLog(fmt.Sprintf("Failed to load config file: %s. Error: %s", configFile, err))
			return fmt.Errorf("error loading config file: %w", err)
		}
		log.Debug("Config file loaded", "file", configFile)
	} else {
		log.Debug("No config file found, using defaults and environment variables")
	}

	// Support environment variable overrides for specific config keys
	envOverrides := map[string]string{
		"TUSK_TRACES_DIR":              "traces.dir",
		"TUSK_API_URL":                 "tusk_api.url",
		"TUSK_AUTH0_DOMAIN":            "tusk_api.auth0_domain",
		"TUSK_AUTH0_CLIENT_ID":         "tusk_api.auth0_client_id",
		"TUSK_RESULTS_DIR":             "results.dir",
		"TUSK_RECORDING_SAMPLING_RATE": "recording.sampling_rate",
	}

	for envKey, configKey := range envOverrides {
		if val := os.Getenv(envKey); val != "" {
			if err := k.Set(configKey, val); err != nil {
				return fmt.Errorf("error setting %s from env: %w", envKey, err)
			}
		}
	}

	hasLoaded = true
	log.Debug("All loaded config", "config", k.All())
	return nil
}

// Get returns the cached config. If not loaded yet, loads from default location.
func Get() (*Config, error) {
	// Ensure config is loaded first
	if err := Load(""); err != nil {
		return nil, err
	}

	loadMutex.Lock()
	defer loadMutex.Unlock()

	// Return cached config if available
	if cachedConfig != nil {
		return cachedConfig, cachedConfigErr
	}

	// Parse and cache the config
	cachedConfig, cachedConfigErr = parseAndValidate()
	return cachedConfig, cachedConfigErr
}

// parseAndValidate parses the loaded koanf data into a Config struct and validates it
func parseAndValidate() (*Config, error) {
	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}
	if cfg.Service.Port == 0 {
		cfg.Service.Port = 3000
	}
	if cfg.TestExecution.Concurrency == 0 {
		cfg.TestExecution.Concurrency = 1
	}
	if cfg.TestExecution.Timeout == "" {
		cfg.TestExecution.Timeout = "30s"
	}
	if cfg.Recording.SamplingRate == 0 {
		cfg.Recording.SamplingRate = 0.1
	}
	if cfg.Recording.ExportSpans == nil {
		defaultExportSpans := false
		cfg.Recording.ExportSpans = &defaultExportSpans
	}
	if cfg.Recording.EnableEnvVarRecording == nil {
		defaultEnableEnvVarRecording := false
		cfg.Recording.EnableEnvVarRecording = &defaultEnableEnvVarRecording
	}
	if cfg.Results.Dir == "" {
		cfg.Results.Dir = ".tusk/results"
	}
	if cfg.Traces.Dir == "" {
		cfg.Traces.Dir = ".tusk/traces"
	}
	if cfg.Service.Communication.Type == "" {
		cfg.Service.Communication.Type = "auto"
	}
	if cfg.Service.Communication.TCPPort == 0 {
		cfg.Service.Communication.TCPPort = 9001
	}
	if cfg.TuskAPI.URL == "" {
		cfg.TuskAPI.URL = "https://api.usetusk.ai"
	}
	if cfg.TuskAPI.Auth0Domain == "" {
		cfg.TuskAPI.Auth0Domain = "tusk.us.auth0.com"
	}
	if cfg.TuskAPI.Auth0ClientID == "" {
		cfg.TuskAPI.Auth0ClientID = "gXktT8e38sBmmXGWCGeXMLpwlpeECJS5"
	}

	// Resolve directory paths relative to tusk root
	cfg.Results.Dir = utils.ResolveTuskPath(cfg.Results.Dir)
	cfg.Traces.Dir = utils.ResolveTuskPath(cfg.Traces.Dir)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (cfg *Config) Validate() error {
	var errs []error

	if cfg.Service.Port < 1 || cfg.Service.Port > 65535 {
		errs = append(errs, fmt.Errorf("service.port must be between 1-65535, got %d", cfg.Service.Port))
	}

	if cfg.TestExecution.Timeout != "" {
		if _, err := time.ParseDuration(cfg.TestExecution.Timeout); err != nil {
			errs = append(errs, fmt.Errorf("test_execution.timeout: invalid duration %q", cfg.TestExecution.Timeout))
		}
	}

	if cfg.Service.Readiness.Timeout != "" {
		if _, err := time.ParseDuration(cfg.Service.Readiness.Timeout); err != nil {
			errs = append(errs, fmt.Errorf("service.readiness_check.timeout: invalid duration %q", cfg.Service.Readiness.Timeout))
		}
	}

	if cfg.Service.Readiness.Interval != "" {
		if _, err := time.ParseDuration(cfg.Service.Readiness.Interval); err != nil {
			errs = append(errs, fmt.Errorf("service.readiness_check.interval: invalid duration %q", cfg.Service.Readiness.Interval))
		}
	}

	validCommTypes := map[string]bool{"auto": true, "unix": true, "tcp": true}
	if !validCommTypes[cfg.Service.Communication.Type] {
		errs = append(errs, fmt.Errorf("service.communication.type must be 'auto', 'unix', or 'tcp', got %s", cfg.Service.Communication.Type))
	}

	if cfg.Service.Communication.TCPPort < 1 || cfg.Service.Communication.TCPPort > 65535 {
		errs = append(errs, fmt.Errorf("service.communication.tcp_port must be between 1-65535, got %d", cfg.Service.Communication.TCPPort))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// ValidationResult contains detailed validation results for config files.
type ValidationResult struct {
	Valid       bool     `json:"valid"`
	Errors      []string `json:"errors,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
	UnknownKeys []string `json:"unknown_keys,omitempty"`
	MissingKeys []string `json:"missing_keys,omitempty"`
	SchemaHint  string   `json:"schema_hint,omitempty"`
}

// ValidateConfigFile performs comprehensive validation on a config file.
// It checks for unknown keys, required fields, and value constraints.
func ValidateConfigFile(configPath string) *ValidationResult {
	result := &ValidationResult{Valid: true}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Config file not found: %s", configPath))
		return result
	}

	// Load config fresh
	Invalidate()
	if err := Load(configPath); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to parse config: %s", err))
		return result
	}

	unknownKeys := CheckUnknownKeys()
	if len(unknownKeys) > 0 {
		result.UnknownKeys = unknownKeys
		for _, key := range unknownKeys {
			suggestion := suggestCorrectKey(key)
			if suggestion != "" {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Unknown key '%s' - did you mean '%s'?", key, suggestion))
			} else {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Unknown key '%s' will be ignored", key))
			}
		}
	}

	cfg, err := Get()
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err.Error())
		return result
	}

	missingRequired := cfg.CheckRequiredForReplay()
	if len(missingRequired) > 0 {
		result.Valid = false
		result.MissingKeys = missingRequired
		for _, key := range missingRequired {
			result.Errors = append(result.Errors, fmt.Sprintf("Missing required field: %s", key))
		}
	}

	if !result.Valid || len(result.Warnings) > 0 {
		result.SchemaHint = getMinimalSchemaHint()
	}

	return result
}

// CheckRequiredForReplay returns a list of missing required fields for replay mode.
func (cfg *Config) CheckRequiredForReplay() []string {
	var missing []string

	if cfg.Service.Start.Command == "" {
		missing = append(missing, "service.start.command")
	}

	// Port has a default, but if explicitly set to 0, that's an error
	// (already caught by Validate, but we note it here for completeness)

	return missing
}

// CheckUnknownKeys compares loaded config keys against the valid schema.
// Returns a list of keys that don't match any known config field.
func CheckUnknownKeys() []string {
	validKeys := getValidKeys(reflect.TypeOf(Config{}), "")
	loadedKeys := k.Keys()

	validSet := make(map[string]bool)
	for _, key := range validKeys {
		validSet[key] = true
		// Also add parent paths as valid (e.g., "service" is valid if "service.port" is valid)
		parts := strings.Split(key, ".")
		for i := 1; i < len(parts); i++ {
			validSet[strings.Join(parts[:i], ".")] = true
		}
	}

	var unknown []string
	for _, key := range loadedKeys {
		if !validSet[key] {
			unknown = append(unknown, key)
		}
	}

	return unknown
}

// getValidKeys extracts all valid config key paths from struct tags recursively.
func getValidKeys(t reflect.Type, prefix string) []string {
	var keys []string

	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return keys
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("koanf")
		if tag == "" || tag == "-" {
			continue
		}

		fullKey := tag
		if prefix != "" {
			fullKey = prefix + "." + tag
		}

		keys = append(keys, fullKey)

		// Recurse into nested structs
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() == reflect.Struct {
			keys = append(keys, getValidKeys(fieldType, fullKey)...)
		}
	}

	return keys
}

// suggestCorrectKey suggests a correct key name for common mistakes.
func suggestCorrectKey(unknownKey string) string {
	suggestions := map[string]string{
		"start_command":         "service.start.command",
		"stop_command":          "service.stop.command",
		"readiness_command":     "service.readiness_check.command",
		"port":                  "service.port",
		"name":                  "service.name",
		"timeout":               "test_execution.timeout (or service.readiness_check.timeout)",
		"concurrency":           "test_execution.concurrency",
		"start":                 "service.start",
		"stop":                  "service.stop",
		"readiness":             "service.readiness_check",
		"readiness_check":       "service.readiness_check",
		"service.readiness":     "service.readiness_check",
		"service.start_command": "service.start.command",
	}

	if suggestion, ok := suggestions[unknownKey]; ok {
		return suggestion
	}

	return ""
}

// getMinimalSchemaHint returns a minimal example of the correct config structure.
func getMinimalSchemaHint() string {
	return `service:
  name: my-service        # optional
  port: 3000              # required for replay
  start:
    command: "npm start"  # required for replay
  readiness_check:        # optional but recommended
    command: "curl -fsS http://localhost:3000/health"
    timeout: 30s
    interval: 1s`
}

func findConfigFile() string {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}

	possiblePaths := []string{
		".tusk/config.yaml",
		".tusk/config.yml",
		"tusk.yaml",
		"tusk.yml",
	}

	// Traverse upwards, starting from current directory
	currentDir := wd
	for {
		// Check all possible config paths in current directory
		for _, relPath := range possiblePaths {
			fullPath := filepath.Join(currentDir, relPath)
			if _, err := os.Stat(fullPath); err == nil {
				return fullPath
			}
		}

		parent := filepath.Dir(currentDir)

		if parent == currentDir || parent == "." {
			break
		}

		currentDir = parent
	}

	// Fall back to home directory as a last resort
	homeDir, err := os.UserHomeDir()
	if err == nil {
		globalConfig := filepath.Join(homeDir, ".tusk", "config.yaml")
		if _, err := os.Stat(globalConfig); err == nil {
			return globalConfig
		}
	}

	return ""
}

// Invalidate clears all cached config state, forcing a reload on next Get().
// Used when updating the config file and for testing.
func Invalidate() {
	loadMutex.Lock()
	defer loadMutex.Unlock()
	hasLoaded = false
	cachedConfig = nil
	cachedConfigErr = nil
	k = koanf.New(".")
}
