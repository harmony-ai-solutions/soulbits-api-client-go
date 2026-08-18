package soulbits

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/harmony-ai-solutions/soulbits-api-client-go/gen"
)

// newSessionTestClient builds a client pointed at the given cloud mock server,
// using API key auth (avoids any PASETO refresh machinery in these tests).
func newSessionTestClient(t *testing.T, cloudSrv *httptest.Server) *Client {
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

// TestDeleteSessionData_Success verifies the happy path: 200 with
// status=deleted and the purge counts, plus that the request body carries the
// typed confirmation string.
func TestDeleteSessionData_Success(t *testing.T) {
	t.Parallel()

	var gotBody map[string]interface{}
	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/session/data/delete" || r.Method != http.MethodPost {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			http.Error(w, `{"error":"bad request"}`, 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":           "deleted",
			"objects_deleted":  42,
			"versions_deleted": 7,
			"beats_removed":    3,
			"dek_deleted":      true,
		})
	}))
	defer cloudSrv.Close()

	client := newSessionTestClient(t, cloudSrv)

	result, err := client.DeleteSessionData(context.Background(), DeleteSessionDataRequest{Confirm: "DELETE"})
	if err != nil {
		t.Fatalf("DeleteSessionData failed: %v", err)
	}
	if gotBody["confirm"] != "DELETE" {
		t.Errorf("expected confirm=DELETE in request body, got %v", gotBody["confirm"])
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsDeleted() {
		t.Errorf("IsDeleted() = false, want true")
	}
	if result.IsInProgress() {
		t.Errorf("IsInProgress() = true, want false")
	}
	if result.ObjectsDeleted == nil || *result.ObjectsDeleted != 42 {
		t.Errorf("objects_deleted = %v, want 42", result.ObjectsDeleted)
	}
	if result.VersionsDeleted == nil || *result.VersionsDeleted != 7 {
		t.Errorf("versions_deleted = %v, want 7", result.VersionsDeleted)
	}
	if result.BeatsRemoved == nil || *result.BeatsRemoved != 3 {
		t.Errorf("beats_removed = %v, want 3", result.BeatsRemoved)
	}
	if result.DekDeleted == nil || !*result.DekDeleted {
		t.Errorf("dek_deleted = %v, want true", result.DekDeleted)
	}
}

// TestDeleteSessionData_InProgress verifies a 200 with status=in_progress maps
// to a nil error and IsInProgress()=true.
func TestDeleteSessionData_InProgress(t *testing.T) {
	t.Parallel()

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/session/data/delete" || r.Method != http.MethodPost {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":     "in_progress",
			"request_id": "purge-abc-123",
		})
	}))
	defer cloudSrv.Close()

	client := newSessionTestClient(t, cloudSrv)

	result, err := client.DeleteSessionData(context.Background(), DeleteSessionDataRequest{Confirm: "DELETE"})
	if err != nil {
		t.Fatalf("DeleteSessionData failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsInProgress() {
		t.Errorf("IsInProgress() = false, want true")
	}
	if result.IsDeleted() {
		t.Errorf("IsDeleted() = true, want false")
	}
	if result.RequestId == nil || *result.RequestId != "purge-abc-123" {
		t.Errorf("request_id = %v, want purge-abc-123", result.RequestId)
	}
}

// TestDeleteSessionData_ConfirmationRequired verifies a 400 maps to the typed
// *ConfirmationRequiredError.
func TestDeleteSessionData_ConfirmationRequired(t *testing.T) {
	t.Parallel()

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/session/data/delete" || r.Method != http.MethodPost {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "confirmation_required"})
	}))
	defer cloudSrv.Close()

	client := newSessionTestClient(t, cloudSrv)

	_, err := client.DeleteSessionData(context.Background(), DeleteSessionDataRequest{Confirm: "yes-I-am-sure"})
	if err == nil {
		t.Fatal("expected error on 400, got nil")
	}
	confErr, ok := err.(*ConfirmationRequiredError)
	if !ok {
		t.Fatalf("expected *ConfirmationRequiredError, got %T: %v", err, err)
	}
	if confErr.Error() != "confirmation_required" {
		t.Errorf("unexpected error string: %q", confErr.Error())
	}
}

