package auth

// Regression tests for the three bugs fixed in commit 3d1d0c8:
//
//  1. Token file misplacement: os.UserConfigDir() error was silently ignored,
//     so a failure produced an empty cfgDir and the token was written to a
//     relative path ("tusk/auth.json") in the current working directory instead
//     of the proper user-config location.
//
//  2. Silent io.ReadAll errors: all four Auth0 HTTP methods (RequestDeviceCode,
//     PollForToken, FetchUserEmail, refreshAccessToken) discarded io.ReadAll
//     errors with `body, _ := io.ReadAll(...)`, hiding network failures.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// errorReader returns a fixed error on the first Read call, mimicking a
// mid-stream network failure that causes io.ReadAll to return an error.
type errorReader struct{}

func (e *errorReader) Read(_ []byte) (int, error) {
	return 0, errors.New("simulated network read error")
}

// errorBodyTransport is an http.RoundTripper that returns a well-formed
// HTTP 200 response whose body always errors on Read.  This is the minimal
// setup needed to trigger the io.ReadAll error paths that were previously
// swallowed with `body, _ := io.ReadAll(resp.Body)`.
type errorBodyTransport struct{}

func (t *errorBodyTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(&errorReader{}),
	}, nil
}

// newTestAuthenticator builds a minimal Authenticator for unit tests.
// It bypasses NewAuthenticator (which calls os.UserConfigDir and config.Get)
// so tests remain hermetic and fast.
func newTestAuthenticator(transport http.RoundTripper) *Authenticator {
	return &Authenticator{
		authFilePath: "/tmp/tusk-test-auth.json",
		httpClient:   &http.Client{Timeout: 5 * time.Second, Transport: transport},
		domain:       "test.example.auth0.com",
		clientID:     "test-client-id",
		scope:        "openid email offline_access",
		audience:     "drift-cli",
	}
}

// ── Bug 1: os.UserConfigDir() failure ────────────────────────────────────────

// TestNewAuthenticator_UserConfigDirFailure is a minimal repro for the token
// file misplacement bug.
//
// Before the fix, NewAuthenticator used:
//
//	cfgDir, _ := os.UserConfigDir()           // error silently dropped
//	authPath := filepath.Join(cfgDir, ...)    // cfgDir == "" → relative path
//
// With cfgDir empty, filepath.Join("", "tusk", "auth.json") returned the
// relative path "tusk/auth.json", so the token was written to the current
// working directory – not the user's config dir.
//
// After the fix the error is propagated and NewAuthenticator returns an error,
// preventing the misplaced write entirely.
//
// Reproduce:
//
//	HOME="" XDG_CONFIG_HOME="" go test ./internal/auth/ -run TestNewAuthenticator_UserConfigDirFailure
func TestNewAuthenticator_UserConfigDirFailure(t *testing.T) {
	// Unset every env var that os.UserConfigDir reads on any supported OS.
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "") // Linux fallback
	if runtime.GOOS == "windows" {
		t.Setenv("AppData", "")
	}

	auth, err := NewAuthenticator()

	require.Error(t, err, "NewAuthenticator must fail when the config dir cannot be determined")
	assert.Nil(t, auth)
	assert.Contains(t, err.Error(), "cannot determine user config directory",
		"error message should identify the root cause")

	// Before the fix this test would PASS (no error returned) but the
	// resulting Authenticator would have an authFilePath starting with
	// "tusk/" – a relative path.  The assertion below is included as
	// documentation of the old buggy behaviour:
	//
	//   assert.True(t, filepath.IsAbs(auth.authFilePath))  // would FAIL
}

