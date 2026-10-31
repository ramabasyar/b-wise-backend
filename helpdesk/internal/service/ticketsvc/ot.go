package ticketsvc

import (
	"fmt"
	"log"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"github.com/rama/b-wise/helpdesk/internal/domain/ticketstate"
)

// followUpSurveiKurang - LP-07: aspek bernilai 1 (Kurang) membuat tugas
// tindak lanjut otomatis untuk koordinator tim pemilik tiket.
func (s *Service) followUpSurveiKurang(ticketID string, c entity.CSATResponse) {
	aspek := map[string]int{
		"Keandalan": c.Reliability, "Responsivitas": c.Responsiveness, "Jaminan": c.Assurance,
		"Empati": c.Empathy, "Bukti Fisik": c.Tangible,
	}
	kurang := []string{}
	for nama, v := range aspek {
		if v == 1 {
			kurang = append(kurang, nama)
		}
	}
	if len(kurang) == 0 {
		return
	}
	var t entity.Ticket
	if s.db.Where("id = ?", ticketID).First(&t).Error != nil {
		return
	}
	var team entity.Team
	if s.db.Where("id = ?", t.TeamID).First(&team).Error != nil {
		return
	}
	assignee := ""
	var sup entity.TeamMember
	if s.db.Where("team_id = ? AND role = ? AND is_active = ?", team.ID, "supervisor", true).First(&sup).Error == nil {
		assignee = sup.UserID
	}
	task := entity.TicketTask{
		TicketID: ticketID, TeamID: team.ID, AssigneeID: assignee,
		Title: "Tindak lanjut survei kurang: " + joinComma(kurang), Status: "open",
	}
	due := s.loadCalendar(team.CalendarID).AddBusinessMinutes(s.now(), 1440)
	task.OLADueAt = &due
	if err := s.db.Create(&task).Error; err != nil {
		return
	}
	_ = s.AddSystemEvent(ticketID, "survei kepuasan bernilai Kurang - tugas tindak lanjut dibuat untuk koordinator", "")
	if assignee != "" {
		s.notifyInApp(assignee, "approval_pengingat", &t, "Tindak lanjut survei "+t.Number, "Survei kepuasan pemohon bernilai Kurang pada: "+joinComma(kurang))
	}
}

func joinComma(ss []string) string {
	out := ""
	for i, x := range ss {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}

// scanRequesterReminders - OT-03: menunggu pemohon 3 hari kerja -> pengingat
// (sekali); lewat auto_close_days hari kerja -> tutup otomatis (per layanan).
func (s *Service) scanRequesterReminders() {
	var rows []entity.Ticket
	s.db.Where("status = ?", ticketstate.MenungguPemohon).Find(&rows)
	for _, t := range rows {
		var team entity.Team
		if s.db.Where("id = ?", t.TeamID).First(&team).Error != nil {
			continue
		}
		cal := s.loadCalendar(team.CalendarID)
		age := cal.BusinessMinutesBetween(t.UpdatedAt, s.now())
		autoDays := 0
		if t.ServiceID != "" {
			var sv entity.Service
			if s.db.Where("id = ?", t.ServiceID).First(&sv).Error == nil {
				autoDays = sv.AutoCloseDays
			}
		}
		if autoDays > 0 && age >= autoDays*480 {
			_ = s.db.Model(&t).Updates(map[string]interface{}{"status": ticketstate.Ditutup, "closed_at": s.now()}).Error
			_ = s.AddSystemEvent(t.ID, "ditutup otomatis: pemohon tidak merespons "+fmt.Sprint(autoDays)+" hari kerja (OT-03)", "")
			s.notifyInApp(t.RequesterUserID, "status_tiket", &t, "Tiket "+t.Number+" ditutup otomatis", "Tiket ditutup otomatis karena tidak ada respons pemohon.")
			log.Printf("[ot03] tiket %s ditutup otomatis (pemohon %d hari kerja)", t.Number, autoDays)
		} else if t.RemindedPemohonAt == nil && age >= 3*480 {
			_ = s.db.Model(&t).Update("reminded_pemohon_at", s.now()).Error
			s.notifyInApp(t.RequesterUserID, "approval_pengingat", &t, "Pengingat tiket "+t.Number, "Tiket Anda menunggu respons - mohon balas agar dapat dilanjutkan.")
			_ = s.AddSystemEvent(t.ID, "pengingat pemohon (menunggu 3 hari kerja, OT-03)", "")
		}
	}
}
