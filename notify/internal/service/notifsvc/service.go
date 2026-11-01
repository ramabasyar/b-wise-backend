package notifsvc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rama/b-wise/notify/internal/domain/entity"
	"gorm.io/gorm"
)

// Service — outbox notifikasi terpusat B-Wise (fase-2 keputusan C, F4/PM-13).
// Konsumen kirim via POST /api/internal/notifications (service token); worker
// mengirim kanal email (SMTP), wa (webhook gateway), push (langganan PWA) dengan
// retry backoff 10m × attempts, maks 5 percobaan.
type Service struct {
	db *gorm.DB

	smtpHost, smtpPort, smtpUser, smtpPass, smtpFrom string
	waURL, waToken                                   string
	vapidSubject, vapidPublic, vapidPrivate          string
	client                                           *http.Client
	mu                                               sync.Mutex
}

func New(db *gorm.DB) *Service {
	n := &Service{db: db,
		smtpHost: os.Getenv("SMTP_HOST"), smtpPort: os.Getenv("SMTP_PORT"),
		smtpUser: os.Getenv("SMTP_USER"), smtpPass: os.Getenv("SMTP_PASS"),
		smtpFrom: os.Getenv("SMTP_FROM"), waURL: os.Getenv("WA_WEBHOOK_URL"), waToken: os.Getenv("WA_WEBHOOK_TOKEN"),
		vapidSubject: os.Getenv("PUSH_VAPID_SUBJECT"), vapidPublic: os.Getenv("PUSH_VAPID_PUBLIC"), vapidPrivate: os.Getenv("PUSH_VAPID_PRIVATE"),
		client: &http.Client{Timeout: 10 * time.Second},
	}
	if n.smtpPort == "" {
		n.smtpPort = "587"
	}
	if n.smtpFrom == "" {
		n.smtpFrom = "noreply@binawan.ac.id"
	}
	return n
}

func jsonbOr(v string) string {
	if v == "" {
		return "{}"
	}
	return v
}

var validChannels = map[string]bool{"email": true, "wa": true, "push": true}

// Enqueue — tambah baris outbox pending. Payload string JSON (boleh kosong).
func (s *Service) Enqueue(channel, recipient, userID, topic, title, body, payload string) (*entity.Outbox, error) {
	if !validChannels[channel] {
		return nil, fmt.Errorf("kanal %q tidak dikenal (email|wa|push)", channel)
	}
	if strings.TrimSpace(recipient) == "" {
		return nil, fmt.Errorf("recipient wajib diisi")
	}
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("title wajib diisi")
	}
	o := &entity.Outbox{Channel: channel, Recipient: recipient, UserID: userID, Topic: topic, Title: title, Body: body, Payload: jsonbOr(payload), Status: "pending"}
	if err := s.db.Create(o).Error; err != nil {
		return nil, err
	}
	return o, nil
}

// Get — satu baris outbox (status check konsumen).
func (s *Service) Get(id string) (*entity.Outbox, error) {
	var o entity.Outbox
	if err := s.db.First(&o, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &o, nil
}

// SaveSubscription — daftar/upsert langganan push per user.
func (s *Service) SaveSubscription(userID, endpoint, p256dh, auth, ua string) error {
	if userID == "" || endpoint == "" || p256dh == "" || auth == "" {
		return fmt.Errorf("endpoint, p256dh, auth, dan user wajib")
	}
	var exist entity.PushSubscription
	if err := s.db.Where("endpoint = ?", endpoint).First(&exist).Error; err == nil {
		exist.UserID = userID
		exist.P256DH = p256dh
		exist.Auth = auth
		exist.UserAgent = ua
		return s.db.Save(&exist).Error
	}
	return s.db.Create(&entity.PushSubscription{UserID: userID, Endpoint: endpoint, P256DH: p256dh, Auth: auth, UserAgent: ua}).Error
}

// DeleteSubscription — berhenti berlangganan (dari browser atau user).
func (s *Service) DeleteSubscription(endpoint string) error {
	return s.db.Where("endpoint = ?", endpoint).Delete(&entity.PushSubscription{}).Error
}

const maxAttempts = 5

// RunInBackground — loop ScanOnce tiap `every`; stop opsional.
func (s *Service) RunInBackground(stop <-chan struct{}, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.ScanOnce()
		}
	}
}

