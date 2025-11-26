package cliconfig

import (
	"os"
	"strings"
	"testing"
)

const anonymousIDPrefix = "cli-anon-"
const uuidV4Length = 36 // Standard UUID v4 format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx

func TestGenerateAnonymousID(t *testing.T) {
	id := generateAnonymousID()
	if !strings.HasPrefix(id, anonymousIDPrefix) {
		t.Errorf("anonymous ID should have prefix %q, got %s", anonymousIDPrefix, id)
	}
	expectedLen := len(anonymousIDPrefix) + uuidV4Length
	if len(id) != expectedLen {
		t.Errorf("anonymous ID should be %d chars (prefix %d + UUID %d), got %d", expectedLen, len(anonymousIDPrefix), uuidV4Length, len(id))
	}
}

func TestConfigLoadAndSave(t *testing.T) {
	// Verify GetPath returns something on a normal system
	path := GetPath()
	if path == "" {
		t.Error("GetPath() returned empty string")
	}

	// Create a config manually for testing
	cfg := &Config{
		AnonymousID:        "cli-anon-test-uuid",
		AnalyticsEnabled:   true,
		IsTuskDeveloper:    false,
		NoticeShown:        false,
		UserID:             "user123",
		UserName:           "Test User",
		UserEmail:          "test@example.com",
		SelectedClientID:   "client456",
		SelectedClientName: "Test Corp",
	}

	// Test GetDistinctID
	if cfg.GetDistinctID() != "user123" {
		t.Errorf("GetDistinctID() should return UserID when set, got %s", cfg.GetDistinctID())
	}

	cfg.UserID = ""
	if cfg.GetDistinctID() != "cli-anon-test-uuid" {
		t.Errorf("GetDistinctID() should return AnonymousID when UserID empty, got %s", cfg.GetDistinctID())
	}
}

func TestConfigSetAndClearAuthInfo(t *testing.T) {
	cfg := &Config{
		AnonymousID:      "cli-anon-test",
		AnalyticsEnabled: true,
	}

	// Set auth info
	cfg.SetAuthInfo("user123", "Test User", "test@example.com", "client456", "Test Corp")

	if cfg.UserID != "user123" {
		t.Errorf("UserID should be 'user123', got %s", cfg.UserID)
	}
	if cfg.UserName != "Test User" {
		t.Errorf("UserName should be 'Test User', got %s", cfg.UserName)
	}
	if cfg.UserEmail != "test@example.com" {
		t.Errorf("UserEmail should be 'test@example.com', got %s", cfg.UserEmail)
	}
	if cfg.SelectedClientID != "client456" {
		t.Errorf("SelectedClientID should be 'client456', got %s", cfg.SelectedClientID)
	}
	if cfg.SelectedClientName != "Test Corp" {
		t.Errorf("SelectedClientName should be 'Test Corp', got %s", cfg.SelectedClientName)
	}

	// Mark as aliased
	cfg.MarkAliased("user123")
	if cfg.AliasedToUserID != "user123" {
		t.Errorf("AliasedToUserID should be 'user123', got %s", cfg.AliasedToUserID)
	}

	// Clear auth info
	cfg.ClearAuthInfo()

	if cfg.UserID != "" {
		t.Errorf("UserID should be empty after clear, got %s", cfg.UserID)
	}
	if cfg.UserName != "" {
		t.Errorf("UserName should be empty after clear, got %s", cfg.UserName)
	}
	if cfg.UserEmail != "" {
		t.Errorf("UserEmail should be empty after clear, got %s", cfg.UserEmail)
	}
	// SelectedClientID should NOT be cleared (preserved for next login)
	if cfg.SelectedClientID != "client456" {
		t.Errorf("SelectedClientID should NOT be cleared, got %s", cfg.SelectedClientID)
	}
	if cfg.SelectedClientName != "Test Corp" {
		t.Errorf("SelectedClientName should NOT be cleared, got %s", cfg.SelectedClientName)
	}
	// AliasedToUserID should NOT be cleared
	if cfg.AliasedToUserID != "user123" {
		t.Errorf("AliasedToUserID should NOT be cleared, got %s", cfg.AliasedToUserID)
	}
	// AnonymousID should NOT be cleared
	if cfg.AnonymousID != "cli-anon-test" {
		t.Errorf("AnonymousID should NOT be cleared, got %s", cfg.AnonymousID)
	}
}

func TestGetClientID(t *testing.T) {
	cfg := &Config{
		SelectedClientID: "config-client",
	}

	// Without env var, should return config value
	if cfg.GetClientID() != "config-client" {
		t.Errorf("GetClientID() should return config value, got %s", cfg.GetClientID())
	}

	// With env var, should return env var
	t.Setenv(EnvClientID, "env-client")
	if cfg.GetClientID() != "env-client" {
		t.Errorf("GetClientID() should return env var when set, got %s", cfg.GetClientID())
	}
}

func TestNeedsAlias(t *testing.T) {
	cfg := &Config{}

	// Should need alias when user ID provided and not yet aliased
	if !cfg.NeedsAlias("user123") {
		t.Error("NeedsAlias() should return true for new user")
	}

	// Mark aliased
	cfg.MarkAliased("user123")

	// Should NOT need alias for same user
	if cfg.NeedsAlias("user123") {
		t.Error("NeedsAlias() should return false for already aliased user")
	}

	// Should need alias for different user
	if !cfg.NeedsAlias("user456") {
		t.Error("NeedsAlias() should return true for different user")
	}

	// Should NOT need alias for empty user ID
	if cfg.NeedsAlias("") {
		t.Error("NeedsAlias() should return false for empty user ID")
	}
}

func TestIsAnalyticsEnabled(t *testing.T) {
	// Test env var override
	t.Setenv(EnvAnalyticsDisabled, "1")
	if IsAnalyticsEnabled() {
		t.Error("IsAnalyticsEnabled() should return false when TUSK_ANALYTICS_DISABLED=1")
	}
}

func TestIsAnalyticsEnabledInCI(t *testing.T) {
	// Set CI=true, analytics should be disabled
	t.Setenv(EnvCI, "true")
	if IsAnalyticsEnabled() {
		t.Error("IsAnalyticsEnabled() should return false in CI environment")
	}
}

func TestIsCI(t *testing.T) {
	// Clear env var first
	os.Unsetenv(EnvCI)

	if IsCI() {
		t.Error("IsCI() should return false when CI env var not set")
	}

	t.Setenv(EnvCI, "true")
	if !IsCI() {
		t.Error("IsCI() should return true when CI=true")
	}
}
