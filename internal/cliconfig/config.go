package cliconfig

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

const (
	// EnvAnalyticsDisabled is the environment variable to disable analytics
	EnvAnalyticsDisabled = "TUSK_ANALYTICS_DISABLED"
	// EnvClientID is the environment variable to override client ID
	EnvClientID = "TUSK_CLIENT_ID"
	// EnvCI indicates CI environment (skip notice)
	EnvCI = "CI"
)

// Config represents the user-level CLI configuration stored at ~/.config/tusk/cli.json
type Config struct {
	// Analytics settings
	AnonymousID      string `json:"anonymous_id"`      // "cli-anon-<uuid>" generated on first run
	AnalyticsEnabled bool   `json:"analytics_enabled"` // Default true
	IsTuskDeveloper  bool   `json:"is_tusk_developer"` // For Tusk employees
	NoticeShown      bool   `json:"notice_shown"`      // First-run notice displayed

	// Cached auth info (updated on login/logout, avoids backend calls)
	UserID             string `json:"user_id,omitempty"`              // From authInfo.User.Id
	UserName           string `json:"user_name,omitempty"`            // From authInfo.User.Name
	UserEmail          string `json:"user_email,omitempty"`           // From authInfo.User.Email
	SelectedClientID   string `json:"selected_client_id,omitempty"`   // User's chosen client ID
	SelectedClientName string `json:"selected_client_name,omitempty"` // User's chosen client name
	AliasedToUserID    string `json:"aliased_to_user_id,omitempty"`   // Prevent re-aliasing
}

// GetPath returns the path to the CLI config file.
// Returns empty string if no suitable config directory can be determined.
func GetPath() string {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		cfgDir = os.Getenv("HOME")
		if cfgDir == "" {
			return ""
		}
		// Use ~/.config/tusk when falling back to HOME
		cfgDir = filepath.Join(cfgDir, ".config")
	}
	return filepath.Join(cfgDir, "tusk", "cli.json")
}

// Load loads the CLI config from disk, creating defaults if it doesn't exist
func Load() (*Config, error) {
	path := GetPath()
	if path == "" {
		// No config directory available, return in-memory defaults
		return &Config{
			AnonymousID:      generateAnonymousID(),
			AnalyticsEnabled: true,
		}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Create default config and save to disk so anonymous ID persists
			cfg := &Config{
				AnonymousID:      generateAnonymousID(),
				AnalyticsEnabled: true,
			}
			_ = cfg.Save() // Best effort, ignore errors
			return cfg, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Ensure anonymous ID exists (migration for old configs)
	if cfg.AnonymousID == "" {
		cfg.AnonymousID = generateAnonymousID()
	}

	return &cfg, nil
}

// Save persists the config to disk
func (c *Config) Save() error {
	path := GetPath()
	if path == "" {
		// No config directory available, nothing to save
		return nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o600)
}

// IsAnalyticsEnabled checks if analytics is enabled (env var takes precedence)
func IsAnalyticsEnabled() bool {
	// Environment variable takes highest priority
	if os.Getenv(EnvAnalyticsDisabled) != "" {
		return false
	}

	// Skip analytics in CI environments
	if IsCI() {
		return false
	}

	cfg, err := Load()
	if err != nil {
		// If we can't load config, default to enabled
		return true
	}

	// Developer mode disables analytics
	if cfg.IsTuskDeveloper {
		return false
	}

	return cfg.AnalyticsEnabled
}

// IsCI returns true if running in a CI environment
func IsCI() bool {
	return os.Getenv(EnvCI) != ""
}

// IsAnalyticsDisabledByEnv returns true if analytics is disabled via environment variable
func IsAnalyticsDisabledByEnv() bool {
	return os.Getenv(EnvAnalyticsDisabled) != ""
}

// SetAuthInfo caches auth info from login
func (c *Config) SetAuthInfo(userID, userName, userEmail, selectedClientID, selectedClientName string) {
	c.UserID = userID
	c.UserName = userName
	c.UserEmail = userEmail
	c.SelectedClientID = selectedClientID
	c.SelectedClientName = selectedClientName
}

// ClearAuthInfo clears cached auth info on logout (keeps anonymous_id and client selection)
func (c *Config) ClearAuthInfo() {
	c.UserID = ""
	c.UserName = ""
	c.UserEmail = ""
	// Keep SelectedClientID and SelectedClientName - will be validated on next login
	// Don't clear AliasedToUserID - we want to remember we already aliased
}

// GetDistinctID returns the distinct ID for PostHog (UserID if logged in, else AnonymousID)
func (c *Config) GetDistinctID() string {
	if c.UserID != "" {
		return c.UserID
	}
	return c.AnonymousID
}

// GetClientID returns the client ID (env var takes precedence, then selected client)
func (c *Config) GetClientID() string {
	if envClientID := os.Getenv(EnvClientID); envClientID != "" {
		return envClientID
	}
	return c.SelectedClientID
}

// NeedsAlias returns true if we should alias anonymous ID to user ID
func (c *Config) NeedsAlias(userID string) bool {
	return userID != "" && c.AliasedToUserID != userID
}

// MarkAliased records that we've aliased to this user ID
func (c *Config) MarkAliased(userID string) {
	c.AliasedToUserID = userID
}

// generateAnonymousID creates a new anonymous ID with the cli-anon- prefix
func generateAnonymousID() string {
	return "cli-anon-" + uuid.New().String()
}
