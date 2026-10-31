package ticketsvc

import (
	"fmt"
	"strings"
	"time"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"github.com/rama/b-wise/helpdesk/internal/domain/ticketstate"
)

// ===== Survei CSAT (PM-08, D-12) =====

// SubmitCSAT - pemohon mengisi survei 5 aspek skala 1-4 (hanya saat Selesai, sekali).
func (s *Service) SubmitCSAT(ticketID, userID string, c entity.CSATResponse) (*entity.CSATResponse, error) {
	var t entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	if t.RequesterUserID != userID {
		return nil, ErrForbidden
	}
	if t.Status != ticketstate.Selesai {
		return nil, fmt.Errorf("%w: survei hanya saat status Selesai", ErrValidation)
	}
	aspects := []int{c.Reliability, c.Responsiveness, c.Assurance, c.Empathy, c.Tangible}
	for _, v := range aspects {
		if v < 1 || v > 4 {
			return nil, fmt.Errorf("%w: nilai tiap aspek 1-4", ErrValidation)
		}
	}
	c.Base = entity.Base{}
	c.TicketID = ticketID
	c.SubmittedAt = s.now()
	if err := s.db.Create(&c).Error; err != nil {
		return nil, fmt.Errorf("%w: survei sudah pernah diisi", ErrValidation)
	}
	s.followUpSurveiKurang(ticketID, c)
	return &c, nil
}

// ===== Dashboard & ekspor (LP-01, LP-02) =====

// ReportSummary - ringkasan kinerja tim dalam satu periode.
type ReportSummary struct {
	Total               int64   `json:"total"`
	Open                int64   `json:"open"`
	Selesai             int64   `json:"selesai"`
	SLAOK               int64   `json:"sla_ok"`
	SLATotal            int64   `json:"sla_total"`
	AvgFirstResponseMin int64   `json:"avg_first_response_min"`
	AvgCSAT             float64 `json:"avg_csat"`
}

func (s *Service) ReportSummaryFor(userID string, from, to time.Time) (*ReportSummary, error) {
	team, err := s.TeamOf(userID)
	if err != nil {
		return nil, err
	}
	r := &ReportSummary{}
	s.db.Model(&entity.Ticket{}).Where("team_id = ? AND created_at >= ? AND created_at <= ?", team.ID, from, to).Count(&r.Total)
	s.db.Model(&entity.Ticket{}).Where("team_id = ? AND created_at >= ? AND created_at <= ? AND status IN ?", team.ID, from, to, []string{ticketstate.Baru, ticketstate.MenungguApproval, ticketstate.Terbuka, ticketstate.Dikerjakan, ticketstate.MenungguPihakLain}).Count(&r.Open)
	s.db.Model(&entity.Ticket{}).Where("team_id = ? AND created_at >= ? AND created_at <= ? AND status = ?", team.ID, from, to, ticketstate.Selesai).Count(&r.Selesai)
	r.SLATotal = r.Selesai
	s.db.Model(&entity.Ticket{}).Where("team_id = ? AND created_at >= ? AND created_at <= ? AND status = ? AND resolved_at <= due_resolution_at", team.ID, from, to, ticketstate.Selesai).Count(&r.SLAOK)
	type avgRow struct{ Min *float64 }
	var ar avgRow
	s.db.Model(&entity.Ticket{}).Select("AVG(EXTRACT(EPOCH FROM (first_response_at - created_at))/60) AS min").Where("team_id = ? AND first_response_at IS NOT NULL AND created_at >= ? AND created_at <= ?", team.ID, from, to).Scan(&ar)
	if ar.Min != nil {
		r.AvgFirstResponseMin = int64(*ar.Min)
	}
	type csatRow struct{ Avg *float64 }
	var cr csatRow
	s.db.Raw("SELECT AVG((reliability+responsiveness+assurance+empathy+tangible)/5.0) AS avg FROM csat_responses cr JOIN tickets t ON t.id = cr.ticket_id WHERE t.team_id = ? AND cr.submitted_at >= ? AND cr.submitted_at <= ?", team.ID, from, to).Scan(&cr)
	if cr.Avg != nil {
		r.AvgCSAT = *cr.Avg
	}
	return r, nil
}

// Konstanta karakter CSV via kode rune - bebas masalah escaping.
const (
	DQ = string(rune(34))
	SQ = string(rune(39))
	CM = string(rune(44))
)

func csvField(v string) string {
	return DQ + strings.ReplaceAll(v, DQ, SQ) + DQ
}

func timeStr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}

// ExportCSV - baris tiket tim periode (LP-02).
func (s *Service) ExportCSV(userID string, from, to time.Time) ([]string, error) {
	team, err := s.TeamOf(userID)
	if err != nil {
		return nil, err
	}
	var list []entity.Ticket
	err = s.db.Where("team_id = ? AND created_at >= ? AND created_at <= ?", team.ID, from, to).Order("created_at ASC").Find(&list).Error
	if err != nil {
		return nil, err
	}
	header := []string{"nomor", "subjek", "status", "prioritas", "kanal", "dibuat", "respons_pertama", "selesai", "target_penyelesaian", "patuh_sla"}
	rows := []string{strings.Join(header, CM)}
	for _, t := range list {
		sla := ""
		if t.ResolvedAt != nil && t.DueResolutionAt != nil {
			if !t.ResolvedAt.After(*t.DueResolutionAt) {
				sla = "ya"
			} else {
				sla = "tidak"
			}
		}
		subjek := t.Subject
		if t.Confidentiality == "rahasia" {
			subjek = "[rahasia]"
		}
		fields := []string{t.Number, csvField(subjek), t.Status, t.Priority, t.Channel,
			t.CreatedAt.Format("2006-01-02 15:04"), timeStr(t.FirstResponseAt), timeStr(t.ResolvedAt), timeStr(t.DueResolutionAt), sla}
		rows = append(rows, strings.Join(fields, CM))
	}
	return rows, nil
}
