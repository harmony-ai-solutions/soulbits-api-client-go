package soulbits

import (
	"bytes"
	"io"
	"net/http"
)

// authRoundTripper injects the Authorization: Bearer header into every request
// and handles 401 auto-refresh for PASETO tokens.
type authRoundTripper struct {
	inner  http.RoundTripper
	auth   AuthStrategy
	config *Config
}

// newAuthRoundTripper wraps an http.RoundTripper with auth injection.
// If the inner is nil, http.DefaultTransport is used.
func newAuthRoundTripper(inner http.RoundTripper, auth AuthStrategy, config *Config) http.RoundTripper {
	if inner == nil {
		inner = http.DefaultTransport
	}
	return &authRoundTripper{
		inner:  inner,
		auth:   auth,
		config: config,
	}
}

// RoundTrip implements http.RoundTripper.
func (t *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the body so we can re-read it on retry.
	bodyBytes, err := cloneBody(req)
	if err != nil {
		return nil, err
	}

	// Restore the body for the first attempt.
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	req.ContentLength = int64(len(bodyBytes))

	// Inject Bearer header.
	setBearer(req, t.auth.Token())

	resp, err := t.inner.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	// If the auth strategy supports refresh and we got a 401, try refreshing.
	if resp.StatusCode == http.StatusUnauthorized && t.auth.IsPASETO() {
		pasetoAuth, ok := t.auth.(*PasetoAuth)
		if !ok {
			return resp, nil
		}

		// Perform single-flight refresh.
		if refreshErr := pasetoAuth.Refresh(req.Context()); refreshErr != nil {
			// Refresh failed — return the original 401 response.
			// The caller will see the 401 and can inspect the error body.
			return resp, nil
		}

		// Refresh succeeded — retry with the fresh token.
		// Close the original response body since we're replacing it.
		resp.Body.Close()

		retryReq := req.Clone(req.Context())
		retryReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		retryReq.ContentLength = int64(len(bodyBytes))
		setBearer(retryReq, t.auth.Token())

		return t.inner.RoundTrip(retryReq)
	}

	return resp, nil
}

// setBearer sets the Authorization header with the Bearer token.
func setBearer(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
}

// cloneBody reads the request body so we can restore it for retries.
// After calling cloneBody, req.Body is drained and should be replaced.
func cloneBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	return io.ReadAll(req.Body)
}
