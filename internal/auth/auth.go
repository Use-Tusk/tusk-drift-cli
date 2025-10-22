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

	a := &Authenticator{
		authFilePath:   authPath,
		httpClient:     &http.Client{Timeout: 15 * time.Second},
		domain:         utils.EnvDefault("TUSK_AUTH0_DOMAIN", "tusk.us.auth0.com"),
		scope:          utils.EnvDefault("TUSK_AUTH0_SCOPE", "openid email offline_access"),
		clientID:       utils.EnvDefault("TUSK_AUTH0_CLIENT_ID", "gXktT8e38sBmmXGWCGeXMLpwlpeECJS5"),
		audience:       utils.EnvDefault("TUSK_AUTH0_AUDIENCE", "drift-cli"),
		ignoreSaveFile: os.Getenv("TUSK_AUTH_IGNORE_SAVED") != "",
	}
	if a.clientID == "" {
		return a, errors.New("TUSK_AUTH0_CLIENT_ID is required for login")
	}
	if a.audience == "" {
		return a, errors.New("TUSK_AUTH0_AUDIENCE is required for login")
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

func (a *Authenticator) saveTokenFile() error {
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
			if err := a.refreshAccessToken(c); err != nil {
				return err
			}
			if err := a.saveTokenFile(); err != nil {
				return fmt.Errorf("write token file: %w", err)
			}
			return nil
		}
	}

	dcr, err := a.auth0RequestDeviceCode(c)
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
	browserURL := dcr.VerificationURIComplete
	if browserURL == "" {
		browserURL = dcr.VerificationURI
	}
	_ = openBrowser(browserURL)

	ctx, cancel := context.WithTimeout(c, time.Duration(dcr.ExpiresIn)*time.Second)
	defer cancel()

	if err = a.auth0PollForToken(ctx, dcr); err != nil {
		return fmt.Errorf("polling token failed: %w", err)
	}

	if err = a.fetchUserEmail(c); err != nil {
		return err
	}

	if err := a.saveTokenFile(); err != nil {
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
		if err := a.saveTokenFile(); err != nil {
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

type deviceCodeResp struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
	VerificationURIComplete string `json:"verification_uri_complete"`

	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
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

func (a *Authenticator) auth0RequestDeviceCode(ctx context.Context) (deviceCodeResp, error) {
	form := url.Values{}
	form.Set("client_id", a.clientID)
	if a.scope != "" {
		form.Set("scope", a.scope)
	}
	if a.audience != "" {
		form.Set("audience", a.audience)
	}

	var dcr deviceCodeResp

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

func (a *Authenticator) auth0PollForToken(ctx context.Context, dcr deviceCodeResp) error {
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
		default:
			if tr.Error != "" {
				return fmt.Errorf("unknown token error %d: %s", resp.StatusCode, string(body))
			}
			a.AccessToken = tr.AccessToken
			a.RefreshToken = tr.RefreshToken
			a.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
			return nil
		}
	}
}

func (a *Authenticator) fetchUserEmail(ctx context.Context) error {
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
