package soulbits

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/harmony-ai-solutions/soulbits-api-client-go/gen"
)

// ConnectSessionRequest holds the optional parameters for a session connect.
type ConnectSessionRequest struct {
	// Version is an optional HL image version.
	Version *string

	// DeviceID is the per-install device identity for the D-DEV-04
	// authorization gate. The broker refuses to provision for unauthorized
	// devices — send the device UUID here so the gate can check it. Absent
	// or unknown device → 403 device_authorization_required.
	DeviceID *string
}

// DeviceAuthRequiredError indicates the broker returned 403
// device_authorization_required. The caller must complete the email
// auth-code flow (POST /v1/devices/authorize) and retry connect.
type DeviceAuthRequiredError struct{}

func (e *DeviceAuthRequiredError) Error() string { return "device_authorization_required" }

// ConnectSessionResult wraps the session connect response with additional context.
type ConnectSessionResult struct {
	*gen.SessionConnectResponse

	// RetryAfterMs is set when status is "provisioning" and indicates
	// milliseconds to wait before polling again.
	RetryAfterMs *int
}

// IsProvisioning returns true if the session is still being provisioned.
func (r *ConnectSessionResult) IsProvisioning() bool {
	return r != nil && r.Status == gen.SessionConnectResponseStatusProvisioning
}

// IsReady returns true if the session is ready or active (has a proxy endpoint).
func (r *ConnectSessionResult) IsReady() bool {
	return r != nil && (r.Status == gen.SessionConnectResponseStatusReady ||
		r.Status == gen.SessionConnectResponseStatusActive)
}

// IsFailed returns true if the session provisioning failed.
func (r *ConnectSessionResult) IsFailed() bool {
	return r != nil && r.Status == gen.SessionConnectResponseStatusFailed
}

// ConnectSession acquires a session lock and assigns a warm-pool HL container.
//
// Returns the current session state which may be:
//   - provisioning (202) — poll with backoff using retry_after_ms
//   - ready or active (200) — proxy_endpoint is available
//   - failed (503) — inspect failure_reason for details
//
// A 403 response produces a *DeviceAuthRequiredError — the device must
// complete the email auth-code flow before retrying.
//
// Use ConnectSessionPoll for a convenience method that handles the full polling loop.
func (c *Client) ConnectSession(ctx context.Context, req ConnectSessionRequest) (*ConnectSessionResult, error) {
	body := gen.PostV1SessionConnectJSONRequestBody(gen.PostV1SessionConnectJSONBody{
		Version:  req.Version,
		DeviceId: req.DeviceID,
	})
	resp, err := c.cloudGen.PostV1SessionConnectWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("connect session request: %w", err)
	}

	switch resp.HTTPResponse.StatusCode {
	case http.StatusOK:
		// Session is ready or active.
		if resp.JSON200 == nil {
			return nil, fmt.Errorf("connect session: 200 response with empty body")
		}
		return &ConnectSessionResult{
			SessionConnectResponse: resp.JSON200,
		}, nil

	case http.StatusAccepted:
		// Session is provisioning.
		if resp.JSON202 == nil {
			return nil, fmt.Errorf("connect session: 202 response with empty body")
		}
		result := &ConnectSessionResult{
			SessionConnectResponse: resp.JSON202,
		}
		if resp.JSON202.RetryAfterMs != nil {
			result.RetryAfterMs = resp.JSON202.RetryAfterMs
		}
		return result, nil

	case http.StatusForbidden:
		// D-DEV-04 device authorization gate — terminal, not retryable.
		return nil, &DeviceAuthRequiredError{}

	case http.StatusConflict:
		// A data purge is in progress for this user — connecting is blocked
		// until the purge completes. Terminal, not retryable by the poll loop.
		if resp.JSON409 != nil && resp.JSON409.Error != nil &&
			*resp.JSON409.Error == gen.PurgeInProgress {
			retry := 0
			if resp.JSON409.RetryAfterMs != nil {
				retry = *resp.JSON409.RetryAfterMs
			}
			return nil, &PurgeInProgressError{RetryAfterMs: retry}
		}
		// Unmapped 409 body — fall through to the generic error path.
		return nil, responseError(resp.Body, resp.HTTPResponse)

	case http.StatusServiceUnavailable:
		// Session provisioning failed.
		if resp.JSON503 == nil {
			return nil, fmt.Errorf("connect session: 503 response with empty body")
		}
		return &ConnectSessionResult{
			SessionConnectResponse: resp.JSON503,
		}, nil

	default:
		// 401, 429, or other errors.
		return nil, responseError(resp.Body, resp.HTTPResponse)
	}
}

