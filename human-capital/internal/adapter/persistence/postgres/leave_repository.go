package postgres

import (
	"strings"
	"time"

	"gorm.io/gorm"

	entity "github.com/rama/b-wise/human-capital/internal/domain/entity"
	"github.com/rama/b-wise/human-capital/internal/domain/repository"
)

// leaveRepository — implementasi LeaveRepository (Fase 2: cuti + approval 1 level).
type leaveRepository struct{ db *gorm.DB }

func NewLeaveRepository(db *gorm.DB) repository.LeaveRepository {
	return &leaveRepository{db: db}
}

func leavePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}
	return page, size
}

// ==================== JENIS CUTI ====================

func (r *leaveRepository) CreateType(t *entity.LeaveType) error { return r.db.Create(t).Error }

func (r *leaveRepository) UpdateType(t *entity.LeaveType) error { return r.db.Save(t).Error }

func (r *leaveRepository) DeleteType(id string) error {
	return r.db.Delete(&entity.LeaveType{}, "id = ?", id).Error
}

func (r *leaveRepository) GetTypeByID(id string) (*entity.LeaveType, error) {
	var t entity.LeaveType
	if err := r.db.First(&t, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *leaveRepository) ListTypes(activeOnly bool) ([]entity.LeaveType, error) {
	q := r.db.Model(&entity.LeaveType{})
	if activeOnly {
		q = q.Where("active = ?", true)
	}
	var rows []entity.LeaveType
	err := q.Order("code ASC").Find(&rows).Error
	return rows, err
}

func (r *leaveRepository) CountRequestsByType(typeID string) (int64, error) {
	var n int64
	err := r.db.Model(&entity.LeaveRequest{}).Where("leave_type_id = ?", typeID).Count(&n).Error
	return n, err
}

// ==================== SALDO ====================

func (r *leaveRepository) GetBalance(employeeID, typeID string, year int) (*entity.LeaveBalance, error) {
	var b entity.LeaveBalance
	err := r.db.Where("employee_id = ? AND leave_type_id = ? AND year = ?", employeeID, typeID, year).
		First(&b).Error
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *leaveRepository) CreateBalance(b *entity.LeaveBalance) error { return r.db.Create(b).Error }

func (r *leaveRepository) ListBalances(employeeID string, year int) ([]entity.LeaveBalance, error) {
	var rows []entity.LeaveBalance
	err := r.db.Preload("LeaveType").
		Where("employee_id = ? AND year = ?", employeeID, year).
		Find(&rows).Error
	return rows, err
}

// ==================== PENGAJUAN ====================

func (r *leaveRepository) CreateRequest(req *entity.LeaveRequest) error { return r.db.Create(req).Error }

func (r *leaveRepository) GetRequestByID(id string) (*entity.LeaveRequest, error) {
	var req entity.LeaveRequest
	err := r.db.Preload("Employee").Preload("LeaveType").Preload("Approver").
		First(&req, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *leaveRepository) ListRequestsByEmployee(employeeID, status string, page, size int) ([]entity.LeaveRequest, int64, error) {
	base := r.db.Model(&entity.LeaveRequest{}).Where("employee_id = ?", employeeID)
	if status != "" {
		base = base.Where("status = ?", status)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, size = leavePage(page, size)
	var rows []entity.LeaveRequest
	err := base.Preload("LeaveType").Order("created_at DESC").
		Offset((page - 1) * size).Limit(size).Find(&rows).Error
	return rows, total, err
}

func (r *leaveRepository) ListRequestsForApprover(approverID, status string, page, size int) ([]entity.LeaveRequest, int64, error) {
	base := r.db.Model(&entity.LeaveRequest{}).Where("approver_id = ?", approverID)
	if status != "" {
		base = base.Where("status = ?", status)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, size = leavePage(page, size)
	var rows []entity.LeaveRequest
	err := base.Preload("Employee").Preload("LeaveType").Order("created_at DESC").
		Offset((page - 1) * size).Limit(size).Find(&rows).Error
	return rows, total, err
}

func (r *leaveRepository) ListRequestsAll(f repository.LeaveRequestFilter) ([]entity.LeaveRequest, int64, error) {
	base := r.db.Model(&entity.LeaveRequest{})
	if f.Status != "" {
		base = base.Where("leave_requests.status = ?", f.Status)
	}
	if f.Query != "" {
		like := "%" + strings.ToLower(f.Query) + "%"
		base = base.Joins("JOIN employees ON employees.id = leave_requests.employee_id").
			Where("LOWER(employees.full_name) LIKE ? OR LOWER(leave_requests.number) LIKE ?", like, like)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, size := leavePage(f.Page, f.PageSize)
	var rows []entity.LeaveRequest
	err := base.Preload("Employee").Preload("LeaveType").Preload("Approver").
		Order("leave_requests.created_at DESC").
		Offset((page - 1) * size).Limit(size).Find(&rows).Error
	return rows, total, err
}

// HasOverlap — apakah masih ada pengajuan submitted/approved yang rentangnya tumpang-tindih.
func (r *leaveRepository) HasOverlap(employeeID string, start, end time.Time) (bool, error) {
	var n int64
	err := r.db.Model(&entity.LeaveRequest{}).
		Where("employee_id = ? AND status IN ('submitted','approved') AND start_date <= ? AND end_date >= ?",
			employeeID, end, start).
		Count(&n).Error
	return n > 0, err
}
