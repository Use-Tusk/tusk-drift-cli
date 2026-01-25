package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/config"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
)

type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Email        string    `json:"email"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type saveFile struct {
	Token
}

type Authenticator struct {
	authFilePath string
	httpClient   *http.Client

	domain   string
	clientID string
	scope    string
	audience string

	ignoreSaveFile bool

	Token
}

func NewAuthenticator() (*Authenticator, error) {
	cfgDir, _ := os.UserConfigDir()
	authPath := filepath.Join(cfgDir, "tusk", "auth.json")

	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	a := &Authenticator{
		authFilePath:   authPath,
		httpClient:     &http.Client{Timeout: 15 * time.Second},
		domain:         cfg.TuskAPI.Auth0Domain,
		scope:          utils.EnvDefault("TUSK_AUTH0_SCOPE", "openid email offline_access"),
		clientID:       cfg.TuskAPI.Auth0ClientID,
		audience:       utils.EnvDefault("TUSK_AUTH0_AUDIENCE", "drift-cli"),
		ignoreSaveFile: os.Getenv("TUSK_AUTH_IGNORE_SAVED") != "",
	}
	if a.clientID == "" {
		return nil, errors.New("TUSK_AUTH0_CLIENT_ID is required for login")
	}
	if a.audience == "" {
		return nil, errors.New("TUSK_AUTH0_AUDIENCE is required for login")
	}
	return a, nil
}

// getTokenFromFile loads the cached token file into `a.saveFile`
func (a *Authenticator) getTokenFromFile() error {
	f, err := os.ReadFile(a.authFilePath)
	if err != nil {
		return fmt.Errorf("cannot read auth file at %q: %w", a.authFilePath, err)
	}

	var s saveFile
	if err := json.Unmarshal(f, &s); err != nil {
		return fmt.Errorf("invalid auth file JSON at %q: %w", a.authFilePath, err)
	}
	a.Token = s.Token
	return nil
}

func (a *Authenticator) isValid() bool {
	// Consider a small safety margin to avoid edge-of-expiry flakiness.
	if a.AccessToken == "" || a.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().Add(30 * time.Second).Before(a.ExpiresAt)
}

// SaveTokenFile saves the token to the auth file
func (a *Authenticator) SaveTokenFile() error {
	dir := filepath.Dir(a.authFilePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cannot create config dir %q: %w", dir, err)
	}
	b, err := json.MarshalIndent(a.Token, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(a.authFilePath, b, 0o600); err != nil {
		return fmt.Errorf("cannot write config file %q: %w", a.authFilePath, err)
	}
	return nil
}

func openBrowser(link string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", link).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", link).Start()
	default:
		return exec.Command("xdg-open", link).Start()
	}
}

func (a *Authenticator) Login(c context.Context) error {
	if !a.ignoreSaveFile {
		if err := a.getTokenFromFile(); err == nil && a.isValid() {
			return nil
		}

		if a.RefreshToken != "" {
			if err := a.refreshAccessToken(c); err == nil {
				if err := a.SaveTokenFile(); err != nil {
					return fmt.Errorf("write token file: %w", err)
				}
				return nil
			}

			// Refresh failed - clear the invalid token and fall through to device code flow
			a.RefreshToken = ""
			a.AccessToken = ""
		}
	}

	dcr, err := a.RequestDeviceCode(c)
	if err != nil {
		return err
	}

	fmt.Println("To authenticate, please complete the device verification in your browser:")
	if dcr.VerificationURIComplete != "" {
		fmt.Printf("\t%s\n", dcr.VerificationURIComplete)
	} else {
		fmt.Printf("\t%s  and enter code: %s\n", dcr.VerificationURI, dcr.UserCode)
	}
	fmt.Printf("Your user code is %q (expires in %d seconds)\n", dcr.UserCode, dcr.ExpiresIn)

	// Try to open the browser automatically
	_ = openBrowser(dcr.VerificationURL())

	ctx, cancel := context.WithTimeout(c, time.Duration(dcr.ExpiresIn)*time.Second)
	defer cancel()

	if err = a.PollForToken(ctx, dcr); err != nil {
		return fmt.Errorf("polling token failed: %w", err)
	}

	if err = a.FetchUserEmail(c); err != nil {
		return err
	}

	if err := a.SaveTokenFile(); err != nil {
		return fmt.Errorf("write token file: %w", err)
	}
	return nil
}

func (a *Authenticator) TryExistingAuth(ctx context.Context) error {
	if a.ignoreSaveFile {
		return errors.New("no saved auth available")
	}

	// Try to load existing token
	if err := a.getTokenFromFile(); err != nil {
		return fmt.Errorf("no saved auth: %w", err)
	}

	// If token is still valid, we're good
	if a.isValid() {
		return nil
	}

	// If we have a refresh token, try to refresh
	if a.RefreshToken != "" {
		if err := a.refreshAccessToken(ctx); err != nil {
			return fmt.Errorf("token refresh failed: %w", err)
		}
		// Save the refreshed token
		if err := a.SaveTokenFile(); err != nil {
			return fmt.Errorf("failed to save refreshed token: %w", err)
		}
		return nil
	}

	return errors.New("no valid auth available")
}

func (a *Authenticator) Logout() error {
	a.AccessToken = ""
	a.RefreshToken = ""
	a.Email = ""
	a.ExpiresAt = time.Time{}

	if _, err := os.Stat(a.authFilePath); err == nil {
		if err := os.Remove(a.authFilePath); err != nil {
			return fmt.Errorf("failed to remove auth file at %q: %w", a.authFilePath, err)
		}
	}

	return nil
}

// DeviceCodeResponse contains the response from a device code request
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
	VerificationURIComplete string `json:"verification_uri_complete"`

	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// VerificationURL returns the best URL for device verification
func (d DeviceCodeResponse) VerificationURL() string {
	if d.VerificationURIComplete != "" {
		return d.VerificationURIComplete
	}
	return d.VerificationURI
}

type tokenResp struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`

	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// RequestDeviceCode requests a device code for authentication.
