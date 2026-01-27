package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	"github.com/Use-Tusk/tusk-drift-cli/internal/auth"
	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	core "github.com/Use-Tusk/tusk-drift-schemas/generated/go/core"
	"gopkg.in/yaml.v3"
)

// CloudTools provides cloud-related operations for the agent
type CloudTools struct {
	authenticator      *auth.Authenticator
	bearerToken        string
	userId             string
	userEmail          string
	clientID           string
	clientName         string
	deviceCode         string
	deviceCodeInterval int
	deviceCodeExpiry   int
}

// NewCloudTools creates a new CloudTools instance
func NewCloudTools() *CloudTools {
	return &CloudTools{}
}

// CloudLoginResult contains the result of a cloud login operation
type CloudLoginResult struct {
	Success    bool   `json:"success"`
	Email      string `json:"email"`
	UserId     string `json:"user_id"`
	Error      string `json:"error,omitempty"`
	NeedsLogin bool   `json:"needs_login"`
}

// ClientInfo represents a Tusk client/organization
type ClientInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CheckAuth checks if the user is already authenticated
func (ct *CloudTools) CheckAuth(input json.RawMessage) (string, error) {
	authenticator, err := auth.NewAuthenticator()
	if err != nil {
		return "", fmt.Errorf("failed to initialize authenticator: %w", err)
	}

	ctx := context.Background()
	if err := authenticator.TryExistingAuth(ctx); err != nil {
		result := CloudLoginResult{
			Success:    false,
			NeedsLogin: true,
		}
		data, _ := json.Marshal(result)
		return string(data), nil
	}

	ct.authenticator = authenticator
	ct.bearerToken = authenticator.AccessToken
	ct.userEmail = authenticator.Email

	// Fetch user info to get userId
	client, authOptions, _, err := api.SetupCloud(ctx, false)
	if err != nil {
		result := CloudLoginResult{
			Success:    false,
			NeedsLogin: true,
			Error:      err.Error(),
		}
		data, _ := json.Marshal(result)
		return string(data), nil
	}

	resp, err := client.GetAuthInfo(ctx, &backend.GetAuthInfoRequest{}, authOptions)
	if err != nil {
		result := CloudLoginResult{
			Success:    false,
			NeedsLogin: true,
			Error:      "Session expired or invalid",
		}
		data, _ := json.Marshal(result)
		return string(data), nil
	}

	ct.userId = resp.User.GetId()

	result := CloudLoginResult{
		Success:    true,
		Email:      ct.userEmail,
		UserId:     ct.userId,
		NeedsLogin: false,
	}
	data, _ := json.Marshal(result)
	return string(data), nil
}

// LoginDeviceCodeResult contains the device code info for display
type LoginDeviceCodeResult struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	BrowserOpened   bool   `json:"browser_opened"`
}

// Login initiates the device code login flow - returns device code info for agent to display
// The agent should then call WaitForLogin to complete the authentication
func (ct *CloudTools) Login(input json.RawMessage) (string, error) {
	authenticator, err := auth.NewAuthenticator()
	if err != nil {
		return "", fmt.Errorf("failed to initialize authenticator: %w", err)
	}

	// First check if we can use existing auth or refresh token
	ctx := context.Background()
	if err := authenticator.TryExistingAuth(ctx); err == nil {
		// Already authenticated via existing token
		ct.authenticator = authenticator
		ct.bearerToken = authenticator.AccessToken
		ct.userEmail = authenticator.Email

		// Fetch user info
		client, authOptions, _, err := api.SetupCloud(ctx, false)
		if err != nil {
			return "", fmt.Errorf("failed to setup cloud connection: %w", err)
		}

		resp, err := client.GetAuthInfo(ctx, &backend.GetAuthInfoRequest{}, authOptions)
		if err != nil {
			return "", fmt.Errorf("failed to get user info: %w", err)
		}

		ct.userId = resp.User.GetId()

		result := CloudLoginResult{
			Success: true,
			Email:   ct.userEmail,
			UserId:  ct.userId,
		}
		data, _ := json.Marshal(result)
		return string(data), nil
	}

	// Need to do device code flow - get device code and return info for display
	// The actual login will happen in WaitForLogin
	dcr, err := authenticator.RequestDeviceCode(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get device code: %w", err)
	}

	// Store authenticator and device code for WaitForLogin to use
	ct.authenticator = authenticator
	ct.deviceCode = dcr.DeviceCode
	ct.deviceCodeInterval = dcr.Interval
	ct.deviceCodeExpiry = dcr.ExpiresIn

	// Open browser automatically
	browserOpened := false
	browserURL := dcr.VerificationURL()
	if browserURL != "" {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", browserURL) //nolint:gosec
		case "linux":
			cmd = exec.Command("xdg-open", browserURL) //nolint:gosec
		case "windows":
			cmd = exec.Command("cmd", "/c", "start", browserURL) //nolint:gosec
		}
		if cmd != nil {
			if err := cmd.Start(); err == nil {
				browserOpened = true
			}
		}
	}

	result := LoginDeviceCodeResult{
		DeviceCode:      dcr.DeviceCode,
		UserCode:        dcr.UserCode,
		VerificationURL: browserURL,
		ExpiresIn:       dcr.ExpiresIn,
		BrowserOpened:   browserOpened,
	}
	data, _ := json.Marshal(result)
	return string(data), nil
}

