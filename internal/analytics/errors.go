package analytics

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
)

// Error categories - never log raw error messages (may contain PII)
const (
	ErrorTypeConnectionTimeout = "connection_timeout"
	ErrorTypeAuthFailed        = "auth_failed"
	ErrorTypeConfigInvalid     = "config_invalid"
	ErrorTypeFileNotFound      = "file_not_found"
	ErrorTypeServiceError      = "service_error"
	ErrorTypeUserCancelled     = "user_cancelled"
	ErrorTypeUnknown           = "unknown"
)

// CategorizeError returns a safe category string for an error
// Never returns the actual error message to avoid leaking PII
func CategorizeError(err error) string {
	if err == nil {
		return ""
	}

	errStr := strings.ToLower(err.Error())

	// Check for context cancellation (user cancelled)
	if errors.Is(err, context.Canceled) {
		return ErrorTypeUserCancelled
	}

	// Check for context deadline (timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorTypeConnectionTimeout
	}

	// Check for file not found
	if os.IsNotExist(err) {
		return ErrorTypeFileNotFound
	}

	// Check for network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return ErrorTypeConnectionTimeout
		}
		return ErrorTypeServiceError
	}

	// String-based categorization (fallback)
	switch {
	case strings.Contains(errStr, "timeout"):
		return ErrorTypeConnectionTimeout
	case strings.Contains(errStr, "deadline exceeded"):
		return ErrorTypeConnectionTimeout
	case strings.Contains(errStr, "connection refused"):
		return ErrorTypeServiceError
	case strings.Contains(errStr, "no such host"):
		return ErrorTypeServiceError
	case strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized"):
		return ErrorTypeAuthFailed
	case strings.Contains(errStr, "403") || strings.Contains(errStr, "forbidden"):
		return ErrorTypeAuthFailed
	case strings.Contains(errStr, "authentication"):
		return ErrorTypeAuthFailed
	case strings.Contains(errStr, "config") || strings.Contains(errStr, "yaml") || strings.Contains(errStr, "json"):
		return ErrorTypeConfigInvalid
	case strings.Contains(errStr, "not found") || strings.Contains(errStr, "no such file"):
		return ErrorTypeFileNotFound
	case strings.Contains(errStr, "cancelled") || strings.Contains(errStr, "canceled"):
		return ErrorTypeUserCancelled
	case strings.Contains(errStr, "interrupt"):
		return ErrorTypeUserCancelled
	default:
		return ErrorTypeUnknown
	}
}
