package repository

import (
	"time"

	"github.com/rama/b-wise/human-capital/internal/domain/entity"
)

// EmployeeRepository — employee aggregate (termasuk educations)
type EmployeeRepository interface {
	Create(emp *entity.Employee) error
	Update(emp *entity.Employee) error
	Delete(id string) error
	GetByID(id string) (*entity.Employee, error)
	GetByNumber(employeeNumber string) (*entity.Employee, error)
	GetByNIK(nik string) (*entity.Employee, error)
	GetByWorkEmail(email string) (*entity.Employee, error)
	GetByUserID(userID string) (*entity.Employee, error)
	List(filter EmployeeFilter) ([]entity.Employee, int64, error)
	CountByStatus() (map[string]int64, error)

	CreateEducation(edu *entity.EmployeeEducation) error
	DeleteEducations(employeeID string) error
	ListEducations(employeeID string) ([]entity.EmployeeEducation, error)
}

type EmployeeFilter struct {
	Page, PageSize int
	Status         string
	DepartmentID   string
	BranchID       string
	EmploymentTypeID string
	Query          string // nama / NIP / email
	UserID         string // cari pegawai dari akun SSO (dipakai IKU utk unit-scope)
}

// MasterRepository — pola seragam utk master org (generic-ish via closure)
type DepartmentRepository interface {
	Create(d *entity.Department) error
	Update(d *entity.Department) error
	Delete(id string) error
	GetByID(id string) (*entity.Department, error)
	List(activeOnly bool) ([]entity.Department, error)
}

type DesignationRepository interface {
	Create(d *entity.Designation) error
	Update(d *entity.Designation) error
	Delete(id string) error
	GetByID(id string) (*entity.Designation, error)
	List(activeOnly bool) ([]entity.Designation, error)
}

type EmploymentTypeRepository interface {
	Create(t *entity.EmploymentType) error
	Update(t *entity.EmploymentType) error
	Delete(id string) error
	GetByID(id string) (*entity.EmploymentType, error)
	List(activeOnly bool) ([]entity.EmploymentType, error)
}

type GradeRepository interface {
	Create(g *entity.Grade) error
	Update(g *entity.Grade) error
	Delete(id string) error
	GetByID(id string) (*entity.Grade, error)
	List(activeOnly bool) ([]entity.Grade, error)
}

type BranchRepository interface {
	Create(b *entity.Branch) error
	Update(b *entity.Branch) error
	Delete(id string) error
	GetByID(id string) (*entity.Branch, error)
	List(activeOnly bool) ([]entity.Branch, error)
}

// MovementRepository — lifecycle documents
type MovementRepository interface {
	Create(m *entity.EmployeeMovement, details []entity.MovementDetail) error
	List(filter MovementFilter) ([]entity.EmployeeMovement, int64, error)
	ListByEmployee(employeeID string) ([]entity.EmployeeMovement, error)
}

type MovementFilter struct {
	Page, PageSize int
	EmployeeID     string
	Type           string
}

// LeaveRepository — cuti (Fase 2): jenis, saldo, pengajuan + approval 1 level.
type LeaveRepository interface {
	// jenis cuti (master)
	CreateType(t *entity.LeaveType) error
	UpdateType(t *entity.LeaveType) error
	DeleteType(id string) error
	GetTypeByID(id string) (*entity.LeaveType, error)
	ListTypes(activeOnly bool) ([]entity.LeaveType, error)
	CountRequestsByType(typeID string) (int64, error)

	// saldo per (pegawai, jenis, tahun)
	GetBalance(employeeID, typeID string, year int) (*entity.LeaveBalance, error)
	CreateBalance(b *entity.LeaveBalance) error
	ListBalances(employeeID string, year int) ([]entity.LeaveBalance, error)

	// pengajuan
	CreateRequest(r *entity.LeaveRequest) error
	GetRequestByID(id string) (*entity.LeaveRequest, error)
	ListRequestsByEmployee(employeeID, status string, page, pageSize int) ([]entity.LeaveRequest, int64, error)
	ListRequestsForApprover(approverID, status string, page, pageSize int) ([]entity.LeaveRequest, int64, error)
	ListRequestsAll(filter LeaveRequestFilter) ([]entity.LeaveRequest, int64, error)
	HasOverlap(employeeID string, start, end time.Time) (bool, error)
}

type LeaveRequestFilter struct {
	Page, PageSize int
	Status         string
	Query          string // nomor pengajuan / nama pegawai
}
