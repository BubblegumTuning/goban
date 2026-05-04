package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	gerr "goban-cli/internal/errors"
)

// APIError represents an error returned from the Goban API
type APIError struct {
	StatusCode int
	Message    string
	Body       []byte // Raw response body for debugging
}

// Error implements the error interface
func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Message)
}

// IsAuthError returns true if this is an authentication error (401/403)
func (e *APIError) IsAuthError() bool {
	return e.StatusCode == 401 || e.StatusCode == 403
}

// NewAPIError creates an APIError from status code and response body
func NewAPIError(statusCode int, body []byte) *APIError {
	err := &APIError{
		StatusCode: statusCode,
		Body:       body,
	}

	// Try standard keys first
	var errorResp struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &errorResp) == nil {
		if errorResp.Error != "" {
			err.Message = errorResp.Error
			return err
		}
		if errorResp.Message != "" {
			err.Message = errorResp.Message
			return err
		}
	}

	// Try common alternative keys
	var alt struct {
		Detail      string `json:"detail"`
		Msg         string `json:"msg"`
		Description string `json:"description"`
	}
	if json.Unmarshal(body, &alt) == nil {
		if alt.Detail != "" {
			err.Message = alt.Detail
			return err
		}
		if alt.Msg != "" {
			err.Message = alt.Msg
			return err
		}
		if alt.Description != "" {
			err.Message = alt.Description
			return err
		}
	}

	// Fall back to status code message if no JSON message found
	err.Message = httpStatusMessage(statusCode)
	return err
}

// httpStatusMessage returns a human-readable message for common HTTP status codes
func httpStatusMessage(code int) string {
	messages := map[int]string{
		400: "Bad request",
		401: "Authentication failed - invalid or missing API token",
		403: "Forbidden - insufficient permissions",
		404: "Not found",
		409: "Conflict",
		422: "Unprocessable entity",
		500: "Internal server error",
		502: "Bad gateway",
		503: "Service unavailable",
	}
	if msg, ok := messages[code]; ok {
		return msg
	}
	return fmt.Sprintf("HTTP %d", code)
}

// Classify maps a raw error to a ClassifiedError based on its type and content.
// It inspects APIError status codes, detects network errors via string matching,
// handles context cancellation, and falls back to user errors for unknown types.
func Classify(err error, contextStr string) *gerr.ClassifiedError {
	if err == nil {
		return nil
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode >= 500:
			return gerr.NewServerError(
				fmt.Sprintf("Server error (HTTP %d): %s", apiErr.StatusCode, apiErr.Message),
				contextStr)
		case apiErr.IsAuthError():
			return gerr.NewAuthError(
				fmt.Sprintf("Authentication failed: %s. Check your API token in config.yaml.", apiErr.Message),
				contextStr)
		case apiErr.StatusCode == 404:
			return gerr.NewNotFoundError(
				fmt.Sprintf("Not found (HTTP 404): %s", apiErr.Message),
				contextStr)
		default:
			return gerr.NewUserError(
				fmt.Sprintf("API error (HTTP %d): %s", apiErr.StatusCode, apiErr.Message),
				contextStr)
		}
	}

	// Detect network errors by common substrings in the error message
	errMsg := err.Error()
	for _, kw := range []string{"refused", "timeout", "dial", "no such host", "connection reset"} {
		if strings.Contains(strings.ToLower(errMsg), kw) {
			return gerr.NewNetworkError(
				fmt.Sprintf("Network error: %s", errMsg),
				contextStr)
		}
	}

	// Detect context cancellation
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return gerr.NewUserError("Operation cancelled", contextStr)
	}

	// Fallback: treat as user error
	return gerr.NewUserError(errMsg, contextStr)
}