// WaitForLogin polls for the device code authentication to complete
func (ct *CloudTools) WaitForLogin(input json.RawMessage) (string, error) {
	if ct.deviceCode == "" {
		return "", fmt.Errorf("no device code available - call cloud_login first")
	}
	if ct.authenticator == nil {
		return "", fmt.Errorf("authenticator not initialized - call cloud_login first")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ct.deviceCodeExpiry)*time.Second)
	defer cancel()

	// Construct DeviceCodeResponse from stored values
	dcr := auth.DeviceCodeResponse{
		DeviceCode: ct.deviceCode,
		Interval:   ct.deviceCodeInterval,
		ExpiresIn:  ct.deviceCodeExpiry,
	}

	// Use the exported PollForToken method
	if err := ct.authenticator.PollForToken(ctx, dcr); err != nil {
		return "", fmt.Errorf("authentication failed: %w", err)
	}

	ct.bearerToken = ct.authenticator.AccessToken

	// Fetch user email using the exported method
	if err := ct.authenticator.FetchUserEmail(ctx); err != nil {
		// Non-fatal, continue anyway
		ct.userEmail = ""
	} else {
		ct.userEmail = ct.authenticator.Email
	}

	// Save token file
	if err := ct.authenticator.SaveTokenFile(); err != nil {
		// Non-fatal, continue anyway
		_ = err
	}

	// Fetch user info from API
	apiClient, authOptions, _, err := api.SetupCloud(ctx, false)
	if err != nil {
		return "", fmt.Errorf("failed to setup cloud connection: %w", err)
	}

	authResp, err := apiClient.GetAuthInfo(ctx, &backend.GetAuthInfoRequest{}, authOptions)
	if err != nil {
		return "", fmt.Errorf("failed to get user info: %w", err)
	}

	ct.userId = authResp.User.GetId()
	if ct.userEmail == "" && authResp.User.GetEmail() != "" {
		ct.userEmail = authResp.User.GetEmail()
	}

	// Clear device code
	ct.deviceCode = ""

	result := CloudLoginResult{
		Success: true,
		Email:   ct.userEmail,
		UserId:  ct.userId,
	}
	data, _ := json.Marshal(result)
	return string(data), nil
}

// GetClients returns the list of organizations/clients the user belongs to
func (ct *CloudTools) GetClients(input json.RawMessage) (string, error) {
	ctx := context.Background()

	client, authOptions, _, err := api.SetupCloud(ctx, false)
	if err != nil {
		return "", fmt.Errorf("failed to setup cloud connection: %w", err)
	}

	resp, err := client.GetAuthInfo(ctx, &backend.GetAuthInfoRequest{}, authOptions)
	if err != nil {
		if strings.Contains(err.Error(), "http 401") {
			return "", fmt.Errorf("authentication failed - session may have expired. Please run 'tusk auth logout' followed by 'tusk auth login'")
		}
		return "", fmt.Errorf("failed to get auth info: %w", err)
	}

	clients := make([]ClientInfo, len(resp.Clients))
	for i, c := range resp.Clients {
		name := "Unnamed"
		if c.Name != nil {
			name = *c.Name
		}
		clients[i] = ClientInfo{
			ID:   c.Id,
			Name: name,
		}
	}

	// If there's a previously selected client, note it
	selectedID := cliconfig.CLIConfig.SelectedClientID

	result := struct {
		Clients          []ClientInfo `json:"clients"`
		SelectedClientID string       `json:"selected_client_id,omitempty"`
		UserId           string       `json:"user_id"`
		UserEmail        string       `json:"user_email"`
	}{
		Clients:          clients,
		SelectedClientID: selectedID,
		UserId:           resp.User.GetId(),
		UserEmail:        ct.userEmail,
	}

	data, _ := json.Marshal(result)
	return string(data), nil
}

