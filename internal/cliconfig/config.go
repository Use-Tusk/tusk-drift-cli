package cliconfig

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const (
	// EnvAnalyticsDisabled is the environment variable to disable analytics
	EnvAnalyticsDisabled = "TUSK_ANALYTICS_DISABLED"
	// EnvClientID is the environment variable to override client ID
	EnvClientID = "TUSK_CLIENT_ID"
	// EnvCI indicates CI environment (skip notice)
	EnvCI = "CI"
	// EnvAuthMethod is the environment variable to force a specific auth method
	EnvAuthMethod = "TUSK_AUTH_METHOD"
	// EnvAPIKey is the environment variable for API key authentication
	EnvAPIKey = "TUSK_API_KEY" // #nosec G101 -- This is an env var name, not a credential
)

// GetAPIKey returns the API key from the environment variable
func GetAPIKey() string {
	return os.Getenv(EnvAPIKey)
}

// AuthMethod represents the authentication method being used
type AuthMethod string

const (
	AuthMethodNone   AuthMethod = "none"
	AuthMethodJWT    AuthMethod = "jwt"
	AuthMethodAPIKey AuthMethod = "api_key"
)

var CLIConfig *Config

// Config represents the user-level CLI configuration stored at ~/.config/tusk/cli.json
type Config struct {
	// User settings (configurable via `tusk config`)
	AnalyticsEnabled bool  `json:"analytics_enabled"`  // Default true, enable usage analytics
	DarkMode         *bool `json:"dark_mode"`          // nil = auto-detect, true/false = forced
	AutoUpdate       bool  `json:"auto_update"`        // Whether to auto-update without prompting
	AutoCheckUpdates *bool `json:"auto_check_updates"` // Whether to check for updates on startup

	// Analytics internals
	AnonymousID     string `json:"anonymous_id"`      // "cli-anon-<uuid>" generated on first run
	IsTuskDeveloper bool   `json:"is_tusk_developer"` // For Tusk employees
	NoticeShown     bool   `json:"notice_shown"`      // First-run notice displayed

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

func defaultCfg() *Config {
	return &Config{
		AnonymousID:      generateAnonymousID(),
		AnalyticsEnabled: true,
		DarkMode:         nil,
		AutoUpdate:       true,
	}
}

func init() {
	CLIConfig = defaultCfg()

	path := GetPath()
	if path == "" {
		return
	}

	data, err := os.ReadFile(path) //#nosec G304 -- path is from trusted source (UserConfigDir)
	if err != nil {
		if os.IsNotExist(err) {
			_ = CLIConfig.Save()
			return
		}
		slog.Debug("failed to read CLI config, using defaults", "path", path, "error", err)
		return
	}

	// json.Unmarshal doesn't affect fields that are not in the JSON
	// https://go.dev/play/p/p9rCScN8GcR
	if err := json.Unmarshal(data, CLIConfig); err != nil {
		_ = CLIConfig.Save()
		return
	}
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

	// Developer mode disables analytics
	if CLIConfig.IsTuskDeveloper {
		return false
	}

	return CLIConfig.AnalyticsEnabled
}

// IsCI returns true if running in a CI environment
func IsCI() bool {
	return os.Getenv(EnvCI) != ""
}

// IsAnalyticsDisabledByEnv returns true if analytics is disabled via environment variable
func IsAnalyticsDisabledByEnv() bool {
	return os.Getenv(EnvAnalyticsDisabled) != ""
}

// GetAuthMethod returns the effective auth method based on TUSK_AUTH_METHOD env var
// and available credentials. Returns the raw env var value (normalized) and effective method.
// hasJWT should be true if the user has a valid JWT token (checked via authenticator.TryExistingAuth).
func GetAuthMethod(hasJWT bool) (envValue string, effective AuthMethod) {
	hasAPIKey := GetAPIKey() != ""
	envValue = strings.ToLower(os.Getenv(EnvAuthMethod))
	if envValue == "" {
		envValue = "auto"
	}

	switch envValue {
	case "api_key", "api-key", "apikey":
		envValue = "api_key" // normalize
		if hasAPIKey {
			return envValue, AuthMethodAPIKey
		}
	case "auth0", "jwt":
		envValue = "jwt" // normalize
		if hasJWT {
			return envValue, AuthMethodJWT
		}
	default: // "auto" or empty - prefer JWT over API key
		if hasJWT {
			return envValue, AuthMethodJWT
		}
		if hasAPIKey {
			return envValue, AuthMethodAPIKey
		}
	}
	return envValue, AuthMethodNone
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

// ClientIDSource indicates where the client ID came from
type ClientIDSource string

const (
	ClientIDSourceNone     ClientIDSource = ""
	ClientIDSourceEnvVar   ClientIDSource = "TUSK_CLIENT_ID env var"
	ClientIDSourceSelected ClientIDSource = "selected from login"
)

// GetClientID returns the client ID (env var takes precedence, then selected client)
func (c *Config) GetClientID() string {
	id, _ := c.GetClientIDWithSource()
	return id
}

// GetClientIDWithSource returns the client ID and its source
// Note that if auth method is API key, we don't know the client ID and so return empty string.
func (c *Config) GetClientIDWithSource() (clientID string, source ClientIDSource) {
	if envClientID := os.Getenv(EnvClientID); envClientID != "" {
		return envClientID, ClientIDSourceEnvVar
	}
	if c.SelectedClientID != "" {
		return c.SelectedClientID, ClientIDSourceSelected
	}
	return "", ClientIDSourceNone
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
