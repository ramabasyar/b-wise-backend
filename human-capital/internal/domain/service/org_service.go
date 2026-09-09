package service

import (
	"errors"
	"fmt"

	entity "github.com/rama/b-wise/human-capital/internal/domain/entity"
	repository "github.com/rama/b-wise/human-capital/internal/domain/repository"
	"gorm.io/gorm"
)

// ==================== ORG MASTER SERVICES ====================
// Departments (tree), Designations, EmploymentTypes, Grades, Branches.

type OrgService struct {
	depts repository.DepartmentRepository
	desigs repository.DesignationRepository
	empTypes repository.EmploymentTypeRepository
	grades repository.GradeRepository
	branches repository.BranchRepository
}

func NewOrgService(
	depts repository.DepartmentRepository,
	desigs repository.DesignationRepository,
	empTypes repository.EmploymentTypeRepository,
	grades repository.GradeRepository,
	branches repository.BranchRepository,
) *OrgService {
	return &OrgService{depts, desigs, empTypes, grades, branches}
}

// ---------- generic helpers ----------

func notFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

func dupOrErr(err error) error {
	if isDup(err) {
		return ErrDuplicate
	}
	return err
}

// ---------- DEPARTMENT (tree) ----------

type DepartmentNode struct {
	entity.Department
	Children []*DepartmentNode `json:"children"`
}

func (s *OrgService) ListDepartments(activeOnly bool) ([]entity.Department, error) {
	return s.depts.List(activeOnly)
}

func (s *OrgService) DepartmentTree(activeOnly bool) ([]*DepartmentNode, error) {
	list, err := s.depts.List(activeOnly)
	if err != nil {
		return nil, err
	}
	nodes := make(map[string]*DepartmentNode, len(list))
	var roots []*DepartmentNode
	for i := range list {
		nodes[list[i].ID] = &DepartmentNode{Department: list[i]}
	}
	for i := range list {
		d := &list[i]
		n := nodes[d.ID]
		if d.ParentID != nil && *d.ParentID != "" && nodes[*d.ParentID] != nil {
			nodes[*d.ParentID].Children = append(nodes[*d.ParentID].Children, n)
		} else {
			roots = append(roots, n)
		}
	}
	return roots, nil
}

func (s *OrgService) CreateDepartment(d *entity.Department) error {
	if d.Code == "" || d.Name == "" {
		return fmt.Errorf("%w: code & name wajib diisi", ErrValidation)
	}
	if err := s.depts.Create(d); err != nil {
		return dupOrErr(err)
	}
	return nil
}

func (s *OrgService) UpdateDepartment(d *entity.Department) error {
	existing, err := s.depts.GetByID(d.ID)
	if err != nil {
		if notFound(err) {
			return ErrNotFound
		}
		return err
	}
	// cegah cycle: parent tidak boleh diri sendiri / descendant-nya
	if d.ParentID != nil && *d.ParentID != "" {
		if *d.ParentID == d.ID {
			return fmt.Errorf("%w: parent tidak boleh diri sendiri", ErrValidation)
		}
		// naik dari parent baru ke root — kalau ketemu diri sendiri = cycle
		cur := d.ParentID
		for cur != nil && *cur != "" {
			if *cur == d.ID {
				return fmt.Errorf("%w: parent membentuk siklus", ErrValidation)
			}
			p, err := s.depts.GetByID(*cur)
			if err != nil {
				break
			}
			cur = p.ParentID
		}
	}
	d.Code = existing.Code // code immutable
	if err := s.depts.Update(d); err != nil {
		return dupOrErr(err)
	}
	return nil
}

func (s *OrgService) DeleteDepartment(id string) error {
	if _, err := s.depts.GetByID(id); err != nil {
		if notFound(err) {
			return ErrNotFound
		}
		return err
	}
	return s.depts.Delete(id)
}

// ---------- DESIGNATION ----------

func (s *OrgService) ListDesignations(activeOnly bool) ([]entity.Designation, error) {
	return s.desigs.List(activeOnly)
}

