package postgres

import (
	entity "github.com/rama/b-wise/human-capital/internal/domain/entity"
	repository "github.com/rama/b-wise/human-capital/internal/domain/repository"
	"gorm.io/gorm"
)

// ==================== EMPLOYEE ====================

type EmployeeRepository struct{ db *gorm.DB }

func NewEmployeeRepository(db *gorm.DB) *EmployeeRepository { return &EmployeeRepository{db: db} }

func (r *EmployeeRepository) Create(emp *entity.Employee) error { return r.db.Create(emp).Error }
func (r *EmployeeRepository) Update(emp *entity.Employee) error { return r.db.Save(emp).Error }
func (r *EmployeeRepository) Delete(id string) error {
	return r.db.Delete(&entity.Employee{}, "id = ?", id).Error
}

func (r *EmployeeRepository) GetByID(id string) (*entity.Employee, error) {
	var e entity.Employee
	if err := r.db.Where("id = ?", id).First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *EmployeeRepository) GetByNumber(num string) (*entity.Employee, error) {
	var e entity.Employee
	if err := r.db.Where("employee_number = ?", num).First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *EmployeeRepository) GetByNIK(nik string) (*entity.Employee, error) {
	var e entity.Employee
	if err := r.db.Where("nik = ?", nik).First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *EmployeeRepository) GetByWorkEmail(email string) (*entity.Employee, error) {
	var e entity.Employee
	if err := r.db.Where("work_email = ?", email).First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *EmployeeRepository) GetByUserID(userID string) (*entity.Employee, error) {
	var e entity.Employee
	if err := r.db.Where("user_id = ?", userID).First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *EmployeeRepository) List(f repository.EmployeeFilter) ([]entity.Employee, int64, error) {
	var list []entity.Employee
	var total int64

	q := r.db.Model(&entity.Employee{})
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.DepartmentID != "" {
		q = q.Where("department_id = ?", f.DepartmentID)
	}
	if f.BranchID != "" {
		q = q.Where("branch_id = ?", f.BranchID)
	}
	if f.EmploymentTypeID != "" {
		q = q.Where("employment_type_id = ?", f.EmploymentTypeID)
	}
	if f.UserID != "" {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.Query != "" {
		like := "%" + f.Query + "%"
		q = q.Where("full_name ILIKE ? OR employee_number ILIKE ? OR work_email ILIKE ? OR personal_email ILIKE ?",
			like, like, like, like)
	}

	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	err := q.Order("created_at DESC").
		Limit(f.PageSize).Offset((f.Page - 1) * f.PageSize).
		Find(&list).Error
	return list, total, err
}

func (r *EmployeeRepository) CountByStatus() (map[string]int64, error) {
	var rows []struct {
		Status string
		N      int64
	}
	if err := r.db.Model(&entity.Employee{}).Select("status, count(*) as n").Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[string]int64, len(rows))
	for _, row := range rows {
		m[row.Status] = row.N
	}
	return m, nil
}

func (r *EmployeeRepository) CreateEducation(edu *entity.EmployeeEducation) error {
	return r.db.Create(edu).Error
}

func (r *EmployeeRepository) DeleteEducations(employeeID string) error {
	return r.db.Where("employee_id = ?", employeeID).Delete(&entity.EmployeeEducation{}).Error
}

func (r *EmployeeRepository) ListEducations(employeeID string) ([]entity.EmployeeEducation, error) {
	var list []entity.EmployeeEducation
	err := r.db.Where("employee_id = ?", employeeID).Order("graduation_year ASC").Find(&list).Error
	return list, err
}

// ==================== MASTER (generic) ====================

// masterRepo — implementasi generic utk master org dengan kolom seragam.
type masterRepo[T any] struct{ db *gorm.DB }

