package soulbits

import (
	"context"
	"fmt"
	"net/http"

	"github.com/harmony-ai-solutions/soulbits-api-client-go/gen"
)

// RegisterDevice registers (or refreshes) a client device install.
//
// A new push token refreshes the existing registration. Pass a nil pushToken
// to clear push registration. Devices must complete the email auth-code flow
// (RequestDeviceAuthCode / VerifyDeviceAuthCode) before cloud sessions can be
// provisioned for them — see ConnectSession (403 device_authorization_required).
func (c *Client) RegisterDevice(ctx context.Context, deviceID string, platform gen.RegisterDeviceRequestPlatform, pushToken *string) (*gen.Device, error) {
	if deviceID == "" {
		return nil, fmt.Errorf("device id is required")
	}
	body := gen.RegisterDeviceJSONRequestBody(gen.RegisterDeviceRequest{
		DeviceId:  deviceID,
		Platform:  platform,
		PushToken: pushToken,
	})
	resp, err := c.cloudGen.RegisterDeviceWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("register device request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ListDevices lists the devices registered to the current user.
func (c *Client) ListDevices(ctx context.Context) ([]gen.DeviceListItem, error) {
	resp, err := c.cloudGen.ListDevicesWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("list devices request: %w", err)
	}
	devices, err := checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
	if err != nil {
		return nil, err
	}
	return *devices, nil
}

// RevokeDevice revokes a device's authorization and removes it from the
// user's device list. Idempotent — revoking an unknown device returns 404.
func (c *Client) RevokeDevice(ctx context.Context, deviceID string) error {
	if deviceID == "" {
		return fmt.Errorf("device id is required")
	}
	resp, err := c.cloudGen.RevokeDeviceWithResponse(ctx, deviceID)
	if err != nil {
		return fmt.Errorf("revoke device request: %w", err)
	}
	return responseError(resp.Body, resp.HTTPResponse)
}

// RequestDeviceAuthCode starts the D-DEV-04 device authorization flow: the
// server emails the 6-digit code plus the approval button link and returns
// 202. The caller should then poll GetDeviceAuthorizationStatus until the
// device is authorized (or the user manually enters the code via
// VerifyDeviceAuthCode).
func (c *Client) RequestDeviceAuthCode(ctx context.Context, deviceID string) error {
	if deviceID == "" {
		return fmt.Errorf("device id is required")
	}
	body := gen.AuthorizeDeviceJSONRequestBody(gen.AuthorizeDeviceRequest{DeviceId: deviceID})
	resp, err := c.cloudGen.AuthorizeDeviceWithResponse(ctx, body)
	if err != nil {
		return fmt.Errorf("request device auth code: %w", err)
	}
	_, err = checkOK(resp.JSON202, resp.Body, resp.HTTPResponse)
	return err
}

// VerifyDeviceAuthCode verifies the 6-digit email auth code and marks the
// device authorized. Returns nil on success (200).
func (c *Client) VerifyDeviceAuthCode(ctx context.Context, deviceID, code string) error {
	if deviceID == "" {
		return fmt.Errorf("device id is required")
	}
	if code == "" {
		return fmt.Errorf("device auth code is required")
	}
	body := gen.AuthorizeDeviceJSONRequestBody(gen.AuthorizeDeviceRequest{
		DeviceId: deviceID,
		Code:     &code,
	})
	resp, err := c.cloudGen.AuthorizeDeviceWithResponse(ctx, body)
	if err != nil {
		return fmt.Errorf("verify device auth code: %w", err)
	}
	_, err = checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
	return err
}

// ApproveDevice redeems the high-entropy approval token from the emailed
// approval button URL and returns the approved device ID. The authenticated
// session user must match the user the token was issued for.
//
// Error mapping (all returned as *APIError):
//   - 401 — invalid or expired token
//   - 403 — token belongs to another account
//   - 429 — approval attempt lockout (check APIError.IsRateLimited)
func (c *Client) ApproveDevice(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("device approval token is required")
	}
	body := gen.ApproveDeviceJSONRequestBody(gen.ApproveDeviceRequest{Token: token})
	resp, err := c.cloudGen.ApproveDeviceWithResponse(ctx, body)
	if err != nil {
		return "", fmt.Errorf("approve device request: %w", err)
	}

	switch resp.HTTPResponse.StatusCode {
	case http.StatusOK:
		if resp.JSON200 == nil {
			return "", fmt.Errorf("approve device: 200 response with empty body")
		}
		return resp.JSON200.DeviceId, nil

	case http.StatusUnauthorized:
		return "", &APIError{
			StatusCode: http.StatusUnauthorized,
			Code:       "invalid_token",
			Message:    "invalid or expired token",
		}

	case http.StatusForbidden:
		return "", &APIError{
			StatusCode: http.StatusForbidden,
			Code:       "token_mismatch",
			Message:    "token belongs to another account",
		}

	case http.StatusTooManyRequests:
		return "", &APIError{
			StatusCode: http.StatusTooManyRequests,
			Code:       "device_approval_locked",
			Message:    "too many approval attempts; try again later",
		}

	default:
		return "", responseError(resp.Body, resp.HTTPResponse)
	}
}

// GetDeviceAuthorizationStatus returns the poll state for the device auth
// flow. The app polls until Authorized is true.
func (c *Client) GetDeviceAuthorizationStatus(ctx context.Context, deviceID string) (*gen.DeviceAuthorizationStatus, error) {
	if deviceID == "" {
		return nil, fmt.Errorf("device id is required")
	}
	params := gen.GetDeviceAuthorizationStatusParams{DeviceId: deviceID}
	resp, err := c.cloudGen.GetDeviceAuthorizationStatusWithResponse(ctx, &params)
	if err != nil {
		return nil, fmt.Errorf("get device authorization status: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}
