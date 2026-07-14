package soulbits

import (
	"net/http"
	"time"
)

// Default base URLs for the Soulbits API (beta environment).
const (
	DefaultCloudBaseURL     = "https://beta.cloud.soulbits.app"
	DefaultInferenceBaseURL = "https://beta.api.soulbits.app"
)

// Config configures a Soulbits API Client.
// All fields are optional; sensible defaults are used when omitted.
type Config struct {
	// CloudBaseURL is the base URL for the Cloud API (auth, account, keys,
	// session, subscription). Defaults to https://beta.cloud.soulbits.app.
	CloudBaseURL string

	// InferenceBaseURL is the base URL for the Inference API (chat,
	// embeddings, rerank, audio). Defaults to https://beta.api.soulbits.app.
	InferenceBaseURL string

	// APIKey is an sb_cloud_* API key for Bearer authentication.
	// If set, it takes precedence over PasetoToken.
	APIKey string

	// PasetoToken is a v4.local.* PASETO token for Bearer authentication.
	PasetoToken string

	// RefreshToken is the opaque refresh token used by the auto-refresh
	// transport. Required for PASETO token refresh on 401.
	RefreshToken string

	// HTTPClient is the HTTP client used for API requests.
	// If nil, http.DefaultClient is used.
	HTTPClient *http.Client
}

// ensureDefaults fills in any zero-valued fields with sensible defaults.
func (c *Config) ensureDefaults() {
	if c.CloudBaseURL == "" {
		c.CloudBaseURL = DefaultCloudBaseURL
	}
	if c.InferenceBaseURL == "" {
		c.InferenceBaseURL = DefaultInferenceBaseURL
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
}
