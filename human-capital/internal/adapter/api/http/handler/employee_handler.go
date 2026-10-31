package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/human-capital/internal/adapter/api/http/middleware"
	"github.com/rama/b-wise/human-capital/internal/domain/entity"
	repository "github.com/rama/b-wise/human-capital/internal/domain/repository"
	service "github.com/rama/b-wise/human-capital/internal/domain/service"
	"github.com/rama/b-wise/human-capital/internal/service/onboarding"
	"github.com/rama/b-wise/human-capital/pkg/response"
	"gorm.io/gorm"
)

// EmployeeHandler — employees + movements + onboarding
type EmployeeHandler struct {
	svc     *service.EmployeeService
	movSvc  *service.MovementService
	onboard *onboarding.OnboardingService
	db      *gorm.DB
}

func NewEmployeeHandler(svc *service.EmployeeService, movSvc *service.MovementService) *EmployeeHandler {
	return &EmployeeHandler{svc: svc, movSvc: movSvc}
}

func (h *EmployeeHandler) SetOnboardingService(o *onboarding.OnboardingService) { h.onboard = o }

// SetDB — injeksi koneksi DB (read-only utk lookup ringan spt mapping department_services)
func (h *EmployeeHandler) SetDB(db *gorm.DB) { h.db = db }

func actor(c *gin.Context) string { return middleware.GetUserID(c) }

func errStatus(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	case errors.Is(err, service.ErrDuplicate):
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	case errors.Is(err, service.ErrValidation), errors.Is(err, service.ErrStillEmployed):
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	}
}

func pageParams(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if size > 100 {
		size = 100
	}
	return page, size
}

// ==================== EMPLOYEES ====================

// ListEmployees GET /api/employees
func (h *EmployeeHandler) ListEmployees(c *gin.Context) {
	page, size := pageParams(c)
	f := repository.EmployeeFilter{
		Page: page, PageSize: size,
		Status:           c.Query("status"),
		DepartmentID:     c.Query("department_id"),
		BranchID:         c.Query("branch_id"),
		EmploymentTypeID: c.Query("employment_type_id"),
		Query:            strings.TrimSpace(c.Query("q")),
		UserID:           c.Query("user_id"),
	}
	list, total, err := h.svc.List(f)
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    list,
		"meta":    gin.H{"page": page, "page_size": size, "total": total},
	})
}

// CreateEmployee POST /api/employees — pegawai tanpa akun SSO (user_id nullable)
func (h *EmployeeHandler) CreateEmployee(c *gin.Context) {
	var emp entity.Employee
	if err := c.ShouldBindJSON(&emp); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	emp.CreatedBy = actor(c)
	emp.UpdatedBy = actor(c)
	if err := h.svc.Create(&emp); err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, emp)
}

// GetEmployee GET /api/employees/:id — detail + educations + movements (timeline)
func (h *EmployeeHandler) GetEmployee(c *gin.Context) {
	emp, edus, movs, err := h.svc.Get(c.Param("id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, gin.H{
		"employee":   emp,
		"educations": edus,
		"movements":  movs,
	})
}

// UpdateEmployee PUT /api/employees/:id — hanya field non-employment.
// Field employment (designation/department/grade/branch/employment_type/reports_to/status)
// di-skip — wajib via POST /api/movements (riwayat).
func (h *EmployeeHandler) UpdateEmployee(c *gin.Context) {
	existing, _, _, err := h.svc.Get(c.Param("id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	var in entity.Employee
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}

	// employment fields: preserve dari existing (guard server-side)
	in.ID = existing.ID
	in.UserID = existing.UserID
	in.EmployeeNumber = existing.EmployeeNumber
	in.EmploymentTypeID = existing.EmploymentTypeID
	in.DepartmentID = existing.DepartmentID
	in.DesignationID = existing.DesignationID
	in.GradeID = existing.GradeID
	in.BranchID = existing.BranchID
	in.ReportsTo = existing.ReportsTo
	in.Status = existing.Status
	in.JoinedDate = existing.JoinedDate
	in.CreatedBy = existing.CreatedBy
	in.UpdatedBy = actor(c)

	if err := h.svc.Update(&in); err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, in)
}

