package api

import (
	"encoding/json"
	"fmt"
)

const DocsSetupURL = "https://docs.usetusk.ai/onboarding"

// ApiError represents a non-2xx HTTP response from the Tusk API.
// Consumers can use errors.As to extract the status code and message.
type ApiError struct {
	StatusCode int
	Message    string
	RawBody    string
}

func (e *ApiError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("http %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("http %d: %s", e.StatusCode, e.RawBody)
}

func newApiError(statusCode int, body []byte) *ApiError {
	return &ApiError{
		StatusCode: statusCode,
		Message:    extractJSONErrorMessage(body),
		RawBody:    string(body),
	}
}

func extractJSONErrorMessage(body []byte) string {
	var parsed struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		if parsed.Error != "" {
			return parsed.Error
		}
		if parsed.Message != "" {
			return parsed.Message
		}
	}
	return ""
}
