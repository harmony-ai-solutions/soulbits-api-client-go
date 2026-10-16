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

// TestMintEnrollmentGrant verifies the request body (device_id of the MINTING
// device), Bearer auth, and mapping of the 201 response onto
// *gen.EnrollmentGrantResponse.
func TestMintEnrollmentGrant(t *testing.T) {
	t.Parallel()

	var (
		gotMethod string
		gotAuth   string
		gotBody   map[string]interface{}
	)

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/devices/enrollment-grants" || r.Method != http.MethodPost {
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
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"grant":      "grant-abc123",
			"expires_in": 300,
		})
	}))
	defer cloudSrv.Close()

	client := newDeviceTestClient(t, cloudSrv)

	grant, err := client.MintEnrollmentGrant(context.Background(), "dev-minter-1")
	if err != nil {
		t.Fatalf("MintEnrollmentGrant failed: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Errorf("expected Bearer auth header, got %q", gotAuth)
	}
	if gotBody["device_id"] != "dev-minter-1" {
		t.Errorf("expected device_id in request body, got %v", gotBody["device_id"])
	}
	if grant.Grant != "grant-abc123" {
		t.Errorf("unexpected grant: %s", grant.Grant)
	}
	if grant.ExpiresIn != 300 {
		t.Errorf("unexpected expires_in: %d", grant.ExpiresIn)
	}
}

// TestEnrollDeviceWithGrant verifies the request body (grant + new device id +
// optional metadata) and mapping of the 200 token pair.
func TestEnrollDeviceWithGrant(t *testing.T) {
	t.Parallel()

	var gotBody map[string]interface{}

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/device/enroll" || r.Method != http.MethodPost {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			http.Error(w, `{"error":"bad request"}`, 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token":         "v4.local.enrolled",
			"refresh_token": "refresh-enrolled",
			"expires_at":    "2026-10-16T12:00:00Z",
		})
	}))
	defer cloudSrv.Close()

	client := newDeviceTestClient(t, cloudSrv)

	name := "Living-Room-Tablet"
	platform := gen.DeviceEnrollRequestPlatformAndroid
	tokens, err := client.EnrollDeviceWithGrant(context.Background(), "grant-abc123", "dev-new-1", &name, &platform)
	if err != nil {
		t.Fatalf("EnrollDeviceWithGrant failed: %v", err)
	}
	if gotBody["grant"] != "grant-abc123" {
		t.Errorf("expected grant in request body, got %v", gotBody["grant"])
	}
	if gotBody["device_id"] != "dev-new-1" {
		t.Errorf("expected device_id in request body, got %v", gotBody["device_id"])
	}
	if gotBody["device_name"] != "Living-Room-Tablet" {
		t.Errorf("expected device_name in request body, got %v", gotBody["device_name"])
	}
	if gotBody["platform"] != "android" {
		t.Errorf("expected platform 'android' in request body, got %v", gotBody["platform"])
	}
	if tokens.Token != "v4.local.enrolled" {
		t.Errorf("unexpected token: %s", tokens.Token)
	}
	if tokens.RefreshToken != "refresh-enrolled" {
		t.Errorf("unexpected refresh_token: %s", tokens.RefreshToken)
	}
}

