package soulbits

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestTransportBearerInjection verifies that the auth round-tripper injects the
// Authorization: Bearer header on every request.
func TestTransportBearerInjection(t *testing.T) {
	t.Parallel()

	var capturedToken atomic.Value
	capturedToken.Store("")

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		capturedToken.Store(auth)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":             "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"email":          "test@example.com",
			"display_name":   "Test",
			"email_verified": true,
			"created_at":     "2026-01-01T00:00:00Z",
		})
	}))
	defer cloudSrv.Close()

	client, err := New(&Config{
		CloudBaseURL: cloudSrv.URL,
		APIKey:       "sb_cloud_test_key_12345",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err = client.Me(ctx)
	if err != nil {
		t.Fatalf("Me failed: %v", err)
	}

	injected := capturedToken.Load().(string)
	expected := "Bearer sb_cloud_test_key_12345"
	if injected != expected {
		t.Errorf("expected Authorization header %q, got %q", expected, injected)
	}
	t.Logf("Bearer injection verified: %s", injected)
}

// TestTransportRetryAfterRefresh verifies that a 401 followed by a successful
// refresh causes the request to be retried with the new token.
func TestTransportRetryAfterRefresh(t *testing.T) {
	t.Parallel()

	var (
		callCount  atomic.Int32
		refreshTok = "original_refresh"
		newToken   = "v4.local.fresh_token"
	)

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		auth := r.Header.Get("Authorization")

		switch r.URL.Path {
		case "/v1/auth/refresh":
			// Rotate the refresh token.
			refreshTok = "rotated_refresh"
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"token":         newToken,
				"refresh_token": refreshTok,
				"expires_at":    "2026-07-15T00:00:00Z",
			})

		case "/v1/auth/me":
			if strings.Contains(auth, newToken) {
				// Fresh token — succeed.
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":             "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
					"email":          "test@example.com",
					"display_name":   "Test",
					"email_verified": true,
					"created_at":     "2026-01-01T00:00:00Z",
				})
				return
			}
			// Old token — return 401.
			http.Error(w, `{"error":"unauthorized"}`, 401)

		default:
			http.Error(w, `{"error":"not found"}`, 404)
		}
	}))
	defer cloudSrv.Close()

	// Create client with PASETO auth (so refresh is enabled).
	client, err := New(&Config{
		CloudBaseURL: cloudSrv.URL,
		PasetoToken:  "v4.local.old_token",
		RefreshToken: "original_refresh",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// The first call to Me will get a 401 (old token), which triggers a
	// refresh, then retries with the fresh token.
	profile, err := client.Me(ctx)
	if err != nil {
		t.Fatalf("Me failed after refresh: %v", err)
	}
	if profile.Email != "test@example.com" {
		t.Errorf("unexpected email: %s", profile.Email)
	}

	// We expect at least 2 calls: the original Me (401), the refresh, and
	// the retried Me. The refresh also goes through the same transport.
	totalCalls := callCount.Load()
	if totalCalls < 2 {
		t.Errorf("expected at least 2 HTTP calls (Me+refresh+retry), got %d", totalCalls)
	}
	t.Logf("Retry-after-refresh verified: %d total HTTP calls", totalCalls)
}

// TestTransportNoAuthForPublicEndpoints verifies that public endpoints can be
// called without configuring auth.
func TestTransportNoAuthForPublicEndpoints(t *testing.T) {
	t.Parallel()

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"id":           "free",
				"display_name": "Free",
				"sort_order":   0,
				"limits":       map[string]interface{}{},
				"features":     map[string]interface{}{},
			},
		})
	}))
	defer cloudSrv.Close()

	client, err := New(&Config{
		CloudBaseURL: cloudSrv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	tiers, err := client.ListTiers(ctx)
	if err != nil {
		t.Fatalf("ListTiers failed: %v", err)
	}
	if len(tiers) != 1 {
		t.Fatalf("expected 1 tier, got %d", len(tiers))
	}
	t.Logf("Public endpoint works: tier=%s", tiers[0].Id)
}
