package onboardcloud

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// setupTestEnvironment creates a temporary directory with config files and git repo for testing
func setupTestEnvironment(t *testing.T, apiURL string) string {
	t.Helper()

	// Reset config cache to avoid cross-test contamination
	config.Invalidate()

	tmpDir, err := os.MkdirTemp("", "tusk-test-*")
	require.NoError(t, err)

	// Initialize git repo (needed for createObservableService which calls getAppDir)
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	_ = cmd.Run()

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	_ = cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	_ = cmd.Run()

	tuskDir := filepath.Join(tmpDir, ".tusk")
	err = os.MkdirAll(tuskDir, 0o750)
	require.NoError(t, err)

	configContent := fmt.Sprintf(`tusk_api:
  url: %s
service:
  id: test-service-id
recording:
  sampling_rate: 1.0
`, apiURL)

	err = os.WriteFile(filepath.Join(tuskDir, "config.yaml"), []byte(configContent), 0o600)
	require.NoError(t, err)

	originalWd, err := os.Getwd()
	require.NoError(t, err)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
		_ = os.RemoveAll(tmpDir)
		config.Invalidate()
	})

	return tmpDir
}

// setupTestAuth configures API key authentication for tests.
//
// Why API keys instead of JWT bearer tokens?
//   - JWT tokens are stored at platform-specific paths (~/Library/Application Support on macOS,
//     ~/.config on Linux), and we can't reliably override these paths in tests without risking
//     interference with real user credentials.
//   - API key auth exercises the same API client code paths as JWT auth, just with different headers.
//   - In production, users authenticate via `tusk auth login` (JWT), but for unit test isolation and
//     simplicity, API key mode is more appropriate.
func setupTestAuth(t *testing.T) {
	t.Helper()

	t.Setenv("TUSK_API_KEY", "test-api-key")
	t.Setenv("TUSK_AUTH_METHOD", "api_key")
}

func TestVerifyRepoAccess_Success(t *testing.T) {
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		assert.Equal(t, "/api/drift/client_service/verify_repo_access", r.URL.Path)
		assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))

		w.Header().Set("Content-Type", "application/protobuf")
		resp := &backend.VerifyRepoAccessResponse{
			Response: &backend.VerifyRepoAccessResponse_Success{
				Success: &backend.VerifyRepoAccessResponseSuccess{
					RepoId: 12345,
				},
			},
		}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	setupTestEnvironment(t, server.URL)
	setupTestAuth(t)

	model := &Model{
		GitRepoOwner: "test-owner",
		GitRepoName:  "test-repo",
	}

	cmd := verifyRepoAccess(model)
	msg := cmd()

	assert.True(t, serverCalled, "Server should have been called")
	successMsg, ok := msg.(verifyRepoAccessSuccessMsg)
	assert.True(t, ok, "Expected verifyRepoAccessSuccessMsg, got %T: %+v", msg, msg)
	if ok {
		assert.Equal(t, int64(12345), successMsg.repoID)
	}
}

func TestVerifyRepoAccess_NoCodeHostingResource(t *testing.T) {
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		w.Header().Set("Content-Type", "application/protobuf")
		resp := &backend.VerifyRepoAccessResponse{
			Response: &backend.VerifyRepoAccessResponse_Error{
				Error: &backend.VerifyRepoAccessResponseError{
					Code:    backend.VerifyRepoAccessResponseErrorCode_VERIFY_REPO_ACCESS_RESPONSE_ERROR_CODE_NO_CODE_HOSTING_RESOURCE,
					Message: "No code hosting resource found",
				},
			},
		}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	setupTestEnvironment(t, server.URL)
	setupTestAuth(t)

	model := &Model{
		GitRepoOwner: "test-owner",
		GitRepoName:  "test-repo",
	}

	cmd := verifyRepoAccess(model)
	msg := cmd()

	assert.True(t, serverCalled, "Server should have been called")
	errMsg, ok := msg.(verifyRepoAccessErrorMsg)
	assert.True(t, ok, "Expected verifyRepoAccessErrorMsg, got %T: %+v", msg, msg)
	if ok {
		assert.Contains(t, errMsg.err.Error(), "no GitHub/GitLab connection found")
	}
}

