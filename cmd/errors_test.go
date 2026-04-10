package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Use-Tusk/tusk-cli/internal/api"
	"github.com/stretchr/testify/require"
)

func TestCapitalizeFirst(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty string", input: "", expected: ""},
		{name: "lowercase first letter", input: "hello world", expected: "Hello world"},
		{name: "already uppercase", input: "Hello world", expected: "Hello world"},
		{name: "single lowercase", input: "a", expected: "A"},
		{name: "single uppercase", input: "A", expected: "A"},
		{name: "non-alpha first char", input: "1abc", expected: "1abc"},
		{name: "special char first", input: "!hello", expected: "!hello"},
		{name: "all lowercase", input: "abc", expected: "Abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capitalizeFirst(tt.input)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestFormatApiError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		expectedNil bool
		expectedMsg string
	}{
		{
			name:        "nil error returns nil",
			err:         nil,
			expectedNil: true,
		},
		{
			name:        "non-API error passes through unchanged",
			err:         errors.New("some generic error"),
			expectedMsg: "some generic error",
		},
		{
			name: "401 returns unauthorized message",
			err: &api.ApiError{
				StatusCode: 401,
				Message:    "unauthorized",
			},
			expectedMsg: fmt.Sprintf("Not authorized. Your credentials may be expired or invalid.\nRun `tusk auth login` or set TUSK_API_KEY.\nGet started: %s", api.DocsSetupURL),
		},
		{
			name: "403 returns unauthorized message",
			err: &api.ApiError{
				StatusCode: 403,
				Message:    "forbidden",
			},
			expectedMsg: fmt.Sprintf("Not authorized. Your credentials may be expired or invalid.\nRun `tusk auth login` or set TUSK_API_KEY.\nGet started: %s", api.DocsSetupURL),
		},
		{
			name: "404 with message uses capitalized message",
			err: &api.ApiError{
				StatusCode: 404,
				Message:    "resource not available",
			},
			expectedMsg: "Resource not available.",
		},
		{
			name: "404 with empty message uses default",
			err: &api.ApiError{
				StatusCode: 404,
				Message:    "",
			},
			expectedMsg: "Resource not found.",
		},
		{
			name: "500 returns service error",
			err: &api.ApiError{
				StatusCode: 500,
				Message:    "internal error",
			},
			expectedMsg: "Tusk service error (HTTP 500). Please try again.\nIf the issue persists, please contact support@usetusk.ai.",
		},
		{
			name: "503 returns service error with correct status code",
			err: &api.ApiError{
				StatusCode: 503,
				Message:    "service unavailable",
			},
			expectedMsg: "Tusk service error (HTTP 503). Please try again.\nIf the issue persists, please contact support@usetusk.ai.",
		},
		{
			name: "other status with message returns message with status code",
			err: &api.ApiError{
				StatusCode: 422,
				Message:    "validation failed",
			},
			expectedMsg: "validation failed (HTTP 422)",
		},
		{
			name: "other status with empty message returns original error",
			err: &api.ApiError{
				StatusCode: 400,
				Message:    "",
				RawBody:    "bad request body",
			},
			expectedMsg: "http 400: bad request body",
		},
		{
			name:        "wrapped API error is unwrapped correctly",
			err:         fmt.Errorf("wrapped: %w", &api.ApiError{StatusCode: 401, Message: "unauth"}),
			expectedMsg: fmt.Sprintf("Not authorized. Your credentials may be expired or invalid.\nRun `tusk auth login` or set TUSK_API_KEY.\nGet started: %s", api.DocsSetupURL),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatApiError(tt.err)
			if tt.expectedNil {
				require.NoError(t, got)
				return
			}
			require.Error(t, got)
			require.Equal(t, tt.expectedMsg, got.Error())
		})
	}
}
