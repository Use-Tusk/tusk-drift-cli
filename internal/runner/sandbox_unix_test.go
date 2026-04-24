//go:build darwin || linux || freebsd

package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateReplayFenceConfigMergesCustomConfig(t *testing.T) {
	customConfigPath := filepath.Join(t.TempDir(), "replay.fence.json")
	err := os.WriteFile(customConfigPath, []byte(`{
  "network": {
    "allowedDomains": ["api.example.com"]
  },
  "filesystem": {
    "allowWrite": ["custom-cache"]
  }
}`), 0o600)
	require.NoError(t, err)

	cfg, err := createReplayFenceConfig(customConfigPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.Network.AllowLocalOutbound)

	assert.Contains(t, cfg.Network.AllowedDomains, "localhost")
	assert.Contains(t, cfg.Network.AllowedDomains, "127.0.0.1")
	assert.Contains(t, cfg.Network.AllowedDomains, "api.example.com")
	assert.True(t, cfg.Network.AllowLocalBinding)
	assert.False(t, *cfg.Network.AllowLocalOutbound)
	assert.True(t, cfg.Network.AllowAllUnixSockets)
	assert.Contains(t, cfg.Filesystem.AllowWrite, "custom-cache")
	assert.Contains(t, cfg.Filesystem.AllowWrite, "/")
}

func TestCreateReplayFenceConfigRejectsDeniedLocalhost(t *testing.T) {
	customConfigPath := filepath.Join(t.TempDir(), "replay.fence.json")
	err := os.WriteFile(customConfigPath, []byte(`{
  "network": {
    "deniedDomains": ["localhost"]
  }
}`), 0o600)
	require.NoError(t, err)

	_, err = createReplayFenceConfig(customConfigPath)
	require.ErrorContains(t, err, `cannot deny "localhost"`)
}