// ConnectSessionPollConfig controls the polling behavior for ConnectSessionPoll.
type ConnectSessionPollConfig struct {
	// MaxRetries is the maximum number of polling attempts (default: 30).
	MaxRetries int

	// TotalTimeout is the maximum total time spent polling (default: 5 minutes).
	TotalTimeout time.Duration

	// MinBackoff is the minimum backoff between polls. The actual delay is
	// max(retry_after_ms, min_backoff) (default: 1 second).
	MinBackoff time.Duration

	// MaxBackoff is the maximum backoff between polls (default: 30 seconds).
	MaxBackoff time.Duration
}

// DefaultConnectSessionPollConfig returns the default polling configuration.
func DefaultConnectSessionPollConfig() ConnectSessionPollConfig {
	return ConnectSessionPollConfig{
		MaxRetries:   30,
		TotalTimeout: 5 * time.Minute,
		MinBackoff:   1 * time.Second,
		MaxBackoff:   30 * time.Second,
	}
}

// ConnectSessionPoll connects to a session and polls until ready, active, or
// failed. It uses the retry_after_ms value from the server for backoff between
// polls. Uses default configuration (max 30 retries, 5 minute total timeout).
//
// A *DeviceAuthRequiredError (403) terminates polling immediately — the device
// must complete the email auth-code flow before retrying.
func (c *Client) ConnectSessionPoll(ctx context.Context, req ConnectSessionRequest) (*ConnectSessionResult, error) {
	return c.ConnectSessionPollWithConfig(ctx, req, DefaultConnectSessionPollConfig())
}

// ConnectSessionPollWithConfig connects to a session and polls until ready,
// active, or failed with the given configuration.
func (c *Client) ConnectSessionPollWithConfig(ctx context.Context, req ConnectSessionRequest, cfg ConnectSessionPollConfig) (*ConnectSessionResult, error) {
	if cfg.TotalTimeout <= 0 {
		cfg.TotalTimeout = 5 * time.Minute
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 30
	}
	if cfg.MinBackoff <= 0 {
		cfg.MinBackoff = 1 * time.Second
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.TotalTimeout)
	defer cancel()

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		result, err := c.ConnectSession(ctx, req)
		if err != nil {
			// A *PurgeInProgressError is terminal for the poll loop —
			// connecting is blocked until the purge completes, so polling
			// cannot make progress. Return it immediately.
			var purgeErr *PurgeInProgressError
			if errors.As(err, &purgeErr) {
				return nil, err
			}
			return nil, err
		}

		switch {
		case result.IsReady():
			return result, nil

		case result.IsFailed():
			return result, fmt.Errorf("session provisioning failed: %s",
				coalesceStr(result.FailureReason, "unknown error"))

		case result.IsProvisioning():
			if attempt == cfg.MaxRetries {
				return result, fmt.Errorf("session provisioning timed out after %d attempts", attempt)
			}

			// Determine delay: use server's retry_after_ms, clamped to bounds.
			delay := cfg.MinBackoff
			if result.RetryAfterMs != nil && *result.RetryAfterMs > 0 {
				serverDelay := time.Duration(*result.RetryAfterMs) * time.Millisecond
				if serverDelay > delay {
					delay = serverDelay
				}
			}
			if delay > cfg.MaxBackoff {
				delay = cfg.MaxBackoff
			}

			select {
			case <-ctx.Done():
				return result, fmt.Errorf("session provisioning context cancelled: %w", ctx.Err())
			case <-time.After(delay):
				// Continue polling.
			}

		default:
			return result, fmt.Errorf("unexpected session status: %s", result.Status)
		}
	}

	return nil, fmt.Errorf("session provisioning failed after %d retries", cfg.MaxRetries)
}

// PurgeInProgressError indicates a data purge is active for the user
// (connect is blocked / delete is mid-flight on another request).
type PurgeInProgressError struct {
	// RetryAfterMs is the server-recommended delay before retrying.
	RetryAfterMs int
}

func (e *PurgeInProgressError) Error() string { return "purge_in_progress" }

// SnapshotBusyError indicates the snapshot lease was contended — retry shortly.
type SnapshotBusyError struct {
	// RetryAfterMs is the server-recommended delay before retrying.
	RetryAfterMs int
}

func (e *SnapshotBusyError) Error() string { return "snapshot_busy" }

// ConfirmationRequiredError indicates the purge confirmation string was
// missing or wrong — DeleteSessionDataRequest.Confirm must be "DELETE".
type ConfirmationRequiredError struct{}

func (e *ConfirmationRequiredError) Error() string { return "confirmation_required" }

