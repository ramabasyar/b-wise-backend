package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/human-capital/internal/adapter/api/http/middleware"
	entity "github.com/rama/b-wise/human-capital/internal/domain/entity"
	service "github.com/rama/b-wise/human-capital/internal/domain/service"
	"github.com/rama/b-wise/human-capital/pkg/response"
)

// OrgHandler — master org: departments (tree), designations, employment types, grades, branches.
// Pola generik: satu handler, resource ditentukan route.
type OrgHandler struct {
	svc *service.OrgService
}

func NewOrgHandler(svc *service.OrgService) *OrgHandler { return &OrgHandler{svc: svc} }

func boolQuery(c *gin.Context, key string, def bool) bool {
	v := c.Query(key)
	if v == "" {
		return def
	}
	return v == "true" || v == "1"
}

// ==================== DEPARTMENTS ====================

func (h *OrgHandler) ListDepartments(c *gin.Context) {
	if boolQuery(c, "tree", false) {
		tree, err := h.svc.DepartmentTree(boolQuery(c, "active_only", false))
		if err != nil {
			errStatus(c, err)
			return
		}
		response.Success(c, tree)
		return
	}
	list, err := h.svc.ListDepartments(boolQuery(c, "active_only", false))
	if err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, list)
}

func (h *OrgHandler) CreateDepartment(c *gin.Context) {
	var d entity.Department
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	d.CreatedBy, d.UpdatedBy = middleware.GetUserID(c), middleware.GetUserID(c)
	if err := h.svc.CreateDepartment(&d); err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, d)
}

// updateDepartmentRequest - DTO pointer: hanya field terkirim yang diubah
// (fix bug PUT partial: zero-value menimpa is_active dsb. diam-diam).
type updateDepartmentRequest struct {
	Name           *string `json:"name"`
	Description    *string `json:"description"`
	ParentID       *string `json:"parent_id"`
	OrgType        *string `json:"org_type"`
	IsActive       *bool   `json:"is_active"`
	HeadEmployeeID *string `json:"head_employee_id"`
}

func (h *OrgHandler) UpdateDepartment(c *gin.Context) {
	var req updateDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	fields := map[string]interface{}{}
	if req.Name != nil {
		fields["name"] = *req.Name
	}
	if req.Description != nil {
		fields["description"] = *req.Description
	}
	if req.ParentID != nil {
		fields["parent_id"] = *req.ParentID
	}
	if req.OrgType != nil {
		fields["org_type"] = *req.OrgType
	}
	if req.IsActive != nil {
		fields["is_active"] = *req.IsActive
	}
	if req.HeadEmployeeID != nil {
		fields["head_employee_id"] = *req.HeadEmployeeID
	}
	if len(fields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "tidak ada field yang diubah"}})
		return
	}
	d, err := h.svc.UpdateDepartmentFields(c.Param("id"), fields, middleware.GetUserID(c))
	if err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, d)
}

func (h *OrgHandler) DeleteDepartment(c *gin.Context) {
	if err := h.svc.DeleteDepartment(c.Param("id")); err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "department dihapus"})
}

// ==================== DESIGNATIONS ====================

func (h *OrgHandler) ListDesignations(c *gin.Context) {
	list, err := h.svc.ListDesignations(boolQuery(c, "active_only", false))
	if err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, list)
}

func (h *OrgHandler) CreateDesignation(c *gin.Context) {
	var d entity.Designation
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	d.CreatedBy, d.UpdatedBy = middleware.GetUserID(c), middleware.GetUserID(c)
	if err := h.svc.CreateDesignation(&d); err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, d)
}

type updateDesignationRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Level       *int    `json:"level"`
	IsActive    *bool   `json:"is_active"`
}

func (h *OrgHandler) UpdateDesignation(c *gin.Context) {
	var req updateDesignationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	fields := map[string]interface{}{}
	if req.Name != nil {
		fields["name"] = *req.Name
	}
	if req.Description != nil {
		fields["description"] = *req.Description
	}
	if req.Level != nil {
		fields["level"] = *req.Level
	}
	if req.IsActive != nil {
		fields["is_active"] = *req.IsActive
	}
	if len(fields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "tidak ada field yang diubah"}})
		return
	}
	d, err := h.svc.UpdateDesignationFields(c.Param("id"), fields, middleware.GetUserID(c))
	if err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, d)
}

func (h *OrgHandler) DeleteDesignation(c *gin.Context) {
	if err := h.svc.DeleteDesignation(c.Param("id")); err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "designation dihapus"})
}

// ==================== EMPLOYMENT TYPES ====================

func (h *OrgHandler) ListEmploymentTypes(c *gin.Context) {
	list, err := h.svc.ListEmploymentTypes(boolQuery(c, "active_only", false))
	if err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, list)
}

func (h *OrgHandler) CreateEmploymentType(c *gin.Context) {
	var t entity.EmploymentType
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	t.CreatedBy, t.UpdatedBy = middleware.GetUserID(c), middleware.GetUserID(c)
	if err := h.svc.CreateEmploymentType(&t); err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, t)
}

func (h *OrgHandler) UpdateEmploymentType(c *gin.Context) {
	var t entity.EmploymentType
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	t.ID = c.Param("id")
	t.UpdatedBy = middleware.GetUserID(c)
	if err := h.svc.UpdateEmploymentType(&t); err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, t)
}

func (h *OrgHandler) DeleteEmploymentType(c *gin.Context) {
	if err := h.svc.DeleteEmploymentType(c.Param("id")); err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "employment type dihapus"})
}

// ==================== GRADES ====================

func (h *OrgHandler) ListGrades(c *gin.Context) {
	list, err := h.svc.ListGrades(boolQuery(c, "active_only", false))
	if err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, list)
}

func (h *OrgHandler) CreateGrade(c *gin.Context) {
	var g entity.Grade
	if err := c.ShouldBindJSON(&g); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	g.CreatedBy, g.UpdatedBy = middleware.GetUserID(c), middleware.GetUserID(c)
	if err := h.svc.CreateGrade(&g); err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, g)
}

func (h *OrgHandler) UpdateGrade(c *gin.Context) {
	var g entity.Grade
	if err := c.ShouldBindJSON(&g); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	g.ID = c.Param("id")
	g.UpdatedBy = middleware.GetUserID(c)
	if err := h.svc.UpdateGrade(&g); err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, g)
}

func (h *OrgHandler) DeleteGrade(c *gin.Context) {
	if err := h.svc.DeleteGrade(c.Param("id")); err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "grade dihapus"})
}

// ==================== BRANCHES ====================

func (h *OrgHandler) ListBranches(c *gin.Context) {
	list, err := h.svc.ListBranches(boolQuery(c, "active_only", false))
	if err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, list)
}

func (h *OrgHandler) CreateBranch(c *gin.Context) {
	var b entity.Branch
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	b.CreatedBy, b.UpdatedBy = middleware.GetUserID(c), middleware.GetUserID(c)
	if err := h.svc.CreateBranch(&b); err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, b)
}

func (h *OrgHandler) UpdateBranch(c *gin.Context) {
	var b entity.Branch
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	b.ID = c.Param("id")
	b.UpdatedBy = middleware.GetUserID(c)
	if err := h.svc.UpdateBranch(&b); err != nil {
		errStatus(c, err)
		return
	}
	response.Success(c, b)
}

func (h *OrgHandler) DeleteBranch(c *gin.Context) {
	if err := h.svc.DeleteBranch(c.Param("id")); err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "branch dihapus"})
}