func TestVerifyRepoAccess_RepoNotFound(t *testing.T) {
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		w.Header().Set("Content-Type", "application/protobuf")
		resp := &backend.VerifyRepoAccessResponse{
			Response: &backend.VerifyRepoAccessResponse_Error{
				Error: &backend.VerifyRepoAccessResponseError{
					Code:    backend.VerifyRepoAccessResponseErrorCode_VERIFY_REPO_ACCESS_RESPONSE_ERROR_CODE_REPO_NOT_FOUND,
					Message: "Repository not found",
				},
			},
		}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	setupTestEnvironment(t, server.URL)
	setupTestAuth(t)

	model := &Model{
		GitRepoOwner: "test-owner",
		GitRepoName:  "test-repo",
	}

	cmd := verifyRepoAccess(model)
	msg := cmd()

	assert.True(t, serverCalled, "Server should have been called")
	errMsg, ok := msg.(verifyRepoAccessErrorMsg)
	assert.True(t, ok, "Expected verifyRepoAccessErrorMsg, got %T: %+v", msg, msg)
	if ok {
		assert.Contains(t, errMsg.err.Error(), "repository test-owner/test-repo not found")
	}
}

func TestVerifyRepoAccess_InvalidResponse(t *testing.T) {
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		w.Header().Set("Content-Type", "application/protobuf")
		// Send empty response
		resp := &backend.VerifyRepoAccessResponse{}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	setupTestEnvironment(t, server.URL)
	setupTestAuth(t)

	model := &Model{
		GitRepoOwner: "test-owner",
		GitRepoName:  "test-repo",
	}

	cmd := verifyRepoAccess(model)
	msg := cmd()

	assert.True(t, serverCalled, "Server should have been called")
	errMsg, ok := msg.(verifyRepoAccessErrorMsg)
	assert.True(t, ok, "Expected verifyRepoAccessErrorMsg, got %T: %+v", msg, msg)
	if ok {
		assert.Contains(t, errMsg.err.Error(), "invalid response")
	}
}

func TestCreateObservableService_Success(t *testing.T) {
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		assert.Equal(t, "/api/drift/client_service/create_observable_service", r.URL.Path)

		w.Header().Set("Content-Type", "application/protobuf")
		resp := &backend.CreateObservableServiceResponse{
			Response: &backend.CreateObservableServiceResponse_Success{
				Success: &backend.CreateObservableServiceResponseSuccess{
					ObservableServiceId: "new-service-id",
				},
			},
		}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	setupTestEnvironment(t, server.URL)
	setupTestAuth(t)

	model := &Model{
		GitRepoOwner: "test-owner",
		GitRepoName:  "test-repo",
	}

	cmd := createObservableService(model)
	msg := cmd()

	assert.True(t, serverCalled, "Server should have been called")
	successMsg, ok := msg.(createObservableServiceSuccessMsg)
	assert.True(t, ok, "Expected createObservableServiceSuccessMsg, got %T: %+v", msg, msg)
	if ok {
		assert.Equal(t, "new-service-id", successMsg.serviceID)
	}
}

func TestCreateObservableService_NoCodeHostingResource(t *testing.T) {
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		w.Header().Set("Content-Type", "application/protobuf")
		resp := &backend.CreateObservableServiceResponse{
			Response: &backend.CreateObservableServiceResponse_Error{
				Error: &backend.CreateObservableServiceResponseError{
					Code:    backend.CreateObservableServiceResponseErrorCode_CREATE_OBSERVABLE_SERVICE_RESPONSE_ERROR_CODE_NO_CODE_HOSTING_RESOURCE,
					Message: "No code hosting resource",
				},
			},
		}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	setupTestEnvironment(t, server.URL)
	setupTestAuth(t)

	model := &Model{
		GitRepoOwner: "test-owner",
		GitRepoName:  "test-repo",
	}

	cmd := createObservableService(model)
	msg := cmd()

	assert.True(t, serverCalled, "Server should have been called")
	errMsg, ok := msg.(createObservableServiceErrorMsg)
	assert.True(t, ok, "Expected createObservableServiceErrorMsg, got %T: %+v", msg, msg)
	if ok {
		assert.Contains(t, errMsg.err.Error(), "no GitHub/GitLab connection found")
	}
}

func TestCreateObservableService_NoRepoFound(t *testing.T) {
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		w.Header().Set("Content-Type", "application/protobuf")
		resp := &backend.CreateObservableServiceResponse{
			Response: &backend.CreateObservableServiceResponse_Error{
				Error: &backend.CreateObservableServiceResponseError{
					Code:    backend.CreateObservableServiceResponseErrorCode_CREATE_OBSERVABLE_SERVICE_RESPONSE_ERROR_CODE_NO_REPO_FOUND,
					Message: "Repository not found",
				},
			},
		}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	setupTestEnvironment(t, server.URL)
	setupTestAuth(t)

	model := &Model{
		GitRepoOwner: "test-owner",
		GitRepoName:  "test-repo",
	}

	cmd := createObservableService(model)
	msg := cmd()

	assert.True(t, serverCalled, "Server should have been called")
	errMsg, ok := msg.(createObservableServiceErrorMsg)
	assert.True(t, ok, "Expected createObservableServiceErrorMsg, got %T: %+v", msg, msg)
	if ok {
		assert.Contains(t, errMsg.err.Error(), "repository test-owner/test-repo not found")
	}
}