// DeleteEmployee DELETE /api/employees/:id — soft delete (hanya non-aktif)
func (h *EmployeeHandler) DeleteEmployee(c *gin.Context) {
	if err := h.svc.Delete(c.Param("id")); err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "pegawai dihapus"})
}

// EmployeeStats GET /api/employees/stats
func (h *EmployeeHandler) EmployeeStats(c *gin.Context) {
	m, err := h.svc.Stats()
	if err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, m)
}

// SetEducations PUT /api/employees/:id/educations
func (h *EmployeeHandler) SetEducations(c *gin.Context) {
	var body struct {
		Educations []entity.EmployeeEducation `json:"educations" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	if err := h.svc.SetEducations(c.Param("id"), body.Educations); err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, gin.H{"message": "riwayat pendidikan tersimpan"})
}

// ==================== MOVEMENTS ====================

// ListMovements GET /api/movements
func (h *EmployeeHandler) ListMovements(c *gin.Context) {
	page, size := pageParams(c)
	f := repository.MovementFilter{
		Page: page, PageSize: size,
		EmployeeID: c.Query("employee_id"),
		Type:       c.Query("type"),
	}
	list, total, err := h.movSvc.List(f)
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    list,
		"meta":    gin.H{"page": page, "page_size": size, "total": total},
	})
}

// ListEmployeeMovements GET /api/employees/:id/movements
func (h *EmployeeHandler) ListEmployeeMovements(c *gin.Context) {
	list, err := h.movSvc.ListByEmployee(c.Param("id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, list)
}

// CreateMovement POST /api/movements — promotion/transfer/status_change/separation
func (h *EmployeeHandler) CreateMovement(c *gin.Context) {
	var in service.MovementInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	mov, err := h.movSvc.Create(in, actor(c))
	if err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, mov)
}

// ==================== ONBOARDING ====================

// OnboardCheck GET /api/onboard/check — pre-flight duplikat NIP/NIK (dipakai wizard real-time)
func (h *EmployeeHandler) OnboardCheck(c *gin.Context) {
	if h.onboard == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": gin.H{"message": "onboarding belum terkonfigurasi"}})
		return
	}
	nipTaken, nikTaken, emailTaken, err := h.svc.CheckDuplicates(c.Query("employee_number"), c.Query("nik"), c.Query("email"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	response.Success(c, gin.H{
		"employee_number_taken": nipTaken,
		"nik_taken":             nikTaken,
		"email_taken":           emailTaken,
	})
}

// OnboardDeptServices GET /api/onboard/department-services — mapping departemen ↔ service
// (FE meng-join dgn services+roles dari permission service via permClient)
func (h *EmployeeHandler) OnboardDeptServices(c *gin.Context) {
	var rows []entity.DepartmentService
	if err := h.db.Where("is_active = ?", true).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, gin.H{
			"department_id": r.DepartmentID,
			"service_id":    r.ServiceID,
			"service_name":  r.ServiceName,
		})
	}
	response.Success(c, out)
}

// SetDeptServices PUT /api/onboard/department-services — set mapping departemen→service
// (UI Pengaturan Organisasi → Akses Service per Unit; sumber pilihan role wizard Langkah 3).
func (h *EmployeeHandler) SetDeptServices(c *gin.Context) {
	if h.onboard == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": gin.H{"message": "onboarding belum terkonfigurasi"}})
		return
	}
	var body struct {
		DepartmentID string   `json:"department_id" binding:"required"`
		ServiceIDs   []string `json:"service_ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	if err := h.onboard.SetDepartmentServices(body.DepartmentID, body.ServiceIDs, actor(c)); err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, gin.H{"message": "mapping akses service tersimpan"})
}

// Onboard POST /api/onboard
func (h *EmployeeHandler) Onboard(c *gin.Context) {
	if h.onboard == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": gin.H{"message": "onboarding belum terkonfigurasi"}})
		return
	}
	var req service.OnboardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	res, err := h.onboard.OnboardEmployee(req, actor(c))
	if err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, res)
}

var _ = time.Now

