package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	entity "github.com/rama/b-wise/human-capital/internal/domain/entity"
	repository "github.com/rama/b-wise/human-capital/internal/domain/repository"
	"gorm.io/gorm"
)

// ==================== MOVEMENT SERVICE ====================
// Padanan Frappe Employee Promotion/Transfer/Separation, unified.
// Kontrak: Create() menyimpan dokumen movement + snapshot detail perubahan
// (old→new) LALU mengaplikasikan perubahan ke employee — satu DB transaction.

type MovementService struct {
	movRepo repository.MovementRepository
	empRepo repository.EmployeeRepository
	db      *gorm.DB // untuk transaction apply (repo interface tidak expose tx)
}

func NewMovementService(
	movRepo repository.MovementRepository,
	empRepo repository.EmployeeRepository,
	db *gorm.DB,
) *MovementService {
	return &MovementService{movRepo: movRepo, empRepo: empRepo, db: db}
}

type MovementInput struct {
	EmployeeID    string                    `json:"employee_id"`
	MovementType  entity.MovementType       `json:"movement_type"`
	EffectiveDate time.Time                 `json:"effective_date"`
	ReferenceNo   string                    `json:"reference_no"`
	Notes         string                    `json:"notes"`
	Changes       map[string]string         `json:"changes"` // fieldname → new value (id mentah atau status)
	RelievingDate *time.Time                `json:"relieving_date"`
	ResignationLetterDate *time.Time        `json:"resignation_letter_date"`
	ReasonForLeaving     string             `json:"reason_for_leaving"`
}

func (s *MovementService) List(f repository.MovementFilter) ([]entity.EmployeeMovement, int64, error) {
	return s.movRepo.List(f)
}

func (s *MovementService) ListByEmployee(employeeID string) ([]entity.EmployeeMovement, error) {
	return s.movRepo.ListByEmployee(employeeID)
}

// Create — validasi → snapshot → transaction(insert movement+details, update employee)
func (s *MovementService) Create(in MovementInput, createdBy string) (*entity.EmployeeMovement, error) {
	// validasi dasar
	if in.EmployeeID == "" {
		return nil, fmt.Errorf("%w: employee_id wajib diisi", ErrValidation)
	}
	allowed := map[entity.MovementType]bool{
		entity.MovementPromotion: true, entity.MovementTransfer: true,
		entity.MovementStatusChange: true, entity.MovementSeparation: true,
	}
	if !allowed[in.MovementType] {
		return nil, fmt.Errorf("%w: movement_type harus promotion|transfer|status_change|separation", ErrValidation)
	}
	if in.EffectiveDate.IsZero() {
		return nil, fmt.Errorf("%w: effective_date wajib diisi", ErrValidation)
	}

	emp, err := s.empRepo.GetByID(in.EmployeeID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if emp.Status == "left" {
		return nil, fmt.Errorf("%w: pegawai sudah resign (left)", ErrValidation)
	}

	// separation: wajib relieving_date; changes tidak wajib
	if in.MovementType == entity.MovementSeparation && in.RelievingDate == nil {
		return nil, fmt.Errorf("%w: separation wajib relieving_date", ErrValidation)
	}

	// validasi changes terhadap whitelist
	for field := range in.Changes {
		if _, ok := entity.MovementFieldWhitelist[field]; !ok {
			return nil, fmt.Errorf("%w: field %q tidak boleh diubah via movement (whitelist: designation_id, department_id, grade_id, branch_id, employment_type_id, reports_to, status)", ErrValidation, field)
		}
	}
	if in.MovementType != entity.MovementSeparation && len(in.Changes) == 0 {
		return nil, fmt.Errorf("%w: changes kosong — tidak ada yang diubah", ErrValidation)
	}

	// bangun detail snapshot (old → new) + perubahan utk employee
	var details []entity.MovementDetail
	updates := map[string]interface{}{}
	for _, field := range sortedKeys(in.Changes) {
		newVal := strings.TrimSpace(in.Changes[field])
		oldVal := employeeFieldString(emp, field)
		if oldVal == newVal {
			continue // tidak ada perubahan — skip (idempoten)
		}
		details = append(details, entity.MovementDetail{
			Fieldname: field,
			Property:  entity.MovementFieldWhitelist[field],
			OldValue:  oldVal,
			NewValue:  newVal,
		})
		updates[field] = newVal
	}

	mov := &entity.EmployeeMovement{
		EmployeeID:    in.EmployeeID,
		MovementType:  in.MovementType,
		EffectiveDate: in.EffectiveDate,
		ReferenceNo:   in.ReferenceNo,
		Notes:         in.Notes,
		CreatedBy:     createdBy,
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		// insert movement + details
		if err := tx.Create(mov).Error; err != nil {
			return err
		}
		for i := range details {
			details[i].MovementID = mov.ID
		}
		if len(details) > 0 {
			if err := tx.Create(&details).Error; err != nil {
				return err
			}
		}
		// apply ke employee
		if in.MovementType == entity.MovementSeparation {
			patch := map[string]interface{}{
				"status": "left", "relieving_date": in.RelievingDate,
			}
			if in.ResignationLetterDate != nil {
				patch["resignation_letter_date"] = in.ResignationLetterDate
			}
			if in.ReasonForLeaving != "" {
				patch["reason_for_leaving"] = in.ReasonForLeaving
			}
			if err := tx.Model(&entity.Employee{}).Where("id = ?", in.EmployeeID).Updates(patch).Error; err != nil {
				return err
			}
			// detail snapshot utk separation (status + relieving)
			sepDetails := []entity.MovementDetail{{
				MovementID: mov.ID, Fieldname: "status", Property: "Status",
				OldValue: emp.Status, NewValue: "left",
			}}
			return tx.Create(&sepDetails).Error
		}
		if len(updates) > 0 {
			return tx.Model(&entity.Employee{}).Where("id = ?", in.EmployeeID).Updates(updates).Error
		}
		return nil
	})
	if err != nil {
		if isDup(err) {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	mov.Details = details
	return mov, nil
}

// ---------- helpers ----------

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

// employeeFieldString — representasi string kolom employee utk snapshot
func employeeFieldString(e *entity.Employee, field string) string {
	ptr := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}
	switch field {
	case "designation_id":
		return ptr(e.DesignationID)
	case "department_id":
		return ptr(e.DepartmentID)
	case "grade_id":
		return ptr(e.GradeID)
	case "branch_id":
		return ptr(e.BranchID)
	case "employment_type_id":
		return e.EmploymentTypeID
	case "reports_to":
		return ptr(e.ReportsTo)
	case "status":
		return e.Status
	}
	return ""
}
