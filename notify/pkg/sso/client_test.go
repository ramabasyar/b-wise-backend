package sso

import (
	"context"
	"encoding/json"
	"net/http"
	"net/httptest"
	"testing"
)

func TestClient_LoginAndGetUser(t *testing.T) {
	// Create mock SSO server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			// Mock login
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(loginResponse{
				Success: true,
				Data: struct {
					AccessToken string `json:"access_token"`
				}{AccessToken: "mock-token-123"},
			})
		case "/api/v1/users/user-123":
			// Verify auth header
			if r.Header.Get("Authorization") != "Bearer mock-token-123" {
				w.WriteHeader(401)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data": map[string]interface{}{
					"id":             "user-123",
					"username":       "testuser",
					"email":          "test@example.com",
					"first_name":     "Test",
					"last_name":      "User",
					"status":         "active",
					"email_verified": true,
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "service@test.com", "password", nil)

	user, err := client.GetUser(context.Background(), "user-123")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}

	if user.ID != "user-123" {
		t.Errorf("Expected ID 'user-123', got '%s'", user.ID)
	}
	if user.Username != "testuser" {
		t.Errorf("Expected Username 'testuser', got '%s'", user.Username)
	}
	if user.Email != "test@example.com" {
		t.Errorf("Expected Email 'test@example.com', got '%s'", user.Email)
	}
}

func TestClient_LoginFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"success":false,"error":{"message":"invalid credentials"}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "bad@email.com", "wrongpass", nil)

	_, err := client.GetUser(context.Background(), "user-123")
	if err == nil {
		t.Error("Expected error for failed login, got nil")
	}
}

func TestClient_CheckPermission(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(loginResponse{
				Success: true,
				Data: struct {
					AccessToken string `json:"access_token"`
				}{AccessToken: "mock-token"},
			})
		case "/api/v1/roles/users/user-123/check":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(checkPermissionResponse{
				Success: true,
				Data: struct {
					HasPermission bool `json:"has_permission"`
				}{HasPermission: true},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "service@test.com", "password", nil)

	hasPerm, err := client.CheckPermission(context.Background(), "user-123", "items", "read")
	if err != nil {
		t.Fatalf("CheckPermission failed: %v", err)
	}
	if !hasPerm {
		t.Error("Expected hasPermission=true, got false")
	}
}

func TestClient_CacheHit(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.URL.Path {
		case "/api/v1/auth/login":
			json.NewEncoder(w).Encode(loginResponse{
				Success: true,
				Data: struct {
					AccessToken string `json:"access_token"`
				}{AccessToken: "mock-token"},
			})
		case "/api/v1/users/user-123":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data": map[string]interface{}{
					"id":       "user-123",
					"username": "cacheduser",
					"email":    "cached@test.com",
					"status":   "active",
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	// Create client with mock cache
	mockCache := &mockCache{data: make(map[string]string)}
	client := NewClient(server.URL, "service@test.com", "password", mockCache)

	// First call — should hit server
	_, err := client.GetUser(context.Background(), "user-123")
	if err != nil {
		t.Fatalf("First GetUser failed: %v", err)
	}
	firstCount := callCount

	// Second call — should hit cache
	user, err := client.GetUser(context.Background(), "user-123")
	if err != nil {
		t.Fatalf("Second GetUser failed: %v", err)
	}
	if user.Username != "cacheduser" {
		t.Errorf("Expected cached data, got %s", user.Username)
	}

	// Verify server wasn't called again for user (only login call)
	if callCount > firstCount+1 {
		t.Errorf("Expected cache to prevent second server call, but server was called %d times", callCount)
	}
}

// mockCache implements Cache interface for testing
type mockCache struct {
	data map[string]string
}

func (m *mockCache) Get(ctx context.Context, key string) (string, error) {
	val, ok := m.data[key]
	if !ok {
		return "", ErrCacheMiss
	}
	return val, nil
}

func (m *mockCache) Set(ctx context.Context, key, value string) error {
	m.data[key] = value
	return nil
}

func (m *mockCache) Delete(ctx context.Context, key string) error {
	delete(m.data, key)
	return nil
}
