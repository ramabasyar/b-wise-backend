package service

import (
	"fmt"
	"strings"
	"time"

	entity "github.com/rama/b-wise/human-capital/internal/domain/entity"
)

// OnboardRequest — payload wizard org-driven (akun → profil+dept → akses per service)
type OnboardRequest struct {
	// Step 1: Account
	Email     string `json:"email" binding:"required,email"`
	Username  string `json:"username" binding:"required,min=3"`
	Password  string `json:"password" binding:"required,min=8"`
	FirstName string `json:"first_name" binding:"required"`
	LastName  string `json:"last_name"`
	// Tipe pegawai (Fase A): staff | lecturer | lecturer_staff
	// — realm SSO dipetakan otomatis; "student" TIDAK berlaku di HC (ada service akademik sendiri).
	EmployeeType string `json:"employee_type" binding:"omitempty,oneof=staff lecturer lecturer_staff"`
	BaseRole     string `json:"base_role"` // DEPRECATED — dipakai hanya jika employee_type kosong

	// Step 2: Employee Profile
	EmployeeNumber   string  `json:"employee_number"` // NIP (opsional — auto jika kosong)
	Phone            string  `json:"phone"`
	Gender           string  `json:"gender"`
	BirthDate        *string `json:"birth_date"` // YYYY-MM-DD
	EmploymentTypeID string  `json:"employment_type_id"`
	DepartmentID     *string `json:"department_id"`
	DesignationID    *string `json:"designation_id"`
	GradeID          *string `json:"grade_id"`
	BranchID         *string `json:"branch_id"`
	JoinDate         *string `json:"join_date"`

	// Data tambahan (opsional) — identitas & kontak darurat
	NIK                   string `json:"nik"`      // 16 digit KTP, unik
	NPWP                  string `json:"npwp"`     // 15/16 digit
	EmergencyContactName  string `json:"emergency_contact_name"`
	EmergencyContactPhone string `json:"emergency_contact_phone"`
	CurrentAddress        string `json:"current_address"`

	// Step 3: Permissions — multi-role lintas service (sesuai mapping departemen)
	RoleIDs []string `json:"role_ids"`
	RoleID  string   `json:"role_id"` // DEPRECATED single-role — digabung ke RoleIDs

	// Level organisasi (kode kamus employment_levels)
	EmploymentLevel string `json:"employment_level"`
	// Jenjang akademik dosen (kode kamus academic_ranks)
	AcademicRank string `json:"academic_rank"`
	// Unit kedua (dual affiliation — dosen+staff). department_id = primary.
	SecondaryUnitID string `json:"secondary_unit_id"`
}

// RealmRoles — mapping tipe pegawai → realm SSO (slug). Dipakai kalau konfigurasi kamus
// tidak tersedia (fallback built-in minimum). Sumber utama: employee_types.realm_roles (CSV).
func (r *OnboardRequest) RealmRoles() []string {
	switch r.EmployeeType {
	case "lecturer":
		return []string{"lecture"}
	case "lecturer_staff":
		return []string{"lecture", "staff"}
	case "staff":
		return []string{"staff"}
	}
	// fallback kompatibilitas lama (base_role) / default
	if r.BaseRole == "lecture" {
		return []string{"lecture"}
	}
	return []string{"staff"}
}

// ParseCSV — helper pecah "a,b,c" → []string (trim, skip kosong).
func ParseCSV(s string) []string {
	out := []string{}
	cur := ""
	for _, ch := range s {
		if ch == ',' {
			if t := strings.TrimSpace(cur); t != "" {
				out = append(out, t)
			}
			cur = ""
			continue
		}
		cur += string(ch)
	}
	if t := strings.TrimSpace(cur); t != "" {
		out = append(out, t)
	}
	return out
}

// AllRoleIDs — gabung role_ids[] + role_id tunggal (dedupe, jaga urutan).
func (r *OnboardRequest) AllRoleIDs() []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, id := range r.RoleIDs {
		add(id)
	}
	add(r.RoleID)
	return out
}

type OnboardResult struct {
	UserID         string   `json:"user_id"`
	EmployeeRowID  string   `json:"employee_id"`
	EmployeeNumber string   `json:"employee_number"`
	Username       string   `json:"username"`
	Email          string   `json:"email"`
	BaseRole       string   `json:"base_role"`  // realm primer SSO (verifikasi)
	RealmRoles     []string `json:"realm_roles"` // semua realm yang ter-assign
	EmployeeType   string   `json:"employee_type"`
	Status         string   `json:"status"`
	Warnings       []string `json:"warnings,omitempty"` // best-effort steps yang perlu perhatian admin
}

func parseDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

// BuildOnboardEmployee — entity dari request wizard (dipakai OnboardingService)
func (r *OnboardRequest) BuildOnboardEmployee(userID, createdBy string) *entity.Employee {
	uid, wemail, pemail := userID, r.Email, r.Email
	emp := &entity.Employee{
		UserID:           &uid,
		EmployeeNumber:   strings.TrimSpace(r.EmployeeNumber),
		FirstName:        r.FirstName,
		LastName:         r.LastName,
		WorkEmail:        &wemail,
		PersonalEmail:    pemail,
		Phone:            r.Phone,
		Gender:           r.Gender,
		EmploymentTypeID: r.EmploymentTypeID,
		DepartmentID:     r.DepartmentID,
		DesignationID:    r.DesignationID,
		GradeID:          r.GradeID,
		BranchID:         r.BranchID,
		Status:           "active",
		CreatedBy:        createdBy,
		UpdatedBy:        createdBy,
	}
	// Identitas & kontak darurat (opsional dari wizard)
	if nik := strings.TrimSpace(r.NIK); nik != "" {
		emp.NIK = &nik
	}
	emp.NPWP = strings.TrimSpace(r.NPWP)
	emp.EmergencyContactName = strings.TrimSpace(r.EmergencyContactName)
	emp.EmergencyContactPhone = strings.TrimSpace(r.EmergencyContactPhone)
	emp.CurrentAddress = strings.TrimSpace(r.CurrentAddress)
	emp.EmploymentLevel = strings.TrimSpace(r.EmploymentLevel)
	emp.AcademicRank = strings.TrimSpace(r.AcademicRank)
	if r.BirthDate != nil {
		emp.BirthDate = parseDate(*r.BirthDate)
	}
	if r.JoinDate != nil {
		emp.JoinedDate = parseDate(*r.JoinDate)
	}
	if emp.JoinedDate == nil {
		now := time.Now()
		emp.JoinedDate = &now
	}
	if r.BaseRole == "" && r.EmployeeType == "" {
		r.BaseRole = "staff"
	}
	return emp
}

var _ = fmt.Sprintf // keep fmt import if unused later
