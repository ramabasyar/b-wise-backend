package entity

import (
	"time"

	"gorm.io/gorm"
)

// ==================== EMPLOYEE ====================
// Frappe HR-aligned Employee master (adaptasi Indonesia).
// Employment changes (jabatan/departemen/grade/status) TIDAK diubah langsung —
// wajib via EmployeeMovement (riwayat, transaksional). Field non-employment
// (kontak, personal, alamat) bebas di-update via PUT.

type Employee struct {
	ID             string `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	UserID         *string `json:"user_id,omitempty" gorm:"type:varchar(36);uniqueIndex"` // FK SSO user (NULL = belum punya akun)
	EmployeeNumber string `json:"employee_number" gorm:"type:varchar(50);uniqueIndex;not null"` // NIP

	// Names (Frappe: first/middle/last + employee_name)
	FirstName string `json:"first_name" gorm:"type:varchar(100);not null"`
	MiddleName string `json:"middle_name,omitempty" gorm:"type:varchar(100)"`
	LastName  string `json:"last_name,omitempty" gorm:"type:varchar(100)"`
	FullName  string `json:"full_name" gorm:"type:varchar(255);not null;index"` // generated di service

	// Personal
	Gender        string     `json:"gender,omitempty" gorm:"type:varchar(20)"`
	BirthDate     *time.Time `json:"birth_date,omitempty" gorm:"type:timestamptz"`
	MaritalStatus string     `json:"marital_status,omitempty" gorm:"type:varchar(20)"`
	BloodGroup    string     `json:"blood_group,omitempty" gorm:"type:varchar(10)"`

	// Identitas Indonesia
	NIK            *string `json:"nik,omitempty" gorm:"type:varchar(16);uniqueIndex"` // pointer: NULL ≠ collision unique
	NPWP           string `json:"npwp,omitempty" gorm:"type:varchar(30)"`
	BpjsKesehatanNo string `json:"bpjs_kesehatan_no,omitempty" gorm:"type:varchar(30)"`
	BpjsTkNo       string `json:"bpjs_tk_no,omitempty" gorm:"type:varchar(30)"`
	PassportNumber string `json:"passport_number,omitempty" gorm:"type:varchar(30)"`

	// Contact (Frappe contact tab)
	PersonalEmail string `json:"personal_email,omitempty" gorm:"type:varchar(255)"`
	WorkEmail     *string `json:"work_email,omitempty" gorm:"type:varchar(255);uniqueIndex"`
	Phone         string `json:"phone,omitempty" gorm:"type:varchar(30)"`
	EmergencyContactName   string `json:"emergency_contact_name,omitempty" gorm:"type:varchar(150)"`
	EmergencyContactPhone  string `json:"emergency_contact_phone,omitempty" gorm:"type:varchar(30)"`
	EmergencyRelation      string `json:"emergency_relation,omitempty" gorm:"type:varchar(50)"`

	// Address
	CurrentAddress   string `json:"current_address,omitempty" gorm:"type:text"`
	PermanentAddress string `json:"permanent_address,omitempty" gorm:"type:text"`

	// Employment (Frappe employment tab) — perubahan via movement
	EmploymentTypeID string  `json:"employment_type_id,omitempty" gorm:"type:varchar(36);index"`
	DepartmentID     *string `json:"department_id,omitempty" gorm:"type:varchar(36);index"`
	DesignationID    *string `json:"designation_id,omitempty" gorm:"type:varchar(36);index"`
	GradeID          *string `json:"grade_id,omitempty" gorm:"type:varchar(36);index"`
	BranchID         *string `json:"branch_id,omitempty" gorm:"type:varchar(36);index"`
	ReportsTo        *string `json:"reports_to,omitempty" gorm:"type:varchar(36);index"` // self-link
	// Level organisasi (kode kamus employment_levels)
	EmploymentLevel  string  `json:"employment_level,omitempty" gorm:"type:varchar(30);index"`
	// Jenjang akademik dosen (kode kamus academic_ranks)
	AcademicRank     string  `json:"academic_rank,omitempty" gorm:"type:varchar(30);index"`

	JoinedDate                *time.Time `json:"joined_date,omitempty" gorm:"type:timestamptz;index"`
	ScheduledConfirmationDate *time.Time `json:"scheduled_confirmation_date,omitempty" gorm:"type:timestamptz"`
	FinalConfirmationDate     *time.Time `json:"final_confirmation_date,omitempty" gorm:"type:timestamptz"`
	ContractEndDate           *time.Time `json:"contract_end_date,omitempty" gorm:"type:timestamptz;index"`
	NoticeDays                int        `json:"notice_days,omitempty" gorm:"default:0"`
	RetirementDate            *time.Time `json:"retirement_date,omitempty" gorm:"type:timestamptz"`

	// Status: active | inactive | suspended | left
	Status string `json:"status" gorm:"type:varchar(20);default:'active';index"`

	// Exit (Frappe exit tab — terisi saat separation)
	ResignationLetterDate *time.Time `json:"resignation_letter_date,omitempty" gorm:"type:timestamptz"`
	RelievingDate         *time.Time `json:"relieving_date,omitempty" gorm:"type:timestamptz"`
	ReasonForLeaving      string     `json:"reason_for_leaving,omitempty" gorm:"type:text"`

	// Misc
	AttendanceDeviceID string `json:"attendance_device_id,omitempty" gorm:"type:varchar(50)"` // fingerprint Fase 2
	PhotoURL           string `json:"photo_url,omitempty" gorm:"type:varchar(500)"`
	Bio                string `json:"bio,omitempty" gorm:"type:text"`

	// Audit
	CreatedBy string     `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	UpdatedBy string     `json:"updated_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

func (Employee) TableName() string { return "employees" }

// ==================== EMPLOYEE EDUCATION (child) ====================

type EmployeeEducation struct {
	ID              string    `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	EmployeeID      string    `json:"employee_id" gorm:"type:varchar(36);not null;index"`
	Level           string    `json:"level" gorm:"type:varchar(30)"` // SMA/D3/S1/S2/S3/Profesi
	Institution     string    `json:"institution" gorm:"type:varchar(200)"`
	Major           string    `json:"major,omitempty" gorm:"type:varchar(200)"`
	GraduationYear  int       `json:"graduation_year,omitempty"`
	CertificateNo   string    `json:"certificate_no,omitempty" gorm:"type:varchar(80)"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (EmployeeEducation) TableName() string { return "employee_educations" }

// ==================== DEPARTMENT (tree — Frappe Department) ====================

type Department struct {
	ID              string `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	Code            string `json:"code" gorm:"type:varchar(50);uniqueIndex;not null"`
	Name            string `json:"name" gorm:"type:varchar(200);not null"`
	Description     string `json:"description,omitempty" gorm:"type:text"`
	ParentID        *string `json:"parent_id,omitempty" gorm:"type:varchar(36);index"` // tree
	IsGroup         bool   `json:"is_group" gorm:"default:false"` // node punya anak (mis. Fakultas)
	// Fase B: jenis unit org (kode kamus org_unit_types) — university/faculty/study_program/functional_unit
	OrgType         string `json:"org_type,omitempty" gorm:"type:varchar(30);index"`
	HeadEmployeeID  *string `json:"head_employee_id,omitempty" gorm:"type:varchar(36)"` // kepala unit
	IsActive        bool   `json:"is_active" gorm:"default:true;index"`
	CreatedBy string         `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	UpdatedBy string         `json:"updated_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

func (Department) TableName() string { return "departments" }

// ==================== DESIGNATION (Frappe Designation — jabatan) ====================

type Designation struct {
	ID          string `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	Code        string `json:"code,omitempty" gorm:"type:varchar(50);uniqueIndex"`
	Name        string `json:"name" gorm:"type:varchar(200);not null"`
	Description string `json:"description,omitempty" gorm:"type:text"`
	Level       int    `json:"level,omitempty" gorm:"default:1"` // eselon/golongan: 1 = tertinggi
	IsActive    bool   `json:"is_active" gorm:"default:true;index"`
	CreatedBy string         `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	UpdatedBy string         `json:"updated_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

func (Designation) TableName() string { return "designations" }

// ==================== EMPLOYMENT TYPE (Frappe Employment Type) ====================

type EmploymentType struct {
	ID          string `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	Name        string `json:"name" gorm:"type:varchar(100);uniqueIndex;not null"` // PNS/PPPK/PKWT/...
	Description string `json:"description,omitempty" gorm:"type:text"`
	IsDefault   bool   `json:"is_default" gorm:"default:false"`
	SortOrder   int    `json:"sort_order,omitempty" gorm:"default:0"`
	IsActive    bool   `json:"is_active" gorm:"default:true;index"`
	CreatedBy string         `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	UpdatedBy string         `json:"updated_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

func (EmploymentType) TableName() string { return "employment_types" }

// ==================== GRADE (Frappe Grade — golongan) ====================

type Grade struct {
	ID          string `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	Name        string `json:"name" gorm:"type:varchar(100);uniqueIndex;not null"` // "III/a", "IX — Ahli Pertama"
	Description string `json:"description,omitempty" gorm:"type:text"`
	SortOrder   int    `json:"sort_order,omitempty" gorm:"default:0"`
	IsActive    bool   `json:"is_active" gorm:"default:true;index"`
	CreatedBy string         `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	UpdatedBy string         `json:"updated_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

func (Grade) TableName() string { return "grades" }

// ==================== BRANCH (Frappe Branch → Fakultas/Lembaga/Unit) ====================

type Branch struct {
	ID          string `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	Code        string `json:"code,omitempty" gorm:"type:varchar(50);uniqueIndex"`
	Name        string `json:"name" gorm:"type:varchar(200);not null"`
	Description string `json:"description,omitempty" gorm:"type:text"`
	IsActive    bool   `json:"is_active" gorm:"default:true;index"`
	CreatedBy string         `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	UpdatedBy string         `json:"updated_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

func (Branch) TableName() string { return "branches" }

// ==================== EMPLOYEE MOVEMENT (Frappe Promotion/Transfer/Separation unified) ====================

type MovementType string

const (
	MovementOnboarding   MovementType = "onboarding"
	MovementPromotion    MovementType = "promotion"
	MovementTransfer     MovementType = "transfer"
	MovementStatusChange MovementType = "status_change"
	MovementSeparation   MovementType = "separation"
)

type EmployeeMovement struct {
	ID            string       `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	EmployeeID    string       `json:"employee_id" gorm:"type:varchar(36);not null;index"`
	MovementType  MovementType `json:"movement_type" gorm:"type:varchar(20);not null;index"`
	EffectiveDate time.Time    `json:"effective_date" gorm:"type:timestamptz;not null"`
	ReferenceNo   string       `json:"reference_no,omitempty" gorm:"type:varchar(80)"` // SK/surat keputusan
	Notes         string       `json:"notes,omitempty" gorm:"type:text"`
	CreatedBy     string       `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`

	Details []MovementDetail `json:"details" gorm:"foreignKey:MovementID"`
}

func (EmployeeMovement) TableName() string { return "employee_movements" }

// MovementDetail — padanan Frappe Employee Property History (property/current/new)
type MovementDetail struct {
	ID         string    `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	MovementID string    `json:"movement_id" gorm:"type:varchar(36);not null;index"`
	Fieldname  string    `json:"fieldname" gorm:"type:varchar(50);not null"` // kolom employee yang berubah
	Property   string    `json:"property" gorm:"type:varchar(100)"`          // label human ("Jabatan")
	OldValue   string    `json:"old_value,omitempty" gorm:"type:varchar(255)"`
	NewValue   string    `json:"new_value,omitempty" gorm:"type:varchar(255)"`
}

func (MovementDetail) TableName() string { return "movement_details" }

// MovementFieldWhitelist — kolom employment yang boleh diubah via movement
var MovementFieldWhitelist = map[string]string{
	"designation_id":     "Jabatan",
	"department_id":      "Departemen",
	"grade_id":           "Golongan/Grade",
	"branch_id":          "Unit/Fakultas",
	"employment_type_id": "Jenis Kepegawaian",
	"reports_to":         "Atasan",
	"status":             "Status",
}

// DepartmentService — mapping departemen HC ↔ service B-Wise (Fase A org-driven access).
// Menentukan service apa saja yang bisa diberi role saat onboard karyawan departemen tsb.
// ServiceID = UUID service di permission-service.
type DepartmentService struct {
	ID          string `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	DepartmentID string `json:"department_id" gorm:"type:varchar(36);uniqueIndex:idx_dept_service;not null"`
	ServiceID   string `json:"service_id" gorm:"type:varchar(36);uniqueIndex:idx_dept_service;not null"`
	ServiceName string `json:"service_name" gorm:"type:varchar(100)"` // cache nama utk display
	IsActive    bool   `json:"is_active" gorm:"default:true"`
	CreatedBy   string `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
