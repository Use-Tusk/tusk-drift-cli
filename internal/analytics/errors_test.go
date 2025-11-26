package analytics

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestCategorizeError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			expected: ErrorTypeUserCancelled,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: ErrorTypeConnectionTimeout,
		},
		{
			name:     "file not found",
			err:      os.ErrNotExist,
			expected: ErrorTypeFileNotFound,
		},
		{
			name:     "timeout in message",
			err:      errors.New("connection timeout occurred"),
			expected: ErrorTypeConnectionTimeout,
		},
		{
			name:     "401 unauthorized",
			err:      errors.New("http 401: unauthorized"),
			expected: ErrorTypeAuthFailed,
		},
		{
			name:     "config error",
			err:      errors.New("failed to parse config file"),
			expected: ErrorTypeConfigInvalid,
		},
		{
			name:     "connection refused",
			err:      errors.New("dial tcp: connection refused"),
			expected: ErrorTypeServiceError,
		},
		{
			name:     "unknown error",
			err:      errors.New("something went wrong"),
			expected: ErrorTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CategorizeError(tt.err)
			if result != tt.expected {
				t.Errorf("CategorizeError(%v) = %q, want %q", tt.err, result, tt.expected)
			}
		})
	}
}
