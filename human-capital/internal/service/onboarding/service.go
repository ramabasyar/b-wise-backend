package onboarding

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/rama/b-wise/human-capital/internal/domain/entity"
	"github.com/rama/b-wise/human-capital/internal/domain/service"
	"github.com/rama/b-wise/human-capital/internal/service/permission"
	"github.com/rama/b-wise/human-capital/internal/service/sso"
	"gorm.io/gorm"
)

// OnboardingService — wizard 3 langkah:
// 0. Pre-flight: NIP/NIK/email duplikat dicek SEBELUM registrasi SSO (cegah user yatim)
//   - validasi kamus + validasi server-side role ⊆ mapping departemen (audit M4).
//
// 1. Register user di SSO (role realm via service token)
// 2. Create employee (HC) + movement onboarding otomatis (via EmployeeService.Create)
// 3. Assign role di Permission Service (retry 1x + status per-role — audit H4)
type OnboardingService struct {
	empSvc *service.EmployeeService
	sso    *sso.SSOClient
	perm   *permission.PermissionClient
	db     *gorm.DB // kamus employee_types + employee_unit_assignments + mapping department_services
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
	roleResults := []service.RoleResult{}

	// Step 0: pre-flight HC — cegah user SSO yatim akibat NIP/NIK/email duplikat
	if err := o.preFlight(&req); err != nil {
		return nil, err
	}
	if err := o.validateAgainstDict(&req); err != nil {
		return nil, err
	}

	// Step 0b (audit M4): validasi server-side role ⊆ mapping departemen.
	// Role milik service yang tidak ter-mapping utk unit pegawai → TOLAK sebelum side effect apa pun.
	roleInfo := map[string]permission.RoleInfo{} // roleID → info (nama dipakai lagi di Step 3)
	if ids := req.AllRoleIDs(); len(ids) > 0 {
		if req.DepartmentID == nil || *req.DepartmentID == "" {
			return nil, fmt.Errorf("%w: pegawai tanpa departemen tidak dapat diberi role akses", service.ErrValidation)
		}
		allowedSvc := map[string]bool{}
		if o.db != nil {
			var mapped []entity.DepartmentService
			o.db.Where("department_id = ? AND is_active = ?", *req.DepartmentID, true).Find(&mapped)
			for _, m := range mapped {
				allowedSvc[m.ServiceID] = true
			}
		}
		if len(allowedSvc) == 0 {
			return nil, fmt.Errorf("%w: unit kerja pegawai belum di-mapping ke service mana pun — lengkapi di Pengaturan Organisasi → Akses Service per Unit, atau onboard tanpa role", service.ErrValidation)
		}
		for _, rid := range ids {
			info, gErr := o.perm.GetRole(rid)
			if gErr != nil {
				return nil, fmt.Errorf("%w: %v", service.ErrValidation, gErr)
			}
			if !allowedSvc[info.ServiceID] {
				return nil, fmt.Errorf("%w: role %q tidak dapat diberikan — service-nya tidak ter-mapping untuk unit kerja pegawai ini (cek Akses Service per Unit)", service.ErrValidation, info.Name)
			}
			roleInfo[rid] = *info
		}
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
		// KOMPENSASI (audit H3): hard-delete user SSO otomatis berauth service token,
		// supaya onboard bisa diulang bersih tanpa intervensi admin.
		if dErr := o.sso.DeleteUser(userID); dErr == nil {
			log.Printf("[hc-onboard] KOMPENSASI OK: user SSO %s (id %s) dihapus otomatis setelah step 2 gagal: %v",
				ssoResp.Data.Username, userID, err)
			return nil, fmt.Errorf(
				"step 2 gagal (profil pegawai): %v — user SSO sementara sudah di-rollback otomatis, perbaiki isian lalu ulangi onboard",
				err)
		} else {
			log.Printf("[hc-onboard] ORPHAN: user SSO %s (username=%s, id=%s) terbuat tanpa employee: %v (kompensasi gagal: %v)",
				req.Email, ssoResp.Data.Username, userID, err, dErr)
			return nil, fmt.Errorf(
				"step 2 gagal (profil pegawai): %v — user SSO sementara sudah terbuat (username: %s, id: %s) dan gagal di-rollback otomatis; hapus manual via admin SSO lalu ulangi onboard",
				err, ssoResp.Data.Username, userID)
		}
	}

	// Step 2b: unit assignments (primary + secondary utk dual affiliation) — error
	// tidak lagi ditelan diam-diam (audit M2): gagal → warning eksplisit.
	warnings = append(warnings, o.writeUnitAssignments(emp, &req, createdBy)...)

	// Verifikasi realm roles (audit H4): mismatch = kondisi inkonsisten yang tidak boleh
	// lolos diam-diam — rollback data HC lokal + instruksi hapus user SSO, JANGAN lanjut.
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
	missingRealms := []string{}
	for _, want := range realmRoles {
		if !gotRealms[want] {
			missingRealms = append(missingRealms, want)
		}
	}
	if len(missingRealms) > 0 {
		o.rollbackEmployee(emp.ID) // best-effort: hapus employee+movement+assignment lokal
		if dErr := o.sso.DeleteUser(userID); dErr == nil {
			return nil, fmt.Errorf(
				"verifikasi realm SSO gagal: realm %v tidak ter-assign (diperoleh: %v) — data pegawai lokal & user SSO sudah di-rollback otomatis, ulangi onboard",
				missingRealms, ssoResp.Data.Roles)
		}
		return nil, fmt.Errorf(
			"verifikasi realm SSO gagal: realm %v tidak ter-assign (diperoleh: %v) — data pegawai lokal sudah di-rollback; hapus user SSO %q (id %s) via admin SSO lalu ulangi onboard",
			missingRealms, ssoResp.Data.Roles, ssoResp.Data.Username, userID)
	}

	// Step 3: Role akses lintas service — retry 1x + status per-role (audit H4).
	assign := func(userID, roleID string) error {
		var err error
		for attempt := 0; attempt < 2; attempt++ {
			if err = o.perm.AssignRole(userID, roleID, createdBy); err == nil {
				return nil
			}
			time.Sleep(300 * time.Millisecond)
		}
		return err
	}
	for _, roleID := range req.AllRoleIDs() {
		name := roleID
		if info, ok := roleInfo[roleID]; ok && info.Name != "" {
			name = info.Name
		}
		if err := assign(userID, roleID); err != nil {
			log.Printf("[hc-onboard] WARNING step 3 (assign role %s utk user %s): %v", roleID, userID, err)
			roleResults = append(roleResults, service.RoleResult{
				RoleID: roleID, RoleName: name, Status: "failed", Error: err.Error(),
			})
			warnings = append(warnings,
				fmt.Sprintf("Role %q GAGAL di-assign — employee sudah tersimpan. Assign ulang manual via menu Role/User Permission", name))
			continue
		}
		roleResults = append(roleResults, service.RoleResult{
			RoleID: roleID, RoleName: name, Status: "assigned",
		})
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
		RoleResults:    roleResults,
	}, nil
}

