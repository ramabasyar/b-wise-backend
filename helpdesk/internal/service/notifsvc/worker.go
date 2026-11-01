// Package notifsvc - worker outbox notifikasi (blueprint 9.6: outbox + adaptor kanal).
package notifsvc

import (
	"encoding/json"
	"github.com/rama/b-wise/helpdesk/pkg/notifyclient"
	"net/smtp"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"gorm.io/gorm"
)

// Worker - kirim notifikasi email tertunda (kanal in_app tak perlu diproses).
type Worker struct {
	mu    sync.Mutex
	relay *notifyclient.Client
	db    *gorm.DB
	log   func(format string, args ...interface{})
}

func NewWorker(db *gorm.DB, logFn func(string, ...interface{})) *Worker {
	if logFn == nil {
		logFn = func(string, ...interface{}) {}
	}
	return &Worker{db: db, log: logFn, relay: notifyclient.New()}
}

func smtpReady() bool {
	return os.Getenv("SMTP_HOST") != "" && os.Getenv("SMTP_USER") != ""
}

// sendEmail - kirim via SMTP (App Password Google Workspace; pola pkg/mailer SSO).
func sendEmail(to, subject, body string) error {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	if port == "" {
		port = "587"
	}
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASS")
	from := os.Getenv("SMTP_FROM")
	if from == "" {
		from = user
	}
	msg := strings.Join([]string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")
	auth := smtp.PlainAuth("", user, pass, host)
	return smtp.SendMail(host+":"+port, auth, from, []string{to}, []byte(msg))
}

// ScanOnce - proses baris email pending (maks N per putaran).
func (w *Worker) ScanOnce() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.relay.Enabled() && !smtpReady() {
		return
	}
	var rows []entity.NotificationOutbox
	w.db.Where("channel IN ? AND status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)", []string{"email", "push"}, "pending", time.Now()).Limit(10).Find(&rows)
	for i := range rows {
		r := &rows[i]
		var err error
		if w.relay.Enabled() {
			var payload map[string]string
			_ = json.Unmarshal([]byte(r.Payload), &payload)
			title := payload["title"]
			if title == "" {
				title = "Notifikasi Pusat Layanan"
			}
			body := payload["body"]
			if body == "" {
				body = r.Payload
			}
			result, e := w.relay.Relay("helpdesk:"+r.ID, notifyclient.Message{Channel: r.Channel, Recipient: r.Recipient, UserID: r.Recipient, Topic: r.Template, Title: title, Body: body, Payload: r.Payload})
			err = e
			if err == nil {
				switch result.Status {
				case "sent", "skipped", "failed":
					w.db.Model(r).Update("status", result.Status)
				}
				continue
			}
		} else if r.Channel == "email" {
			err = sendEmail(r.Recipient, "Notifikasi Pusat Layanan", r.Payload)
		} else {
			continue
		}
		if err == nil {
			w.db.Model(r).Update("status", "sent")
			continue
		}
		attempts := r.Attempts + 1
		patch := map[string]interface{}{"attempts": attempts, "next_attempt_at": time.Now().Add(time.Duration(attempts) * 10 * time.Minute)}
		if attempts >= 5 {
			patch["status"] = "failed"
		}
		w.db.Model(r).Updates(patch)
		w.log("[notif-worker] relay/send gagal: %v", err)
	}

}

func (w *Worker) RunInBackground(stopCh <-chan struct{}, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-t.C:
			w.ScanOnce()
		}
	}
}