func TestCreateObservableService_InvalidResponse(t *testing.T) {
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		w.Header().Set("Content-Type", "application/protobuf")
		resp := &backend.CreateObservableServiceResponse{}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	setupTestEnvironment(t, server.URL)
	setupTestAuth(t)

	model := &Model{
		GitRepoOwner: "test-owner",
		GitRepoName:  "test-repo",
	}

	cmd := createObservableService(model)
	msg := cmd()

	assert.True(t, serverCalled, "Server should have been called")
	errMsg, ok := msg.(createObservableServiceErrorMsg)
	assert.True(t, ok, "Expected createObservableServiceErrorMsg, got %T: %+v", msg, msg)
	if ok {
		assert.Contains(t, errMsg.err.Error(), "invalid response")
	}
}

func TestCreateApiKey_Success(t *testing.T) {
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		assert.Equal(t, "/api/drift/client_service/create_api_key", r.URL.Path)

		w.Header().Set("Content-Type", "application/protobuf")
		resp := &backend.CreateApiKeyResponse{
			Response: &backend.CreateApiKeyResponse_Success{
				Success: &backend.CreateApiKeyResponseSuccess{
					ApiKeyId: "new-api-key-id",
					ApiKey:   "tusk_api_key_12345",
				},
			},
		}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	setupTestEnvironment(t, server.URL)
	setupTestAuth(t)

	model := &Model{
		ApiKeyName: "Test API Key",
	}

	cmd := createApiKey(model)
	msg := cmd()

	assert.True(t, serverCalled, "Server should have been called")
	successMsg, ok := msg.(createApiKeySuccessMsg)
	assert.True(t, ok, "Expected createApiKeySuccessMsg, got %T: %+v", msg, msg)
	if ok {
		assert.Equal(t, "new-api-key-id", successMsg.apiKeyID)
		assert.Equal(t, "tusk_api_key_12345", successMsg.apiKey)
	}
}

func TestCreateApiKey_NotAuthorized(t *testing.T) {
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		w.Header().Set("Content-Type", "application/protobuf")
		resp := &backend.CreateApiKeyResponse{
			Response: &backend.CreateApiKeyResponse_Error{
				Error: &backend.CreateApiKeyResponseError{
					Code:    backend.CreateApiKeyResponseErrorCode_CREATE_API_KEY_RESPONSE_ERROR_CODE_NOT_AUTHORIZED,
					Message: "Not authorized",
				},
			},
		}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	setupTestEnvironment(t, server.URL)
	setupTestAuth(t)

	model := &Model{
		ApiKeyName: "Test API Key",
	}

	cmd := createApiKey(model)
	msg := cmd()

	assert.True(t, serverCalled, "Server should have been called")
	errMsg, ok := msg.(createApiKeyErrorMsg)
	assert.True(t, ok, "Expected createApiKeyErrorMsg, got %T: %+v", msg, msg)
	if ok {
		assert.Contains(t, errMsg.err.Error(), "not authorized")
	}
}

func TestCreateApiKey_InvalidResponse(t *testing.T) {
	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		w.Header().Set("Content-Type", "application/protobuf")
		resp := &backend.CreateApiKeyResponse{}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	setupTestEnvironment(t, server.URL)
	setupTestAuth(t)

	model := &Model{
		ApiKeyName: "Test API Key",
	}

	cmd := createApiKey(model)
	msg := cmd()

	assert.True(t, serverCalled, "Server should have been called")
	errMsg, ok := msg.(createApiKeyErrorMsg)
	assert.True(t, ok, "Expected createApiKeyErrorMsg, got %T: %+v", msg, msg)
	if ok {
		assert.Contains(t, errMsg.err.Error(), "invalid response")
	}
}

func TestVerifyRepoAccess_MissingConfig(t *testing.T) {
	// Don't set up environment, so config is missing
	tmpDir, err := os.MkdirTemp("", "tusk-test-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpDir)
		config.Invalidate()
	})

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWd) })

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	setupTestAuth(t)

	model := &Model{
		GitRepoOwner: "test-owner",
		GitRepoName:  "test-repo",
	}

	cmd := verifyRepoAccess(model)
	msg := cmd()

	errMsg, ok := msg.(verifyRepoAccessErrorMsg)
	assert.True(t, ok, "Expected verifyRepoAccessErrorMsg, got %T: %+v", msg, msg)
	if ok {
		assert.Contains(t, errMsg.err.Error(), "failed to setup cloud connection")
	}
}