// DeleteSessionDataRequest holds parameters for the data purge.
type DeleteSessionDataRequest struct {
	// Confirm is the typed confirmation guard — must be "DELETE".
	Confirm string
}

// DeleteSessionDataResult wraps the purge response.
type DeleteSessionDataResult struct {
	*gen.SessionDataDeleteResponse
}

// IsInProgress returns true if a purge is currently running for the user
// (a concurrent request owns it — the caller may wait or retry later).
func (r *DeleteSessionDataResult) IsInProgress() bool {
	return r != nil && r.Status == gen.InProgress
}

// IsDeleted returns true if the purge completed and the user's cloud-side
// engine data has been permanently removed.
func (r *DeleteSessionDataResult) IsDeleted() bool {
	return r != nil && r.Status == gen.Deleted
}

// DeleteSessionData purges the user's cloud-side engine data (hard-kills any
// live session, deletes ALL S3 versions, beat schedule, and DEK).
// Idempotent; returns *PurgeInProgressError (200 in_progress or 409) and
// *SnapshotBusyError (409) for typed retry handling.
func (c *Client) DeleteSessionData(ctx context.Context, req DeleteSessionDataRequest) (*DeleteSessionDataResult, error) {
	body := gen.DeleteSessionDataJSONRequestBody(gen.DeleteSessionDataJSONBody{
		Confirm: gen.DeleteSessionDataJSONBodyConfirm(req.Confirm),
	})
	resp, err := c.cloudGen.DeleteSessionDataWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("delete session data request: %w", err)
	}

	switch resp.HTTPResponse.StatusCode {
	case http.StatusOK:
		// Both status values ("deleted" and "in_progress") map to a success
		// result with a nil error — callers use IsInProgress()/IsDeleted().
		if resp.JSON200 == nil {
			return nil, fmt.Errorf("delete session data: 200 response with empty body")
		}
		return &DeleteSessionDataResult{
			SessionDataDeleteResponse: resp.JSON200,
		}, nil

	case http.StatusBadRequest:
		// Missing or wrong confirmation string.
		return nil, &ConfirmationRequiredError{}

	case http.StatusConflict:
		// Snapshot lease contended (snapshot_busy) or purge mid-flight
		// (purge_in_progress) — both carry retry_after_ms.
		if resp.JSON409 == nil || resp.JSON409.Error == nil {
			return nil, responseError(resp.Body, resp.HTTPResponse)
		}
		retry := 0
		if resp.JSON409.RetryAfterMs != nil {
			retry = *resp.JSON409.RetryAfterMs
		}
		switch *resp.JSON409.Error {
		case gen.SnapshotBusy:
			return nil, &SnapshotBusyError{RetryAfterMs: retry}
		case "purge_in_progress":
			// Returning this on 409 keeps parity with connect's 409 handling
			// for callers that issue concurrent purges.
			return nil, &PurgeInProgressError{RetryAfterMs: retry}
		}
		// Unmapped 409 body — fall through to the generic error path.
		return nil, responseError(resp.Body, resp.HTTPResponse)

	default:
		return nil, responseError(resp.Body, resp.HTTPResponse)
	}
}

// coalesceStr returns the first non-empty string, or the fallback.
func coalesceStr(s *string, fallback string) string {
	if s != nil && *s != "" {
		return *s
	}
	return fallback
}

// DisconnectSession transitions the session to a 5-minute grace period.
func (c *Client) DisconnectSession(ctx context.Context, sessionID *string, reason *string) (*gen.SessionDisconnectResponse, error) {
	body := gen.PostV1SessionDisconnectJSONRequestBody(gen.PostV1SessionDisconnectJSONBody{
		Reason: reason,
	})
	if sessionID != nil {
		uid, err := uuid.Parse(*sessionID)
		if err != nil {
			return nil, fmt.Errorf("invalid session id %q: %w", *sessionID, err)
		}
		// We need to rebuild the body with the session_id set.
		body = gen.PostV1SessionDisconnectJSONRequestBody(gen.PostV1SessionDisconnectJSONBody{
			SessionId: &uid,
			Reason:    reason,
		})
	}
	resp, err := c.cloudGen.PostV1SessionDisconnectWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("disconnect session request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ReportConnected reports that the WebSocket upgrade has completed.
// Called by the conduct proxy; typically not called by client code directly.
func (c *Client) ReportConnected(ctx context.Context) (*gen.SessionConnectedResponse, error) {
	resp, err := c.cloudGen.PostV1SessionConnectedWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("report connected request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// Versions lists available Harmony Link image versions.
func (c *Client) Versions(ctx context.Context) (*gen.VersionsResponse, error) {
	resp, err := c.cloudGen.GetV1VersionsWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("versions request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}