// TestCreateAndPollDeviceConnectSession drives the portal-mediated engine
// sign-in: create (201, secret shown once), then two status polls — the first
// pending, the second approved (token pair), with the session secret carried
// as the Bearer credential on each poll. It uses an UNauthenticated client so
// the secret is not clobbered by the auth round-tripper.
func TestCreateAndPollDeviceConnectSession(t *testing.T) {
	t.Parallel()

	var (
		createBody map[string]interface{}
		pollAuth   []string
		pollCount  int
	)

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/devices/connect-session" && r.Method == http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&createBody); err != nil {
				http.Error(w, `{"error":"bad request"}`, 400)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"connect_id":     "conn-1",
				"connect_url":    "https://portal.soulbits.app/connect?request=conn-1",
				"expires_in":     600,
				"session_secret": "sess-secret-xyz",
			})
		case r.URL.Path == "/v1/devices/connect-session/conn-1" && r.Method == http.MethodGet:
			pollAuth = append(pollAuth, r.Header.Get("Authorization"))
			pollCount++
			w.Header().Set("Content-Type", "application/json")
			if pollCount == 1 {
				json.NewEncoder(w).Encode(map[string]interface{}{"status": "pending"})
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":        "approved",
				"token":         "v4.local.connect",
				"refresh_token": "refresh-connect",
				"expires_at":    "2026-10-16T12:00:00Z",
			})
		default:
			http.Error(w, `{"error":"not found"}`, 404)
		}
	}))
	defer cloudSrv.Close()

	// Unauthenticated client: the connect flow is public and the poll carries
	// the session secret in the Authorization header.
	client, err := New(&Config{CloudBaseURL: cloudSrv.URL})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	ctx := context.Background()

	name := "Studio-PC"
	platform := gen.DeviceConnectSessionRequestPlatformWeb
	session, err := client.CreateDeviceConnectSession(ctx, "engine-dev-1", &name, &platform)
	if err != nil {
		t.Fatalf("CreateDeviceConnectSession failed: %v", err)
	}
	if createBody["device_id"] != "engine-dev-1" {
		t.Errorf("expected device_id in create body, got %v", createBody["device_id"])
	}
	if createBody["device_name"] != "Studio-PC" {
		t.Errorf("expected device_name in create body, got %v", createBody["device_name"])
	}
	if session.ConnectId != "conn-1" {
		t.Errorf("unexpected connect_id: %s", session.ConnectId)
	}
	if session.SessionSecret != "sess-secret-xyz" {
		t.Errorf("unexpected session_secret: %s", session.SessionSecret)
	}
	if session.ExpiresIn != 600 {
		t.Errorf("unexpected expires_in: %d", session.ExpiresIn)
	}

	// Poll 1 — pending.
	pending, err := client.GetDeviceConnectSession(ctx, session.ConnectId, session.SessionSecret)
	if err != nil {
		t.Fatalf("GetDeviceConnectSession (pending) failed: %v", err)
	}
	if pending.Status != gen.DeviceConnectStatusResponseStatusPending {
		t.Errorf("expected pending, got %s", pending.Status)
	}
	if pending.Token != nil {
		t.Errorf("expected no token while pending, got %v", *pending.Token)
	}

	// Poll 2 — approved (token pair, once).
	approved, err := client.GetDeviceConnectSession(ctx, session.ConnectId, session.SessionSecret)
	if err != nil {
		t.Fatalf("GetDeviceConnectSession (approved) failed: %v", err)
	}
	if approved.Status != gen.DeviceConnectStatusResponseStatusApproved {
		t.Errorf("expected approved, got %s", approved.Status)
	}
	if approved.Token == nil || *approved.Token != "v4.local.connect" {
		t.Errorf("unexpected token: %v", approved.Token)
	}
	if approved.RefreshToken == nil || *approved.RefreshToken != "refresh-connect" {
		t.Errorf("unexpected refresh_token: %v", approved.RefreshToken)
	}

	// The session secret must be the Bearer credential on every poll.
	if len(pollAuth) != 2 {
		t.Fatalf("expected 2 polls, got %d", len(pollAuth))
	}
	for i, auth := range pollAuth {
		if auth != "Bearer sess-secret-xyz" {
			t.Errorf("poll %d: expected session secret as Bearer, got %q", i+1, auth)
		}
	}
}

// TestGetDeviceConnectSessionErrorMapping verifies that a non-200 poll (wrong
// or missing secret → 401) surfaces as *APIError.
func TestGetDeviceConnectSessionErrorMapping(t *testing.T) {
	t.Parallel()

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid session secret"})
	}))
	defer cloudSrv.Close()

	client, err := New(&Config{CloudBaseURL: cloudSrv.URL})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	_, err = client.GetDeviceConnectSession(context.Background(), "conn-1", "wrong-secret")
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
}
