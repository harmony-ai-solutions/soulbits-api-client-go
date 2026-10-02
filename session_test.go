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

// TestDeleteSessionData_Success verifies the async happy path: 202 with
// status=in_progress and a request_id for polling, plus that the request body
// carries the typed confirmation string.
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
		w.WriteHeader(http.StatusAccepted)
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
	if gotBody["confirm"] != "DELETE" {
		t.Errorf("expected confirm=DELETE in request body, got %v", gotBody["confirm"])
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsInProgress() {
		t.Errorf("IsInProgress() = false, want true")
	}
	if got := result.RequestID(); got != "purge-abc-123" {
		t.Errorf("RequestID() = %q, want purge-abc-123", got)
	}
	if result.RequestId == nil || *result.RequestId != "purge-abc-123" {
		t.Errorf("request_id = %v, want purge-abc-123", result.RequestId)
	}
}

// TestDeleteSessionData_AlreadyInProgress verifies a 200 with status=in_progress
// (the purge is already running by another request) maps to a nil error and
// IsInProgress()=true.
func TestDeleteSessionData_AlreadyInProgress(t *testing.T) {
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

// TestDeleteSessionDataStatusHelperSanity pins the enum values the wrapper
// helpers rely on for the POST response (in_progress only) and the status
// union variants.
func TestDeleteSessionDataStatusHelperSanity(t *testing.T) {
	t.Parallel()

	if gen.SessionDataDeleteResponseStatusInProgress != "in_progress" {
		t.Errorf("SessionDataDeleteResponseStatusInProgress = %q, want in_progress", gen.SessionDataDeleteResponseStatusInProgress)
	}
	if gen.SessionDataDeleteStatusResponse0StatusInProgress != "in_progress" {
		t.Errorf("SessionDataDeleteStatusResponse0StatusInProgress = %q, want in_progress", gen.SessionDataDeleteStatusResponse0StatusInProgress)
	}
	if gen.SessionDataDeleteStatusResponse1Status(gen.Deleted) != gen.SessionDataDeleteStatusResponse1Status("deleted") {
		t.Errorf("gen.Deleted = %q, want deleted", gen.Deleted)
	}
	if gen.SessionDataDeleteStatusResponse2StatusFailed != "failed" {
		t.Errorf("SessionDataDeleteStatusResponse2StatusFailed = %q, want failed", gen.SessionDataDeleteStatusResponse2StatusFailed)
	}
	if gen.SessionDataDeleteStatusResponse3StatusNone != "none" {
		t.Errorf("SessionDataDeleteStatusResponse3StatusNone = %q, want none", gen.SessionDataDeleteStatusResponse3StatusNone)
	}
}

// TestDeleteSessionDataStatus_InProgress verifies the status endpoint reports
// an active purge (phase/request_id/attempts) and the wrapper surfaces it.
func TestDeleteSessionDataStatus_InProgress(t *testing.T) {
	t.Parallel()

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/session/data/delete/status" || r.Method != http.MethodGet {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":     "in_progress",
			"phase":      "s3",
			"request_id": "purge-abc-123",
			"attempts":   2,
		})
	}))
	defer cloudSrv.Close()

	client := newSessionTestClient(t, cloudSrv)

	result, err := client.DeleteSessionDataStatus(context.Background())
	if err != nil {
		t.Fatalf("DeleteSessionDataStatus failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsInProgress() {
		t.Fatalf("IsInProgress() = false, want true")
	}
	v, ok := result.InProgress()
	if !ok {
		t.Fatal("InProgress() = false, want true")
	}
	if v.RequestId != "purge-abc-123" || v.Phase != "s3" || v.Attempts != 2 {
		t.Errorf("unexpected in_progress variant: %+v", v)
	}
}

