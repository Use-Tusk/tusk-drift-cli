package api

import (
	"context"
	"errors"
	"fmt"

	backend "github.com/Use-Tusk/tusk-drift-schemas/generated/go/backend"
)

// RateLimitError is returned by CreateLocalCodeReviewRun when the backend
// indicates a per-user rate limit has been hit. Carries the human-readable
// message (for surfacing verbatim) and an ISO-8601 retry time when available.
type RateLimitError struct {
	Message           string
	RetryAfterIso8601 string
}

func (e *RateLimitError) Error() string { return e.Message }

// IsRateLimitError reports whether err (or anything it wraps) is a *RateLimitError.
func IsRateLimitError(err error) bool {
	var r *RateLimitError
	return errors.As(err, &r)
}

// RepoNotFoundError means the (owner/name) isn't connected to the caller's org.
// Surfaced as a distinct type so the CLI can show the onboarding link.
type RepoNotFoundError struct {
	Message string
}

func (e *RepoNotFoundError) Error() string { return e.Message }

// IsRepoNotFoundError reports whether err (or anything it wraps) is a *RepoNotFoundError.
func IsRepoNotFoundError(err error) bool {
	var r *RepoNotFoundError
	return errors.As(err, &r)
}

// PatchInvalidError signals the backend rejected the uploaded patch
// (bad bytes, empty after filtering, etc.).
type PatchInvalidError struct {
	Message string
}

func (e *PatchInvalidError) Error() string { return e.Message }

// IsPatchInvalidError reports whether err (or anything it wraps) is a *PatchInvalidError.
func IsPatchInvalidError(err error) bool {
	var p *PatchInvalidError
	return errors.As(err, &p)
}

// NoSeatError signals the caller isn't entitled to run a review. The
// backend tailors the message per cause (JWT without linked code-hosting
// username; API key with no PR on the branch; user with no active seat).
// The CLI renders the message verbatim.
type NoSeatError struct {
	Message string
}

func (e *NoSeatError) Error() string { return e.Message }

// IsNoSeatError reports whether err (or anything it wraps) is a *NoSeatError.
func IsNoSeatError(err error) bool {
	var n *NoSeatError
	return errors.As(err, &n)
}

// NotAuthorizedError signals the caller's org (or client plan) doesn't
// have the code-review feature enabled. Distinct from NoSeatError — no
// per-user remediation; requires a plan / admin change. Backend-supplied
// message is rendered verbatim.
type NotAuthorizedError struct {
	Message string
}

func (e *NotAuthorizedError) Error() string { return e.Message }

// IsNotAuthorizedError reports whether err (or anything it wraps) is a *NotAuthorizedError.
func IsNotAuthorizedError(err error) bool {
	var n *NotAuthorizedError
	return errors.As(err, &n)
}

func (c *TuskClient) CreateLocalCodeReviewRun(ctx context.Context, in *backend.CreateLocalCodeReviewRunRequest, auth AuthOptions) (string, error) {
	var out backend.CreateLocalCodeReviewRunResponse
	if err := c.makeCodeReviewServiceRequest(ctx, "create_local_code_review_run", in, &out, auth, DefaultRetryConfig(3)); err != nil {
		return "", err
	}

	if s := out.GetSuccess(); s != nil {
		return s.GetRunId(), nil
	}
	if e := out.GetError(); e != nil {
		switch e.GetCode() {
		case backend.CreateLocalCodeReviewRunResponseErrorCode_CREATE_LOCAL_CODE_REVIEW_RUN_RESPONSE_ERROR_CODE_RATE_LIMITED:
			return "", &RateLimitError{
				Message:           e.GetMessage(),
				RetryAfterIso8601: e.GetRetryAfterIso8601(),
			}
		case backend.CreateLocalCodeReviewRunResponseErrorCode_CREATE_LOCAL_CODE_REVIEW_RUN_RESPONSE_ERROR_CODE_REPO_NOT_FOUND:
			return "", &RepoNotFoundError{Message: e.GetMessage()}
		case backend.CreateLocalCodeReviewRunResponseErrorCode_CREATE_LOCAL_CODE_REVIEW_RUN_RESPONSE_ERROR_CODE_PATCH_INVALID:
			return "", &PatchInvalidError{Message: e.GetMessage()}
		case backend.CreateLocalCodeReviewRunResponseErrorCode_CREATE_LOCAL_CODE_REVIEW_RUN_RESPONSE_ERROR_CODE_NO_SEAT:
			return "", &NoSeatError{Message: e.GetMessage()}
		case backend.CreateLocalCodeReviewRunResponseErrorCode_CREATE_LOCAL_CODE_REVIEW_RUN_RESPONSE_ERROR_CODE_NOT_AUTHORIZED:
			return "", &NotAuthorizedError{Message: e.GetMessage()}
		}
		// Fallback for unmapped codes: surface the backend's human-readable
		// message only. The proto enum name is not user-facing.
		return "", fmt.Errorf("%s", e.GetMessage())
	}
	return "", fmt.Errorf("invalid response")
}

func (c *TuskClient) GetCodeReviewRunStatus(ctx context.Context, in *backend.GetCodeReviewRunStatusRequest, auth AuthOptions) (*backend.GetCodeReviewRunStatusResponseSuccess, error) {
	var out backend.GetCodeReviewRunStatusResponse
	if err := c.makeCodeReviewServiceRequest(ctx, "get_code_review_run_status", in, &out, auth, DefaultRetryConfig(3)); err != nil {
		return nil, err
	}

	if s := out.GetSuccess(); s != nil {
		return s, nil
	}
	if e := out.GetError(); e != nil {
		// Surface only the backend's human-readable message; the proto enum
		// name is not user-facing.
		return nil, fmt.Errorf("%s", e.GetMessage())
	}
	return nil, fmt.Errorf("invalid response")
}

func (c *TuskClient) CancelCodeReviewRun(ctx context.Context, in *backend.CancelCodeReviewRunRequest, auth AuthOptions) error {
	var out backend.CancelCodeReviewRunResponse
	if err := c.makeCodeReviewServiceRequest(ctx, "cancel_code_review_run", in, &out, auth, DefaultRetryConfig(0)); err != nil {
		return err
	}

	if s := out.GetSuccess(); s != nil {
		_ = s
		return nil
	}
	if e := out.GetError(); e != nil {
		// Surface only the backend's human-readable message; the proto enum
		// name is not user-facing.
		return fmt.Errorf("%s", e.GetMessage())
	}
	return fmt.Errorf("invalid response")
}
