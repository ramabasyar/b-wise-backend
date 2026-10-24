package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	entity "github.com/rama/b-wise/human-capital/internal/domain/entity"
	"github.com/rama/b-wise/human-capital/internal/domain/repository"
)

// ==================== Errors (prefix ErrLeave* — hindari tabrakan dgn service lain) ====================

var (
	ErrLeaveNoProfile    = errors.New("akun ini belum terhubung ke data pegawai")
	ErrLeaveNoSupervisor = errors.New("pegawai belum punya atasan langsung (Reports To belum diatur) — hubungi HC")
	ErrLeaveNotFound     = errors.New("pengajuan cuti tidak ditemukan")
	ErrLeaveNotPending   = errors.New("pengajuan sudah diputuskan atau dibatalkan")
	ErrLeaveNotApprover  = errors.New("Anda bukan atasan yang ditunjuk untuk pengajuan ini")
	ErrLeaveNotOwner     = errors.New("Anda bukan pemilik pengajuan ini")
	ErrLeaveRange        = errors.New("tanggal selesai tidak boleh lebih awal dari tanggal mulai")
	ErrLeaveTooLong      = errors.New("rentang cuti maksimal 365 hari")
	ErrLeaveReason       = errors.New("alasan wajib diisi (minimal 3 karakter)")
	ErrLeaveOverlap      = errors.New("masih ada pengajuan cuti yang tumpang-tindih pada rentang tanggal tersebut")
	ErrLeaveInsufficient = errors.New("saldo cuti tidak mencukupi")
	ErrLeaveTypeInactive = errors.New("jenis cuti tidak aktif")
	ErrLeaveTypeNotFound = errors.New("jenis cuti tidak ditemukan")
	ErrLeaveTypeInUse    = errors.New("jenis cuti masih dipakai pengajuan — nonaktifkan (toggle Active) daripada hapus")
	ErrLeaveNumberGen    = errors.New("gagal membuat nomor pengajuan — coba lagi")
)

