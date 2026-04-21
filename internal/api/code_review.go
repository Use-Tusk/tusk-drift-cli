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
		}
		return "", fmt.Errorf("%s: %s", e.GetCode().String(), e.GetMessage())
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
		return nil, fmt.Errorf("%s: %s", e.GetCode().String(), e.GetMessage())
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
		return fmt.Errorf("%s: %s", e.GetCode().String(), e.GetMessage())
	}
	return fmt.Errorf("invalid response")
}
