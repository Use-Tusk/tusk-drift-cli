package onboard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestGetCurrentConfig_UsesAdaptiveSamplingMode(t *testing.T) {
	inputs := make([]textinput.Model, 1)
	inputs[0] = textinput.New()

	m := &Model{
		ServiceName:       "test-service",
		ServicePort:       "3000",
		StartCmd:          "npm start",
		ReadinessCmd:      "curl http://localhost:3000/health",
		ReadinessTimeout:  "30s",
		ReadinessInterval: "1s",
		SamplingRate:      "1.0",
		inputs:            inputs,
	}

	cfg := m.getCurrentConfig()

	assert.Equal(t, "adaptive", cfg.Recording.Sampling.Mode)
	assert.Equal(t, 1.0, cfg.Recording.Sampling.BaseRate)

	// Verify YAML output uses nested sampling config
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	require.NoError(t, enc.Encode(cfg))
	_ = enc.Close()
	yamlStr := buf.String()

	assert.Contains(t, yamlStr, "mode: adaptive")
	assert.Contains(t, yamlStr, "base_rate: 1")
	assert.NotContains(t, yamlStr, "sampling_rate:")
}

func TestGetCurrentConfig_CustomSamplingRate(t *testing.T) {
	inputs := make([]textinput.Model, 1)
	inputs[0] = textinput.New()

	m := &Model{
		ServiceName:       "test-service",
		ServicePort:       "8080",
		StartCmd:          "python app.py",
		ReadinessCmd:      "curl http://localhost:8080/health",
		ReadinessTimeout:  "30s",
		ReadinessInterval: "1s",
		SamplingRate:      "0.1",
		inputs:            inputs,
	}

	cfg := m.getCurrentConfig()

	assert.Equal(t, "adaptive", cfg.Recording.Sampling.Mode)
	assert.Equal(t, 0.1, cfg.Recording.Sampling.BaseRate)
}