// TestDeleteSessionDataStatus_Deleted verifies the terminal-success variant
// with the S3 / beat / DEK counters.
func TestDeleteSessionDataStatus_Deleted(t *testing.T) {
	t.Parallel()

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/session/data/delete/status" || r.Method != http.MethodGet {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":           "deleted",
			"request_id":       "purge-abc-123",
			"attempts":         1,
			"objects_deleted":  42,
			"versions_deleted": 7,
			"beats_removed":    3,
			"dek_deleted":      true,
			"finished_at":      1724000000,
		})
	}))
	defer cloudSrv.Close()

	client := newSessionTestClient(t, cloudSrv)

	result, err := client.DeleteSessionDataStatus(context.Background())
	if err != nil {
		t.Fatalf("DeleteSessionDataStatus failed: %v", err)
	}
	if !result.IsDeleted() {
		t.Fatalf("IsDeleted() = false, want true")
	}
	v, ok := result.Deleted()
	if !ok {
		t.Fatal("Deleted() = false, want true")
	}
	if v.ObjectsDeleted != 42 || v.VersionsDeleted != 7 || v.BeatsRemoved != 3 || !v.DekDeleted {
		t.Errorf("unexpected counters: %+v", v)
	}
	if v.FinishedAt != 1724000000 {
		t.Errorf("finished_at = %d, want 1724000000", v.FinishedAt)
	}
}

// TestDeleteSessionDataStatus_Failed verifies the terminal-failure variant
// carries the failure reason.
func TestDeleteSessionDataStatus_Failed(t *testing.T) {
	t.Parallel()

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/session/data/delete/status" || r.Method != http.MethodGet {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":           "failed",
			"request_id":       "purge-abc-123",
			"attempts":         5,
			"error":            "attempt cap reached",
			"objects_deleted":  0,
			"versions_deleted": 0,
			"beats_removed":    0,
			"dek_deleted":      false,
			"finished_at":      1724000100,
		})
	}))
	defer cloudSrv.Close()

	client := newSessionTestClient(t, cloudSrv)

	result, err := client.DeleteSessionDataStatus(context.Background())
	if err != nil {
		t.Fatalf("DeleteSessionDataStatus failed: %v", err)
	}
	if !result.IsFailed() {
		t.Fatalf("IsFailed() = false, want true")
	}
	v, ok := result.Failed()
	if !ok {
		t.Fatal("Failed() = false, want true")
	}
	if v.Error != "attempt cap reached" {
		t.Errorf("error = %q, want attempt cap reached", v.Error)
	}
}

// TestDeleteSessionDataStatus_None verifies the empty variant.
func TestDeleteSessionDataStatus_None(t *testing.T) {
	t.Parallel()

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/session/data/delete/status" || r.Method != http.MethodGet {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "none",
		})
	}))
	defer cloudSrv.Close()

	client := newSessionTestClient(t, cloudSrv)

	result, err := client.DeleteSessionDataStatus(context.Background())
	if err != nil {
		t.Fatalf("DeleteSessionDataStatus failed: %v", err)
	}
	if !result.IsNone() {
		t.Fatalf("IsNone() = false, want true")
	}
	if _, ok := result.None(); !ok {
		t.Fatal("None() = false, want true")
	}
}

// TestDeleteSessionDataStatus_ServiceUnavailable verifies a 503 maps to an
// *APIError rather than a success result.
func TestDeleteSessionDataStatus_ServiceUnavailable(t *testing.T) {
	t.Parallel()

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/session/data/delete/status" || r.Method != http.MethodGet {
			http.Error(w, `{"error":"not found"}`, 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error_code": "service_unavailable",
			"message":    "Valkey down",
		})
	}))
	defer cloudSrv.Close()

	client := newSessionTestClient(t, cloudSrv)

	_, err := client.DeleteSessionDataStatus(context.Background())
	if err == nil {
		t.Fatal("expected error on 503, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want 503", apiErr.StatusCode)
	}
	if apiErr.Code != "service_unavailable" {
		t.Errorf("Code = %q, want service_unavailable", apiErr.Code)
	}
}