// SelectClient saves the selected client to CLI config
func (ct *CloudTools) SelectClient(input json.RawMessage) (string, error) {
	var params struct {
		ClientID   string `json:"client_id"`
		ClientName string `json:"client_name"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	ct.clientID = params.ClientID
	ct.clientName = params.ClientName

	cfg := cliconfig.CLIConfig
	cfg.SelectedClientID = params.ClientID
	cfg.SelectedClientName = params.ClientName
	if err := cfg.Save(); err != nil {
		return "", fmt.Errorf("failed to save client selection: %w", err)
	}

	return fmt.Sprintf("Selected organization: %s (ID: %s)", params.ClientName, params.ClientID), nil
}

// DetectGitRepo detects the git repository information
func (ct *CloudTools) DetectGitRepo(input json.RawMessage) (string, error) {
	// Check if we're in a git repo
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("not a git repository. Please run this from a git repository")
	}

	// Get remotes
	cmd = exec.Command("git", "remote", "-v")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to list git remotes: %w", err)
	}

	output := strings.TrimSpace(string(out))
	if output == "" {
		return "", fmt.Errorf("no git remotes configured. Please add a remote with 'git remote add origin <url>'")
	}

	// Parse remotes
	remotes := make(map[string]string)
	for line := range strings.SplitSeq(output, "\n") {
		if !strings.Contains(line, "(fetch)") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			remotes[parts[0]] = parts[1]
		}
	}

	// Try to find origin first
	remoteURL := ""
	remoteName := ""
	if url, ok := remotes["origin"]; ok {
		remoteURL = url
		remoteName = "origin"
	} else if len(remotes) == 1 {
		for name, url := range remotes {
			remoteURL = url
			remoteName = name
		}
	} else {
		// Return list of remotes for user to choose
		result := struct {
			MultipleRemotes bool              `json:"multiple_remotes"`
			Remotes         map[string]string `json:"remotes"`
		}{
			MultipleRemotes: true,
			Remotes:         remotes,
		}
		data, _ := json.Marshal(result)
		return string(data), nil
	}

	// Parse the remote URL
	owner, repo, hostingType, err := parseGitRemoteURL(remoteURL)
	if err != nil {
		return "", err
	}

	result := struct {
		Owner       string `json:"owner"`
		Repo        string `json:"repo"`
		HostingType string `json:"hosting_type"`
		RemoteName  string `json:"remote_name"`
		RemoteURL   string `json:"remote_url"`
	}{
		Owner:       owner,
		Repo:        repo,
		HostingType: hostingType,
		RemoteName:  remoteName,
		RemoteURL:   remoteURL,
	}
	data, _ := json.Marshal(result)
	return string(data), nil
}

// parseGitRemoteURL extracts owner, repo, and hosting type from a git remote URL
func parseGitRemoteURL(remoteURL string) (owner, repo, hostingType string, err error) {
	// Handle GitHub
	if strings.Contains(remoteURL, "github.com") {
		hostingType = "github"
		owner, repo = parseRepoPath(remoteURL, "github.com")
		if owner != "" && repo != "" {
			return owner, repo, hostingType, nil
		}
	}

	// Handle GitLab
	if strings.Contains(remoteURL, "gitlab.com") {
		hostingType = "gitlab"
		owner, repo = parseRepoPath(remoteURL, "gitlab.com")
		if owner != "" && repo != "" {
			return owner, repo, hostingType, nil
		}
	}

	return "", "", "", fmt.Errorf("repository must be hosted on GitHub or GitLab. Remote URL: %s", remoteURL)
}

// parseRepoPath extracts owner and repo from a git URL
func parseRepoPath(remoteURL, host string) (owner, repo string) {
	var path string

	// Handle SSH format: git@github.com:owner/repo.git
	if strings.HasPrefix(remoteURL, "git@"+host+":") {
		path = strings.TrimPrefix(remoteURL, "git@"+host+":")
	} else if strings.Contains(remoteURL, host+"/") {
		// Handle HTTPS format: https://github.com/owner/repo.git
		parts := strings.Split(remoteURL, host+"/")
		if len(parts) > 1 {
			path = parts[1]
		}
	}

	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return "", ""
}

// VerifyRepoAccess verifies that Tusk has access to the repository
func (ct *CloudTools) VerifyRepoAccess(input json.RawMessage) (string, error) {
	var params struct {
		Owner    string `json:"owner"`
		Repo     string `json:"repo"`
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	ctx := context.Background()
	client, authOptions, _, err := api.SetupCloud(ctx, false)
	if err != nil {
		return "", fmt.Errorf("failed to setup cloud connection: %w", err)
	}

	if params.ClientID != "" {
		authOptions.TuskClientID = params.ClientID
	}

	req := &backend.VerifyRepoAccessRequest{
		RepoOwnerName: params.Owner,
		RepoName:      params.Repo,
	}

	resp, err := client.VerifyRepoAccess(ctx, req, authOptions)
	if err != nil {
		return "", fmt.Errorf("API call failed: %w", err)
	}

	if errResp := resp.GetError(); errResp != nil {
		switch errResp.Code {
		case backend.VerifyRepoAccessResponseErrorCode_VERIFY_REPO_ACCESS_RESPONSE_ERROR_CODE_NO_CODE_HOSTING_RESOURCE:
			return "", fmt.Errorf("NO_CODE_HOSTING_RESOURCE: No GitHub/GitLab connection found. Please install the Tusk app on your repository")
		case backend.VerifyRepoAccessResponseErrorCode_VERIFY_REPO_ACCESS_RESPONSE_ERROR_CODE_REPO_NOT_FOUND:
			return "", fmt.Errorf("REPO_NOT_FOUND: Repository %s/%s not found or not accessible. Please ensure Tusk has access", params.Owner, params.Repo)
		default:
			return "", fmt.Errorf("failed to verify repo access: %s", errResp.Message)
		}
	}

	successResp := resp.GetSuccess()
	if successResp == nil {
		return "", fmt.Errorf("invalid response from server")
	}

	result := struct {
		Success bool  `json:"success"`
		RepoID  int64 `json:"repo_id"`
	}{
		Success: true,
		RepoID:  successResp.RepoId,
	}
	data, _ := json.Marshal(result)
	return string(data), nil
}

// GetCodeHostingAuthURL returns the URL for GitHub/GitLab app installation
func (ct *CloudTools) GetCodeHostingAuthURL(input json.RawMessage) (string, error) {
	var params struct {
		HostingType string `json:"hosting_type"` // "github" or "gitlab"
		ClientID    string `json:"client_id"`
		UserID      string `json:"user_id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	var authURL string
	switch params.HostingType {
	case "github":
		stateObj := map[string]string{
			"clientId": params.ClientID,
			"userId":   params.UserID,
			"source":   "cli-setup-agent",
		}
		stateJSON, err := json.Marshal(stateObj)
		if err != nil {
			return "", fmt.Errorf("failed to marshal state: %w", err)
		}
		encodedState := url.QueryEscape(string(stateJSON))
		githubAppName := utils.EnvDefault("GITHUB_APP_NAME", "use-tusk")
		authURL = fmt.Sprintf("https://github.com/apps/%s/installations/new?state=%s", githubAppName, encodedState)
	case "gitlab":
		authURL = "https://app.usetusk.ai/app/settings/connect-gitlab"
	default:
		return "", fmt.Errorf("unsupported hosting type: %s", params.HostingType)
	}

	return authURL, nil
}

// OpenBrowser opens a URL in the default browser
func (ct *CloudTools) OpenBrowser(input json.RawMessage) (string, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", params.URL) //nolint:gosec
	case "linux":
		cmd = exec.Command("xdg-open", params.URL) //nolint:gosec
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", params.URL) //nolint:gosec
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to open browser: %w", err)
	}

	return fmt.Sprintf("Opened browser to: %s", params.URL), nil
}

