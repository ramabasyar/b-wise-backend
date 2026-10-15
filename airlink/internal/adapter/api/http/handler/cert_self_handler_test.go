package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSelfUsernameUsesVerifiedSSOIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/userinfo" || r.Header.Get("Authorization") != "Bearer user-token" {
			t.Errorf("userinfo request tidak membawa bearer token yang benar")
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sub":"42","preferred_username":"rama.staff"}`))
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/airlink/certs/me", nil)
	context.Request.Header.Set("Authorization", "Bearer user-token")
	context.Set("user_id", "42")
	username, ok := (&CertHandler{ssoURL: server.URL}).selfUsername(context)
	if !ok || username != "rama.staff" || recorder.Code != http.StatusOK {
		t.Fatalf("identity valid ditolak: username=%q ok=%v status=%d", username, ok, recorder.Code)
	}
}

func TestSelfUsernameRejectsMismatchedTokenSubject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sub":"99","preferred_username":"other-user"}`))
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/airlink/certs/me", nil)
	context.Request.Header.Set("Authorization", "Bearer user-token")
	context.Set("user_id", "42")
	if username, ok := (&CertHandler{ssoURL: server.URL}).selfUsername(context); ok || username != "" || recorder.Code != http.StatusBadGateway {
		t.Fatalf("subject berbeda diterima: username=%q ok=%v status=%d", username, ok, recorder.Code)
	}
}
