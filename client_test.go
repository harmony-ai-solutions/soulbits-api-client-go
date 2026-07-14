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

// TestClientLoginListModelsChat verifies a full login → list models → chat
// flow against mocked cloud and inference servers.
func TestClientLoginListModelsChat(t *testing.T) {
	t.Parallel()

	// ----- Cloud server mock -----
	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/login":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", 405)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"token":         "v4.local.test_token",
				"refresh_token": "test_refresh",
				"expires_at":    "2026-07-15T00:00:00Z",
			})

		case "/v1/auth/me":
			if r.Header.Get("Authorization") == "" {
				http.Error(w, `{"error":"unauthorized"}`, 401)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
				"email":          "test@example.com",
				"display_name":   "Test User",
				"email_verified": true,
				"created_at":     "2026-01-01T00:00:00Z",
			})

		case "/v1/subscription/tiers":
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

		default:
			http.Error(w, `{"error":"not found"}`, 404)
		}
	}))
	defer cloudSrv.Close()

	// ----- Inference server mock -----
	inferSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"model_id":    "qwen-35-9b",
					"display_name": "Qwen 35B",
					"model_type":  "llm",
					"min_tier":    "free",
					"description": "models.descriptions.qwen-35-9b",
				},
			})

		case "/v1/chat/completions":
			var reqBody map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
				http.Error(w, `{"error":"bad request"}`, 400)
				return
			}
			model, _ := reqBody["model"].(string)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":      "chat-1",
				"object":  "chat.completion",
				"created": 1710000000,
				"model":   model,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Hello! How can I help you?",
						},
						"finish_reason": "stop",
					},
				},
			})

		default:
			http.Error(w, `{"error":"not found"}`, 404)
		}
	}))
	defer inferSrv.Close()

	// Create the client pointed at our mocks.
	client, err := New(&Config{
		CloudBaseURL:     cloudSrv.URL,
		InferenceBaseURL: inferSrv.URL,
		APIKey:           "sb_cloud_test_key",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Login
	tokenResp, err := client.Login(ctx, "test@example.com", "password123")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if tokenResp.Token != "v4.local.test_token" {
		t.Errorf("unexpected token: %s", tokenResp.Token)
	}
	if tokenResp.RefreshToken != "test_refresh" {
		t.Errorf("unexpected refresh: %s", tokenResp.RefreshToken)
	}
	t.Logf("Login succeeded: token=%s...", tokenResp.Token[:min(len(tokenResp.Token), 20)])

	// 2. List models
	models, err := client.ListModels(ctx, nil)
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].ModelId != "qwen-35-9b" {
		t.Errorf("unexpected model id: %s", models[0].ModelId)
	}
	t.Logf("ListModels succeeded: %s (%s)", models[0].ModelId, models[0].DisplayName)

	// 3. Chat — use JSON round-trip to construct ChatMessage (union field is unexported)
	var chatReq gen.ChatCompletionRequest
	chatReq.Model = "qwen-35-9b"
	if err := json.Unmarshal([]byte(`{"messages":[{"role":"user","content":"Hello"}]}`), &chatReq); err != nil {
		t.Fatalf("failed to construct chat request: %v", err)
	}
	// Ensure the model field survives the round-trip (it's in the outer struct, not the embedded JSON)
	chatReq.Model = "qwen-35-9b"

	chatResp, err := client.Chat(ctx, &chatReq)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if len(chatResp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(chatResp.Choices))
	}
	if chatResp.Choices[0].Message == nil {
		t.Fatal("expected message in choice")
	}
	if chatResp.Choices[0].Message.Role != "assistant" {
		t.Errorf("unexpected role: %s", chatResp.Choices[0].Message.Role)
	}
	t.Logf("Chat succeeded: assistant said %q", chatResp.Choices[0].Message.Content)
}

// TestClientRegisterAndProfile tests registration and profile retrieval.
func TestClientRegisterAndProfile(t *testing.T) {
	t.Parallel()

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/register":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", 405)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]string{
				"message": "Account created; verification email sent.",
			})

		case "/v1/auth/me":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
				"email":          "test@example.com",
				"display_name":   "Test",
				"email_verified": true,
				"created_at":     "2026-01-01T00:00:00Z",
			})

		default:
			http.Error(w, `{"error":"not found"}`, 404)
		}
	}))
	defer cloudSrv.Close()

	client, err := New(&Config{
		CloudBaseURL: cloudSrv.URL,
		APIKey:       "sb_cloud_test",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Register
	msg, err := client.Register(ctx, "test@example.com", "password123", nil)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if !strings.Contains(msg.Message, "created") {
		t.Errorf("unexpected register message: %s", msg.Message)
	}
	t.Logf("Register succeeded: %s", msg.Message)

	// Get profile
	profile, err := client.Me(ctx)
	if err != nil {
		t.Fatalf("Me failed: %v", err)
	}
	if profile.Email != "test@example.com" {
		t.Errorf("unexpected email: %s", profile.Email)
	}
	t.Logf("Profile succeeded: %s (%s)", profile.DisplayName, profile.Email)
}

// TestClientErrors checks that error responses are properly parsed.
func TestClientErrors(t *testing.T) {
	t.Parallel()

	cloudSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "unauthorized",
			"message": "Invalid or expired token",
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

	_, err = client.Me(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", apiErr.StatusCode)
	}
	if apiErr.Code != "unauthorized" {
		t.Errorf("expected code 'unauthorized', got %q", apiErr.Code)
	}
	if !apiErr.IsUnauthorized() {
		t.Error("IsUnauthorized() should be true")
	}
	t.Logf("Got expected error: %v", apiErr)
}

// TestModelFilter validates ModelFilter behavior.
func TestModelFilter(t *testing.T) {
	t.Parallel()

	f := &ModelFilter{
		ModelType:        gen.ModelEntryModelTypeLlm,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
	}

	if err := f.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}

	bad := &ModelFilter{ModelType: "invalid"}
	if err := bad.Validate(); err == nil {
		t.Error("expected validation error for invalid model type")
	}
}
