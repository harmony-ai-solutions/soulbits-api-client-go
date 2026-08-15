package soulbits

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/harmony-ai-solutions/soulbits-api-client-go/gen"
)

// newDeviceTestClient builds a client pointed at the given cloud mock server,
// using API key auth (avoids any PASETO refresh machinery in these tests).
func newDeviceTestClient(t *testing.T, cloudSrv *httptest.Server) *Client {
	t.Helper()
	client, err := New(&Config{
		CloudBaseURL: cloudSrv.URL,
		APIKey:       "sb_cloud_test_key",
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	return client
}

// TestRegisterDeviceShapeAndMapping verifies the RegisterDevice request body
// shape and that the 200 response maps onto *gen.Device.
func TestRegisterDeviceShapeAndMapping(t *testing.T) {
	t.Parallel()

	var (
		gotMethod string
		gotAuth   string
		gotBody   map[string]interface{}
	)

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/devices" {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			http.Error(w, `{"error":"bad request"}`, 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"device_id":     "dev-123",
			"user_id":       "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"platform":      "web",
			"push_token":    "push-token-abc",
			"authorized":    false,
			"authorized_at": nil,
			"last_seen_at":  nil,
			"created_at":    "2026-07-01T12:00:00Z",
			"updated_at":    "2026-07-01T12:00:00Z",
		})
	}))
	defer cloudSrv.Close()

	client := newDeviceTestClient(t, cloudSrv)

	pushToken := "push-token-abc"
	device, err := client.RegisterDevice(context.Background(), "dev-123", gen.RegisterDeviceRequestPlatformWeb, &pushToken)
	if err != nil {
		t.Fatalf("RegisterDevice failed: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Errorf("expected Bearer auth header, got %q", gotAuth)
	}
	if gotBody["device_id"] != "dev-123" {
		t.Errorf("expected device_id in request body, got %v", gotBody["device_id"])
	}
	if gotBody["platform"] != "web" {
		t.Errorf("expected platform 'web' in request body, got %v", gotBody["platform"])
	}
	if gotBody["push_token"] != "push-token-abc" {
		t.Errorf("expected push_token in request body, got %v", gotBody["push_token"])
	}

	if device.DeviceId != "dev-123" {
		t.Errorf("unexpected device_id: %s", device.DeviceId)
	}
	if device.Platform != gen.DevicePlatformWeb {
		t.Errorf("unexpected platform: %s", device.Platform)
	}
	if device.PushToken == nil || *device.PushToken != "push-token-abc" {
		t.Errorf("unexpected push_token: %v", device.PushToken)
	}
	if device.Authorized {
		t.Error("expected authorized=false")
	}
}

// TestRequestDeviceAuthCodeSendsNoCodeAndMaps429 verifies that the authorize
// request body carries device_id but no `code` field, 202 returns nil, and a
// 429 response maps to *APIError.
func TestRequestDeviceAuthCodeSendsNoCodeAndMaps429(t *testing.T) {
	t.Parallel()

	var (
		gotBody  map[string]interface{}
		failMode string
	)

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/devices/authorize" || r.Method != http.MethodPost {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			http.Error(w, `{"error":"bad request"}`, 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if failMode == "429" {
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "rate_limit",
				"message": "too many requests",
			})
			return
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Auth code emailed.",
		})
	}))
	defer cloudSrv.Close()

	client := newDeviceTestClient(t, cloudSrv)
	ctx := context.Background()

	// 1. Happy path: request the code (no code field) → 202 → nil.
	if err := client.RequestDeviceAuthCode(ctx, "dev-456"); err != nil {
		t.Fatalf("RequestDeviceAuthCode failed: %v", err)
	}
	if _, ok := gotBody["code"]; ok {
		t.Errorf("request body must NOT contain a code field, got %v", gotBody["code"])
	}
	if gotBody["device_id"] != "dev-456" {
		t.Errorf("expected device_id in request body, got %v", gotBody["device_id"])
	}

	// 2. 429 → *APIError with StatusCode 429.
	failMode = "429"
	err := client.RequestDeviceAuthCode(ctx, "dev-456")
	if err == nil {
		t.Fatal("expected error on 429, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if !apiErr.IsRateLimited() {
		t.Errorf("expected 429 status, got %d", apiErr.StatusCode)
	}
}

