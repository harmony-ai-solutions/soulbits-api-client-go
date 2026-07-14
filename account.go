package soulbits

import (
	"context"
	"fmt"

	"github.com/harmony-ai-solutions/soulbits-api-client-go/gen"
)

// ---------------------------------------------------------------------------
// Registration & Login
// ---------------------------------------------------------------------------

// Register creates a new account. Returns 202 Accepted on success (email
// verification sent). No token is returned at this stage.
func (c *Client) Register(ctx context.Context, email, password string, displayName *string) (*gen.Message, error) {
	body := gen.PostV1AuthRegisterJSONRequestBody(gen.PostV1AuthRegisterJSONBody{
		Email:       openapiEmail(email),
		Password:    password,
		DisplayName: displayName,
	})
	resp, err := c.cloudGen.PostV1AuthRegisterWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("register request: %w", err)
	}
	return checkOK(resp.JSON202, resp.Body, resp.HTTPResponse)
}

// Login authenticates with email and password. Returns a token pair.
// The account must have email_verified=true.
func (c *Client) Login(ctx context.Context, email, password string) (*gen.TokenResponse, error) {
	body := gen.PostV1AuthLoginJSONRequestBody(gen.PostV1AuthLoginJSONBody{
		Email:    openapiEmail(email),
		Password: password,
	})
	resp, err := c.cloudGen.PostV1AuthLoginWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("login request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// Refresh refreshes the PASETO token using the stored refresh token.
// This is only available when the client was configured with a PasetoAuth
// strategy. If using API key auth, this method will return an error.
func (c *Client) Refresh(ctx context.Context) (*gen.TokenResponse, error) {
	paseto, ok := c.auth.(*PasetoAuth)
	if !ok {
		return nil, fmt.Errorf("refresh requires PASETO auth strategy, but current auth is %T", c.auth)
	}

	body := gen.PostV1AuthRefreshJSONRequestBody(gen.PostV1AuthRefreshJSONBody{
		RefreshToken: paseto.RefreshToken(),
	})
	resp, err := c.cloudGen.PostV1AuthRefreshWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("refresh request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// Logout revokes refresh tokens and writes a PASETO cutoff.
// If refreshToken is provided, only that token is revoked; otherwise all are.
func (c *Client) Logout(ctx context.Context, refreshToken *string) (*gen.Message, error) {
	body := gen.PostV1AuthLogoutJSONRequestBody(gen.PostV1AuthLogoutJSONBody{
		RefreshToken: refreshToken,
	})
	resp, err := c.cloudGen.PostV1AuthLogoutWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("logout request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ---------------------------------------------------------------------------
// OAuth
// ---------------------------------------------------------------------------

// GoogleSignIn validates a Google id_token (native/Android).
func (c *Client) GoogleSignIn(ctx context.Context, idToken string) (*gen.TokenResponse, error) {
	body := gen.PostV1AuthGoogleJSONRequestBody(gen.PostV1AuthGoogleJSONBody{IdToken: idToken})
	resp, err := c.cloudGen.PostV1AuthGoogleWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("google sign-in request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// GoogleSignInWeb exchanges an authorization code for a token (web OAuth code flow).
func (c *Client) GoogleSignInWeb(ctx context.Context, authCode, redirectURI string, codeVerifier *string) (*gen.TokenResponse, error) {
	body := gen.PostV1AuthGoogleWebJSONRequestBody(gen.PostV1AuthGoogleWebJSONBody{
		AuthCode:     authCode,
		RedirectUri:  redirectURI,
		CodeVerifier: codeVerifier,
	})
	resp, err := c.cloudGen.PostV1AuthGoogleWebWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("google sign-in web request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// AppleSignIn validates an Apple identity_token (native/iOS).
func (c *Client) AppleSignIn(ctx context.Context, identityToken string) (*gen.TokenResponse, error) {
	body := gen.PostV1AuthAppleJSONRequestBody(gen.PostV1AuthAppleJSONBody{IdentityToken: identityToken})
	resp, err := c.cloudGen.PostV1AuthAppleWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("apple sign-in request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// AppleSignInWeb exchanges an authorization code for a token (web OAuth code flow).
func (c *Client) AppleSignInWeb(ctx context.Context, code, redirectURI string) (*gen.TokenResponse, error) {
	body := gen.PostV1AuthAppleWebJSONRequestBody(gen.PostV1AuthAppleWebJSONBody{
		Code:        code,
		RedirectUri: redirectURI,
	})
	resp, err := c.cloudGen.PostV1AuthAppleWebWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("apple sign-in web request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ---------------------------------------------------------------------------
// Profile
// ---------------------------------------------------------------------------

// Me returns the current user's profile.
func (c *Client) Me(ctx context.Context) (*gen.UserProfile, error) {
	resp, err := c.cloudGen.GetV1AuthMeWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("me request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// UpdateProfile updates the current user's profile (only display_name is mutable).
func (c *Client) UpdateProfile(ctx context.Context, displayName string) (*gen.UserProfile, error) {
	body := gen.PatchV1AuthMeJSONRequestBody(gen.PatchV1AuthMeJSONBody{DisplayName: displayName})
	resp, err := c.cloudGen.PatchV1AuthMeWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("update profile request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ChangePassword changes the current user's password. Writes a Valkey cutoff
// that forces re-login everywhere.
func (c *Client) ChangePassword(ctx context.Context, oldPassword, newPassword string) (*gen.Message, error) {
	body := gen.PutV1AuthPasswordJSONRequestBody(gen.PutV1AuthPasswordJSONBody{
		OldPassword: oldPassword,
		NewPassword: newPassword,
	})
	resp, err := c.cloudGen.PutV1AuthPasswordWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("change password request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// LinkGoogle links a Google identity to the current account.
func (c *Client) LinkGoogle(ctx context.Context, idToken string) (*gen.Message, error) {
	body := gen.PostV1AuthLinkGoogleJSONRequestBody(gen.PostV1AuthLinkGoogleJSONBody{IdToken: idToken})
	resp, err := c.cloudGen.PostV1AuthLinkGoogleWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("link google request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// UnlinkGoogle unlinks the Google identity from the current account.
func (c *Client) UnlinkGoogle(ctx context.Context) error {
	resp, err := c.cloudGen.DeleteV1AuthLinkGoogleWithResponse(ctx)
	if err != nil {
		return fmt.Errorf("unlink google request: %w", err)
	}
	return responseError(resp.Body, resp.HTTPResponse)
}

// LinkApple links an Apple identity to the current account.
func (c *Client) LinkApple(ctx context.Context, identityToken string) (*gen.Message, error) {
	body := gen.PostV1AuthLinkAppleJSONRequestBody(gen.PostV1AuthLinkAppleJSONBody{IdentityToken: identityToken})
	resp, err := c.cloudGen.PostV1AuthLinkAppleWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("link apple request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// UnlinkApple unlinks the Apple identity from the current account.
func (c *Client) UnlinkApple(ctx context.Context) error {
	resp, err := c.cloudGen.DeleteV1AuthLinkAppleWithResponse(ctx)
	if err != nil {
		return fmt.Errorf("unlink apple request: %w", err)
	}
	return responseError(resp.Body, resp.HTTPResponse)
}

// ---------------------------------------------------------------------------
// Email verification
// ---------------------------------------------------------------------------

// VerifyEmail verifies an email address with a verification token.
// Returns a token pair on success.
func (c *Client) VerifyEmail(ctx context.Context, token string) (*gen.TokenResponse, error) {
	params := gen.GetV1AuthVerifyParams{Token: token}
	resp, err := c.cloudGen.GetV1AuthVerifyWithResponse(ctx, &params)
	if err != nil {
		return nil, fmt.Errorf("verify email request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}

// ResendVerification resends the verification email. Per-user 60s cooldown.
func (c *Client) ResendVerification(ctx context.Context, email string) (*gen.Message, error) {
	body := gen.PostV1AuthResendVerificationJSONRequestBody(gen.PostV1AuthResendVerificationJSONBody{
		Email: openapiEmail(email),
	})
	resp, err := c.cloudGen.PostV1AuthResendVerificationWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("resend verification request: %w", err)
	}
	return checkOK(resp.JSON202, resp.Body, resp.HTTPResponse)
}

// ---------------------------------------------------------------------------
// Password reset
// ---------------------------------------------------------------------------

// RequestPasswordReset requests a password reset email.
func (c *Client) RequestPasswordReset(ctx context.Context, email string) (*gen.Message, error) {
	body := gen.PostV1AuthPasswordResetRequestJSONRequestBody(gen.PostV1AuthPasswordResetRequestJSONBody{
		Email: openapiEmail(email),
	})
	resp, err := c.cloudGen.PostV1AuthPasswordResetRequestWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("password reset request: %w", err)
	}
	return checkOK(resp.JSON202, resp.Body, resp.HTTPResponse)
}

// ConfirmPasswordReset confirms a password reset with a token and new password.
func (c *Client) ConfirmPasswordReset(ctx context.Context, token, newPassword string) (*gen.Message, error) {
	body := gen.PostV1AuthPasswordResetConfirmJSONRequestBody(gen.PostV1AuthPasswordResetConfirmJSONBody{
		Token:       token,
		NewPassword: newPassword,
	})
	resp, err := c.cloudGen.PostV1AuthPasswordResetConfirmWithResponse(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("password reset confirm request: %w", err)
	}
	return checkOK(resp.JSON200, resp.Body, resp.HTTPResponse)
}
