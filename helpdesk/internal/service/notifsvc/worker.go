// Package notifsvc - worker outbox notifikasi (blueprint 9.6: outbox + adaptor kanal).
package notifsvc

import (
	"fmt"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"gorm.io/gorm"
)

// Worker - kirim notifikasi email tertunda (kanal in_app tak perlu diproses).
type Worker struct {
	db   *gorm.DB
	log  func(format string, args ...interface{})
}

func NewWorker(db *gorm.DB, logFn func(string, ...interface{})) *Worker {
	if logFn == nil {
		logFn = func(string, ...interface{}) {}
	}
	return &Worker{db: db, log: logFn}
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
	if !smtpReady() {
		return // email belum dikonfigurasi (F0-3) - in_app tetap jalan
	}
	var rows []entity.NotificationOutbox
	w.db.Where("channel = ? AND status = ?", "email", "pending").Limit(10).Find(&rows)
	for i := range rows {
		r := &rows[i]
		if err := sendEmail(r.Recipient, "Notifikasi Pusat Layanan", r.Payload); err != nil {
			w.log("[notif-worker] email gagal ke %s: %v", r.Recipient, err)
			w.db.Model(r).Updates(map[string]interface{}{"attempts": r.Attempts + 1, "next_attempt_at": time.Now().Add(time.Hour)})
			continue
		}
		w.db.Model(r).Update("status", "sent")
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

var _ = fmt.Sprintf
