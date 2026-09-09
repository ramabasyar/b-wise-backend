package onboarding

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/rama/b-wise/human-capital/internal/domain/entity"
	"github.com/rama/b-wise/human-capital/internal/domain/service"
	"github.com/rama/b-wise/human-capital/internal/service/permission"
	"gorm.io/gorm"
	"github.com/rama/b-wise/human-capital/internal/service/sso"
)

// OnboardingService — wizard 3 langkah:
// 0. Pre-flight: NIP/NIK duplikat dicek SEBELUM registrasi SSO (cegah user yatim)
// 1. Register user di SSO (role realm via service token)
// 2. Create employee (HC) + movement onboarding otomatis (via EmployeeService.Create)
// 3. Assign role di Permission Service (best-effort → warning, bukan error)
type OnboardingService struct {
	empSvc *service.EmployeeService
	sso    *sso.SSOClient
	perm   *permission.PermissionClient
	db     *gorm.DB // kamus employee_types + tulis employee_unit_assignments
}

func NewOnboardingService(
	empSvc *service.EmployeeService,
	ssoClient *sso.SSOClient,
	permClient *permission.PermissionClient,
	db *gorm.DB,
) *OnboardingService {
	return &OnboardingService{empSvc: empSvc, sso: ssoClient, perm: permClient, db: db}
}

func (o *OnboardingService) OnboardEmployee(req service.OnboardRequest, createdBy string) (*service.OnboardResult, error) {
	warnings := []string{}

	// Step 0: pre-flight HC — cegah user SSO yatim akibat NIP/NIK/email duplikat
	if err := o.preFlight(&req); err != nil {
		return nil, err
	}
	if err := o.validateAgainstDict(&req); err != nil {
		return nil, err
	}

	// Step 1: SSO user — realm roles dari KAMUS employee_types (dynamic); fallback built-in.
	realmRoles := req.RealmRoles()
	if o.db != nil {
		var et entity.EmployeeType
		if err := o.db.Where("code = ? AND is_active = ?", req.EmployeeType, true).First(&et).Error; err == nil && et.RealmRoles != "" {
			if csv := service.ParseCSV(et.RealmRoles); len(csv) > 0 {
				realmRoles = csv
			}
		}
	}
	ssoResp, err := o.sso.RegisterUser(sso.RegisterUserRequest{
		Email:     req.Email,
		Username:  req.Username,
		Password:  req.Password,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		BaseRole:  realmRoles[0], // kompatibilitas — utama dikirim via Roles
		Roles:     realmRoles,
	})
	if err != nil {
		return nil, fmt.Errorf("step 1 gagal (registrasi SSO): %w", err)
	}
	userID := ssoResp.Data.ID

	// Step 2: Employee + movement onboarding
	emp := req.BuildOnboardEmployee(userID, createdBy)
	if err := o.empSvc.Create(emp); err != nil {
		// User SSO sudah terbuat tapi profil HC gagal → user yatim.
		// Tidak ada endpoint cleanup berauth service-token di SSO (admin-only),
		// jadi beri petunjuk eksplisit utk admin.
		log.Printf("[hc-onboard] ORPHAN: user SSO %s (username=%s, id=%s) terbuat tanpa employee: %v",
			req.Email, ssoResp.Data.Username, userID, err)
		return nil, fmt.Errorf(
			"step 2 gagal (profil pegawai): %v — user SSO sementara sudah terbuat (username: %s, id: %s). "+
				"Hapus user tersebut via admin portal SSO lalu ulangi onboard",
			err, ssoResp.Data.Username, userID)
	}

	// Step 2b: unit assignments (primary + secondary utk dual affiliation)
	o.writeUnitAssignments(emp, &req, createdBy)

	// Verifikasi realm roles — SSO bisa menurunkan ke default jika service token bermasalah.
	// Normalisasi: SSO mengembalikan slug array (roles) + display name (role).
	roleNameToSlug := map[string]string{"lecturer": "lecture", "student": "student", "staff": "staff"}
	gotRealms := map[string]bool{}
	for _, g := range ssoResp.Data.Roles {
		gl := strings.ToLower(g)
		if norm, ok := roleNameToSlug[gl]; ok {
			gl = norm
		}
		gotRealms[gl] = true
	}
	if got := strings.ToLower(ssoResp.Data.BaseRole); got != "" && len(ssoResp.Data.Roles) == 0 {
		if norm, ok := roleNameToSlug[got]; ok {
			got = norm
		}
		gotRealms[got] = true
	}
	for _, want := range realmRoles {
		if !gotRealms[want] {
			warnings = append(warnings, fmt.Sprintf(
				"Realm SSO %q tidak ter-assign (diperoleh: %v) — cek client credentials HC di SSO", want, ssoResp.Data.Roles))
		}
	}

	// Step 3: Role akses lintas service (best-effort per role — employee sudah jadi).
	// NOTE Fase A: validasi role⊆mapping-departemen dilakukan di UI; server-side menyusul (Fase B).
	for _, roleID := range req.AllRoleIDs() {
		if err := o.perm.AssignRole(userID, roleID, createdBy); err != nil {
			log.Printf("[hc-onboard] WARNING step 3 (assign role %s utk user %s): %v", roleID, userID, err)
			warnings = append(warnings,
				"Sebagian role GAGAL di-assign — employee sudah tersimpan. Assign ulang manual via menu Role/User Permission")
		}
	}

	return &service.OnboardResult{
		UserID:         userID,
		EmployeeRowID:  emp.ID,
		EmployeeNumber: emp.EmployeeNumber,
		Username:       ssoResp.Data.Username,
		Email:          ssoResp.Data.Email,
		BaseRole:       ssoResp.Data.BaseRole,
		RealmRoles:     ssoResp.Data.Roles,
		EmployeeType:   req.EmployeeType,
		Status:         "active",
		Warnings:       warnings,
	}, nil
}