// ResolveApprover GET /api/org/resolve-approver?user_id=&service_id= — internal (permission-service).
// Logika (keputusan terkunci): approver = kepala unit pengguna yang MAPPING service target;
// fallback: kepala unit primary; fallback2: null (pending tanpa approver — admin HC putuskan).
func (h *EmployeeHandler) ResolveApprover(c *gin.Context) {
	userID := c.Query("user_id")
	serviceID := c.Query("service_id")
	if userID == "" || serviceID == "" || h.db == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "user_id & service_id wajib"}})
		return
	}

	// posisi struktural yang dianggap kepala unit (IsApprovalHead)
	var headPositions []entity.StructuralPosition
	h.db.Where("is_approval_head = ? AND is_active = ?", true, true).Find(&headPositions)
	headSet := map[string]bool{}
	for _, p := range headPositions {
		headSet[p.Code] = true
	}

	// unit assignment user (aktif)
	var assigns []entity.EmployeeUnitAssignment
	h.db.Where("employee_id IN (SELECT id FROM employees WHERE user_id = ?) AND (valid_to IS NULL OR valid_to > now())", userID).Find(&assigns)
	if len(assigns) == 0 {
		response.Success(c, gin.H{"approver_user_id": nil, "reason": "no_assignment"})
		return
	}
	empIDs := map[string]string{} // employeeID → unitID primary marker (unit di simpan di assigns)
	_ = empIDs

	// cari unit yang mapping service target
	unitIDs := map[string]bool{}
	for _, a := range assigns {
		unitIDs[a.UnitID] = true
	}
	var mappedUnits []entity.DepartmentService
	h.db.Where("is_active = ? AND service_id = ?", true, serviceID).Find(&mappedUnits)
	var targetUnit string
	for _, m := range mappedUnits {
		if unitIDs[m.DepartmentID] {
			targetUnit = m.DepartmentID
			break
		}
	}
	primaryUnit := ""
	for _, a := range assigns {
		if a.IsPrimary {
			primaryUnit = a.UnitID
		}
	}
	if targetUnit == "" {
		targetUnit = primaryUnit // fallback ke unit primary
	}
	if targetUnit == "" {
		response.Success(c, gin.H{"approver_user_id": nil, "reason": "no_unit_for_service"})
		return
	}

	// kepala unit target
	var heads []struct {
		EmployeeID         string
		StructuralPosition string
	}
	h.db.Table("employee_unit_assignments").
		Where("unit_id = ? AND (valid_to IS NULL OR valid_to > now()) AND structural_position <> ''", targetUnit).
		Find(&heads)
	for _, hd := range heads {
		if !headSet[hd.StructuralPosition] {
			continue
		}
		// employee → user SSO id
		var emp entity.Employee
		if err := h.db.Select("user_id").Where("id = ?", hd.EmployeeID).First(&emp).Error; err != nil || emp.UserID == nil {
			continue
		}
		response.Success(c, gin.H{"approver_user_id": *emp.UserID, "unit_id": targetUnit})
		return
	}
	response.Success(c, gin.H{"approver_user_id": nil, "reason": "no_head", "unit_id": targetUnit})
}

// UnitMembers GET /api/org/units/:id/members — HC-1 (helpdesk): anggota sebuah unit.
// Sumber: employee_unit_assignments aktif + profil pegawai (SSO user_id).
func (h *EmployeeHandler) UnitMembers(c *gin.Context) {
	unitID := c.Param("id")
	if unitID == "" || h.db == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "unit id wajib"}})
		return
	}
	var rows []struct {
		EmployeeID         string
		IsPrimary          bool
		StructuralPosition string
	}
	h.db.Table("employee_unit_assignments").
		Where("unit_id = ? AND (valid_to IS NULL OR valid_to > now())", unitID).
		Find(&rows)
	type member struct {
		EmployeeID         string `json:"employee_id"`
		UserID             string `json:"user_id,omitempty"`
		FullName           string `json:"full_name"`
		IsPrimary          bool   `json:"is_primary"`
		StructuralPosition string `json:"structural_position,omitempty"`
	}
	members := make([]member, 0, len(rows))
	for _, r := range rows {
		var emp entity.Employee
		if err := h.db.Select("id, user_id, full_name").Where("id = ? AND status = ?", r.EmployeeID, "active").First(&emp).Error; err != nil {
			continue // pegawai tidak aktif tidak disajikan
		}
		m := member{EmployeeID: r.EmployeeID, FullName: emp.FullName, IsPrimary: r.IsPrimary, StructuralPosition: r.StructuralPosition}
		if emp.UserID != nil {
			m.UserID = *emp.UserID
		}
		members = append(members, m)
	}
	response.Success(c, gin.H{"unit_id": unitID, "members": members})
}