// SetDepartmentServices — set ulang mapping departemen→service (UI Pengaturan Organisasi,
// tab "Akses Service per Unit"). Validasi: departemen harus ada; setiap service_id harus
// terdaftar di permission-service. Transaksional: replace mapping lama departemen tsb.
func (o *OnboardingService) SetDepartmentServices(deptID string, serviceIDs []string, actor string) error {
	if o.db == nil {
		return fmt.Errorf("konfigurasi DB tidak tersedia")
	}
	var dept entity.Department
	if err := o.db.First(&dept, "id = ?", deptID).Error; err != nil {
		return fmt.Errorf("%w: departemen tidak ditemukan", service.ErrValidation)
	}
	svcList, err := o.perm.ListServices()
	if err != nil {
		return fmt.Errorf("gagal memuat daftar service dari permission-service: %w", err)
	}
	known := map[string]string{} // id → name
	for _, s := range svcList {
		known[s.ID] = s.Name
	}
	seen := map[string]bool{}
	for _, sid := range serviceIDs {
		if _, ok := known[sid]; !ok {
			return fmt.Errorf("%w: service %s tidak terdaftar di permission-service", service.ErrValidation, sid)
		}
		seen[sid] = true
	}
	return o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("department_id = ?", deptID).Delete(&entity.DepartmentService{}).Error; err != nil {
			return err
		}
		for sid := range seen {
			row := entity.DepartmentService{
				DepartmentID: deptID,
				ServiceID:    sid,
				ServiceName:  known[sid],
				IsActive:     true,
				CreatedBy:    actor,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// rollbackEmployee — hapus data HC lokal pegawai (employee + movement + unit assignment).
// Best-effort; dipakai saat verifikasi realm gagal agar tidak ada data setengah jadi (audit H4).
func (o *OnboardingService) rollbackEmployee(employeeID string) {
	if o.db == nil || employeeID == "" {
		return
	}
	o.db.Exec("DELETE FROM movement_details WHERE movement_id IN (SELECT id FROM employee_movements WHERE employee_id = ?)", employeeID)
	o.db.Exec("DELETE FROM employee_movements WHERE employee_id = ?", employeeID)
	o.db.Exec("DELETE FROM employee_unit_assignments WHERE employee_id = ?", employeeID)
	o.db.Exec("DELETE FROM employees WHERE id = ?", employeeID)
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

// writeUnitAssignments — primary + secondary (dual affiliation). Error create
// dicatat + dikembalikan sebagai warning (audit M2 — sebelumnya ditelan diam-diam).
func (o *OnboardingService) writeUnitAssignments(emp *entity.Employee, req *service.OnboardRequest, createdBy string) []string {
	warnings := []string{}
	if o.db == nil || emp.DepartmentID == nil || *emp.DepartmentID == "" {
		return warnings
	}
	now := time.Now()
	primary := &entity.EmployeeUnitAssignment{
		EmployeeID: emp.ID, UnitID: *emp.DepartmentID, IsPrimary: true,
		ValidFrom: &now, CreatedBy: createdBy,
	}
	if err := o.db.Create(primary).Error; err != nil {
		log.Printf("[hc-onboard] WARNING unit assignment (primary) utk employee %s: %v", emp.ID, err)
		warnings = append(warnings, "Penempatan unit UTAMA gagal tersimpan — perbaiki manual di data pegawai")
	}
	if req.SecondaryUnitID != "" && req.SecondaryUnitID != *emp.DepartmentID {
		secondary := &entity.EmployeeUnitAssignment{
			EmployeeID: emp.ID, UnitID: req.SecondaryUnitID, IsPrimary: false,
			ValidFrom: &now, CreatedBy: createdBy,
		}
		if err := o.db.Create(secondary).Error; err != nil {
			log.Printf("[hc-onboard] WARNING unit assignment (secondary) utk employee %s: %v", emp.ID, err)
			warnings = append(warnings, "Penempatan unit KEDUA gagal tersimpan — perbaiki manual di data pegawai")
		}
	}
	return warnings
}

func (o *OnboardingService) preFlight(req *service.OnboardRequest) error {
	// Validasi format tanggal (audit M3 — sebelumnya tanggal invalid hilang diam-diam).
	if req.BirthDate != nil && *req.BirthDate != "" {
		if _, err := time.Parse("2006-01-02", *req.BirthDate); err != nil {
			return fmt.Errorf("%w: birth_date harus format YYYY-MM-DD", service.ErrValidation)
		}
	}
	if req.JoinDate != nil && *req.JoinDate != "" {
		if _, err := time.Parse("2006-01-02", *req.JoinDate); err != nil {
			return fmt.Errorf("%w: join_date harus format YYYY-MM-DD", service.ErrValidation)
		}
	}
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
