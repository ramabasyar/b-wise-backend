package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"github.com/rama/b-wise/human-capital/internal/adapter/api/http/middleware"
	"github.com/rama/b-wise/human-capital/internal/domain/entity"
	repository "github.com/rama/b-wise/human-capital/internal/domain/repository"
	service "github.com/rama/b-wise/human-capital/internal/domain/service"
	"github.com/rama/b-wise/human-capital/internal/service/onboarding"
	"github.com/rama/b-wise/human-capital/pkg/response"
)

// EmployeeHandler — employees + movements + onboarding
type EmployeeHandler struct {
	svc   *service.EmployeeService
	movSvc *service.MovementService
	onboard *onboarding.OnboardingService
	db *gorm.DB
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
		EmployeeID string
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
