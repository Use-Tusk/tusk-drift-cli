package onboardcloud

import (
	"context"
	"fmt"

	"github.com/Use-Tusk/tusk-drift-cli/internal/api"
	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
	tea "github.com/charmbracelet/bubbletea"
)

type (
	verifyRepoAccessSuccessMsg struct {
		repoID int64
	}
	verifyRepoAccessErrorMsg struct {
		err error
	}

	createObservableServiceSuccessMsg struct {
		serviceID string
	}
	createObservableServiceErrorMsg struct {
		err error
	}
	createApiKeySuccessMsg struct {
		apiKeyID string
		apiKey   string
	}
	createApiKeyErrorMsg struct {
		err error
	}
)

func verifyRepoAccess(m *Model) tea.Cmd {
	return func() tea.Msg {
		client, authOptions, _, err := api.SetupCloud(context.Background(), false)
		if err != nil {
			return verifyRepoAccessErrorMsg{err: fmt.Errorf("failed to setup cloud connection: %w", err)}
		}

		if m.SelectedClient != nil {
			authOptions.TuskClientID = m.SelectedClient.ID
		}

		req := &backend.VerifyRepoAccessRequest{
			RepoOwnerName: m.GitRepoOwner,
			RepoName:      m.GitRepoName,
		}

		resp, err := client.VerifyRepoAccess(context.Background(), req, authOptions)
		if err != nil {
			return verifyRepoAccessErrorMsg{err: fmt.Errorf("API call failed: %w", err)}
		}

		if errResp := resp.GetError(); errResp != nil {
			switch errResp.Code {
			case backend.VerifyRepoAccessResponseErrorCode_VERIFY_REPO_ACCESS_RESPONSE_ERROR_CODE_NO_CODE_HOSTING_RESOURCE:
				return verifyRepoAccessErrorMsg{err: fmt.Errorf("no GitHub/GitLab connection found for your account")}
			case backend.VerifyRepoAccessResponseErrorCode_VERIFY_REPO_ACCESS_RESPONSE_ERROR_CODE_REPO_NOT_FOUND:
				return verifyRepoAccessErrorMsg{err: fmt.Errorf("repository %s/%s not found or not accessible", m.GitRepoOwner, m.GitRepoName)}
			default:
				return verifyRepoAccessErrorMsg{err: fmt.Errorf("failed to verify repo access: %s", errResp.Message)}
			}
		}

		// Success
		successResp := resp.GetSuccess()
		if successResp == nil {
			return verifyRepoAccessErrorMsg{err: fmt.Errorf("invalid response from server")}
		}

		return verifyRepoAccessSuccessMsg{
			repoID: successResp.RepoId,
		}
	}
}

func createObservableService(m *Model) tea.Cmd {
	return func() tea.Msg {
		client, authOptions, _, err := api.SetupCloud(context.Background(), false)
		if err != nil {
			return createObservableServiceErrorMsg{err: fmt.Errorf("failed to setup cloud connection: %w", err)}
		}

		if m.SelectedClient != nil {
			authOptions.TuskClientID = m.SelectedClient.ID
		}

		// TODO: update this when we support more SDKs
		serviceType := backend.ServiceType_SERVICE_TYPE_NODE

		appDir, err := getAppDir()
		if err != nil {
			return createObservableServiceErrorMsg{err: fmt.Errorf("failed to determine app directory: %w", err)}
		}

		var appDirPtr *string
		if appDir != "" {
			appDirPtr = &appDir
		}

		req := &backend.CreateObservableServiceRequest{
			RepoOwnerName: m.GitRepoOwner,
			RepoName:      m.GitRepoName,
			ServiceType:   serviceType,
			AppDir:        appDirPtr,
		}

		resp, err := client.CreateObservableService(context.Background(), req, authOptions)
		if err != nil {
			return createObservableServiceErrorMsg{err: fmt.Errorf("API call failed: %w", err)}
		}

		if errResp := resp.GetError(); errResp != nil {
			switch errResp.Code {
			case backend.CreateObservableServiceResponseErrorCode_CREATE_OBSERVABLE_SERVICE_RESPONSE_ERROR_CODE_NO_CODE_HOSTING_RESOURCE:
				return createObservableServiceErrorMsg{err: fmt.Errorf("no GitHub/GitLab connection found")}
			case backend.CreateObservableServiceResponseErrorCode_CREATE_OBSERVABLE_SERVICE_RESPONSE_ERROR_CODE_NO_REPO_FOUND:
				return createObservableServiceErrorMsg{err: fmt.Errorf("repository %s/%s not found", m.GitRepoOwner, m.GitRepoName)}
			default:
				return createObservableServiceErrorMsg{err: fmt.Errorf("failed to create service: %s", errResp.Message)}
			}
		}

		successResp := resp.GetSuccess()
		if successResp == nil {
			return createObservableServiceErrorMsg{err: fmt.Errorf("invalid response from server")}
		}

		if err := saveServiceIDToConfig(successResp.ObservableServiceId); err != nil {
			return createObservableServiceErrorMsg{err: fmt.Errorf("service created but failed to save to config: %w", err)}
		}

		return createObservableServiceSuccessMsg{
			serviceID: successResp.ObservableServiceId,
		}
	}
}

func createApiKey(m *Model) tea.Cmd {
	return func() tea.Msg {
		client, authOptions, _, err := api.SetupCloud(context.Background(), false)
		if err != nil {
			return createApiKeyErrorMsg{err: fmt.Errorf("failed to setup cloud connection: %w", err)}
		}

		if m.SelectedClient != nil {
			authOptions.TuskClientID = m.SelectedClient.ID
		}

		req := &backend.CreateApiKeyRequest{
			Name: m.ApiKeyName,
		}

		resp, err := client.CreateApiKey(context.Background(), req, authOptions)
		if err != nil {
			return createApiKeyErrorMsg{err: fmt.Errorf("API call failed: %w", err)}
		}

		if errResp := resp.GetError(); errResp != nil {
			switch errResp.Code {
			case backend.CreateApiKeyResponseErrorCode_CREATE_API_KEY_RESPONSE_ERROR_CODE_NOT_AUTHORIZED:
				return createApiKeyErrorMsg{err: fmt.Errorf("not authorized to create API keys")}
			default:
				return createApiKeyErrorMsg{err: fmt.Errorf("failed to create API key: %s", errResp.Message)}
			}
		}

		successResp := resp.GetSuccess()
		if successResp == nil {
			return createApiKeyErrorMsg{err: fmt.Errorf("invalid response from server")}
		}

		return createApiKeySuccessMsg{
			apiKeyID: successResp.ApiKeyId,
			apiKey:   successResp.ApiKey,
		}
	}
}
