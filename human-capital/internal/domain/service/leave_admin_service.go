package service

import (
	"errors"
	"fmt"
	"time"

	entity "github.com/rama/b-wise/human-capital/internal/domain/entity"
	"gorm.io/gorm"
)

// LeaveAdminService — kebijakan cuti, alokasi saldo (per tahun), tanggal blokir.
// Pola direct-db (satu tx per operasi) mengikuti MovementService.
type LeaveAdminService struct {
	db *gorm.DB
}

func NewLeaveAdminService(db *gorm.DB) *LeaveAdminService { return &LeaveAdminService{db: db} }

var (
	ErrPolicyNotFound  = errors.New("kebijakan cuti tidak ditemukan")
	ErrBlockDateExists = errors.New("tanggal blokir sudah ada")
	ErrBlockDateInUse  = errors.New("tanggal blokir tidak ditemukan")
	ErrBalanceNotFound = errors.New("saldo cuti tidak ditemukan")
)

// EffectiveLeaveRule — aturan efektif setelah resolusi kebijakan.
type EffectiveLeaveRule struct {
	AnnualDays        float64
	CarryOverMax      float64
	CarryExpiryMonths int
	AllowHalfDay      bool
	Source            string // policy | policy-default | leave-type
}

// ResolveRule — kebijakan spesifik jenis kepegawaian → default → field LeaveType.
func (s *LeaveAdminService) ResolveRule(employmentTypeID *string, leaveTypeID string) (EffectiveLeaveRule, error) {
	var lt entity.LeaveType
	if err := s.db.Where("id = ?", leaveTypeID).First(&lt).Error; err != nil {
		return EffectiveLeaveRule{}, err
	}
	rule := EffectiveLeaveRule{
		AnnualDays: float64(lt.DefaultDays), CarryOverMax: lt.CarryOverMax,
		CarryExpiryMonths: 3, AllowHalfDay: lt.AllowHalfDay, Source: "leave-type",
	}
	var pol entity.LeavePolicy
	found := false
	if employmentTypeID != nil && *employmentTypeID != "" {
		if err := s.db.Where("leave_type_id = ? AND employment_type_id = ? AND active = true", leaveTypeID, *employmentTypeID).First(&pol).Error; err == nil {
			found = true
			rule.Source = "policy"
		}
	}
	if !found {
		if err := s.db.Where("leave_type_id = ? AND employment_type_id IS NULL AND active = true", leaveTypeID).First(&pol).Error; err == nil {
			found = true
			rule.Source = "policy-default"
		}
	}
	if found {
		rule.AnnualDays = pol.AnnualDays
		rule.CarryOverMax = pol.CarryOverMax
		if pol.CarryExpiryMonths > 0 {
			rule.CarryExpiryMonths = pol.CarryExpiryMonths
		}
		rule.AllowHalfDay = pol.AllowHalfDay
	}
	return rule, nil
}

// LeaveRemaining — sisa efektif: entitled (+ carry yang belum hangus) - used.
func LeaveRemaining(b *entity.LeaveBalance, now time.Time) float64 {
	rem := b.Entitled - b.Used
	if b.CarriedOver > 0 && (b.CarryExpiresAt == nil || now.Before(*b.CarryExpiresAt)) {
		rem += b.CarriedOver
	}
	return rem
}

// ---------- Kebijakan ----------

func (s *LeaveAdminService) ListPolicies() ([]entity.LeavePolicy, error) {
	var out []entity.LeavePolicy
	err := s.db.Preload("LeaveType").Order("leave_type_id, employment_type_id NULLS FIRST").Find(&out).Error
	return out, err
}

// UpsertPolicy — idempoten berdasarkan pasangan (employment_type, leave_type).
func (s *LeaveAdminService) UpsertPolicy(p *entity.LeavePolicy) error {
	q := s.db.Where("leave_type_id = ?", p.LeaveTypeID)
	if p.EmploymentTypeID != nil && *p.EmploymentTypeID != "" {
		q = q.Where("employment_type_id = ?", *p.EmploymentTypeID)
	} else {
		q = q.Where("employment_type_id IS NULL")
		p.EmploymentTypeID = nil
	}
	var existing entity.LeavePolicy
	if err := q.First(&existing).Error; err == nil {
		p.ID = existing.ID
		return s.db.Model(&entity.LeavePolicy{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
			"name": p.Name, "annual_days": p.AnnualDays, "carry_over_max": p.CarryOverMax,
			"carry_expiry_months": p.CarryExpiryMonths, "allow_half_day": p.AllowHalfDay,
			"active": p.Active, "employment_type_id": p.EmploymentTypeID,
		}).Error
	}
	return s.db.Create(p).Error
}