// TestVerifyDeviceAuthCode verifies the request body carries device_id + code,
// the 200 happy path returns nil, and 401/429 map to *APIError.
func TestVerifyDeviceAuthCode(t *testing.T) {
	t.Parallel()

	var (
		gotBody  map[string]interface{}
		failMode string
	)

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/devices/authorize" || r.Method != http.MethodPost {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			http.Error(w, `{"error":"bad request"}`, 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch failMode {
		case "401":
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		case "429":
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "rate_limit",
				"message": "too many attempts",
			})
		default:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"message": "Code verified."})
		}
	}))
	defer cloudSrv.Close()

	client := newDeviceTestClient(t, cloudSrv)
	ctx := context.Background()

	// 1. Happy path → nil.
	if err := client.VerifyDeviceAuthCode(ctx, "dev-456", "123456"); err != nil {
		t.Fatalf("VerifyDeviceAuthCode happy path failed: %v", err)
	}
	if gotBody["device_id"] != "dev-456" {
		t.Errorf("expected device_id in request body, got %v", gotBody["device_id"])
	}
	if gotBody["code"] != "123456" {
		t.Errorf("expected code in request body, got %v", gotBody["code"])
	}

	// 2. 401 → *APIError, IsUnauthorized.
	failMode = "401"
	err := client.VerifyDeviceAuthCode(ctx, "dev-456", "123456")
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if !apiErr.IsUnauthorized() {
		t.Errorf("expected 401 status, got %d", apiErr.StatusCode)
	}

	// 3. 429 → *APIError, IsRateLimited.
	failMode = "429"
	err = client.VerifyDeviceAuthCode(ctx, "dev-456", "123456")
	if err == nil {
		t.Fatal("expected error on 429, got nil")
	}
	apiErr, ok = err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if !apiErr.IsRateLimited() {
		t.Errorf("expected 429 status, got %d", apiErr.StatusCode)
	}
}

// TestApproveDevice verifies the approve request body, the happy path returning
// the device_id, and the 401/403/429 error mapping.
func TestApproveDevice(t *testing.T) {
	t.Parallel()

	var (
		gotBody  map[string]interface{}
		failMode string
	)

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/devices/approve" || r.Method != http.MethodPost {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			http.Error(w, `{"error":"bad request"}`, 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch failMode {
		case "401":
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		case "403":
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "token_mismatch",
				"message": "token belongs to another account",
			})
		case "429":
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "rate_limit",
				"message": "too many attempts",
			})
		default:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"message":   "Device approved.",
				"device_id": "dev-approved-1",
			})
		}
	}))
	defer cloudSrv.Close()

	client := newDeviceTestClient(t, cloudSrv)
	ctx := context.Background()

	// 1. Happy path → device_id returned.
	deviceID, err := client.ApproveDevice(ctx, "abcDEF1234567890_-abcDEF1234567890_-abcDE")
	if err != nil {
		t.Fatalf("ApproveDevice happy path failed: %v", err)
	}
	if gotBody["token"] != "abcDEF1234567890_-abcDEF1234567890_-abcDE" {
		t.Errorf("expected token in request body, got %v", gotBody["token"])
	}
	if deviceID != "dev-approved-1" {
		t.Errorf("unexpected device_id: %s", deviceID)
	}

	assertApproveError := func(fail string, wantStatus int) {
		t.Helper()
		failMode = fail
		_, err := client.ApproveDevice(ctx, "abcDEF1234567890_-abcDEF1234567890_-abcDE")
		if err == nil {
			t.Fatalf("expected error on %s, got nil", fail)
		}
		apiErr, ok := err.(*APIError)
		if !ok {
			t.Fatalf("expected *APIError, got %T: %v", err, err)
		}
		if apiErr.StatusCode != wantStatus {
			t.Errorf("expected status %d, got %d", wantStatus, apiErr.StatusCode)
		}
	}

	// 2. 401 → invalid/expired token.
	assertApproveError("401", http.StatusUnauthorized)

	// 3. 403 → token belongs to another account.
	assertApproveError("403", http.StatusForbidden)

	// 4. 429 → lockout.
	assertApproveError("429", http.StatusTooManyRequests)
}

// TestGetDeviceAuthorizationStatus verifies the device_id query parameter and
// response mapping, plus the error mapping on non-200.
func TestGetDeviceAuthorizationStatus(t *testing.T) {
	t.Parallel()

	failMode := ""

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/devices/authorize/status" || r.Method != http.MethodGet {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		if r.URL.Query().Get("device_id") != "dev-status-1" {
			http.Error(w, `{"error":"bad request"}`, 400)
			return
		}
		if failMode == "404" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not_found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"device_id":             "dev-status-1",
			"authorized":            true,
			"authorization_pending": false,
		})
	}))
	defer cloudSrv.Close()

	client := newDeviceTestClient(t, cloudSrv)
	ctx := context.Background()

	// 1. Happy path → mapped DeviceAuthorizationStatus.
	status, err := client.GetDeviceAuthorizationStatus(ctx, "dev-status-1")
	if err != nil {
		t.Fatalf("GetDeviceAuthorizationStatus failed: %v", err)
	}
	if status.DeviceId != "dev-status-1" {
		t.Errorf("unexpected device_id: %s", status.DeviceId)
	}
	if !status.Authorized {
		t.Error("expected authorized=true")
	}
	if status.AuthorizationPending {
		t.Error("expected authorization_pending=false")
	}

	// 2. Non-200 → *APIError.
	failMode = "404"
	_, err = client.GetDeviceAuthorizationStatus(ctx, "dev-status-1")
	if err == nil {
		t.Fatal("expected error on 404, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if !apiErr.IsNotFound() {
		t.Errorf("expected 404 status, got %d", apiErr.StatusCode)
	}
}
