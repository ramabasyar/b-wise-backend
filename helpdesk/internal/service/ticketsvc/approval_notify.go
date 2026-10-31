package ticketsvc

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"github.com/rama/b-wise/helpdesk/internal/domain/ticketstate"
)

// RunApprovalWatchers - pengingat & eskalasi approval berkala (AP-04; blueprint 6.5).
func (s *Service) RunApprovalWatchers(ctx context.Context, every time.Duration) {
	if ctx == nil {
		ctx = context.Background()
	}
	if every <= 0 {
		every = time.Minute
	}
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		s.scanApprovals()
		s.scanRequesterReminders()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.scanApprovals()
				s.scanRequesterReminders()
			}
		}
	}()
	log.Printf("[approval-watch] aktif (interval %s)", every)
}

// scanApprovals - pengingat pada 75% batas waktu; eskalasi ke atasan approver
// saat lewat batas (sekali; AP-04). Baris tanpa keputusan saja yang diproses.
func (s *Service) scanApprovals() {
	var rows []entity.TicketApproval
	s.db.Where("decision IS NULL OR decision = ''").Where("due_at IS NOT NULL").Find(&rows)
	now := s.now()
	for _, a := range rows {
		var t entity.Ticket
		if s.db.Where("id = ?", a.TicketID).First(&t).Error != nil {
			continue
		}
		if t.Status != ticketstate.MenungguApproval && t.Status != ticketstate.MenungguPemohon {
			continue
		}
		if now.After(*a.DueAt) {
			if a.EscalatedAt != nil {
				continue
			}
			esc := ""
			if a.ApproverID != "" {
				if sv, err := s.resolveSupervisor(a.ApproverID); err == nil && sv != "" {
					esc = sv
				}
			}
			updates := map[string]interface{}{"escalated_at": now}
			if a.RemindedAt == nil {
				updates["reminded_at"] = now
			}
			if esc != "" {
				updates["escalated_to"] = esc
				s.notifyInApp(esc, "approval_eskalasi", &t, "Eskalasi approval "+t.Number, "Approval tiket '"+t.Subject+"' melewati batas waktu keputusan - mohon perhatian.")
			}
			if a.ApproverID != "" {
				s.notifyInApp(a.ApproverID, "approval_pengingat", &t, "Pengingat approval "+t.Number, "Approval telah MELEWATI batas waktu keputusan.")
			}
			info := ""
			if esc != "" {
				info = " - dieskalasi ke atasan approver"
			}
			_ = s.AddSystemEvent(a.TicketID, "approval langkah "+fmt.Sprint(a.Step)+" lewat batas waktu"+info, "")
			s.db.Model(&a).Updates(updates)
		} else if a.RemindedAt == nil {
			total := a.DueAt.Sub(a.CreatedAt)
			elapsed := now.Sub(a.CreatedAt)
			if total > 0 && elapsed*4 >= total*3 {
				s.db.Model(&a).Update("reminded_at", now)
				if a.ApproverID != "" {
					s.notifyInApp(a.ApproverID, "approval_pengingat", &t, "Pengingat approval "+t.Number, "Approval mendekati batas waktu keputusan (75%).")
				}
				_ = s.AddSystemEvent(a.TicketID, "pengingat approval langkah "+fmt.Sprint(a.Step)+" (75% batas waktu)", "")
			}
		}
	}
}
