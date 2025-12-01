package onboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"gopkg.in/yaml.v3"
)

const (
	configDir  = ".tusk"
	configFile = "config.yaml"
)

type Config struct {
	Service       Service       `yaml:"service"`
	Traces        Traces        `yaml:"traces"`
	TestExecution TestExecution `yaml:"test_execution"`
	Recording     Recording     `yaml:"recording"`
	TuskAPI       *TuskAPI      `yaml:"tusk_api,omitempty"`
}

type Service struct {
	ID            string         `yaml:"id,omitempty"`
	Name          string         `yaml:"name"`
	Port          int            `yaml:"port"`
	Start         Start          `yaml:"start"`
	Stop          *Stop          `yaml:"stop,omitempty"`
	Communication *Communication `yaml:"communication,omitempty"`
	Readiness     Readiness      `yaml:"readiness_check"`
}

type Start struct {
	Command string `yaml:"command"`
}

type Stop struct {
	Command string `yaml:"command,omitempty"`
}

type Communication struct {
	Type    string `yaml:"type,omitempty"`
	TCPPort int    `yaml:"tcp_port,omitempty"`
}

type Readiness struct {
	Command  string `yaml:"command"`
	Timeout  string `yaml:"timeout"`
	Interval string `yaml:"interval"`
}

type TestExecution struct {
	Timeout string `yaml:"timeout"`
}

type Recording struct {
	SamplingRate          float64 `yaml:"sampling_rate"`
	ExportSpans           bool    `yaml:"export_spans"`
	EnableEnvVarRecording bool    `yaml:"enable_env_var_recording"`
}

type Traces struct {
	Dir string `yaml:"dir"`
}

type TuskAPI struct {
	URL string `yaml:"url"`
}

func (m *Model) getCurrentConfig() Config {
	timeout := strings.TrimSpace(m.ReadinessTimeout)
	if timeout == "" {
		timeout = "30s"
	}
	interval := strings.TrimSpace(m.ReadinessInterval)
	if interval == "" {
		interval = "1s"
	}

	samplingRate := 1.0
	if r, err := parseFloatSafe(strings.TrimSpace(m.SamplingRate)); err == nil {
		samplingRate = r
	}

	cfg := Config{
		Service: Service{
			Name: m.ServiceName,
			Port: m.currentPortInt(),
			Start: Start{
				Command: m.StartCmd,
			},
			Readiness: Readiness{
				Command:  m.ReadinessCmd,
				Timeout:  timeout,
				Interval: interval,
			},
		},
		Traces: Traces{
			Dir: ".tusk/traces",
		},
		TuskAPI: &TuskAPI{
			URL: api.DefaultBaseURL,
		},
		TestExecution: TestExecution{
			Timeout: "30s",
		},
		Recording: Recording{
			SamplingRate:          samplingRate,
			ExportSpans:           false,
			EnableEnvVarRecording: true,
		},
	}

	if m.UseDocker {
		stopCmd := m.StopCmd
		if strings.TrimSpace(stopCmd) == "" {
			stopCmd = m.stopCommandDefault()
		}
		cfg.Service.Stop = &Stop{
			Command: stopCmd,
		}
		cfg.Service.Communication = &Communication{
			Type:    "tcp",
			TCPPort: 9001,
		}
	}

	return cfg
}

func (m *Model) saveConfig() error {
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	cfgPath := filepath.Join(configDir, configFile)

	cfg := m.getCurrentConfig()
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("failed to close encoder: %w", err)
	}
	raw := []byte(buf.String())
	data := formatYAMLWithBlankLines(raw)

	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if m.DockerType == dockerTypeCompose {
		if err := m.createDockerComposeOverrideFile(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not create docker-compose override file: %v\n", err)
			fmt.Fprintf(os.Stderr, "You will need to manually create docker-compose.tusk-override.yml.\n")
			fmt.Fprintf(os.Stderr, "Refer to: %s\n", styles.LinkStyle.Render("https://github.com/Use-Tusk/tusk-drift-cli/blob/main/docs/configuration.md#docker-support"))
		}
	}
	return nil
}
