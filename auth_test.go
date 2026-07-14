package soulbits

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPasetoAuthSingleFlightRefresh verifies that when multiple concurrent
// requests receive 401s, only ONE refresh call is made. All waiters retry
// with the fresh token.
func TestPasetoAuthSingleFlightRefresh(t *testing.T) {
	t.Parallel()

	var (
		mu           sync.Mutex
		refreshCount atomic.Int32
		refreshToken = "original_refresh_token"
		tokenSeq     atomic.Int32
	)

	// Mock cloud server.
	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/refresh":
			refreshCount.Add(1)

			var body struct {
				RefreshToken string `json:"refresh_token"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, `{"error":"bad request"}`, 400)
				return
			}

			mu.Lock()
			if body.RefreshToken != refreshToken {
				mu.Unlock()
				http.Error(w, `{"error":"invalid refresh token"}`, 401)
				return
			}
			refreshToken = "rotated_refresh_token"
			mu.Unlock()

			seq := tokenSeq.Add(1)
			resp := map[string]interface{}{
				"token":         "v4.local.fresh_token_" + itoa(int(seq)),
				"refresh_token": refreshToken,
				"expires_at":    "2026-07-15T00:00:00Z",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case "/v1/auth/me":
			auth := r.Header.Get("Authorization")
			if auth == "Bearer v4.local.fresh_token_1" || auth == "Bearer v4.local.fresh_token_2" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":             "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
					"email":          "test@example.com",
					"display_name":   "Test User",
					"email_verified": true,
					"created_at":     "2026-01-01T00:00:00Z",
				})
				return
			}
			http.Error(w, `{"error":"unauthorized"}`, 401)
		}
	}))
	defer cloudSrv.Close()

	pasetoAuth := NewPasetoAuth("v4.local.orig_token", "original_refresh_token",
		cloudSrv.Client(), cloudSrv.URL)

	if tok := pasetoAuth.Token(); tok != "v4.local.orig_token" {
		t.Fatalf("expected initial token v4.local.orig_token, got %s", tok)
	}

	// Launch 5 concurrent goroutines that each trigger a refresh.
	var wg sync.WaitGroup
	errs := make(chan error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := pasetoAuth.Refresh(context.Background()); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("refresh error: %v", err)
	}

	if n := refreshCount.Load(); n != 1 {
		t.Errorf("expected 1 refresh call, got %d", n)
	}

	if tok := pasetoAuth.Token(); tok == "v4.local.orig_token" {
		t.Error("token was not refreshed")
	} else {
		t.Logf("token refreshed to: %s", tok)
	}

	if rt := pasetoAuth.RefreshToken(); rt != "rotated_refresh_token" {
		t.Errorf("expected refresh_token=rotated_refresh_token, got %s", rt)
	}
}

// TestPasetoAuthRefreshTimeout verifies that a context timeout during
// single-flight wait is respected.
func TestPasetoAuthRefreshTimeout(t *testing.T) {
	t.Parallel()

	// Channel to control when the blocking server responds.
	blockCh := make(chan struct{})

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/refresh" {
			// Block until unblocked or request cancelled.
			select {
			case <-blockCh:
			case <-r.Context().Done():
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"token":         "v4.local.new_token",
				"refresh_token": "new_refresh",
				"expires_at":    "2026-07-15T00:00:00Z",
			})
		}
	}))
	defer cloudSrv.Close()

	pasetoAuth := NewPasetoAuth("v4.local.orig_token", "original_refresh_token",
		cloudSrv.Client(), cloudSrv.URL)

	// First call starts the refresh (will block at the server).
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	go func() {
		_ = pasetoAuth.Refresh(ctx1)
	}()

	// Give the first call time to start the refresh and block on the server.
	time.Sleep(50 * time.Millisecond)

	// Second call should wait, but we give it a short timeout.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel2()

	err := pasetoAuth.Refresh(ctx2)
	if err == nil {
		t.Error("expected timeout error, got nil")
	} else {
		t.Logf("got expected error: %v", err)
	}

	// Unblock the first goroutine so it can complete.
	close(blockCh)
	time.Sleep(50 * time.Millisecond)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