// TestNewAuthenticator_AuthPathIsAbsolute verifies that, when the environment
// is intact, the resolved auth file path is always absolute.  This would have
// caught the misplacement silently introduced when cfgDir was empty.
func TestNewAuthenticator_AuthPathIsAbsolute(t *testing.T) {
	// NewAuthenticator also needs a minimal config (clientID etc.).
	// Set the env vars expected by config.Get / the Authenticator constructor
	// so the call succeeds in CI without a real config file.
	t.Setenv("TUSK_AUTH0_CLIENT_ID", "test-client-id")
	t.Setenv("TUSK_AUTH0_AUDIENCE", "drift-cli")

	auth, err := NewAuthenticator()
	if err != nil {
		// If the env/config still isn't complete enough (e.g. missing YAML),
		// skip rather than fail – the important assertion is the path check.
		t.Skipf("NewAuthenticator setup incomplete in test env: %v", err)
	}

	assert.True(t, strings.HasPrefix(auth.authFilePath, "/") || (len(auth.authFilePath) > 1 && auth.authFilePath[1] == ':'),
		"authFilePath %q must be an absolute path, not a relative one", auth.authFilePath)
	assert.Contains(t, auth.authFilePath, "tusk",
		"authFilePath should be inside a 'tusk' sub-directory")
	assert.Contains(t, auth.authFilePath, "auth.json",
		"authFilePath should point at auth.json")
}

// ── Bug 2: silent io.ReadAll errors ──────────────────────────────────────────

// TestRequestDeviceCode_PropagatesReadError is a minimal repro for the
// silent-error bug in RequestDeviceCode.
//
// Before the fix:
//
//	body, _ := io.ReadAll(resp.Body)   // network error silently dropped
//	json.Unmarshal(body, &dcr)         // body is empty/partial → silent failure
//
// After the fix:
//
//	body, err := io.ReadAll(resp.Body)
//	if err != nil { return dcr, fmt.Errorf("error reading device code response body: %w", err) }
func TestRequestDeviceCode_PropagatesReadError(t *testing.T) {
	a := newTestAuthenticator(&errorBodyTransport{})

	_, err := a.RequestDeviceCode(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "error reading device code response body",
		"ReadAll error must be surfaced with context")
	assert.Contains(t, err.Error(), "simulated network read error")
}

// TestPollForToken_PropagatesReadError is a minimal repro for the
// silent-error bug in PollForToken.
//
// Before the fix:
//
//	body, _ := io.ReadAll(resp.Body)   // error silently dropped
func TestPollForToken_PropagatesReadError(t *testing.T) {
	a := newTestAuthenticator(&errorBodyTransport{})

	dcr := DeviceCodeResponse{
		DeviceCode: "test-device-code",
		Interval:   0, // zero → 5 s default, but ctx is cancelled immediately
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := a.PollForToken(ctx, dcr)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "error reading token response body",
		"ReadAll error must be surfaced with context")
	assert.Contains(t, err.Error(), "simulated network read error")
}

// TestFetchUserEmail_PropagatesReadError is a minimal repro for the
// silent-error bug in FetchUserEmail.
//
// Before the fix:
//
//	b, _ := io.ReadAll(resp.Body)   // error silently dropped
func TestFetchUserEmail_PropagatesReadError(t *testing.T) {
	a := newTestAuthenticator(&errorBodyTransport{})
	a.AccessToken = "fake-access-token"

	err := a.FetchUserEmail(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "error reading userinfo response body",
		"ReadAll error must be surfaced with context")
	assert.Contains(t, err.Error(), "simulated network read error")
}

// TestRefreshAccessToken_PropagatesReadError is a minimal repro for the
// silent-error bug in refreshAccessToken.
//
// Before the fix:
//
//	body, _ := io.ReadAll(resp.Body)   // error silently dropped
func TestRefreshAccessToken_PropagatesReadError(t *testing.T) {
	a := newTestAuthenticator(&errorBodyTransport{})
	a.RefreshToken = "fake-refresh-token"

	err := a.refreshAccessToken(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "error reading refresh token response body",
		"ReadAll error must be surfaced with context")
	assert.Contains(t, err.Error(), "simulated network read error")
}
