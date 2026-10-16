package service

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// wib — zona waktu pelaporan (Asia/Jakarta, +07:00 tanpa DST).
var wib = time.FixedZone("WIB", 7*3600)

// ReportService — agregasi laporan kunjungan (read-only).
type ReportService struct {
	db *gorm.DB
}

func NewReportService(db *gorm.DB) *ReportService {
	return &ReportService{db: db}
}

// ParseRange mengubah from/to (YYYY-MM-DD atau RFC3339, dianggap WIB) menjadi rentang UTC.
// Default: hari ini WIB.
func ParseRange(fromStr, toStr string) (time.Time, time.Time, error) {
	now := time.Now().In(wib)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, wib)
	to := from.Add(24 * time.Hour)
	if fromStr != "" {
		t, err := parseWIB(fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, ErrValidation
		}
		from = t
	}
	if toStr != "" {
		t, err := parseWIB(toStr)
		if err != nil {
			return time.Time{}, time.Time{}, ErrValidation
		}
		if len(toStr) <= 10 { // tanggal saja → sampai akhir hari
			to = t.Add(24 * time.Hour)
		} else {
			to = t
		}
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, ErrValidation
	}
	return from.UTC(), to.UTC(), nil
}

func parseWIB(s string) (time.Time, error) {
	if len(s) == 10 {
		return time.ParseInLocation("2006-01-02", s, wib)
	}
	return time.ParseInLocation("2006-01-02T15:04", s, wib)
}

func (s *ReportService) scope(from, to time.Time) *gorm.DB {
	return s.db.Table("library_visits").Where("status = ? AND check_in_at >= ? AND check_in_at < ?", "recorded", from, to)
}

type ReportSummary struct {
	TotalVisits    int64        `json:"total_visits"`
	UniqueVisitors int64        `json:"unique_visitors"`
	ByType         []LabelCount `json:"by_type"`
	ByMethod       []LabelCount `json:"by_method"`
	ByPurpose      []LabelCount `json:"by_purpose"`
	NimFallback    int64        `json:"nim_fallback"`
	From           time.Time    `json:"from"`
	To             time.Time    `json:"to"`
}

type LabelCount struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

var methodNames = map[string]string{
	"nfc": "NFC", "member_number": "NIM (lupa kartu)", "guest_barcode": "Tamu (barcode)", "manual": "Manual",
}

func (s *ReportService) Summary(from, to time.Time) (*ReportSummary, error) {
	out := &ReportSummary{From: from.In(wib), To: to.In(wib)}
	q := s.scope(from, to)
	if err := q.Count(&out.TotalVisits).Error; err != nil {
		return nil, err
	}
	if err := q.Session(&gorm.Session{}).
		Select("COUNT(DISTINCT COALESCE(member_id, guest_id))").
		Scan(&out.UniqueVisitors).Error; err != nil {
		return nil, err
	}
	out.ByType = []LabelCount{}
	if err := q.Session(&gorm.Session{}).
		Select("COALESCE(NULLIF(visitor_type,''),'lainnya') AS label, COUNT(*) AS count").
		Group("COALESCE(NULLIF(visitor_type,''),'lainnya')").Order("count DESC").Scan(&out.ByType).Error; err != nil {
		return nil, err
	}
	rows := []LabelCount{}
	if err := q.Session(&gorm.Session{}).
		Select("entry_method AS label, COUNT(*) AS count").
		Group("entry_method").Order("count DESC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		if n, ok := methodNames[r.Label]; ok {
			r.Label = n
		}
		if r.Label == "member_number" {
			out.NimFallback = r.Count
		}
		out.ByMethod = append(out.ByMethod, r)
	}
	out.ByPurpose = []LabelCount{}
	if err := q.Session(&gorm.Session{}).
		Select("COALESCE(p.name,'(tanpa tujuan)') AS label, COUNT(*) AS count").
		Joins("LEFT JOIN library_visit_purposes p ON p.id = library_visits.purpose_id").
		Group("COALESCE(p.name,'(tanpa tujuan)')").Order("count DESC").Scan(&out.ByPurpose).Error; err != nil {
		return nil, err
	}
	return out, nil
}

type BucketCount struct {
	Bucket string `json:"bucket"`
	Count  int64  `json:"count"`
}

// Timeseries — bucket per jam (g="hour") atau hari (g="day"); default otomatis
// (rentang ≤48 jam → jam, selain itu hari). Waktu dalam WIB.
func (s *ReportService) Timeseries(from, to time.Time, g string) ([]BucketCount, string, error) {
	if g != "hour" && g != "day" {
		if to.Sub(from) <= 48*time.Hour {
			g = "hour"
		} else {
			g = "day"
		}
	}
	trunc := "hour"
	fmtStr := "2006-01-02T15"
	if g == "day" {
		trunc = "day"
		fmtStr = "2006-01-02"
	}
	rows := []struct {
		B time.Time `json:"-"  gorm:"column:b"`
		C int64     `json:"-"  gorm:"column:c"`
	}{}
	err := s.scope(from, to).
		Select(fmt.Sprintf("date_trunc('%s', check_in_at AT TIME ZONE 'Asia/Jakarta') AS b, COUNT(*) AS c", trunc)).
		Group(fmt.Sprintf("date_trunc('%s', check_in_at AT TIME ZONE 'Asia/Jakarta')", trunc)).Order("b").Scan(&rows).Error
	if err != nil {
		return nil, "", err
	}
	out := make([]BucketCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, BucketCount{Bucket: r.B.Format(fmtStr), Count: r.C})
	}
	return out, g, nil
}

