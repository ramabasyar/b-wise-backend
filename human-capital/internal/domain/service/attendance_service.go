package service

import (
	"errors"
	"fmt"
	"time"

	entity "github.com/rama/b-wise/human-capital/internal/domain/entity"
	"gorm.io/gorm"
)

// AttendanceService — presensi: check-in/out mandiri, input manual admin, impor CSV.
type AttendanceService struct {
	db *gorm.DB
}

func NewAttendanceService(db *gorm.DB) *AttendanceService { return &AttendanceService{db: db} }

var (
	ErrAttNoProfile  = errors.New("profil pegawai tidak ditemukan untuk user ini")
	ErrAttCheckedIn  = errors.New("sudah check-in hari ini")
	ErrAttNoCheckIn  = errors.New("belum check-in hari ini")
	ErrAttCheckedOut = errors.New("sudah check-out hari ini")
	ErrAttBadDate    = errors.New("format tanggal tidak valid (YYYY-MM-DD)")
)

func (s *AttendanceService) myEmployee(userID string) (*entity.Employee, error) {
	var e entity.Employee
	if err := s.db.Where("user_id = ?", userID).First(&e).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAttNoProfile
		}
		return nil, err
	}
	return &e, nil
}

func today() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
}

// CheckIn — mulai presensi hari ini (upsert baris tanggal hari ini).
func (s *AttendanceService) CheckIn(userID string) (*entity.AttendanceRecord, error) {
	emp, err := s.myEmployee(userID)
	if err != nil {
		return nil, err
	}
	var rec entity.AttendanceRecord
	d := today()
	err = s.db.Where("employee_id = ? AND date = ?", emp.ID, d.Format("2006-01-02")).First(&rec).Error
	now := time.Now()
	if err == nil {
		if rec.CheckIn != nil {
			return nil, ErrAttCheckedIn
		}
		upd := s.db.Model(&entity.AttendanceRecord{}).Where("id = ?", rec.ID).Updates(map[string]interface{}{"check_in": now, "status": "present"})
		if upd.Error != nil {
			return nil, upd.Error
		}
		rec.CheckIn = &now
		return &rec, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	rec = entity.AttendanceRecord{EmployeeID: emp.ID, Date: d, CheckIn: &now, Status: "present", Source: "app"}
	if err := s.db.Create(&rec).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// CheckOut — tutup presensi hari ini.
func (s *AttendanceService) CheckOut(userID string) (*entity.AttendanceRecord, error) {
	emp, err := s.myEmployee(userID)
	if err != nil {
		return nil, err
	}
	var rec entity.AttendanceRecord
	if err := s.db.Where("employee_id = ? AND date = ?", emp.ID, today().Format("2006-01-02")).First(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAttNoCheckIn
		}
		return nil, err
	}
	if rec.CheckIn == nil {
		return nil, ErrAttNoCheckIn
	}
	if rec.CheckOut != nil {
		return nil, ErrAttCheckedOut
	}
	now := time.Now()
	if err := s.db.Model(&entity.AttendanceRecord{}).Where("id = ?", rec.ID).Update("check_out", now).Error; err != nil {
		return nil, err
	}
	rec.CheckOut = &now
	return &rec, nil
}

// MyRecords — riwayat presensi bulan tertentu (format 2026-11; kosong = bulan ini).
func (s *AttendanceService) MyRecords(userID, month string) ([]entity.AttendanceRecord, error) {
	emp, err := s.myEmployee(userID)
	if err != nil {
		return nil, err
	}
	from, to, err := monthRange(month)
	if err != nil {
		return nil, err
	}
	var out []entity.AttendanceRecord
	err = s.db.Where("employee_id = ? AND date >= ? AND date < ?", emp.ID, from.Format("2006-01-02"), to.Format("2006-01-02")).Order("date DESC").Find(&out).Error
	return out, err
}

func monthRange(month string) (time.Time, time.Time, error) {
	if month == "" {
		n := time.Now()
		from := time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, n.Location())
		return from, from.AddDate(0, 1, 0), nil
	}
	from, err := time.Parse("2006-01", month)
	if err != nil {
		return time.Time{}, time.Time{}, ErrAttBadDate
	}
	return from, from.AddDate(0, 1, 0), nil
}

// AttendanceFilter — filter daftar admin.
type AttendanceFilter struct {
	EmployeeID string
	From, To   string // YYYY-MM-DD
	Status     string
	Page, Size int
}