func (s *LeaveAdminService) DeletePolicy(id string) error {
	res := s.db.Where("id = ?", id).Delete(&entity.LeavePolicy{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrPolicyNotFound
	}
	return nil
}

// ---------- Tanggal blokir ----------

func (s *LeaveAdminService) ListBlockDates(year int) ([]entity.LeaveBlockDate, error) {
	var out []entity.LeaveBlockDate
	q := s.db.Order("date DESC")
	if year > 0 {
		q = q.Where("date >= ? AND date < ?", fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-01-01", year+1))
	}
	err := q.Find(&out).Error
	return out, err
}

func (s *LeaveAdminService) CreateBlockDate(b *entity.LeaveBlockDate) error {
	trx := s.db.Where("date = ?", b.Date.Format("2006-01-02")).FirstOrCreate(b)
	if trx.RowsAffected == 0 {
		return ErrBlockDateExists
	}
	return trx.Error
}

func (s *LeaveAdminService) DeleteBlockDate(id string) error {
	res := s.db.Where("id = ?", id).Delete(&entity.LeaveBlockDate{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrBlockDateInUse
	}
	return nil
}

// BlockedInRange — apakah ada tanggal blokir aktif dalam [start,end].
func (s *LeaveAdminService) BlockedInRange(start, end time.Time) (*entity.LeaveBlockDate, error) {
	var b entity.LeaveBlockDate
	err := s.db.Where("active = true AND date >= ? AND date <= ?", start.Format("2006-01-02"), end.Format("2006-01-02")).First(&b).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// ---------- Saldo / alokasi ----------

// BalanceAdminView — baris saldo utk halaman admin.
type BalanceAdminView struct {
	entity.LeaveBalance
	EmployeeName   string  `json:"employee_name"`
	EmployeeNumber string  `json:"employee_number"`
	TypeCode       string  `json:"type_code"`
	TypeName       string  `json:"type_name"`
	Remaining      float64 `json:"remaining"`
}

func (s *LeaveAdminService) ListBalances(year int, query string, page, size int) ([]BalanceAdminView, int64, error) {
	var rows []entity.LeaveBalance
	q := s.db.Preload("LeaveType").Where("year = ?", year)
	var total int64
	if err := q.Model(&entity.LeaveBalance{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("updated_at DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	// join manual nama pegawai (hindari N+1 berat: satu query utk semua employee_id)
	empIDs := make([]string, 0, len(rows))
	for i := range rows {
		empIDs = append(empIDs, rows[i].EmployeeID)
	}
	type empRow struct{ ID, FullName, EmployeeNumber string }
	temps := map[string]empRow{}
	if len(empIDs) > 0 {
		var er []empRow
		s.db.Table("employees").Select("id, full_name, employee_number").Where("id IN ?", empIDs).Scan(&er)
		for _, e := range er {
			temps[e.ID] = e
		}
	}
	out := make([]BalanceAdminView, 0, len(rows))
	for i := range rows {
		v := BalanceAdminView{LeaveBalance: rows[i], Remaining: LeaveRemaining(&rows[i], time.Now())}
		if e, ok := temps[rows[i].EmployeeID]; ok {
			v.EmployeeName, v.EmployeeNumber = e.FullName, e.EmployeeNumber
		}
		if rows[i].LeaveType != nil {
			v.TypeCode, v.TypeName = rows[i].LeaveType.Code, rows[i].LeaveType.Name
		}
		if query == "" || containsFold(v.EmployeeName, query) || containsFold(v.EmployeeNumber, query) || containsFold(v.TypeCode, query) {
			out = append(out, v)
		}
	}
	return out, total, nil
}

func containsFold(s, q string) bool {
	return len(q) == 0 || (len(s) >= len(q) && indexFold(s, q) >= 0)
}

func indexFold(s, q string) int {
	for i := 0; i+len(q) <= len(s); i++ {
		if equalFold(s[i:i+len(q)], q) {
			return i
		}
	}
	return -1
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 65 && ca <= 90 {
			ca += 32
		}
		if cb >= 65 && cb <= 90 {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// AdjustBalance — ubah entitled/carried manual oleh admin.
func (s *LeaveAdminService) AdjustBalance(id string, entitled, carriedOver *float64) error {
	upd := map[string]interface{}{}
	if entitled != nil {
		upd["entitled"] = *entitled
	}
	if carriedOver != nil {
		upd["carried_over"] = *carriedOver
	}
	if len(upd) == 0 {
		return nil
	}
	res := s.db.Model(&entity.LeaveBalance{}).Where("id = ?", id).Updates(upd)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrBalanceNotFound
	}
	return nil
}

// GenerateBalances — isi saldo tahun tertentu utk semua pegawai aktif × jenis aktif.
// Idempoten: baris yang sudah ada TIDAK ditimpa entitled-nya; carry-over dihitung
// dari sisa tahun sebelumnya (dibatasi CarryOverMax kebijakan) dan hanya diisi bila masih 0.
func (s *LeaveAdminService) GenerateBalances(year int) (created int, err error) {
	var employees []entity.Employee
	if err = s.db.Where("status = ?", "active").Find(&employees).Error; err != nil {
		return 0, err
	}
	var types []entity.LeaveType
	if err = s.db.Where("active = true").Find(&types).Error; err != nil {
		return 0, err
	}
	now := time.Now()
	for i := range employees {
		for j := range types {
			rule, rerr := s.ResolveRule(strPtr(employees[i].EmploymentTypeID), types[j].ID)
			if rerr != nil {
				continue
			}
			var bal entity.LeaveBalance
			errFound := s.db.Where("employee_id = ? AND leave_type_id = ? AND year = ?", employees[i].ID, types[j].ID, year).First(&bal).Error
			if errFound == nil {
				// isi carry-over bila belum ada
				if bal.CarriedOver == 0 && rule.CarryOverMax > 0 {
					var prev entity.LeaveBalance
					if pe := s.db.Where("employee_id = ? AND leave_type_id = ? AND year = ?", employees[i].ID, types[j].ID, year-1).First(&prev).Error; pe == nil {
						remain := LeaveRemaining(&prev, now)
						if remain < 0 {
							remain = 0
						}
						if remain > rule.CarryOverMax {
							remain = rule.CarryOverMax
						}
						if remain > 0 {
							exp := time.Date(year, time.Month(1), 1, 0, 0, 0, 0, now.Location()).AddDate(0, rule.CarryExpiryMonths, 0)
							s.db.Model(&entity.LeaveBalance{}).Where("id = ?", bal.ID).Updates(map[string]interface{}{"carried_over": remain, "carry_expires_at": exp})
						}
					}
				}
				continue
			}
			if !errors.Is(errFound, gorm.ErrRecordNotFound) {
				return created, errFound
			}
			nb := entity.LeaveBalance{EmployeeID: employees[i].ID, LeaveTypeID: types[j].ID, Year: year, Entitled: rule.AnnualDays}
			if rule.CarryOverMax > 0 {
				var prev entity.LeaveBalance
				if pe := s.db.Where("employee_id = ? AND leave_type_id = ? AND year = ?", employees[i].ID, types[j].ID, year-1).First(&prev).Error; pe == nil {
					remain := LeaveRemaining(&prev, now)
					if remain < 0 {
						remain = 0
					}
					if remain > rule.CarryOverMax {
						remain = rule.CarryOverMax
					}
					if remain > 0 {
						nb.CarriedOver = remain
						exp := time.Date(year, time.Month(1), 1, 0, 0, 0, 0, now.Location()).AddDate(0, rule.CarryExpiryMonths, 0)
						nb.CarryExpiresAt = &exp
					}
				}
			}
			if ce := s.db.Create(&nb).Error; ce != nil {
				continue // race/unique — skip
			}
			created++
		}
	}
	return created, nil
}

// strPtr — string kosong diperlakukan sebagai kebijakan default (nil).
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
