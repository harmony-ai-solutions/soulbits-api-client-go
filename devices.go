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

// ---------------------------------------------------------------------------
// Delegated device enrollment (phase 7-15)
// ---------------------------------------------------------------------------

// MintEnrollmentGrant mints a short-lived, single-use delegated-enrollment
// grant from an already-authorized device (deviceID is the MINTING device).
// The opaque grant is returned ONCE; only its SHA-256 is stored server-side.
// Requires account auth (device-scoped session or interactive/PASETO).
func (c *Client) MintEnrollmentGrant(ctx context.Context, deviceID string) (*gen.EnrollmentGrantResponse, error) {
	if deviceID == "" {
		return nil, fmt.Errorf("device id is required")
	}
	body := gen.MintEnrollmentGrantJSONRequestBody(gen.EnrollmentGrantRequest{DeviceId: deviceID})
	resp, err := c.cloudGen.MintEnrollmentGrantWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("mint enrollment grant request: %w", err)
	}
	return checkOK(resp.JSON201, resp.Body, resp.HTTPResponse)
}

// EnrollDeviceWithGrant redeems an enrollment grant for the NEW device's own
// device-scoped session. Public: no account auth is sent — the grant IS the
// credential. deviceName and platform are optional metadata; pass a nil
// platform to let the server default it to "android".
func (c *Client) EnrollDeviceWithGrant(ctx context.Context, grant, deviceID string, deviceName *string, platform *gen.DeviceEnrollRequestPlatform) (*gen.TokenResponse, error) {
	if grant == "" {
		return nil, fmt.Errorf("enrollment grant is required")
	}
	if deviceID == "" {
		return nil, fmt.Errorf("device id is required")
	}
	body := gen.EnrollDeviceJSONRequestBody(gen.DeviceEnrollRequest{
		Grant:      grant,
		DeviceId:   deviceID,
		DeviceName: deviceName,
		Platform:   platform,
	})
	resp, err := c.cloudGen.EnrollDeviceWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("enroll device request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ---------------------------------------------------------------------------
// Portal-mediated connect sessions (phase 7-15 C2)
// ---------------------------------------------------------------------------

// CreateDeviceConnectSession starts the portal-mediated engine sign-in flow.
// Public (the engine is not authenticated yet): the server mints an
// unguessable connect_id plus a once-shown 256-bit session_secret and returns
// the portal URL the engine should open in the system browser. Poll
// GetDeviceConnectSession with the returned secret until the user allows it.
//
// deviceName and platform are optional; pass a nil platform to let the server
// default it to "web".
func (c *Client) CreateDeviceConnectSession(ctx context.Context, deviceID string, deviceName *string, platform *gen.DeviceConnectSessionRequestPlatform) (*gen.DeviceConnectSessionResponse, error) {
	if deviceID == "" {
		return nil, fmt.Errorf("device id is required")
	}
	body := gen.CreateDeviceConnectSessionJSONRequestBody(gen.DeviceConnectSessionRequest{
		DeviceId:   deviceID,
		DeviceName: deviceName,
		Platform:   platform,
	})
	resp, err := c.cloudGen.CreateDeviceConnectSessionWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("create device connect session request: %w", err)
	}
	return checkOK(resp.JSON201, resp.Body, resp.HTTPResponse)
}

// GetDeviceConnectSession polls a connect session using the once-shown
// session_secret as the Bearer credential. Status is one of
// pending|approved|consumed|expired. The token pair is populated ONLY on the
// approved transition and returned exactly once (the server marks the row
// consumed in the same statement), so persist it immediately.
//
// Use an UNauthenticated client (no API key / PASETO) for this call: the
// secret is set as the Authorization header and, when account auth is
// configured, the auth round-tripper would overwrite it (see transport.go).
func (c *Client) GetDeviceConnectSession(ctx context.Context, connectID, sessionSecret string) (*gen.DeviceConnectStatusResponse, error) {
	if connectID == "" {
		return nil, fmt.Errorf("connect id is required")
	}
	if sessionSecret == "" {
		return nil, fmt.Errorf("session secret is required")
	}
	resp, err := c.cloudGen.GetDeviceConnectSessionWithResponse(ctx, connectID, connectSecretEditor(sessionSecret))
	if err != nil {
		return nil, fmt.Errorf("get device connect session request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// connectSecretEditor returns a gen.RequestEditorFn that sets
// Authorization: Bearer <sessionSecret> for a connect-session poll.
func connectSecretEditor(sessionSecret string) gen.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+sessionSecret)
		return nil
	}
}
