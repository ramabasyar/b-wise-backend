package service

import (
	"time"

	entity "github.com/rama/b-wise/iku/internal/domain/entity"
)

// ==================== OUTBOX WORKER (opsi C fase-1, pola notifsvc helpdesk) ====================
// Email/WA kini di-enqueue sebagai baris pending oleh Notify(); worker ini
// mengirim dengan retry backoff (10m × attempts, maks 5) — bukan kirim-lupa.

const notifMaxAttempts = 5

// ScanOnce — proses baris email/wa pending yang sudah due (maks 10 per putaran).
func (n *Notifier) ScanOnce() {
	if n.smtpHost == "" && n.waURL == "" {
		return // kanal eksternal tak dikonfigurasi — in-app tetap jalan
	}
	var rows []entity.Notification
	now := time.Now()
	n.db.Where("channel IN ? AND status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)",
		[]string{"email", "wa"}, "pending", now).Limit(10).Find(&rows)
	for i := range rows {
		r := &rows[i]
		var err error
		switch r.Channel {
		case "email":
			err = n.sendEmail(r)
		case "wa":
			err = n.sendWA(r)
		default:
			continue
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
