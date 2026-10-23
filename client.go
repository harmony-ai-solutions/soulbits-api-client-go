// Package soulbits provides a first-party Go client for the Soulbits API.
//
// The Soulbits API spans two REST hosts:
//   - Cloud API (cloud.soulbits.app): authentication, account, API keys,
//     session lifecycle, subscription, and OpenAPI spec download.
//   - Inference API (api.soulbits.app): LLM chat, text embeddings, rerank,
//     TTS, STT, VAD, speaker embedding, audio/voice conversion.
//
// Two auth methods are supported:
//   - API Key (sb_cloud_* prefix): long-lived, best for programmatic access.
//   - PASETO v4.local token: refreshable, best for user-facing sessions.
//
// The client automatically routes requests to the correct host based on the
// operation and injects the configured Bearer token. PASETO tokens are
// automatically refreshed on 401 with single-flight semantics (concurrent
// 401s trigger only one refresh call).
//
// # Generation
//
// The request/response types and HTTP client bindings in the gen/ subpackage
// are generated from openapi.yaml via oapi-codegen. To regenerate:
//
//	go generate ./...
//
// # Embedding the spec
//
// The OpenAPI 3.0.3 specification is embedded in the binary. Call Spec() to
// retrieve the raw YAML bytes for tooling and testing.
package soulbits

import (
	_ "embed"
	"fmt"
	"net/http"
	"strings"

	"github.com/harmony-ai-solutions/soulbits-api-client-go/gen"
)

//go:embed openapi.yaml
var specYAML []byte

// Spec returns the raw OpenAPI 3.0.3 specification YAML bytes.
func Spec() []byte {
	return specYAML
}

// Client is the high-level Soulbits API client.
//
// Create a new client with New(), then use the methods on the returned
// *Client to interact with the API. All public methods accept a
// context.Context as the first argument.
//
// Internally the Client uses two oapi-codegen generated clients:
//   - cloudGen routes to the Cloud API (auth, account, keys, session, subscription).
//   - inferGen routes to the Inference API (chat, embeddings, audio, etc.).
//
// Auth is injected via an http.RoundTripper wrapping the underlying
// http.Client transport — the generated security-scope helpers
// (gen.ApiKeyScopes / gen.PasetoScopes) are NOT used; the RoundTripper
// sets the Authorization header on every outgoing request.
type Client struct {
	config   *Config
	auth     AuthStrategy
	cloudURL string
	inferURL string

	cloudGen *gen.ClientWithResponses
	inferGen *gen.ClientWithResponses
}

// New creates a new Soulbits API Client.
//
// If the config provides an AuthStrategy, it is used verbatim and the string
// credential fields below are ignored. Otherwise, if the config provides an
// APIKey, API key auth is used. Otherwise, if a PasetoToken is provided,
// PASETO auth is used. If none are set, the client is created without auth
// (suitable for public endpoints like model listing and subscription tiers).
//
// The config is modified in place to fill defaults; the caller should not
// reuse it after passing to New.
//
// Auth wiring: the generated gen.ClientWithResponses receives an http.Client
// whose transport is wrapped with the auth round-tripper. The round-tripper
// injects the Authorization: Bearer header on every outgoing request and
// handles PASETO 401 auto-refresh. The generated security-scope context keys
// (gen.ApiKeyScopes / gen.PasetoScopes) are NOT used — all auth flows
// through the transport layer instead.
func New(cfg *Config) (*Client, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	cfg.ensureDefaults()

	var auth AuthStrategy
	switch {
	case cfg.AuthStrategy != nil:
		// Injected strategy wins verbatim; the string credential fields are
		// ignored. This allows a dynamic source whose Token() is resolved on
		// every request by the auth round-tripper.
		auth = cfg.AuthStrategy
	case cfg.APIKey != "":
		auth = NewAPIKeyAuth(cfg.APIKey)
	case cfg.PasetoToken != "":
		// PasetoAuth uses the raw cfg.HTTPClient for its own refresh calls
		// (no auth wrapping — avoids refresh loops).
		auth = NewPasetoAuth(cfg.PasetoToken, cfg.RefreshToken, cfg.HTTPClient, cfg.CloudBaseURL)
	}

	// Wrap the HTTP client's Transport with the auth round-tripper.
	var transport http.RoundTripper
	if auth != nil {
		transport = newAuthRoundTripper(cfg.HTTPClient.Transport, auth, cfg)
	} else {
		transport = cfg.HTTPClient.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
	}

	authHTTPClient := &http.Client{
		Transport: transport,
		Timeout:   cfg.HTTPClient.Timeout,
	}

	// Create the two oapi-codegen generated clients, both using the
	// auth-wrapped HTTP client.
	cloudBase := strings.TrimRight(cfg.CloudBaseURL, "/")
	cloudGen, err := gen.NewClientWithResponses(cloudBase, gen.WithHTTPClient(authHTTPClient))
	if err != nil {
		return nil, fmt.Errorf("create cloud gen client: %w", err)
	}

	inferBase := strings.TrimRight(cfg.InferenceBaseURL, "/")
	inferGen, err := gen.NewClientWithResponses(inferBase, gen.WithHTTPClient(authHTTPClient))
	if err != nil {
		return nil, fmt.Errorf("create inference gen client: %w", err)
	}

	return &Client{
		config:   cfg,
		auth:     auth,
		cloudURL: cloudBase,
		inferURL: inferBase,
		cloudGen: cloudGen,
		inferGen: inferGen,
	}, nil
}

// Config returns the client's configuration.
func (c *Client) Config() *Config {
	return c.config
}

// Auth returns the client's auth strategy, or nil if no auth is configured.
func (c *Client) Auth() AuthStrategy {
	return c.auth
}

// String returns a human-readable description of the client.
func (c *Client) String() string {
	return fmt.Sprintf("SoulbitsClient{cloud: %s, infer: %s}", c.cloudURL, c.inferURL)
}
