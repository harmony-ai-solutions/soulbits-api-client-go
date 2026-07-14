package soulbits

import (
	"encoding/json"
	"fmt"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"
)

// APIError represents an error returned by the Soulbits API.
// It implements the error interface and supports errors.As for type checking.
type APIError struct {
	StatusCode int
	Code       string // error code from the API ("rate_limit", "quota_exceeded", etc.)
	Message    string // human-readable message

	// Optional fields present on 402/504 errors
	UpgradeURL           *string
	CurrentTier          *string
	RequiredTier         *string
	SoulCreditsAvailable *int
	TaskID               *string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("soulbits API error %d: %s — %s", e.StatusCode, e.Code, e.Message)
}

// IsStatusCode returns true if the error has the given HTTP status code.
func (e *APIError) IsStatusCode(code int) bool {
	return e.StatusCode == code
}

// Common status code checks.
func (e *APIError) IsUnauthorized() bool { return e.StatusCode == 401 }
func (e *APIError) IsForbidden() bool    { return e.StatusCode == 403 }
func (e *APIError) IsNotFound() bool     { return e.StatusCode == 404 }
func (e *APIError) IsRateLimited() bool  { return e.StatusCode == 429 }
func (e *APIError) IsTimeout() bool      { return e.StatusCode == 504 }

// responseError converts a generated response to an *APIError when the
// request failed. Returns nil if status is 2xx.
// It uses the raw Body bytes with parseErrorBody for consistency.
func responseError(body []byte, httpResp *http.Response) error {
	if httpResp == nil {
		return fmt.Errorf("nil HTTP response")
	}
	if httpResp.StatusCode >= 200 && httpResp.StatusCode < 300 {
		return nil
	}
	return parseErrorBody(httpResp.StatusCode, body)
}

// checkOK is a convenience wrapper: if the JSON success field is non-nil it
// returns nil; otherwise calls responseError.
func checkOK[T any](success *T, body []byte, httpResp *http.Response) (*T, error) {
	if success != nil {
		return success, nil
	}
	return nil, responseError(body, httpResp)
}

// parseErrorBody attempts to parse an error response body into an *APIError.
// Handles all three error envelope shapes:
//   - Inference gateway: {"error": "<code>", "message": "<text>"}
//   - Auth-service:      {"error": "<message>"}
//   - Session-broker:    {"error_code": "<code>", "message": "<text>"}
func parseErrorBody(statusCode int, body []byte) *APIError {
	// Try to unmarshal into the generic error envelope.
	var raw struct {
		Error      *string `json:"error"`
		ErrorCode  *string `json:"error_code"`
		Message    *string `json:"message,omitempty"`
		UpgradeURL *string `json:"upgrade_url,omitempty"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return &APIError{
			StatusCode: statusCode,
			Code:       "parse_error",
			Message:    fmt.Sprintf("failed to parse error body: %s", string(body)),
		}
	}

	ae := &APIError{StatusCode: statusCode}

	// Determine the code and message from the envelope shape.
	if raw.ErrorCode != nil {
		// Session-broker shape: {"error_code": "<code>", ...}
		ae.Code = *raw.ErrorCode
	} else if raw.Error != nil {
		// Could be inference shape (error is a code) or auth shape (error is a message).
		// If message is also present, it's inference shape.
		if raw.Message != nil {
			ae.Code = *raw.Error
		} else {
			// Auth shape: error field is actually the message.
			ae.Message = *raw.Error
			ae.Code = fmt.Sprintf("http_%d", statusCode)
		}
	}

	if raw.Message != nil {
		ae.Message = *raw.Message
	}
	if raw.UpgradeURL != nil {
		ae.UpgradeURL = raw.UpgradeURL
	}

	if ae.Code == "" {
		ae.Code = fmt.Sprintf("http_%d", statusCode)
	}
	if ae.Message == "" {
		ae.Message = fmt.Sprintf("HTTP %d", statusCode)
	}

	return ae
}

// openapiEmail converts a plain string to openapi_types.Email.
func openapiEmail(s string) openapi_types.Email {
	return openapi_types.Email(s)
}
