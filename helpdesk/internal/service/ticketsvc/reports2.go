package ticketsvc

import (
	"fmt"
	"time"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"github.com/rama/b-wise/helpdesk/internal/service/slasvc"
)

// TeamByCode - cari tim aktif berdasarkan kode (filter laporan).
func (s *Service) TeamByCode(code string) (*entity.Team, error) {
	var t entity.Team
	if err := s.db.Where("code = ? AND is_active = ?", code, true).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	return &t, nil
}

// ===== Laporan lanjutan (LP-03..LP-06, blueprint 6.10) =====

// LKPSAspek - distribusi kepuasan satu aspek skala LKPS (SB/B/C/K).
type LKPSAspek struct {
	Aspek      string  `json:"aspek"`
	SangatBaik int     `json:"sangat_baik"`
	Baik       int     `json:"baik"`
	Cukup      int     `json:"cukup"`
	Kurang     int     `json:"kurang"`
	Total      int     `json:"total"`
	RataRata   float64 `json:"rata_rata"`
}

func dist(kolom string) string {
	return "SUM(CASE WHEN cr." + kolom + " = 4 THEN 1 ELSE 0 END) AS sangat_baik, " +
		"SUM(CASE WHEN cr." + kolom + " = 3 THEN 1 ELSE 0 END) AS baik, " +
		"SUM(CASE WHEN cr." + kolom + " = 2 THEN 1 ELSE 0 END) AS cukup, " +
		"SUM(CASE WHEN cr." + kolom + " = 1 THEN 1 ELSE 0 END) AS kurang, " +
		"COUNT(*) AS total, COALESCE(AVG(cr." + kolom + "),0) AS rata_rata"
}

// CSATLKPS - laporan kepuasan 5 aspek format LKPS (LP-04; filter unit penyedia,
// unit pemohon passthrough requester_unit_id, dan periode).
func (s *Service) CSATLKPS(from, to time.Time, teamID, requesterUnit string) ([]LKPSAspek, error) {
	aspek := []struct{ nama, kolom string }{
		{"Keandalan", "reliability"}, {"Responsivitas", "responsiveness"}, {"Jaminan", "assurance"},
		{"Empati", "empathy"}, {"Bukti Fisik", "tangible"},
	}
	out := make([]LKPSAspek, 0, len(aspek))
	for _, a := range aspek {
		q := s.db.Table("csat_responses cr").
			Joins("JOIN tickets t ON t.id = cr.ticket_id").
			Where("cr.submitted_at >= ? AND cr.submitted_at <= ?", from, to).
			Select(dist(a.kolom))
		if teamID != "" {
			q = q.Where("t.team_id = ?", teamID)
		}
		if requesterUnit != "" {
			q = q.Where("t.requester_unit_id = ?", requesterUnit)
		}
		var row LKPSAspek
		if err := q.Scan(&row).Error; err != nil {
			return nil, err
		}
		row.Aspek = a.nama
		out = append(out, row)
	}
	return out, nil
}

// CSVLKPS - baris CSV laporan LKPS + kolom rencana tindak lanjut (diisi prodi).
func CSVLKPS(rows []LKPSAspek) [][]string {
	out := [][]string{{"Aspek", "Sangat Baik", "Baik", "Cukup", "Kurang", "Jumlah", "Rata-rata (1-4)", "Rencana Tindak Lanjut"}}
	for _, r := range rows {
		out = append(out, []string{r.Aspek,
			fmt.Sprintf("%d", r.SangatBaik), fmt.Sprintf("%d", r.Baik),
			fmt.Sprintf("%d", r.Cukup), fmt.Sprintf("%d", r.Kurang),
			fmt.Sprintf("%d", r.Total), fmt.Sprintf("%.2f", r.RataRata), ""})
	}
	return out
}

// ServicePerformance - standar vs realisasi per layanan (LP-05).
type ServicePerformance struct {
	ServiceID    string  `json:"service_id"`
	Nama         string  `json:"nama"`
	TargetJam    float64 `json:"target_jam"`
	RealisasiJam float64 `json:"realisasi_jam"`
	Selesai      int     `json:"selesai"`
	SLAPatuhi    int     `json:"sla_patuh"`
}

