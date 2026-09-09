package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"

	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	"gorm.io/gorm"
)

// ==================== NOTIFICATION SERVICE (F7) ====================
// Notifikasi in-app selalu dibuat; email/WA dikirim via driver env-gated
// (SMTP_*, WA_WEBHOOK_*) — tanpa env → channel "log" (simulasi, aman utk dev).

type Notifier struct {
	db *gorm.DB

	smtpHost, smtpPort, smtpUser, smtpPass, smtpFrom string
	waURL, waToken                                   string
	adminIDs                                         []string
}

func NewNotifier(db *gorm.DB) *Notifier {
	n := &Notifier{db: db,
		smtpHost: os.Getenv("SMTP_HOST"), smtpPort: os.Getenv("SMTP_PORT"),
		smtpUser: os.Getenv("SMTP_USER"), smtpPass: os.Getenv("SMTP_PASS"),
		smtpFrom: os.Getenv("SMTP_FROM"), waURL: os.Getenv("WA_WEBHOOK_URL"), waToken: os.Getenv("WA_WEBHOOK_TOKEN"),
	}
	if n.smtpPort == "" {
		n.smtpPort = "587"
	}
	if n.smtpFrom == "" {
		n.smtpFrom = "iku-noreply@binawan.ac.id"
	}
	ids := os.Getenv("NOTIFICATION_ADMIN_IDS")
	if ids == "" {
		ids = "8" // default: super admin SSO
	}
	n.adminIDs = strings.Split(ids, ",")
	return n
}

// Notify — buat notifikasi in-app + kirim channel eksternal (best-effort, async-safe).
func (n *Notifier) Notify(userIDs []string, topic, title, body, refType, refID string) {
	for _, uid := range userIDs {
		if uid == "" {
			continue
		}
		inapp := &entity.Notification{UserID: uid, Channel: "inapp", Status: "sent", Topic: topic, Title: title, Body: body, RefType: refType, RefID: refID, SentAt: ptrTime(time.Now())}
		_ = n.db.Create(inapp).Error

		if n.smtpHost != "" {
			n.dispatch(&entity.Notification{UserID: uid, Channel: "email", Topic: topic, Title: title, Body: body}, false)
		}
		if n.waURL != "" {
			n.dispatch(&entity.Notification{UserID: uid, Channel: "wa", Topic: topic, Title: title, Body: body}, false)
		}
	}
}

// NotifyAdmins — broadcast ke NOTIFICATION_ADMIN_IDS (pimpinan/operator IKU).
func (n *Notifier) NotifyAdmins(topic, title, body, refType, refID string) {
	n.Notify(n.adminIDs, topic, title, body, refType, refID)
}

// dispatch — kirim satu channel & catat hasil (row audit pengiriman).
func (n *Notifier) dispatch(row *entity.Notification, persist bool) {
	if persist {
		_ = n.db.Create(row).Error
	}
	var err error
	switch row.Channel {
	case "email":
		err = n.sendEmail(row)
	case "wa":
		err = n.sendWA(row)
	}
	patch := map[string]interface{}{"status": "sent", "sent_at": time.Now()}
	if err != nil {
		patch["status"], patch["error"] = "failed", trunc(err.Error(), 480)
	}
	if persist {
		_ = n.db.Model(row).Updates(patch).Error
	}
}

func (n *Notifier) sendEmail(row *entity.Notification) error {
	addr := n.smtpHost + ":" + n.smtpPort
	msg := []byte("To: user-" + row.UserID + "\r\n" +
		"Subject: " + row.Title + "\r\n" +
		"MIME-version: 1.0;\r\nContent-Type: text/plain; charset=\"UTF-8\";\r\n\r\n" +
		row.Body + "\r\n")
	var auth smtp.Auth
	if n.smtpUser != "" {
		auth = smtp.PlainAuth("", n.smtpUser, n.smtpPass, n.smtpHost)
	}
	return smtp.SendMail(addr, auth, n.smtpFrom, []string{"user-" + row.UserID + "@binawan.ac.id"}, msg)
}

func (n *Notifier) sendWA(row *entity.Notification) error {
	payload, _ := json.Marshal(map[string]string{"user_id": row.UserID, "title": row.Title, "body": row.Body, "topic": row.Topic})
	req, err := http.NewRequest(http.MethodPost, n.waURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if n.waToken != "" {
		req.Header.Set("Authorization", "Bearer "+n.waToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook status %d", resp.StatusCode)
	}
	return nil
}

func ptrTime(t time.Time) *time.Time { return &t }
func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