// LeaveBalanceView — saldo utk tampilan Cuti Saya.
type LeaveBalanceView struct {
	LeaveTypeID string  `json:"leave_type_id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Year        int     `json:"year"`
	Entitled    float64 `json:"entitled"`
	Used        float64 `json:"used"`
	Remaining   float64 `json:"remaining"`
}

// LeaveSummaryView — ringkasan Cuti Saya.
type LeaveSummaryView struct {
	EmployeeID     string             `json:"employee_id"`
	EmployeeNumber string             `json:"employee_number"`
	FullName       string             `json:"full_name"`
	Year           int                `json:"year"`
	Balances       []LeaveBalanceView `json:"balances"`
	PendingCount   int64              `json:"pending_count"`
}

// LeaveService — cuti self-service + approval 1 LEVEL (atasan langsung reports_to).
// Fase 2 ROADMAP HC. Saldo lazy-init per (pegawai, jenis, tahun) dari LeaveType.DefaultDays;
// pengurangan saldo otoritatif terjadi di Decide(approve) dalam transaksi dgn row-lock.
type LeaveService struct {
	repo    repository.LeaveRepository
	empRepo repository.EmployeeRepository
	db      *gorm.DB
}

func NewLeaveService(repo repository.LeaveRepository, empRepo repository.EmployeeRepository, db *gorm.DB) *LeaveService {
	return &LeaveService{repo: repo, empRepo: empRepo, db: db}
}

func (s *LeaveService) myEmployee(userID string) (*entity.Employee, error) {
	emp, err := s.empRepo.GetByUserID(userID)
	if err != nil || emp == nil {
		return nil, ErrLeaveNoProfile
	}
	return emp, nil
}

// ==================== JENIS CUTI (admin) ====================

func (s *LeaveService) ListTypes(activeOnly bool) ([]entity.LeaveType, error) {
	return s.repo.ListTypes(activeOnly)
}

func (s *LeaveService) CreateType(t *entity.LeaveType) error {
	t.Code = strings.ToUpper(strings.TrimSpace(t.Code))
	if t.Code == "" || strings.TrimSpace(t.Name) == "" {
		return errors.New("kode & nama jenis cuti wajib diisi")
	}
	if t.DefaultDays < 0 || t.DefaultDays > 366 {
		return errors.New("default hari harus 0-366")
	}
	return s.repo.CreateType(t)
}

func (s *LeaveService) UpdateType(t *entity.LeaveType) error {
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("nama jenis cuti wajib diisi")
	}
	if t.DefaultDays < 0 || t.DefaultDays > 366 {
		return errors.New("default hari harus 0-366")
	}
	return s.repo.UpdateType(t)
}

func (s *LeaveService) DeleteType(id string) error {
	n, err := s.repo.CountRequestsByType(id)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrLeaveTypeInUse
	}
	return s.repo.DeleteType(id)
}

// ==================== CUTI SAYA (self-service) ====================

// MySummary — saldo tahun berjalan + info pegawai (lazy-init semua jenis aktif).
func (s *LeaveService) MySummary(userID string) (*LeaveSummaryView, error) {
	emp, err := s.myEmployee(userID)
	if err != nil {
		return nil, err
	}
	year := time.Now().Year()
	bals, err := s.ensureBalances(emp.ID, year)
	if err != nil {
		return nil, err
	}
	_, pending, err := s.repo.ListRequestsByEmployee(emp.ID, "submitted", 1, 1)
	if err != nil {
		return nil, err
	}
	return &LeaveSummaryView{
		EmployeeID: emp.ID, EmployeeNumber: emp.EmployeeNumber, FullName: emp.FullName,
		Year: year, Balances: bals, PendingCount: pending,
	}, nil
}

// ensureBalances — pastikan baris saldo ada utk semua jenis aktif pada tahun tsb (lazy-init).
func (s *LeaveService) ensureBalances(employeeID string, year int) ([]LeaveBalanceView, error) {
	types, err := s.repo.ListTypes(true)
	if err != nil {
		return nil, err
	}
	out := []LeaveBalanceView{}
	for i := range types {
		t := &types[i]
		b, err := s.repo.GetBalance(employeeID, t.ID, year)
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
			nb := &entity.LeaveBalance{EmployeeID: employeeID, LeaveTypeID: t.ID, Year: year, Entitled: float64(t.DefaultDays)}
			if err := s.repo.CreateBalance(nb); err != nil {
				// race dgn request paralel → baris sudah ada: ambil yang ada
				b, err = s.repo.GetBalance(employeeID, t.ID, year)
				if err != nil {
					return nil, err
				}
			} else {
				b = nb
			}
		}
		if b == nil {
			continue
		}
		out = append(out, LeaveBalanceView{
			LeaveTypeID: t.ID, Code: t.Code, Name: t.Name, Year: year,
			Entitled: b.Entitled, Used: b.Used, Remaining: b.Entitled - b.Used,
		})
	}
	return out, nil
}

// Submit — ajukan cuti (status langsung submitted; approver = snapshot reports_to).
func (s *LeaveService) Submit(userID, typeID, start, end, reason string) (*entity.LeaveRequest, error) {
	emp, err := s.myEmployee(userID)
	if err != nil {
		return nil, err
	}
	if emp.ReportsTo == nil || *emp.ReportsTo == "" {
		return nil, ErrLeaveNoSupervisor
	}
	if len(strings.TrimSpace(reason)) < 3 {
		return nil, ErrLeaveReason
	}
	sd, err := time.Parse("2006-01-02", start)
	if err != nil {
		return nil, errors.New("format tanggal mulai tidak valid (YYYY-MM-DD)")
	}
	ed, err := time.Parse("2006-01-02", end)
	if err != nil {
		return nil, errors.New("format tanggal selesai tidak valid (YYYY-MM-DD)")
	}
	if ed.Before(sd) {
		return nil, ErrLeaveRange
	}
	days := int(ed.Sub(sd).Hours()/24) + 1
	if days <= 0 {
		return nil, ErrLeaveRange
	}
	if days > 365 {
		return nil, ErrLeaveTooLong
	}
	t, err := s.repo.GetTypeByID(typeID)
	if err != nil {
		return nil, ErrLeaveTypeNotFound
	}
	if !t.Active {
		return nil, ErrLeaveTypeInactive
	}
	if ok, err := s.repo.HasOverlap(emp.ID, sd, ed); err != nil {
		return nil, err
	} else if ok {
		return nil, ErrLeaveOverlap
	}
	// fast-fail saldo (cek otoritatif ada di Decide dgn row-lock)
	if _, err := s.ensureBalances(emp.ID, sd.Year()); err != nil {
		return nil, err
	}
	if b, err := s.repo.GetBalance(emp.ID, t.ID, sd.Year()); err != nil {
		return nil, err
	} else if b.Entitled-b.Used < float64(days) {
		return nil, ErrLeaveInsufficient
	}

	req := &entity.LeaveRequest{
		EmployeeID: emp.ID, LeaveTypeID: t.ID, StartDate: sd, EndDate: ed, Days: days,
		Reason: strings.TrimSpace(reason), Status: "submitted", ApproverID: *emp.ReportsTo,
		SubmittedAt: time.Now(),
	}
	for i := 0; i < 3; i++ {
		num, err := s.generateNumber()
		if err != nil {
			return nil, err
		}
		req.Number = num
		if err := s.repo.CreateRequest(req); err == nil {
			return req, nil
		} else if !isDup(err) {
			return nil, err
		}
	}
	return nil, ErrLeaveNumberGen
}

// generateNumber — naming series LR-YYYYMM-#### (sequence per bulan, retry-on-duplicate di Submit).
func (s *LeaveService) generateNumber() (string, error) {
	prefix := fmt.Sprintf("LR-%s-", time.Now().Format("200601"))
	rows, _, err := s.repo.ListRequestsAll(repository.LeaveRequestFilter{Query: prefix, Page: 1, PageSize: 100})
	if err != nil {
		return "", err
	}
	max := 0
	for i := range rows {
		suffix := strings.TrimPrefix(rows[i].Number, prefix)
		if n, err := strconv.Atoi(suffix); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("%s%04d", prefix, max+1), nil
}

func (s *LeaveService) MyRequests(userID, status string, page, size int) ([]entity.LeaveRequest, int64, error) {
	emp, err := s.myEmployee(userID)
	if err != nil {
		return nil, 0, err
	}
	return s.repo.ListRequestsByEmployee(emp.ID, status, page, size)
}

// Cancel — pemilik bisa membatalkan selama masih submitted (saldo belum berkurang).
func (s *LeaveService) Cancel(userID, requestID string) error {
	emp, err := s.myEmployee(userID)
	if err != nil {
		return err
	}
	req, err := s.repo.GetRequestByID(requestID)
	if err != nil {
		return ErrLeaveNotFound
	}
	if req.EmployeeID != emp.ID {
		return ErrLeaveNotOwner
	}
	if req.Status != "submitted" {
		return ErrLeaveNotPending
	}
	res := s.db.Model(&entity.LeaveRequest{}).
		Where("id = ? AND status = 'submitted'", requestID).
		Update("status", "cancelled")
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrLeaveNotPending
	}
	return nil
}

// ==================== APPROVAL (atasan, 1 level) ====================

func (s *LeaveService) MyApprovals(userID, status string, page, size int) ([]entity.LeaveRequest, int64, error) {
	emp, err := s.myEmployee(userID)
	if err != nil {
		return nil, 0, err
	}
	return s.repo.ListRequestsForApprover(emp.ID, status, page, size)
}

// Decide — approve/reject oleh atasan yang ditunjuk. Approve mengurangi saldo
// dalam SATU transaksi dgn SELECT ... FOR UPDATE (anti race double-approve/over-deduct),
// plus guard kondisional status='submitted' (anti double-decide).
func (s *LeaveService) Decide(userID, requestID, action, note string) (*entity.LeaveRequest, error) {
	emp, err := s.myEmployee(userID)
	if err != nil {
		return nil, err
	}
	req, err := s.repo.GetRequestByID(requestID)
	if err != nil {
		return nil, ErrLeaveNotFound
	}
	if req.ApproverID != emp.ID {
		return nil, ErrLeaveNotApprover
	}
	if req.Status != "submitted" {
		return nil, ErrLeaveNotPending
	}
	status := "approved"
	if action == "reject" {
		status = "rejected"
	}
	now := time.Now()
	approve := status == "approved"

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if approve {
			var bal entity.LeaveBalance
			lockQ := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("employee_id = ? AND leave_type_id = ? AND year = ?", req.EmployeeID, req.LeaveTypeID, req.StartDate.Year())
			err := lockQ.First(&bal).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// saldo belum ter-init → init dari default jenis (race antar-approve ditangani unique index)
				t, terr := s.repo.GetTypeByID(req.LeaveTypeID)
				if terr != nil {
					return terr
				}
				nb := entity.LeaveBalance{EmployeeID: req.EmployeeID, LeaveTypeID: req.LeaveTypeID,
					Year: req.StartDate.Year(), Entitled: float64(t.DefaultDays)}
				if cerr := tx.Create(&nb).Error; cerr != nil {
					if !isDup(cerr) {
						return cerr
					}
					if err := lockQ.First(&bal).Error; err != nil {
						return err
					}
				} else {
					bal = nb
				}
			} else if err != nil {
				return err
			}
			if bal.Entitled-bal.Used < float64(req.Days) {
				return ErrLeaveInsufficient
			}
			if err := tx.Model(&entity.LeaveBalance{}).Where("id = ?", bal.ID).
				Update("used", bal.Used+float64(req.Days)).Error; err != nil {
				return err
			}
		}
		res := tx.Model(&entity.LeaveRequest{}).
			Where("id = ? AND status = 'submitted'", requestID).
			Updates(map[string]interface{}{
				"status":        status,
				"decision_note": strings.TrimSpace(note),
				"decided_at":    now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrLeaveNotPending
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetRequestByID(requestID)
}

// ==================== ADMIN (monitoring) ====================

func (s *LeaveService) ListAllRequests(status, query string, page, size int) ([]entity.LeaveRequest, int64, error) {
	return s.repo.ListRequestsAll(repository.LeaveRequestFilter{Status: status, Query: query, Page: page, PageSize: size})
}
