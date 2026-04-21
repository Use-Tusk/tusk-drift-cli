package cmd

import (
	"errors"
	"fmt"

	"github.com/Use-Tusk/tusk-cli/internal/api"
)

// ExitCodeError wraps an error with a specific process exit code. main.go
// unwraps this to pick the right os.Exit value; without it, Cobra-returned
// errors map to exit 1.
type ExitCodeError struct {
	Code int
	Err  error
}

func (e *ExitCodeError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ExitCodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ExitCodeOf returns the exit code embedded in err (or any wrapper in its
// chain), defaulting to 1 if none is present.
func ExitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var ec *ExitCodeError
	if errors.As(err, &ec) {
		return ec.Code
	}
	return 1
}

// formatApiError converts raw API errors into user-friendly messages with
// actionable guidance. Non-API errors pass through unchanged.
func formatApiError(err error) error {
	if err == nil {
		return nil
	}

	var apiErr *api.ApiError
	if !errors.As(err, &apiErr) {
		return err
	}

	switch {
	case apiErr.StatusCode == 401 || apiErr.StatusCode == 403:
		return fmt.Errorf("Not authorized. Your credentials may be expired or invalid.\nRun `tusk auth login` or set TUSK_API_KEY.\nGet started: %s", api.DocsSetupURL)
	case apiErr.StatusCode == 404:
		msg := "Resource not found"
		if apiErr.Message != "" {
			msg = capitalizeFirst(apiErr.Message)
		}
		return fmt.Errorf("%s.", msg)
	case apiErr.StatusCode >= 500:
		return fmt.Errorf("Tusk service error (HTTP %d). Please try again.\nIf the issue persists, please contact support@usetusk.ai.", apiErr.StatusCode)
	default:
		if apiErr.Message != "" {
			return fmt.Errorf("%s (HTTP %d)", apiErr.Message, apiErr.StatusCode)
		}
		return err
	}
}

func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-32) + s[1:]
	}
	return s
}