// ResolveRole GET /api/org/resolve-role?unit_id=&position=&climb=1 — HC-2 (helpdesk):
// cari pemegang jabatan struktural (kode structural_positions, mis. kaprodi/dekan/kepala_unit)
// pada sebuah unit; climb -> naik ke unit induk bila belum ketemu, sampai akar pohon.
func (h *EmployeeHandler) ResolveRole(c *gin.Context) {
	unitID := c.Query("unit_id")
	position := c.Query("position")
	climb := c.Query("climb") == "1" || c.Query("climb") == "true"
	if unitID == "" || position == "" || h.db == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "unit_id & position wajib"}})
		return
	}
	visited := map[string]bool{}
	current := unitID
	for current != "" && !visited[current] {
		visited[current] = true
		var holds []struct {
			EmployeeID         string
			StructuralPosition string
		}
		h.db.Table("employee_unit_assignments").
			Where("unit_id = ? AND (valid_to IS NULL OR valid_to > now()) AND structural_position = ?", current, position).
			Find(&holds)
		for _, hd := range holds {
			var emp entity.Employee
			if err := h.db.Select("id, user_id, full_name").Where("id = ? AND status = ?", hd.EmployeeID, "active").First(&emp).Error; err != nil || emp.UserID == nil {
				continue
			}
			deptCode := ""
			var dept entity.Department
			if err := h.db.Select("code").Where("id = ?", current).First(&dept).Error; err == nil {
				deptCode = dept.Code
			}
			response.Success(c, gin.H{"user_id": *emp.UserID, "employee_id": hd.EmployeeID, "full_name": emp.FullName, "unit_id": current, "unit_code": deptCode, "position": position})
			return
		}
		if !climb {
			break
		}
		var dept entity.Department
		err := h.db.Select("parent_id").Where("id = ?", current).First(&dept).Error
		if err != nil || dept.ParentID == nil {
			break
		}
		current = *dept.ParentID
	}
	response.Success(c, gin.H{"user_id": nil, "reason": "not_found", "position": position})
}

// DirectSupervisor GET /api/org/direct-supervisor?user_id= — HC-3 (helpdesk):
// atasan langsung (employees.reports_to) sebagai SSO user_id.
// OnLeave - AP-06: apakah user sedang cuti disetujui hari ini (helpdesk delegasi approval).
func (h *EmployeeHandler) OnLeave(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" || h.db == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "user_id wajib"}})
		return
	}
	var emp entity.Employee
	if err := h.db.Select("id").Where("user_id = ?", userID).First(&emp).Error; err != nil {
		response.Success(c, gin.H{"on_leave": false, "reason": "no_employee"})
		return
	}
	var n int64
	h.db.Table("leave_requests").Where("employee_id = ? AND status = ? AND start_date <= CURRENT_DATE AND end_date >= CURRENT_DATE", emp.ID, "approved").Count(&n)
	response.Success(c, gin.H{"on_leave": n > 0})
}

func (h *EmployeeHandler) DirectSupervisor(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" || h.db == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "user_id wajib"}})
		return
	}
	var emp entity.Employee
	if err := h.db.Select("id, reports_to").Where("user_id = ?", userID).First(&emp).Error; err != nil || emp.ReportsTo == nil {
		response.Success(c, gin.H{"user_id": nil, "reason": "no_supervisor"})
		return
	}
	var sup entity.Employee
	if err := h.db.Select("id, user_id, full_name").Where("id = ? AND status = ?", *emp.ReportsTo, "active").First(&sup).Error; err != nil || sup.UserID == nil {
		response.Success(c, gin.H{"user_id": nil, "reason": "supervisor_inactive_or_no_account"})
		return
	}
	response.Success(c, gin.H{"user_id": *sup.UserID, "employee_id": sup.ID, "full_name": sup.FullName})
}