func (s *OrgService) CreateDesignation(d *entity.Designation) error {
	if d.Name == "" {
		return fmt.Errorf("%w: name wajib diisi", ErrValidation)
	}
	if err := s.desigs.Create(d); err != nil {
		return dupOrErr(err)
	}
	return nil
}

func (s *OrgService) UpdateDesignation(d *entity.Designation) error {
	if _, err := s.desigs.GetByID(d.ID); err != nil {
		if notFound(err) {
			return ErrNotFound
		}
		return err
	}
	if err := s.desigs.Update(d); err != nil {
		return dupOrErr(err)
	}
	return nil
}

func (s *OrgService) DeleteDesignation(id string) error {
	if _, err := s.desigs.GetByID(id); err != nil {
		if notFound(err) {
			return ErrNotFound
		}
		return err
	}
	return s.desigs.Delete(id)
}

// ---------- EMPLOYMENT TYPE ----------

func (s *OrgService) ListEmploymentTypes(activeOnly bool) ([]entity.EmploymentType, error) {
	return s.empTypes.List(activeOnly)
}

func (s *OrgService) CreateEmploymentType(t *entity.EmploymentType) error {
	if t.Name == "" {
		return fmt.Errorf("%w: name wajib diisi", ErrValidation)
	}
	if err := s.empTypes.Create(t); err != nil {
		return dupOrErr(err)
	}
	return nil
}

func (s *OrgService) UpdateEmploymentType(t *entity.EmploymentType) error {
	if _, err := s.empTypes.GetByID(t.ID); err != nil {
		if notFound(err) {
			return ErrNotFound
		}
		return err
	}
	if err := s.empTypes.Update(t); err != nil {
		return dupOrErr(err)
	}
	return nil
}

func (s *OrgService) DeleteEmploymentType(id string) error {
	if _, err := s.empTypes.GetByID(id); err != nil {
		if notFound(err) {
			return ErrNotFound
		}
		return err
	}
	return s.empTypes.Delete(id)
}

// ---------- GRADE ----------

func (s *OrgService) ListGrades(activeOnly bool) ([]entity.Grade, error) {
	return s.grades.List(activeOnly)
}

func (s *OrgService) CreateGrade(g *entity.Grade) error {
	if g.Name == "" {
		return fmt.Errorf("%w: name wajib diisi", ErrValidation)
	}
	if err := s.grades.Create(g); err != nil {
		return dupOrErr(err)
	}
	return nil
}

func (s *OrgService) UpdateGrade(g *entity.Grade) error {
	if _, err := s.grades.GetByID(g.ID); err != nil {
		if notFound(err) {
			return ErrNotFound
		}
		return err
	}
	if err := s.grades.Update(g); err != nil {
		return dupOrErr(err)
	}
	return nil
}

func (s *OrgService) DeleteGrade(id string) error {
	if _, err := s.grades.GetByID(id); err != nil {
		if notFound(err) {
			return ErrNotFound
		}
		return err
	}
	return s.grades.Delete(id)
}

// ---------- BRANCH ----------

func (s *OrgService) ListBranches(activeOnly bool) ([]entity.Branch, error) {
	return s.branches.List(activeOnly)
}

func (s *OrgService) CreateBranch(b *entity.Branch) error {
	if b.Name == "" {
		return fmt.Errorf("%w: name wajib diisi", ErrValidation)
	}
	if err := s.branches.Create(b); err != nil {
		return dupOrErr(err)
	}
	return nil
}

func (s *OrgService) UpdateBranch(b *entity.Branch) error {
	if _, err := s.branches.GetByID(b.ID); err != nil {
		if notFound(err) {
			return ErrNotFound
		}
		return err
	}
	if err := s.branches.Update(b); err != nil {
		return dupOrErr(err)
	}
	return nil
}

func (s *OrgService) DeleteBranch(id string) error {
	if _, err := s.branches.GetByID(id); err != nil {
		if notFound(err) {
			return ErrNotFound
		}
		return err
	}
	return s.branches.Delete(id)
}