// preFlight — validasi lokal HC sebelum menyentuh SSO (fail-fast, no side effect)
// validateAgainstDict — level/rank/type harus ada di kamus aktif (dynamic master data).
func (o *OnboardingService) validateAgainstDict(req *service.OnboardRequest) error {
	if o.db == nil {
		return nil
	}
	if req.EmployeeType != "" {
		var n int64
		o.db.Model(&entity.EmployeeType{}).Where("code = ? AND is_active = ?", req.EmployeeType, true).Count(&n)
		if n == 0 {
			return fmt.Errorf("tipe pegawai %q tidak dikenal (cek kamus employee_types)", req.EmployeeType)
		}
	}
	if req.EmploymentLevel != "" {
		var n int64
		o.db.Model(&entity.EmploymentLevel{}).Where("code = ? AND is_active = ?", req.EmploymentLevel, true).Count(&n)
		if n == 0 {
			return fmt.Errorf("level %q tidak dikenal (cek kamus employment_levels)", req.EmploymentLevel)
		}
	}
	if req.AcademicRank != "" {
		var n int64
		o.db.Model(&entity.AcademicRank{}).Where("code = ? AND is_active = ?", req.AcademicRank, true).Count(&n)
		if n == 0 {
			return fmt.Errorf("jenjang akademik %q tidak dikenal (cek kamus academic_ranks)", req.AcademicRank)
		}
	}
	return nil
}

// writeUnitAssignments — primary + secondary (dual affiliation) + izinkan role ⊆ mapping nanti.
func (o *OnboardingService) writeUnitAssignments(emp *entity.Employee, req *service.OnboardRequest, createdBy string) {
	if o.db == nil || emp.DepartmentID == nil || *emp.DepartmentID == "" {
		return
	}
	now := time.Now()
	primary := &entity.EmployeeUnitAssignment{
		EmployeeID: emp.ID, UnitID: *emp.DepartmentID, IsPrimary: true,
		ValidFrom: &now, CreatedBy: createdBy,
	}
	o.db.Create(primary)
	if req.SecondaryUnitID != "" && req.SecondaryUnitID != *emp.DepartmentID {
		o.db.Create(&entity.EmployeeUnitAssignment{
			EmployeeID: emp.ID, UnitID: req.SecondaryUnitID, IsPrimary: false,
			ValidFrom: &now, CreatedBy: createdBy,
		})
	}
}

func (o *OnboardingService) preFlight(req *service.OnboardRequest) error {
	if nik := strings.TrimSpace(req.NIK); nik != "" && len(nik) != 16 {
		return fmt.Errorf("NIK harus 16 digit")
	}
	nipTaken, nikTaken, emailTaken, err := o.empSvc.CheckDuplicates(req.EmployeeNumber, req.NIK, req.Email)
	if err != nil {
		return fmt.Errorf("pre-flight gagal (cek duplikat): %w", err)
	}
	if nipTaken {
		return fmt.Errorf("NIP %s sudah dipakai pegawai lain", req.EmployeeNumber)
	}
	if nikTaken {
		return fmt.Errorf("NIK sudah terdaftar pegawai lain")
	}
	if emailTaken {
		return fmt.Errorf("Email %s sudah dipakai pegawai lain — gunakan email lain atau edit pegawai terkait", req.Email)
	}
	return nil
}

// compile-time guard entity masih dipakai
var _ = entity.Employee{}
