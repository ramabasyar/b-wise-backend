package notifsvc

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/rama/b-wise/notify/internal/domain/entity"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func testKeys(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(make([]byte, 16))
}
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("NOTIFY_TEST_DSN")
	if dsn == "" {
		t.Skip("NOTIFY_TEST_DSN required for isolated PostgreSQL schema tests")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	schema := fmt.Sprintf("notify_test_%d", time.Now().UnixNano())
	if err = db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("SET search_path TO public")
		db.Exec("DROP SCHEMA " + schema + " CASCADE")
		sqlDB.Close()
	})
	if err = db.Exec("SET search_path TO " + schema).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&entity.Outbox{}, &entity.PushSubscription{}); err != nil {
		t.Fatal(err)
	}
	return db
}
func TestSubscriptionValidation(t *testing.T) {
	key, auth := testKeys(t)
	for _, endpoint := range []string{"https://fcm.googleapis.com/fcm/send/x", "https://updates.push.services.mozilla.com/wpush/v2/x", "https://web.push.apple.com/x"} {
		if err := validateSubscription(endpoint, key, auth); err != nil {
			t.Fatal(err)
		}
	}
	for _, endpoint := range []string{"http://127.0.0.1/x", "https://169.254.169.254/x", "https://fcm.googleapis.com.evil.example/x", "https://u:p@fcm.googleapis.com/x"} {
		if validateSubscription(endpoint, key, auth) == nil {
			t.Fatal("accepted unsafe endpoint", endpoint)
		}
	}
	if validateSubscription("https://fcm.googleapis.com/x", "bad", auth) == nil {
		t.Fatal("accepted invalid key")
	}
}
func TestPushPayload(t *testing.T) {
	b, err := pushPayload(&entity.Outbox{Title: "Tiket", Body: "Baru", Payload: `{"url":"https://evil.example"}`})
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]interface{}
	json.Unmarshal(b, &v)
	if v["title"] != "Tiket" || v["body"] != "Baru" || v["url"] != "/dashboard" {
		t.Fatal(v)
	}
}
func TestPushRetryOwnershipAndIdempotency(t *testing.T) {
	db := testDB(t)
	s := New(db)
	s.vapidPrivate, s.vapidPublic, _ = webpush.GenerateVAPIDKeys()
	s.vapidSubject = "mailto:test@example.com"
	key, auth := testKeys(t)
	for _, suffix := range []string{"ok", "retry", "gone"} {
		if err := s.SaveSubscription("user", "https://fcm.googleapis.com/"+suffix, key, auth, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteSubscription("other", "https://fcm.googleapis.com/ok"); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&entity.PushSubscription{}).Count(&count)
	if count != 3 {
		t.Fatal("cross-user delete")
	}
	calls := map[string]int{}
	retryStatus := 429
	s.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls[r.URL.Path]++
		if r.Header.Get("Authorization") == "" || r.Header.Get("Content-Encoding") != "aes128gcm" {
			t.Fatal("missing encrypted web-push/VAPID")
		}
		code := 201
		switch r.URL.Path {
		case "/retry":
			code = retryStatus
		case "/gone":
			code = 410
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	first, err := s.Enqueue("push", "user", "user", "ticket", "Update", "Body", `{"url":"/dashboard"}`, "same-event")
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Enqueue("push", "user", "user", "ticket", "Update", "Body", `{}`, "same-event")
	if err != nil || again.ID != first.ID {
		t.Fatal("not idempotent", err)
	}
	s.ScanOnce()
	after, _ := s.Get(first.ID)
	if after.Status != "pending" || after.Attempts != 1 {
		t.Fatal(after)
	}
	db.Model(&entity.PushSubscription{}).Count(&count)
	if count != 2 {
		t.Fatal("429 deleted or 410 retained")
	}
	s.ScanOnce()
	if calls["/retry"] != 1 {
		t.Fatal("ignored backoff")
	}
	retryStatus = 201
	db.Model(first).Update("next_attempt_at", time.Now().Add(-time.Minute))
	s.ScanOnce()
	after, _ = s.Get(first.ID)
	if after.Status != "sent" {
		t.Fatal(after)
	}
	if calls["/ok"] != 1 || calls["/retry"] != 2 {
		t.Fatal("resent delivered device", calls)
	}
	empty, err := s.Enqueue("push", "nobody", "nobody", "t", "Title", "Body", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	s.ScanOnce()
	empty, _ = s.Get(empty.ID)
	if empty.Status != "skipped" {
		t.Fatal(empty)
	}
}
