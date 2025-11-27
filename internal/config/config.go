package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/logging"
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
	URL string `koanf:"url"`
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
	EnableTelemetry *bool `koanf:"enable_telemetry"`
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
		slog.Debug("Config already loaded, skipping reload")
		return nil
	}

	if configFile == "" {
		configFile = findConfigFile()
	}

	if configFile != "" {
		if err := k.Load(file.Provider(configFile), yaml.Parser()); err != nil {
			logging.LogToService(fmt.Sprintf("Failed to load config file: %s. Error: %s", configFile, err))
			return fmt.Errorf("error loading config file: %w", err)
		}
		slog.Debug("Config file loaded", "file", configFile)
	} else {
		slog.Debug("No config file found, using defaults and environment variables")
	}

	// Support environment variable overrides for specific config keys
	envOverrides := map[string]string{
		"TUSK_TRACES_DIR":              "traces.dir",
		"TUSK_API_URL":                 "tusk_api.url",
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
	slog.Debug("All loaded config", "config", k.All())
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
		cfg.TestExecution.Concurrency = 5
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

	// Resolve directory paths relative to tusk root
	cfg.Results.Dir = utils.ResolveTuskPath(cfg.Results.Dir)
	cfg.Traces.Dir = utils.ResolveTuskPath(cfg.Traces.Dir)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("error validating config: %w", err)
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
			errs = append(errs, fmt.Errorf("test_execution.timeout invalid: %w", err))
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
		return fmt.Errorf("configuration validation failed: %w", errors.Join(errs...))
	}

	return nil
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