func (s *AttendanceService) ListRecords(f AttendanceFilter) ([]entity.AttendanceRecord, int64, error) {
	q := s.db.Model(&entity.AttendanceRecord{})
	if f.EmployeeID != "" {
		q = q.Where("employee_id = ?", f.EmployeeID)
	}
	if f.From != "" && f.To != "" {
		q = q.Where("date >= ? AND date <= ?", f.From, f.To)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []entity.AttendanceRecord
	size := f.Size
	if size <= 0 {
		size = 20
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	err := q.Preload("Employee").Order("date DESC, updated_at DESC").Offset((page - 1) * size).Limit(size).Find(&out).Error
	return out, total, err
}

// UpsertManual — input/koreksi manual oleh admin (source=manual).
func (s *AttendanceService) UpsertManual(employeeID, date, checkIn, checkOut, status, notes, actor string) (*entity.AttendanceRecord, error) {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, ErrAttBadDate
	}
	if status == "" {
		status = "present"
	}
	var rec entity.AttendanceRecord
	err = s.db.Where("employee_id = ? AND date = ?", employeeID, d.Format("2006-01-02")).First(&rec).Error
	upd := map[string]interface{}{"status": status, "source": "manual"}
	if notes != "" {
		upd["notes"] = notes
	}
	if checkIn != "" {
		ci, e := time.Parse("2006-01-02 15:04", date+" "+checkIn)
		if e != nil {
			return nil, ErrAttBadDate
		}
		upd["check_in"] = ci
	}
	if checkOut != "" {
		co, e := time.Parse("2006-01-02 15:04", date+" "+checkOut)
		if e != nil {
			return nil, ErrAttBadDate
		}
		upd["check_out"] = co
	}
	if err == nil {
		if e := s.db.Model(&entity.AttendanceRecord{}).Where("id = ?", rec.ID).Updates(upd).Error; e != nil {
			return nil, e
		}
		s.db.Where("id = ?", rec.ID).First(&rec)
		return &rec, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	rec = entity.AttendanceRecord{EmployeeID: employeeID, Date: d, Status: status, Source: "manual", Notes: notes}
	if v, ok := upd["check_in"]; ok {
		rec.CheckIn = v.(*time.Time)
	}
	if v, ok := upd["check_out"]; ok && rec.CheckIn != nil {
		rec.CheckOut = v.(*time.Time)
	}
	if err := s.db.Create(&rec).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// AttendanceImportRow — satu baris CSV impor.
type AttendanceImportRow struct {
	EmployeeNumber string // NIP
	Date           string // YYYY-MM-DD
	CheckIn        string // HH:MM (opsional)
	CheckOut       string // HH:MM (opsional)
	Status         string // present|half|absent|leave (opsional, default present)
}

// ImportRows — impor massal (source=import). Pegawai dicocokkan via NIP.
func (s *AttendanceService) ImportRows(rows []AttendanceImportRow) (okCount, failCount int, firstErr string) {
	for _, r := range rows {
		if r.EmployeeNumber == "" || r.Date == "" {
			failCount++
			if firstErr == "" {
				firstErr = fmt.Sprintf("baris dengan NIP/tanggal kosong")
			}
			continue
		}
		var emp entity.Employee
		if err := s.db.Where("employee_number = ?", r.EmployeeNumber).First(&emp).Error; err != nil {
			failCount++
			if firstErr == "" {
				firstErr = fmt.Sprintf("NIP %s tidak ditemukan", r.EmployeeNumber)
			}
			continue
		}
		status := r.Status
		if status == "" {
			status = "present"
		}
		if _, err := s.UpsertManual(emp.ID, r.Date, r.CheckIn, r.CheckOut, status, "", ""); err != nil {
			failCount++
			if firstErr == "" {
				firstErr = fmt.Sprintf("NIP %s tanggal %s: %v", r.EmployeeNumber, r.Date, err)
			}
			continue
		}
		okCount++
	}
	return okCount, failCount, firstErr
}

// AttendanceSummaryRow — rekap bulanan per pegawai.
type AttendanceSummaryRow struct {
	EmployeeID     string `json:"employee_id"`
	EmployeeName   string `json:"employee_name"`
	EmployeeNumber string `json:"employee_number"`
	Present        int64  `json:"present"`
	Half           int64  `json:"half"`
	Absent         int64  `json:"absent"`
	Leave          int64  `json:"leave"`
}

func (s *AttendanceService) MonthlySummary(month string) ([]AttendanceSummaryRow, error) {
	from, to, err := monthRange(month)
	if err != nil {
		return nil, err
	}
	type aggRow struct {
		EmployeeID     string
		Status         string
		C              int64
		FullName       string
		EmployeeNumber string
	}
	var aggs []aggRow
	err = s.db.Table("attendance_records").
		Select("attendance_records.employee_id, attendance_records.status, count(*) as c, employees.full_name, employees.employee_number").
		Joins("JOIN employees ON employees.id = attendance_records.employee_id").
		Where("date >= ? AND date < ?", from.Format("2006-01-02"), to.Format("2006-01-02")).
		Group("attendance_records.employee_id, attendance_records.status, employees.full_name, employees.employee_number").
		Scan(&aggs).Error
	if err != nil {
		return nil, err
	}
	idx := map[string]*AttendanceSummaryRow{}
	out := []AttendanceSummaryRow{}
	for _, a := range aggs {
		row, ok := idx[a.EmployeeID]
		if !ok {
			row = &AttendanceSummaryRow{EmployeeID: a.EmployeeID, EmployeeName: a.FullName, EmployeeNumber: a.EmployeeNumber}
			idx[a.EmployeeID] = row
			out = append(out, *row)
		}
		switch a.Status {
		case "present":
			row.Present = a.C
		case "half":
			row.Half = a.C
		case "absent":
			row.Absent = a.C
		case "leave":
			row.Leave = a.C
		}
	}
	// perbaiki pointer (out berisi nilai awal — salin ulang)
	final := make([]AttendanceSummaryRow, 0, len(out))
	for _, a := range aggs {
		if row, ok := idx[a.EmployeeID]; ok {
			_ = row
		}
	}
	for id := range idx {
		final = append(final, *idx[id])
	}
	return final, nil
}