// ScanOnce — proses baris due (maks 20 per putaran).
func (s *Service) ScanOnce() {
	s.mu.Lock()
	defer s.mu.Unlock()
	var rows []entity.Outbox
	now := time.Now()
	s.db.Where("status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)", "pending", now).Limit(20).Find(&rows)
	for i := range rows {
		r := &rows[i]
		var err error
		switch r.Channel {
		case "email":
			err = s.sendEmail(r)
		case "wa":
			err = s.sendWA(r)
		case "push":
			err = s.sendPush(r)
		default:
			err = fmt.Errorf("kanal tidak dikenal: %s", r.Channel)
		}
		if err == nil {
			_ = s.db.Model(r).Updates(map[string]interface{}{"status": "sent", "sent_at": time.Now(), "error": ""}).Error
			continue
		}
		att := r.Attempts + 1
		patch := map[string]interface{}{"attempts": att, "error": trunc(err.Error(), 480)}
		if att >= maxAttempts {
			patch["status"] = "failed"
		} else {
			patch["next_attempt_at"] = time.Now().Add(time.Duration(att) * 10 * time.Minute)
		}
		_ = s.db.Model(r).Updates(patch).Error
	}
}

func trunc(v string, n int) string {
	if len(v) > n {
		return v[:n]
	}
	return v
}

// sendEmail — pola notifsvc helpdesk (SMTP_*, App Password Workspace).
func (s *Service) sendEmail(o *entity.Outbox) error {
	if s.smtpHost == "" || s.smtpUser == "" {
		return fmt.Errorf("SMTP belum dikonfigurasi (SMTP_HOST/SMTP_USER)")
	}
	msg := strings.Join([]string{
		"From: " + s.smtpFrom,
		"To: " + o.Recipient,
		"Subject: " + o.Title,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		o.Body,
	}, "\r\n")
	auth := smtp.PlainAuth("", s.smtpUser, s.smtpPass, s.smtpHost)
	return smtp.SendMail(s.smtpHost+":"+s.smtpPort, auth, s.smtpFrom, []string{o.Recipient}, []byte(msg))
}

// sendWA — pola IKU: POST webhook gateway (KN-05) + bearer.
func (s *Service) sendWA(o *entity.Outbox) error {
	if s.waURL == "" {
		return fmt.Errorf("WA gateway belum dikonfigurasi (WA_WEBHOOK_URL)")
	}
	payload, _ := json.Marshal(map[string]string{"to": o.Recipient, "title": o.Title, "body": o.Body, "topic": o.Topic})
	req, err := http.NewRequest(http.MethodPost, s.waURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.waToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.waToken)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("WA gateway status %d", resp.StatusCode)
	}
	return nil
}

// sendPush — kirim web-push ke semua langganan user (v1: pengiriman menyusul
// bersama lib web-push; langganan + antrean + infra sudah siap).
func (s *Service) sendPush(o *entity.Outbox) error {
	uid := o.Recipient
	if uid == "" {
		uid = o.UserID
	}
	var subs []entity.PushSubscription
	s.db.Where("user_id = ?", uid).Find(&subs)
	if len(subs) == 0 {
		return fmt.Errorf("tidak ada langganan push utk user %s", uid)
	}
	if s.vapidPublic == "" || s.vapidPrivate == "" {
		return fmt.Errorf("push VAPID belum dikonfigurasi (PUSH_VAPID_*) — %d langganan menunggu", len(subs))
	}
	return fmt.Errorf("pengiriman web-push aktif menyusul (integrasi lib web-push) — %d langganan", len(subs))
}