func (s *Service) ServicesPerformance(teamID string, from, to time.Time) ([]ServicePerformance, error) {
	var svcs []entity.Service
	if err := s.db.Where("team_id = ?", teamID).Find(&svcs).Error; err != nil {
		return nil, err
	}
	policyTargets := map[string]float64{}
	var pols []entity.SLAPolicy
	s.db.Find(&pols)
	for _, p := range pols {
		tg, _ := slasvc.ParseTargets(p.Targets)
		if t, ok := tg["normal"]; ok {
			policyTargets[p.ID] = float64(t.ResolutionMin) / 60.0
		}
	}
	out := []ServicePerformance{}
	for _, sv := range svcs {
		var agg struct {
			Selesai int
			Patuh   int
			AvgJam  *float64
		}
		s.db.Raw("SELECT COUNT(*) AS selesai, SUM(CASE WHEN resolved_at <= due_resolution_at THEN 1 ELSE 0 END) AS patuh, AVG(EXTRACT(EPOCH FROM (resolved_at - created_at))/3600.0) AS avg_jam FROM tickets WHERE service_id = ? AND status = 'selesai' AND created_at >= ? AND created_at <= ?", sv.ID, from, to).Scan(&agg)
		row := ServicePerformance{ServiceID: sv.ID, Nama: sv.Name, Selesai: agg.Selesai, SLAPatuhi: agg.Patuh}
		if tg, ok := policyTargets[sv.SLAPolicyID]; ok {
			row.TargetJam = tg
		}
		if agg.AvgJam != nil {
			row.RealisasiJam = *agg.AvgJam
		}
		out = append(out, row)
	}
	return out, nil
}

// RoomReport - kerusakan/permintaan berulang per ruang (LP-06).
type RoomReport struct {
	Room           string `json:"room"`
	Jumlah         int    `json:"jumlah"`
	Terakhir       string `json:"terakhir"`
	SubjekTerakhir string `json:"subjek_terakhir"`
}

func (s *Service) RoomReport(from, to time.Time) ([]RoomReport, error) {
	rows := []RoomReport{}
	err := s.db.Raw("SELECT room_ref AS room, COUNT(*) AS jumlah, MAX(created_at)::text AS terakhir, (SELECT subject FROM tickets t2 WHERE t2.room_ref = t.room_ref ORDER BY created_at DESC LIMIT 1) AS subjek_terakhir FROM tickets t WHERE room_ref IS NOT NULL AND room_ref <> '' AND created_at >= ? AND created_at <= ? GROUP BY room_ref ORDER BY jumlah DESC, terakhir DESC LIMIT 50", from, to).Scan(&rows).Error
	return rows, err
}

// UnitLoad - beban kerja satu unit utk dashboard pimpinan (LP-03).
type UnitLoad struct {
	TeamCode  string  `json:"team_code"`
	TeamName  string  `json:"team_name"`
	Total     int     `json:"total"`
	Terbuka   int     `json:"terbuka"`
	Selesai   int     `json:"selesai"`
	SLAPatuhi int     `json:"sla_patuh"`
	SLATotal  int     `json:"sla_total"`
	AvgCSAT   float64 `json:"avg_csat"`
}

// Overview - lintas unit: beban tiap tim + layanan teratas (LP-03).
// TopService - layanan teratas berdasarkan volume (LP-03).
type TopService struct {
	Nama   string `json:"nama"`
	Tim    string `json:"tim"`
	Jumlah int    `json:"jumlah"`
}

// Overview - lintas unit: beban tiap tim + layanan teratas (LP-03).
type Overview struct {
	Units       []UnitLoad   `json:"units"`
	TopServices []TopService `json:"top_services"`
}

func (s *Service) Overview(from, to time.Time) (*Overview, error) {
	ov := &Overview{Units: []UnitLoad{}, TopServices: []TopService{}}
	err := s.db.Raw("SELECT tm.code AS team_code, tm.name AS team_name, COUNT(*) AS total, SUM(CASE WHEN t.status IN ('baru','menunggu_approval','terbuka','dikerjakan','menunggu_pihak_lain') THEN 1 ELSE 0 END) AS terbuka, SUM(CASE WHEN t.status IN ('selesai','ditutup') THEN 1 ELSE 0 END) AS selesai, SUM(CASE WHEN t.status IN ('selesai','ditutup') AND t.resolved_at IS NOT NULL AND t.resolved_at <= t.due_resolution_at THEN 1 ELSE 0 END) AS sla_patuh, SUM(CASE WHEN t.status IN ('selesai','ditutup') AND t.resolved_at IS NOT NULL THEN 1 ELSE 0 END) AS sla_total, COALESCE((SELECT AVG((cr.reliability+cr.responsiveness+cr.assurance+cr.empathy+cr.tangible)/5.0) FROM csat_responses cr JOIN tickets t2 ON t2.id = cr.ticket_id WHERE t2.team_id = tm.id AND cr.submitted_at >= ? AND cr.submitted_at <= ?),0) AS avg_csat FROM tickets t JOIN teams tm ON tm.id = t.team_id WHERE t.created_at >= ? AND t.created_at <= ? GROUP BY tm.code, tm.name, tm.id ORDER BY total DESC", from, to, from, to).Scan(&ov.Units).Error
	if err != nil {
		return nil, err
	}
	err = s.db.Raw("SELECT sv.name AS nama, tm.code AS tim, COUNT(*) AS jumlah FROM tickets t JOIN services sv ON sv.id = t.service_id JOIN teams tm ON tm.id = sv.team_id WHERE t.created_at >= ? AND t.created_at <= ? GROUP BY sv.name, tm.code ORDER BY jumlah DESC LIMIT 10", from, to).Scan(&ov.TopServices).Error
	return ov, err
}
