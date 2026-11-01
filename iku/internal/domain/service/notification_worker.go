package service

import (
	"fmt"
	"github.com/rama/b-wise/iku/pkg/notifyclient"
	"time"

	entity "github.com/rama/b-wise/iku/internal/domain/entity"
)

// ==================== OUTBOX WORKER (opsi C fase-1, pola notifsvc helpdesk) ====================
// Email/WA kini di-enqueue sebagai baris pending oleh Notify(); worker ini
// mengirim dengan retry backoff (10m × attempts, maks 5) — bukan kirim-lupa.

const notifMaxAttempts = 5

// ScanOnce — proses baris email/wa pending yang sudah due (maks 10 per putaran).
func (n *Notifier) ScanOnce() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.relay.Enabled() && n.smtpHost == "" && n.waURL == "" {
		return // kanal eksternal tak dikonfigurasi — in-app tetap jalan
	}
	var rows []entity.Notification
	now := time.Now()
	n.db.Where("channel IN ? AND status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)",
		[]string{"email", "wa", "push"}, "pending", now).Limit(10).Find(&rows)
	for i := range rows {
		r := &rows[i]
		var err error
		if n.relay.Enabled() {
			// Do not fabricate email addresses from user IDs. Fail visibly until a real contact is available.
			if r.Channel == "email" {
				err = fmt.Errorf("alamat email penerima belum tersedia; perlu resolusi profil SSO")
			} else {
				result, e := n.relay.Relay("iku:"+r.ID, notifyclient.Message{Channel: r.Channel, Recipient: r.UserID, UserID: r.UserID, Topic: r.Topic, Title: r.Title, Body: r.Body, Payload: `{"url":"/dashboard/iku/notifications"}`})
				err = e
				if err == nil {
					patch := map[string]interface{}{"error": result.Error}
					switch result.Status {
					case "sent":
						patch["status"] = "sent"
						patch["sent_at"] = time.Now()
					case "failed", "skipped":
						patch["status"] = result.Status
					}
					n.db.Model(r).Updates(patch)
					continue
				}
			}
		} else {
			switch r.Channel {
			case "email":
				err = n.sendEmail(r)
			case "wa":
				err = n.sendWA(r)
			default:
				continue
			}
		}
		if err == nil {
			_ = n.db.Model(r).Updates(map[string]interface{}{"status": "sent", "sent_at": time.Now(), "error": ""}).Error
			continue
		}
		att := r.Attempts + 1
		patch := map[string]interface{}{"attempts": att, "error": trunc(err.Error(), 480)}
		if att >= notifMaxAttempts {
			patch["status"] = "failed"
		} else {
			patch["next_attempt_at"] = time.Now().Add(time.Duration(att) * 10 * time.Minute)
		}
		_ = n.db.Model(r).Updates(patch).Error
	}
}

// RunInBackground — loop ScanOnce tiap `every`; stop channel opsional.
func (n *Notifier) RunInBackground(stop <-chan struct{}, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			n.ScanOnce()
		}
	}
}