func TestCreateObservableService_MissingConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tusk-test-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpDir)
		config.Invalidate()
	})

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWd) })

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	setupTestAuth(t)

	model := &Model{
		GitRepoOwner: "test-owner",
		GitRepoName:  "test-repo",
	}

	cmd := createObservableService(model)
	msg := cmd()

	errMsg, ok := msg.(createObservableServiceErrorMsg)
	assert.True(t, ok, "Expected createObservableServiceErrorMsg, got %T: %+v", msg, msg)
	if ok {
		// Either config or git error is acceptable
		err := errMsg.err.Error()
		assert.True(t,
			strings.Contains(err, "failed to setup cloud connection") ||
				strings.Contains(err, "failed to determine app directory"),
			"Error should mention either config or git: %s", err)
	}
}

func TestCreateApiKey_MissingConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tusk-test-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpDir)
		config.Invalidate()
	})

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(originalWd) })

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	setupTestAuth(t)

	model := &Model{
		ApiKeyName: "Test API Key",
	}

	cmd := createApiKey(model)
	msg := cmd()

	errMsg, ok := msg.(createApiKeyErrorMsg)
	assert.True(t, ok, "Expected createApiKeyErrorMsg, got %T: %+v", msg, msg)
	if ok {
		assert.Contains(t, errMsg.err.Error(), "failed to setup cloud connection")
	}
}

// Test that the request body contains expected data
func TestVerifyRepoAccess_RequestBody(t *testing.T) {
	var receivedReq *backend.VerifyRepoAccessRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		req := &backend.VerifyRepoAccessRequest{}
		_ = proto.Unmarshal(body, req)
		receivedReq = req

		w.Header().Set("Content-Type", "application/protobuf")
		resp := &backend.VerifyRepoAccessResponse{
			Response: &backend.VerifyRepoAccessResponse_Success{
				Success: &backend.VerifyRepoAccessResponseSuccess{
					RepoId: 12345,
				},
			},
		}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	setupTestEnvironment(t, server.URL)
	setupTestAuth(t)

	model := &Model{
		GitRepoOwner: "test-owner",
		GitRepoName:  "test-repo",
	}

	cmd := verifyRepoAccess(model)
	_ = cmd()

	assert.NotNil(t, receivedReq, "Server should have received a request")
	if receivedReq != nil {
		assert.Equal(t, "test-owner", receivedReq.RepoOwnerName)
		assert.Equal(t, "test-repo", receivedReq.RepoName)
	}
}

func TestCreateObservableService_RequestBody(t *testing.T) {
	var receivedReq *backend.CreateObservableServiceRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		req := &backend.CreateObservableServiceRequest{}
		_ = proto.Unmarshal(body, req)
		receivedReq = req

		w.Header().Set("Content-Type", "application/protobuf")
		resp := &backend.CreateObservableServiceResponse{
			Response: &backend.CreateObservableServiceResponse_Success{
				Success: &backend.CreateObservableServiceResponseSuccess{
					ObservableServiceId: "new-service-id",
				},
			},
		}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	setupTestEnvironment(t, server.URL)
	setupTestAuth(t)

	model := &Model{
		GitRepoOwner: "test-owner",
		GitRepoName:  "test-repo",
	}

	cmd := createObservableService(model)
	_ = cmd()

	assert.NotNil(t, receivedReq, "Server should have received a request")
	if receivedReq != nil {
		assert.Equal(t, "test-owner", receivedReq.RepoOwnerName)
		assert.Equal(t, "test-repo", receivedReq.RepoName)
		assert.Equal(t, backend.ServiceType_SERVICE_TYPE_NODE, receivedReq.ServiceType)
	}
}

func TestCreateApiKey_RequestBody(t *testing.T) {
	var receivedReq *backend.CreateApiKeyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		req := &backend.CreateApiKeyRequest{}
		_ = proto.Unmarshal(body, req)
		receivedReq = req

		w.Header().Set("Content-Type", "application/protobuf")
		resp := &backend.CreateApiKeyResponse{
			Response: &backend.CreateApiKeyResponse_Success{
				Success: &backend.CreateApiKeyResponseSuccess{
					ApiKeyId: "new-api-key-id",
					ApiKey:   "tusk_api_key_12345",
				},
			},
		}
		bin, _ := proto.Marshal(resp)
		_, _ = w.Write(bin)
	}))
	defer server.Close()

	setupTestEnvironment(t, server.URL)
	setupTestAuth(t)

	model := &Model{
		ApiKeyName: "My Test Key",
	}

	cmd := createApiKey(model)
	_ = cmd()

	assert.NotNil(t, receivedReq, "Server should have received a request")
	if receivedReq != nil {
		assert.Equal(t, "My Test Key", receivedReq.Name)
	}
}
