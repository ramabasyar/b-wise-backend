package notifyclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRelayDoesNotConfuseAcceptedWithSent(t *testing.T) {
	calls := 0
	tokenCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/oauth/token" {
			tokenCalls++
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"access_token": "test-service"}})
			return
		}
		if r.Header.Get("Idempotency-Key") != "event-1" || r.Header.Get("Authorization") != "Bearer test-service" {
			t.Error("missing auth/idempotency")
		}
		calls++
		status := "pending"
		if calls == 2 {
			status = "sent"
		}
		json.NewEncoder(w).Encode(map[string]any{"data": Result{ID: "outbox-1", Status: status}})
	}))
	defer server.Close()
	t.Setenv("NOTIFY_URL", server.URL)
	t.Setenv("SSO_URL", server.URL)
	c := New()
	if !c.Enabled() {
		t.Fatal("not enabled")
	}
	first, err := c.Relay("event-1", Message{Channel: "push"})
	if err != nil || first.Status != "pending" {
		t.Fatal(first, err)
	}
	next, err := c.Relay("event-1", Message{Channel: "push"})
	if err != nil || next.Status != "sent" {
		t.Fatal(next, err)
	}
	if tokenCalls != 1 {
		t.Fatal("token cache not used")
	}
}
func TestRelayRejectsErrorAndMalformedResponse(t *testing.T) {
	code := 503
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/oauth/token" {
			json.NewEncoder(w).Encode(map[string]string{"access_token": "test"})
			return
		}
		w.WriteHeader(code)
		w.Write([]byte(`{"data":{}}`))
	}))
	defer server.Close()
	t.Setenv("NOTIFY_URL", server.URL)
	t.Setenv("SSO_URL", server.URL)
	c := New()
	if _, err := c.Relay("event-1", Message{}); err == nil {
		t.Fatal("accepted HTTP 503")
	}
	code = 201
	if _, err := c.Relay("event-1", Message{}); err == nil {
		t.Fatal("accepted malformed response")
	}
}
