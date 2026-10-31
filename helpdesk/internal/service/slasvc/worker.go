package slasvc

import (
	"fmt"
	"time"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"github.com/rama/b-wise/helpdesk/internal/domain/ticketstate"
	"gorm.io/gorm"
)

// Worker - pemantau SLA berkala (SL-04): peringatan 75%, pelanggaran, eskalasi 150%.
type Worker struct {
	db   *gorm.DB
	log  func(format string, args ...interface{})
	now  func() time.Time
}

func NewWorker(db *gorm.DB, logFn func(string, ...interface{})) *Worker {
	if logFn == nil {
		logFn = func(string, ...interface{}) {}
	}
	return &Worker{db: db, log: logFn, now: time.Now}
}

func (w *Worker) event(ticketID, body string) {
	ev := entity.TicketEvent{Base: entity.Base{}, TicketID: ticketID, Kind: "system", Visibility: "public", Body: body, Meta: "{}"}
	_ = w.db.Create(&ev).Error
}

var activeStatuses = []string{ticketstate.Baru, ticketstate.MenungguApproval, ticketstate.Terbuka, ticketstate.Dikerjakan, ticketstate.MenungguPihakLain}

// ScanOnce - satu putaran pemantauan (resolusi clock; respons via laporan).
func (w *Worker) ScanOnce() {
	var list []entity.Ticket
	w.db.Where("status IN ? AND paused_at IS NULL AND due_resolution_at IS NOT NULL", activeStatuses).Find(&list)
	now := w.now()
	warned, breached, escalated := 0, 0, 0
	for i := range list {
		t := &list[i]
		if t.CreatedAt.IsZero() {
			continue
		}
		total := t.DueResolutionAt.Sub(t.CreatedAt)
		if total <= 0 {
			continue
		}
		elapsed := now.Sub(t.CreatedAt)
		pct := elapsed.Seconds() / total.Seconds()
		if now.After(*t.DueResolutionAt) {
			escalateAt := t.CreatedAt.Add(total * 3 / 2)
			if t.SLAEscalatedAt == nil && now.After(escalateAt) {
				w.db.Model(t).Update("sla_escalated_at", now)
				w.event(t.ID, "SLA ESKALASI: melewati 150% target - perlu perhatian admin unit")
				escalated++
			} else if t.SLABreachedAt == nil {
				w.db.Model(t).Update("sla_breached_at", now)
				w.event(t.ID, "SLA PELANGGARAN: melewati batas waktu penyelesaian")
				breached++
			}
		} else if pct >= 0.75 && t.SLAWarnedAt == nil {
			w.db.Model(t).Update("sla_warned_at", now)
			w.event(t.ID, "SLA peringatan: sudah 75% batas waktu penyelesaian")
			warned++
		}
	}
	if warned+breached+escalated > 0 {
		w.log("[sla-worker] peringatan=%d pelanggaran=%d eskalasi=%d", warned, breached, escalated)
	}
}

// RunInBackground - scan tiap interval (default 1 menit; blueprint 9.5).
func (w *Worker) RunInBackground(stopCh <-chan struct{}, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	w.log("[sla-worker] aktif (interval %s)", interval)
	w.ScanOnce() // scan pertama langsung, tak menunggu tick
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