// This is the first step of the device code flow - returns the code and URL
// for the user to complete authentication in their browser.
func (a *Authenticator) RequestDeviceCode(ctx context.Context) (DeviceCodeResponse, error) {
	form := url.Values{}
	form.Set("client_id", a.clientID)
	if a.scope != "" {
		form.Set("scope", a.scope)
	}
	if a.audience != "" {
		form.Set("audience", a.audience)
	}

	var dcr DeviceCodeResponse

	endpoint := fmt.Sprintf("https://%s/oauth/device/code", a.domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return dcr, fmt.Errorf("error creating device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return dcr, fmt.Errorf("error making device code request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return dcr, fmt.Errorf(
			"error returned by device code endpoint %d: %s",
			resp.StatusCode,
			string(body),
		)
	}
	if err = json.Unmarshal(body, &dcr); err != nil {
		return dcr, fmt.Errorf("error parsing device code response: %w", err)
	}
	return dcr, nil
}

// PollForToken polls Auth0 for token completion after a device code request.
// This blocks until the user completes authentication or the context is cancelled.
func (a *Authenticator) PollForToken(ctx context.Context, dcr DeviceCodeResponse) error {
	endpoint := fmt.Sprintf("https://%s/oauth/token", a.domain)
	interval := time.Second * time.Duration(dcr.Interval)
	if interval <= 0 {
		interval = 5 * time.Second
	}

	for {
		form := url.Values{}
		form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		form.Set("device_code", dcr.DeviceCode)
		form.Set("client_id", a.clientID)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := a.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("error making request for token: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		var tr tokenResp
		if err := json.Unmarshal(body, &tr); err != nil {
			return fmt.Errorf("cannot unmarshal token response: %w", err)
		}

		switch tr.Error {
		case "":
			a.AccessToken = tr.AccessToken
			a.RefreshToken = tr.RefreshToken
			a.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
			return nil
		case "authorization_pending":
			select {
			case <-time.After(interval):
			case <-ctx.Done():
				return ctx.Err()
			}
		case "slow_down":
			interval += time.Second
			select {
			case <-time.After(interval):
			case <-ctx.Done():
				return ctx.Err()
			}
		case "access_denied":
			return errors.New("authentication was denied or cancelled. Please try again.")
		case "expired_token":
			return errors.New("the authentication request expired. Please try again.")
		default:
			if tr.ErrorDescription != "" {
				return fmt.Errorf("authentication failed (%s): %s", tr.Error, tr.ErrorDescription)
			}
			return fmt.Errorf("authentication failed: %s", tr.Error)
		}
	}
}

// FetchUserEmail fetches the user's email from the Auth0 userinfo endpoint.
// The authenticator must have a valid AccessToken set.
func (a *Authenticator) FetchUserEmail(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s/userinfo", a.domain), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.AccessToken)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("userinfo http %d: %s", resp.StatusCode, string(b))
	}
	var payload struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return fmt.Errorf("cannot unmarshal userinfo: %w", err)
	}
	a.Email = payload.Email
	return nil
}

func (a *Authenticator) refreshAccessToken(ctx context.Context) error {
	if a.RefreshToken == "" {
		return errors.New("no refresh_token available")
	}

	endpoint := fmt.Sprintf("https://%s/oauth/token", a.domain)
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", a.clientID)
	form.Set("refresh_token", a.RefreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error requesting refresh token endpoint %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh http %d: %s", resp.StatusCode, string(body))
	}

	var tr tokenResp
	if err := json.Unmarshal(body, &tr); err != nil {
		return fmt.Errorf("could not unmarshal refresh token response: %w", err)
	}

	a.AccessToken = tr.AccessToken
	if tr.ExpiresIn > 0 {
		a.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	if tr.RefreshToken != "" {
		a.RefreshToken = tr.RefreshToken
	}

	return nil
}