// Breakdown — agregasi per dimensi: faculty|study_program|purpose|type|method|location.
func (s *ReportService) Breakdown(dimension string, from, to time.Time) ([]LabelCount, error) {
	var sel, join string
	switch dimension {
	case "faculty":
		sel = "COALESCE(NULLIF(snapshot_faculty,''),'(tanpa fakultas)')"
	case "study_program":
		sel = "COALESCE(NULLIF(snapshot_study_program,''),'(tanpa prodi)')"
	case "purpose":
		sel = "COALESCE(p.name,'(tanpa tujuan)')"
		join = "LEFT JOIN library_visit_purposes p ON p.id = library_visits.purpose_id"
	case "type":
		sel = "COALESCE(NULLIF(visitor_type,''),'lainnya')"
	case "method":
		sel = "entry_method"
	case "location":
		sel = "COALESCE(l.name,'(tanpa lokasi)')"
		join = "LEFT JOIN library_locations l ON l.id = library_visits.location_id"
	default:
		return nil, ErrValidation
	}
	out := []LabelCount{}
	q := s.scope(from, to).Select(sel + " AS label, COUNT(*) AS count")
	if join != "" {
		q = q.Joins(join)
	}
	if err := q.Group(sel).Order("count DESC").Limit(50).Scan(&out).Error; err != nil {
		return nil, err
	}
	if dimension == "method" {
		for i := range out {
			if n, ok := methodNames[out[i].Label]; ok {
				out[i].Label = n
			}
		}
	}
	return out, nil
}

// ExportVisitRow — baris ekspor CSV (waktu sudah diformat WIB oleh SQL).
type ExportVisitRow struct {
	CheckIn string `gorm:"column:checkin"`
	Name    string `gorm:"column:name"`
	VType   string `gorm:"column:vtype"`
	Method  string `gorm:"column:method"`
	Purpose string `gorm:"column:purpose"`
	Faculty string `gorm:"column:faculty"`
	Program string `gorm:"column:program"`
	Unit    string `gorm:"column:unit"`
}

// ExportRows — maksimum 50.000 baris, urut terbaru dulu.
func (s *ReportService) ExportRows(from, to time.Time, visitorType, locationID string) ([]ExportVisitRow, error) {
	q := s.scope(from, to).
		Select(`to_char(library_visits.check_in_at AT TIME ZONE 'Asia/Jakarta', 'YYYY-MM-DD HH24:MI') AS checkin,
			COALESCE(library_visits.snapshot_name,'') AS name,
			library_visits.visitor_type AS vtype,
			library_visits.entry_method AS method,
			COALESCE(p.name,'') AS purpose,
			COALESCE(library_visits.snapshot_faculty,'') AS faculty,
			COALESCE(library_visits.snapshot_study_program,'') AS program,
			COALESCE(library_visits.snapshot_unit,'') AS unit`).
		Joins("LEFT JOIN library_visit_purposes p ON p.id = library_visits.purpose_id")
	if visitorType != "" {
		q = q.Where("library_visits.visitor_type = ?", visitorType)
	}
	if locationID != "" {
		q = q.Where("library_visits.location_id = ?", locationID)
	}
	rows := []ExportVisitRow{}
	if err := q.Order("library_visits.check_in_at DESC").Limit(50000).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// KioskHourCount — satu baris sebaran per jam.
type KioskHourCount struct {
	Hour  int   `json:"hour"`
	Count int64 `json:"count"`
}

// KioskDaily — bentuk respons stats kiosk.
type KioskDaily struct {
	Today     int64            `json:"today"`
	Yesterday int64            `json:"yesterday"`
	Hourly    []KioskHourCount `json:"hourly"`
}

// KioskDailyReport — ringkasan ringan untuk layar kiosk: total hari ini,
// total kemarin (untuk delta), dan sebaran kunjungan per jam hari ini (WIB).
func (s *ReportService) KioskDailyReport() (*KioskDaily, error) {
	now := time.Now().In(wib)
	todayFrom := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, wib)
	todayTo := todayFrom.Add(24 * time.Hour)
	yFrom := todayFrom.Add(-24 * time.Hour)
	out := &KioskDaily{Hourly: []KioskHourCount{}}
	if err := s.scope(todayFrom.UTC(), todayTo.UTC()).Count(&out.Today).Error; err != nil {
		return nil, err
	}
	if err := s.scope(yFrom.UTC(), todayFrom.UTC()).Count(&out.Yesterday).Error; err != nil {
		return nil, err
	}
	rows := []struct {
		H int64 `gorm:"column:h"`
		C int64 `gorm:"column:c"`
	}{}
	if err := s.scope(todayFrom.UTC(), todayTo.UTC()).
		Select("EXTRACT(HOUR FROM check_in_at AT TIME ZONE 'Asia/Jakarta') AS h, COUNT(*) AS c").
		Group("h").Order("h").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out.Hourly = append(out.Hourly, KioskHourCount{Hour: int(r.H), Count: r.C})
	}
	return out, nil
}

// WIB mengekspos zona waktu laporan untuk pemakaian lintas package.
func WIB() *time.Location { return wib }
