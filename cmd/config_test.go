package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/Use-Tusk/tusk-cli/internal/cliconfig"
	"github.com/stretchr/testify/require"
)

func TestConfigCmdHelp(t *testing.T) {
	// configCmd.Run calls cmd.Help(); verify it executes without panic
	configCmd.Run(configCmd, []string{})
}

func TestConfigGetCmd(t *testing.T) {
	origConfig := cliconfig.CLIConfig
	t.Cleanup(func() { cliconfig.CLIConfig = origConfig })

	t.Run("analytics", func(t *testing.T) {
		cliconfig.CLIConfig = &cliconfig.Config{AnalyticsEnabled: true}
		err := configGetCmd.RunE(configGetCmd, []string{"analytics"})
		require.NoError(t, err)
	})

	t.Run("darkmode nil", func(t *testing.T) {
		cliconfig.CLIConfig = &cliconfig.Config{}
		err := configGetCmd.RunE(configGetCmd, []string{"darkMode"})
		require.NoError(t, err)
	})

	t.Run("darkmode set", func(t *testing.T) {
		val := true
		cliconfig.CLIConfig = &cliconfig.Config{DarkMode: &val}
		err := configGetCmd.RunE(configGetCmd, []string{"darkMode"})
		require.NoError(t, err)
	})

	t.Run("autoupdate", func(t *testing.T) {
		cliconfig.CLIConfig = &cliconfig.Config{AutoUpdate: true}
		err := configGetCmd.RunE(configGetCmd, []string{"autoUpdate"})
		require.NoError(t, err)
	})

	t.Run("autocheckupdates nil", func(t *testing.T) {
		cliconfig.CLIConfig = &cliconfig.Config{}
		err := configGetCmd.RunE(configGetCmd, []string{"autoCheckUpdates"})
		require.NoError(t, err)
	})

	t.Run("autocheckupdates set", func(t *testing.T) {
		val := false
		cliconfig.CLIConfig = &cliconfig.Config{AutoCheckUpdates: &val}
		err := configGetCmd.RunE(configGetCmd, []string{"autoCheckUpdates"})
		require.NoError(t, err)
	})

	t.Run("unknown key returns error", func(t *testing.T) {
		cliconfig.CLIConfig = &cliconfig.Config{}
		err := configGetCmd.RunE(configGetCmd, []string{"unknownKey"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown config key")
	})
}

func TestConfigSetCmd(t *testing.T) {
	origConfig := cliconfig.CLIConfig
	t.Cleanup(func() { cliconfig.CLIConfig = origConfig })
	origHome := os.Getenv("HOME")

	// Sandbox all config resolution paths across OSes:
	// - Linux typically honors XDG_CONFIG_HOME
	// - macOS uses HOME/Library/Application Support via os.UserConfigDir
	// - Windows uses APPDATA/LOCALAPPDATA via os.UserConfigDir
	sandbox := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", sandbox)
	t.Setenv("HOME", sandbox)
	t.Setenv("APPDATA", sandbox)
	t.Setenv("LOCALAPPDATA", sandbox)

	cfgPath := cliconfig.GetPath()
	require.NotEmpty(t, cfgPath)
	require.True(t, strings.HasPrefix(cfgPath, sandbox))

	if origHome != "" {
		require.False(t, strings.HasPrefix(cfgPath, origHome))
	}

	t.Run("analytics true clears developer mode", func(t *testing.T) {
		cliconfig.CLIConfig = &cliconfig.Config{IsTuskDeveloper: true}
		err := configSetCmd.RunE(configSetCmd, []string{"analytics", "true"})
		require.NoError(t, err)
		require.True(t, cliconfig.CLIConfig.AnalyticsEnabled)
		require.False(t, cliconfig.CLIConfig.IsTuskDeveloper)
	})

	t.Run("analytics false", func(t *testing.T) {
		cliconfig.CLIConfig = &cliconfig.Config{AnalyticsEnabled: true}
		err := configSetCmd.RunE(configSetCmd, []string{"analytics", "false"})
		require.NoError(t, err)
		require.False(t, cliconfig.CLIConfig.AnalyticsEnabled)
	})

	t.Run("analytics invalid value", func(t *testing.T) {
		cliconfig.CLIConfig = &cliconfig.Config{}
		err := configSetCmd.RunE(configSetCmd, []string{"analytics", "maybe"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid value for analytics")
	})

	t.Run("darkmode true", func(t *testing.T) {
		cliconfig.CLIConfig = &cliconfig.Config{}
		err := configSetCmd.RunE(configSetCmd, []string{"darkMode", "true"})
		require.NoError(t, err)
		require.NotNil(t, cliconfig.CLIConfig.DarkMode)
		require.True(t, *cliconfig.CLIConfig.DarkMode)
	})

	t.Run("darkmode invalid value", func(t *testing.T) {
		cliconfig.CLIConfig = &cliconfig.Config{}
		err := configSetCmd.RunE(configSetCmd, []string{"darkMode", "maybe"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid value for darkMode")
	})

	t.Run("autoupdate true", func(t *testing.T) {
		cliconfig.CLIConfig = &cliconfig.Config{}
		err := configSetCmd.RunE(configSetCmd, []string{"autoUpdate", "true"})
		require.NoError(t, err)
		require.True(t, cliconfig.CLIConfig.AutoUpdate)
	})

	t.Run("autoupdate invalid value", func(t *testing.T) {
		cliconfig.CLIConfig = &cliconfig.Config{}
		err := configSetCmd.RunE(configSetCmd, []string{"autoUpdate", "maybe"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid value for autoUpdate")
	})

	t.Run("autocheckupdates false", func(t *testing.T) {
		cliconfig.CLIConfig = &cliconfig.Config{}
		err := configSetCmd.RunE(configSetCmd, []string{"autoCheckUpdates", "false"})
		require.NoError(t, err)
		require.NotNil(t, cliconfig.CLIConfig.AutoCheckUpdates)
		require.False(t, *cliconfig.CLIConfig.AutoCheckUpdates)
	})

	t.Run("autocheckupdates invalid value", func(t *testing.T) {
		cliconfig.CLIConfig = &cliconfig.Config{}
		err := configSetCmd.RunE(configSetCmd, []string{"autoCheckUpdates", "maybe"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid value for autoCheckUpdates")
	})

	t.Run("unknown key", func(t *testing.T) {
		cliconfig.CLIConfig = &cliconfig.Config{}
		err := configSetCmd.RunE(configSetCmd, []string{"unknownKey", "true"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown config key")
	})
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    bool
		expectError bool
	}{
		// True values
		{name: "true", input: "true", expected: true},
		{name: "TRUE uppercase", input: "TRUE", expected: true},
		{name: "True mixed case", input: "True", expected: true},
		{name: "1", input: "1", expected: true},
		{name: "yes", input: "yes", expected: true},
		{name: "YES uppercase", input: "YES", expected: true},
		{name: "on", input: "on", expected: true},
		{name: "ON uppercase", input: "ON", expected: true},
		// False values
		{name: "false", input: "false", expected: false},
		{name: "FALSE uppercase", input: "FALSE", expected: false},
		{name: "False mixed case", input: "False", expected: false},
		{name: "0", input: "0", expected: false},
		{name: "no", input: "no", expected: false},
		{name: "NO uppercase", input: "NO", expected: false},
		{name: "off", input: "off", expected: false},
		{name: "OFF uppercase", input: "OFF", expected: false},
		// Invalid values
		{name: "empty string", input: "", expectError: true},
		{name: "random string", input: "maybe", expectError: true},
		{name: "2", input: "2", expectError: true},
		{name: "tru", input: "tru", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBool(tt.input)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, got)
			}
		})
	}
}
