package repository

import (
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