// CreateObservableService creates a new observable service in Tusk Cloud
func (ct *CloudTools) CreateObservableService(input json.RawMessage) (string, error) {
	var params struct {
		Owner       string `json:"owner"`
		Repo        string `json:"repo"`
		ClientID    string `json:"client_id"`
		ProjectType string `json:"project_type"` // "nodejs" or "python"
		AppDir      string `json:"app_dir"`      // Optional: relative path from repo root
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	ctx := context.Background()
	client, authOptions, _, err := api.SetupCloud(ctx, false)
	if err != nil {
		return "", fmt.Errorf("failed to setup cloud connection: %w", err)
	}

	if params.ClientID != "" {
		authOptions.TuskClientID = params.ClientID
	}

	serviceType := backend.ServiceType_SERVICE_TYPE_NODE
	if params.ProjectType == "python" {
		serviceType = backend.ServiceType_SERVICE_TYPE_PYTHON
	}

	var appDirPtr *string
	if params.AppDir != "" {
		appDirPtr = &params.AppDir
	}

	req := &backend.CreateObservableServiceRequest{
		RepoOwnerName: params.Owner,
		RepoName:      params.Repo,
		ServiceType:   serviceType,
		AppDir:        appDirPtr,
	}

	resp, err := client.CreateObservableService(ctx, req, authOptions)
	if err != nil {
		return "", fmt.Errorf("API call failed: %w", err)
	}

	if errResp := resp.GetError(); errResp != nil {
		switch errResp.Code {
		case backend.CreateObservableServiceResponseErrorCode_CREATE_OBSERVABLE_SERVICE_RESPONSE_ERROR_CODE_NO_CODE_HOSTING_RESOURCE:
			return "", fmt.Errorf("no GitHub/GitLab connection found")
		case backend.CreateObservableServiceResponseErrorCode_CREATE_OBSERVABLE_SERVICE_RESPONSE_ERROR_CODE_NO_REPO_FOUND:
			return "", fmt.Errorf("repository %s/%s not found", params.Owner, params.Repo)
		default:
			return "", fmt.Errorf("failed to create service: %s", errResp.Message)
		}
	}

	successResp := resp.GetSuccess()
	if successResp == nil {
		return "", fmt.Errorf("invalid response from server")
	}

	result := struct {
		Success   bool   `json:"success"`
		ServiceID string `json:"service_id"`
	}{
		Success:   true,
		ServiceID: successResp.ObservableServiceId,
	}
	data, _ := json.Marshal(result)
	return string(data), nil
}

// CreateApiKey creates a new API key
func (ct *CloudTools) CreateApiKey(input json.RawMessage) (string, error) {
	var params struct {
		Name     string `json:"name"`
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	ctx := context.Background()
	client, authOptions, _, err := api.SetupCloud(ctx, false)
	if err != nil {
		return "", fmt.Errorf("failed to setup cloud connection: %w", err)
	}

	if params.ClientID != "" {
		authOptions.TuskClientID = params.ClientID
	}

	req := &backend.CreateApiKeyRequest{
		Name: params.Name,
	}

	resp, err := client.CreateApiKey(ctx, req, authOptions)
	if err != nil {
		return "", fmt.Errorf("API call failed: %w", err)
	}

	if errResp := resp.GetError(); errResp != nil {
		switch errResp.Code {
		case backend.CreateApiKeyResponseErrorCode_CREATE_API_KEY_RESPONSE_ERROR_CODE_NOT_AUTHORIZED:
			return "", fmt.Errorf("not authorized to create API keys")
		default:
			return "", fmt.Errorf("failed to create API key: %s", errResp.Message)
		}
	}

	successResp := resp.GetSuccess()
	if successResp == nil {
		return "", fmt.Errorf("invalid response from server")
	}

	result := struct {
		Success bool   `json:"success"`
		KeyID   string `json:"key_id"`
		Key     string `json:"key"`
	}{
		Success: true,
		KeyID:   successResp.ApiKeyId,
		Key:     successResp.ApiKey,
	}
	data, _ := json.Marshal(result)
	return string(data), nil
}

// CheckApiKeyExists checks if the user already has an API key configured
func (ct *CloudTools) CheckApiKeyExists(input json.RawMessage) (string, error) {
	hasKey := cliconfig.GetAPIKey() != ""

	result := struct {
		HasApiKey bool `json:"has_api_key"`
	}{
		HasApiKey: hasKey,
	}
	data, _ := json.Marshal(result)
	return string(data), nil
}

// SaveCloudConfig saves the cloud configuration to .tusk/config.yaml
func (ct *CloudTools) SaveCloudConfig(input json.RawMessage) (string, error) {
	var params struct {
		ServiceID             string  `json:"service_id"`
		SamplingRate          float64 `json:"sampling_rate"`
		ExportSpans           bool    `json:"export_spans"`
		EnableEnvVarRecording bool    `json:"enable_env_var_recording"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	configPath := ".tusk/config.yaml"
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("failed to parse config file: %w", err)
	}

	if params.ServiceID != "" {
		if service, ok := config["service"].(map[string]any); ok {
			if _, hasID := service["id"]; !hasID {
				service["id"] = params.ServiceID
			}
		}
	}

	config["recording"] = map[string]any{
		"sampling_rate":            params.SamplingRate,
		"export_spans":             params.ExportSpans,
		"enable_env_var_recording": params.EnableEnvVarRecording,
	}

	if _, hasTuskAPI := config["tusk_api"]; !hasTuskAPI {
		config["tusk_api"] = map[string]any{
			"url": api.DefaultBaseURL,
		}
	}

	output, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, output, 0o600); err != nil {
		return "", fmt.Errorf("failed to write config file: %w", err)
	}

	return "Cloud configuration saved to .tusk/config.yaml", nil
}

// WaitForAuth waits for authentication to complete with polling
// This is called after opening the browser for device code flow
func (ct *CloudTools) WaitForAuth(input json.RawMessage) (string, error) {
	var params struct {
		TimeoutSeconds int `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	timeout := 120 * time.Second
	if params.TimeoutSeconds > 0 {
		timeout = time.Duration(params.TimeoutSeconds) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Poll for authentication
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("authentication timed out after %v", timeout)
		case <-ticker.C:
			authenticator, err := auth.NewAuthenticator()
			if err != nil {
				continue
			}
			if err := authenticator.TryExistingAuth(ctx); err == nil {
				ct.authenticator = authenticator
				ct.bearerToken = authenticator.AccessToken
				ct.userEmail = authenticator.Email

				result := CloudLoginResult{
					Success: true,
					Email:   ct.userEmail,
				}
				data, _ := json.Marshal(result)
				return string(data), nil
			}
		}
	}
}

// UploadTraces uploads local traces to Tusk Cloud
func (ct *CloudTools) UploadTraces(input json.RawMessage) (string, error) {
	var params struct {
		ServiceID string `json:"service_id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if params.ServiceID == "" {
		return "", fmt.Errorf("service_id is required")
	}

	// Find all trace files in .tusk/traces/
	tracesDir := utils.GetTracesDir()
	if _, err := os.Stat(tracesDir); os.IsNotExist(err) {
		result := map[string]interface{}{
			"success":         false,
			"message":         "No traces directory found",
			"traces_uploaded": 0,
		}
		data, _ := json.Marshal(result)
		return string(data), nil
	}

	// Collect all .jsonl files
	var traceFiles []string
	err := filepath.Walk(tracesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".jsonl") {
			traceFiles = append(traceFiles, path)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to scan traces directory: %w", err)
	}

	if len(traceFiles) == 0 {
		result := map[string]interface{}{
			"success":         false,
			"message":         "No trace files found",
			"traces_uploaded": 0,
		}
		data, _ := json.Marshal(result)
		return string(data), nil
	}

	// Parse and collect all spans from all trace files
	var allSpans []*core.Span
	for _, traceFile := range traceFiles {
		spans, err := utils.ParseSpansFromFile(traceFile, nil)
		if err != nil {
			// Log warning but continue with other files
			continue
		}
		allSpans = append(allSpans, spans...)
	}

	if len(allSpans) == 0 {
		result := map[string]interface{}{
			"success":         false,
			"message":         "No spans found in trace files",
			"traces_uploaded": 0,
		}
		data, _ := json.Marshal(result)
		return string(data), nil
	}

	// Upload spans to cloud
	ctx := context.Background()
	client, authOptions, _, err := api.SetupCloud(ctx, false)
	if err != nil {
		return "", fmt.Errorf("failed to setup cloud connection: %w", err)
	}

	req := &backend.ExportSpansRequest{
		ObservableServiceId: params.ServiceID,
		Environment:         "setup-agent",
		SdkVersion:          "cli-setup",
		SdkInstanceId:       fmt.Sprintf("setup-%d", time.Now().Unix()),
		Spans:               allSpans,
	}

	resp, err := client.ExportSpans(ctx, req, authOptions)
	if err != nil {
		return "", fmt.Errorf("failed to upload traces: %w", err)
	}

	if !resp.Success {
		result := map[string]interface{}{
			"success":         false,
			"message":         resp.Message,
			"traces_uploaded": 0,
		}
		data, _ := json.Marshal(result)
		return string(data), nil
	}

	result := map[string]interface{}{
		"success":         true,
		"message":         "Traces uploaded successfully",
		"traces_uploaded": len(traceFiles),
		"spans_uploaded":  len(allSpans),
	}
	data, _ := json.Marshal(result)
	return string(data), nil
}