func (r *masterRepo[T]) Create(v *T) error { return r.db.Create(v).Error }
func (r *masterRepo[T]) Update(v *T) error { return r.db.Save(v).Error }
func (r *masterRepo[T]) Delete(id string) error {
	var v T
	return r.db.Delete(&v, "id = ?", id).Error
}
func (r *masterRepo[T]) GetByID(id string) (*T, error) {
	var v T
	if err := r.db.Where("id = ?", id).First(&v).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

// DepartmentRepository — list dipresisi dengan tree ordering
type DepartmentRepository struct{ masterRepo[entity.Department] }

func NewDepartmentRepository(db *gorm.DB) *DepartmentRepository {
	return &DepartmentRepository{masterRepo[entity.Department]{db}}
}

func (r *DepartmentRepository) List(activeOnly bool) ([]entity.Department, error) {
	var list []entity.Department
	q := r.db
	if activeOnly {
		q = q.Where("is_active = ?", true)
	}
	return list, q.Order("parent_id NULLS FIRST, name ASC").Find(&list).Error
}

type DesignationRepository struct{ masterRepo[entity.Designation] }

func NewDesignationRepository(db *gorm.DB) *DesignationRepository {
	return &DesignationRepository{masterRepo[entity.Designation]{db}}
}

func (r *DesignationRepository) List(activeOnly bool) ([]entity.Designation, error) {
	var list []entity.Designation
	q := r.db
	if activeOnly {
		q = q.Where("is_active = ?", true)
	}
	return list, q.Order("level ASC, name ASC").Find(&list).Error
}

type EmploymentTypeRepository struct{ masterRepo[entity.EmploymentType] }

func NewEmploymentTypeRepository(db *gorm.DB) *EmploymentTypeRepository {
	return &EmploymentTypeRepository{masterRepo[entity.EmploymentType]{db}}
}

func (r *EmploymentTypeRepository) List(activeOnly bool) ([]entity.EmploymentType, error) {
	var list []entity.EmploymentType
	q := r.db
	if activeOnly {
		q = q.Where("is_active = ?", true)
	}
	return list, q.Order("sort_order ASC, name ASC").Find(&list).Error
}

type GradeRepository struct{ masterRepo[entity.Grade] }

func NewGradeRepository(db *gorm.DB) *GradeRepository {
	return &GradeRepository{masterRepo[entity.Grade]{db}}
}

func (r *GradeRepository) List(activeOnly bool) ([]entity.Grade, error) {
	var list []entity.Grade
	q := r.db
	if activeOnly {
		q = q.Where("is_active = ?", true)
	}
	return list, q.Order("sort_order ASC, name ASC").Find(&list).Error
}

type BranchRepository struct{ masterRepo[entity.Branch] }

func NewBranchRepository(db *gorm.DB) *BranchRepository {
	return &BranchRepository{masterRepo[entity.Branch]{db}}
}

func (r *BranchRepository) List(activeOnly bool) ([]entity.Branch, error) {
	var list []entity.Branch
	q := r.db
	if activeOnly {
		q = q.Where("is_active = ?", true)
	}
	return list, q.Order("name ASC").Find(&list).Error
}

// ==================== MOVEMENT ====================

type MovementRepository struct{ db *gorm.DB }

func NewMovementRepository(db *gorm.DB) *MovementRepository { return &MovementRepository{db} }

// Create — satu transaction: movement + details
func (r *MovementRepository) Create(m *entity.EmployeeMovement, details []entity.MovementDetail) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return err
		}
		for i := range details {
			details[i].MovementID = m.ID
		}
		if len(details) > 0 {
			if err := tx.Create(&details).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *MovementRepository) List(f repository.MovementFilter) ([]entity.EmployeeMovement, int64, error) {
	var list []entity.EmployeeMovement
	var total int64

	q := r.db.Model(&entity.EmployeeMovement{})
	if f.EmployeeID != "" {
		q = q.Where("employee_id = ?", f.EmployeeID)
	}
	if f.Type != "" {
		q = q.Where("movement_type = ?", f.Type)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if f.PageSize <= 0 {
		f.PageSize = 50
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	err := q.Preload("Details").
		Order("effective_date DESC, created_at DESC").
		Limit(f.PageSize).Offset((f.Page - 1) * f.PageSize).
		Find(&list).Error
	return list, total, err
}

func (r *MovementRepository) ListByEmployee(employeeID string) ([]entity.EmployeeMovement, error) {
	var list []entity.EmployeeMovement
	err := r.db.Preload("Details").
		Where("employee_id = ?", employeeID).
		Order("effective_date DESC, created_at DESC").
		Find(&list).Error
	return list, err
}
