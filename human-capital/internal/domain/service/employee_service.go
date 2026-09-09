package service

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	entity "github.com/rama/b-wise/human-capital/internal/domain/entity"
	repository "github.com/rama/b-wise/human-capital/internal/domain/repository"
	"gorm.io/gorm"
)

var (
	ErrNotFound       = errors.New("data tidak ditemukan")
	ErrDuplicate      = errors.New("data sudah ada (kode/nama duplikat)")
	ErrValidation     = errors.New("validasi gagal")
	ErrStillEmployed  = errors.New("pegawai masih tercatat aktif — proses separation dulu")
)

// CheckDuplicates — pre-flight utk wizard onboard: NIP/NIK/work_email sudah dipakai?
// Dipanggil SEBELUM registrasi SSO supaya tidak membuat user yatim.
func (s *EmployeeService) CheckDuplicates(employeeNumber, nik, workEmail string) (nipTaken, nikTaken, emailTaken bool, err error) {
	nipTaken, nikTaken, emailTaken = false, false, false
	if num := strings.TrimSpace(employeeNumber); num != "" {
		if _, gErr := s.repo.GetByNumber(num); gErr == nil {
			nipTaken = true
		} else if !errors.Is(gErr, gorm.ErrRecordNotFound) {
			return false, false, false, gErr
		}
	}
	if k := strings.TrimSpace(nik); k != "" {
		if _, gErr := s.repo.GetByNIK(k); gErr == nil {
			nikTaken = true
		} else if !errors.Is(gErr, gorm.ErrRecordNotFound) {
			return false, false, false, gErr
		}
	}
	if m := strings.TrimSpace(workEmail); m != "" {
		if _, gErr := s.repo.GetByWorkEmail(m); gErr == nil {
			emailTaken = true
		} else if !errors.Is(gErr, gorm.ErrRecordNotFound) {
			return false, false, false, gErr
		}
	}
	return nipTaken, nikTaken, emailTaken, nil
}

// ==================== EMPLOYEE SERVICE ====================

type EmployeeService struct {
	repo   repository.EmployeeRepository
	movRepo repository.MovementRepository
}

func NewEmployeeService(repo repository.EmployeeRepository, movRepo repository.MovementRepository) *EmployeeService {
	return &EmployeeService{repo: repo, movRepo: movRepo}
}

func (s *EmployeeService) normalize(emp *entity.Employee) error {
	emp.FirstName = strings.TrimSpace(emp.FirstName)
	emp.LastName = strings.TrimSpace(emp.LastName)
	if emp.FirstName == "" {
		return fmt.Errorf("%w: first_name wajib diisi", ErrValidation)
	}
	emp.FullName = strings.TrimSpace(strings.Join(
		strings.Fields(emp.FirstName+" "+emp.MiddleName+" "+emp.LastName), " "))
	if emp.EmployeeNumber == "" {
		n, err := s.generateNumber()
		if err != nil {
			return err
		}
		emp.EmployeeNumber = n
	}
	if emp.Status == "" {
		emp.Status = "active"
	}
	// unique-nullable: string kosong → NULL (hindari collision '')
	nilIfEmpty := func(sp *string) *string {
		if sp == nil || *sp == "" {
			return nil
		}
		return sp
	}
	emp.UserID = nilIfEmpty(emp.UserID)
	emp.NIK = nilIfEmpty(emp.NIK)
	emp.WorkEmail = nilIfEmpty(emp.WorkEmail)
	return nil
}

// generateNumber — naming series ala Frappe: BIN-YYYYMM-#### (sequence per bulan)
func (s *EmployeeService) generateNumber() (string, error) {
	prefix := fmt.Sprintf("BIN-%s-", time.Now().Format("200601"))
	list, _, err := s.repo.List(repository.EmployeeFilter{Query: prefix, Page: 1, PageSize: 100})
	if err != nil {
		return "", err
	}
	max := 0
	for _, e := range list {
		suffix := strings.TrimPrefix(e.EmployeeNumber, prefix)
		if n, err := strconv.Atoi(suffix); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("%s%04d", prefix, max+1), nil
}

func isDup(err error) bool {
	return err != nil && (errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate key"))
}

func (s *EmployeeService) Create(emp *entity.Employee) error {
	if err := s.normalize(emp); err != nil {
		return err
	}
	if err := s.repo.Create(emp); err != nil {
		if isDup(err) {
			return ErrDuplicate
		}
		return err
	}
	// catat movement onboarding (best-effort — kegagalan dilog, bukan swallow)
	if emp.JoinedDate != nil {
		if mErr := s.movRepo.Create(&entity.EmployeeMovement{
			EmployeeID: emp.ID, MovementType: entity.MovementOnboarding,
			EffectiveDate: *emp.JoinedDate, CreatedBy: emp.CreatedBy,
		}, nil); mErr != nil {
			log.Printf("[hc] movement onboarding gagal utk employee %s: %v", emp.ID, mErr)
		}
	}
	return nil
}

func (s *EmployeeService) Update(emp *entity.Employee) error {
	if err := s.normalize(emp); err != nil {
		return err
	}
	if err := s.repo.Update(emp); err != nil {
		if isDup(err) {
			return ErrDuplicate
		}
		return err
	}
	return nil
}

func (s *EmployeeService) Get(id string) (*entity.Employee, []entity.EmployeeEducation, []entity.EmployeeMovement, error) {
	emp, err := s.repo.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, ErrNotFound
		}
		return nil, nil, nil, err
	}
	edus, _ := s.repo.ListEducations(id)
	movs, _ := s.movRepo.ListByEmployee(id)
	return emp, edus, movs, nil
}

func (s *EmployeeService) List(f repository.EmployeeFilter) ([]entity.Employee, int64, error) {
	return s.repo.List(f)
}

func (s *EmployeeService) Delete(id string) error {
	emp, err := s.repo.GetByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if emp.Status == "active" {
		return ErrStillEmployed
	}
	return s.repo.Delete(id)
}

func (s *EmployeeService) Stats() (map[string]int64, error) {
	return s.repo.CountByStatus()
}

// SetEducations — replace-all educations milik employee
func (s *EmployeeService) SetEducations(employeeID string, edus []entity.EmployeeEducation) error {
	if _, err := s.repo.GetByID(employeeID); err != nil {
		return ErrNotFound
	}
	if err := s.repo.DeleteEducations(employeeID); err != nil {
		return err
	}
	for i := range edus {
		edus[i].EmployeeID = employeeID
		if err := s.repo.CreateEducation(&edus[i]); err != nil {
			return err
		}
	}
	return nil
}