// TestDeleteSessionData_SnapshotBusy verifies a 409 snapshot_busy maps to
// *SnapshotBusyError carrying retry_after_ms.
func TestDeleteSessionData_SnapshotBusy(t *testing.T) {
	t.Parallel()

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/session/data/delete" || r.Method != http.MethodPost {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":          "snapshot_busy",
			"retry_after_ms": 1500,
		})
	}))
	defer cloudSrv.Close()

	client := newSessionTestClient(t, cloudSrv)

	_, err := client.DeleteSessionData(context.Background(), DeleteSessionDataRequest{Confirm: "DELETE"})
	if err == nil {
		t.Fatal("expected error on 409, got nil")
	}
	busyErr, ok := err.(*SnapshotBusyError)
	if !ok {
		t.Fatalf("expected *SnapshotBusyError, got %T: %v", err, err)
	}
	if busyErr.RetryAfterMs != 1500 {
		t.Errorf("RetryAfterMs = %d, want 1500", busyErr.RetryAfterMs)
	}
}

// TestDeleteSessionData_PurgeInProgress409 verifies a 409 body carrying
// error=purge_in_progress on the delete endpoint maps to *PurgeInProgressError
// (parity with connect's 409 handling).
func TestDeleteSessionData_PurgeInProgress409(t *testing.T) {
	t.Parallel()

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/session/data/delete" || r.Method != http.MethodPost {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":          "purge_in_progress",
			"retry_after_ms": 500,
		})
	}))
	defer cloudSrv.Close()

	client := newSessionTestClient(t, cloudSrv)

	_, err := client.DeleteSessionData(context.Background(), DeleteSessionDataRequest{Confirm: "DELETE"})
	if err == nil {
		t.Fatal("expected error on 409, got nil")
	}
	purgeErr, ok := err.(*PurgeInProgressError)
	if !ok {
		t.Fatalf("expected *PurgeInProgressError, got %T: %v", err, err)
	}
	if purgeErr.RetryAfterMs != 500 {
		t.Errorf("RetryAfterMs = %d, want 500", purgeErr.RetryAfterMs)
	}
}

// TestConnectSession_PurgeInProgress verifies a 409 purge_in_progress on
// connect maps to *PurgeInProgressError (terminal, not retryable).
func TestConnectSession_PurgeInProgress(t *testing.T) {
	t.Parallel()

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/session/connect" || r.Method != http.MethodPost {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":          "purge_in_progress",
			"retry_after_ms": 800,
		})
	}))
	defer cloudSrv.Close()

	client := newSessionTestClient(t, cloudSrv)

	_, err := client.ConnectSession(context.Background(), ConnectSessionRequest{})
	if err == nil {
		t.Fatal("expected error on 409, got nil")
	}
	purgeErr, ok := err.(*PurgeInProgressError)
	if !ok {
		t.Fatalf("expected *PurgeInProgressError, got %T: %v", err, err)
	}
	if purgeErr.RetryAfterMs != 800 {
		t.Errorf("RetryAfterMs = %d, want 800", purgeErr.RetryAfterMs)
	}
}

// TestConnectSessionPoll_PurgeInProgressTerminates verifies that a 409
// purge_in_progress from ConnectSession is terminal for ConnectSessionPoll:
// exactly ONE HTTP request is observed — the poll loop must not retry.
func TestConnectSessionPoll_PurgeInProgressTerminates(t *testing.T) {
	t.Parallel()

	var calls int32
	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/v1/session/connect" || r.Method != http.MethodPost {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":          "purge_in_progress",
			"retry_after_ms": 60000,
		})
	}))
	defer cloudSrv.Close()

	client := newSessionTestClient(t, cloudSrv)

	_, err := client.ConnectSessionPoll(context.Background(), ConnectSessionRequest{})
	if err == nil {
		t.Fatal("expected error on 409, got nil")
	}
	purgeErr, ok := err.(*PurgeInProgressError)
	if !ok {
		t.Fatalf("expected *PurgeInProgressError, got %T: %v", err, err)
	}
	if purgeErr.RetryAfterMs != 60000 {
		t.Errorf("RetryAfterMs = %d, want 60000", purgeErr.RetryAfterMs)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("ConnectSessionPoll made %d HTTP requests, want exactly 1 (poll must abort immediately)", got)
	}
}

// TestDeleteSessionDataStatusHelperSanity pins the SessionDataDeleteResponse
// status enum values the wrapper helpers rely on.
func TestDeleteSessionDataStatusHelperSanity(t *testing.T) {
	t.Parallel()

	if gen.Deleted != "deleted" {
		t.Errorf("gen.Deleted = %q, want deleted", gen.Deleted)
	}
	if gen.InProgress != "in_progress" {
		t.Errorf("gen.InProgress = %q, want in_progress", gen.InProgress)
	}
}
