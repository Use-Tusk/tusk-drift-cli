package analytics

import (
	"testing"

	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
)

func TestBackfillJWTIdentity_ShortCircuitsWhenUserIDSet(t *testing.T) {
	// Create a client with a config that already has a UserID set.
	// backfillJWTIdentity should return immediately without making any
	// network calls (verified by the fact that no auth/api setup is needed).
	cfg := &cliconfig.Config{
		UserID: "existing-user-id",
	}
	client := &Client{
		config: cfg,
	}

	// This should not panic or make any network calls — it short-circuits
	// because config.UserID is already set.
	client.backfillJWTIdentity()
}

func TestBackfillJWTIdentity_ShortCircuitsWithNilConfig(t *testing.T) {
	client := &Client{
		config: nil,
	}

	// Should not panic when config is nil — the UserID check handles it.
	client.backfillJWTIdentity()
}
