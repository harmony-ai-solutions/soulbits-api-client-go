package soulbits

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/harmony-ai-solutions/soulbits-api-client-go/gen"
)

// AuthStrategy represents a method of authentication for the Soulbits API.
type AuthStrategy interface {
	// Token returns the current Bearer token value.
	Token() string

	// IsPASETO returns true if this strategy uses PASETO tokens (supports refresh).
	IsPASETO() bool
}

// APIKeyAuth authenticates using a long-lived sb_cloud_* API key.
type APIKeyAuth struct {
	apiKey string
}

// NewAPIKeyAuth creates a new API key auth strategy.
func NewAPIKeyAuth(apiKey string) *APIKeyAuth {
	return &APIKeyAuth{apiKey: apiKey}
}

func (a *APIKeyAuth) Token() string  { return a.apiKey }
func (a *APIKeyAuth) IsPASETO() bool { return false }

// PasetoAuth authenticates using a PASETO v4.local token with automatic
// refresh on 401. Refresh is single-flight: concurrent 401s trigger only
// one refresh call; all waiters retry with the fresh token.
type PasetoAuth struct {
	mu           sync.RWMutex
	pasetoToken  string
	refreshToken string
	httpClient   *http.Client
	cloudBaseURL string

	// refreshMu serialises refresh and provides single-flight semantics.
	refreshMu    sync.Mutex
	refreshInFly bool
	refreshErr   error
	refreshWait  chan struct{}
}

// NewPasetoAuth creates a new PASETO auth strategy with auto-refresh.
func NewPasetoAuth(pasetoToken, refreshToken string, httpClient *http.Client, cloudBaseURL string) *PasetoAuth {
	return &PasetoAuth{
		pasetoToken:  pasetoToken,
		refreshToken: refreshToken,
		httpClient:   httpClient,
		cloudBaseURL: cloudBaseURL,
	}
}

func (p *PasetoAuth) Token() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.pasetoToken
}

func (p *PasetoAuth) IsPASETO() bool { return true }

// RefreshToken returns the current refresh token value.
func (p *PasetoAuth) RefreshToken() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.refreshToken
}

// Refresh performs a single-flight token refresh. If a refresh is already
// in flight, it waits for that one to complete and returns its result.
func (p *PasetoAuth) Refresh(ctx context.Context) error {
	p.refreshMu.Lock()
	if p.refreshInFly {
		// Wait for the in-flight refresh to complete.
		wait := p.refreshWait
		p.refreshMu.Unlock()
		select {
		case <-wait:
			return p.refreshErr
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// We are the first caller — start the refresh.
	p.refreshInFly = true
	p.refreshWait = make(chan struct{})
	p.refreshMu.Unlock()

	// Perform the actual refresh.
	err := p.doRefresh(ctx)

	// Signal all waiters.
	p.refreshMu.Lock()
	p.refreshInFly = false
	close(p.refreshWait)
	p.refreshErr = err
	p.refreshMu.Unlock()

	return err
}

// doRefresh performs the actual HTTP call to POST /v1/auth/refresh.
func (p *PasetoAuth) doRefresh(ctx context.Context) error {
	body := map[string]string{"refresh_token": p.refreshToken}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal refresh body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cloudBaseURL+"/v1/auth/refresh",
		bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("refresh returned %d: %s", resp.StatusCode, string(respBody))
	}

	var tr gen.TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return fmt.Errorf("decode refresh response: %w", err)
	}

	// Atomically update both tokens.
	p.mu.Lock()
	p.pasetoToken = tr.Token
	p.refreshToken = tr.RefreshToken
	p.mu.Unlock()

	return nil
}
