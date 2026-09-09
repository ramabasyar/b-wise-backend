package handler

import (
	"fmt"
	"github.com/rama/b-wise/permission-service/internal/adapter/persistence/postgres"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/permission-service/internal/domain/dto/mapper"
	"github.com/rama/b-wise/permission-service/internal/domain/dto/request"
	"github.com/rama/b-wise/permission-service/internal/domain/dto/response"
	"github.com/rama/b-wise/permission-service/internal/domain/entity"
	"github.com/rama/b-wise/permission-service/internal/domain/service"
)

type PermissionHandler struct {
	svc *service.PermissionService
}

func NewPermissionHandler(svc *service.PermissionService) *PermissionHandler {
	return &PermissionHandler{svc: svc}
}

// ==================== RESPONSE HELPERS ====================

func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func okWithMeta(c *gin.Context, data interface{}, meta *response.Pagination) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data, "meta": meta})
}

func okMsg(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": msg})
}

func created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": data})
}

func badRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": msg})
}

func notFound(c *gin.Context, msg string) {
	c.JSON(http.StatusNotFound, gin.H{"success": false, "error": msg})
}

func internalError(c *gin.Context, msg string) {
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": msg})
}

func conflict(c *gin.Context, msg string) {
	c.JSON(http.StatusConflict, gin.H{"success": false, "error": msg})
}

// ==================== SERVICE ENDPOINTS ====================

func (h *PermissionHandler) CreateService(c *gin.Context) {
	var req request.CreateService
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}

	userID, _ := c.Get("user_id")
	svc := mapper.ToServiceEntity(&req, toString(userID))

	if err := h.svc.CreateService(svc); err != nil {
		conflict(c, err.Error())
		return
	}

	created(c, mapper.ToServiceResponse(svc))
}

func (h *PermissionHandler) ListServices(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	services, total, err := h.svc.ListServices(page, pageSize)
	if err != nil {
		internalError(c, err.Error())
		return
	}

	okWithMeta(c, mapper.ToServiceList(services), &response.Pagination{
		Page: page, PageSize: pageSize, Total: total,
	})
}

func (h *PermissionHandler) GetService(c *gin.Context) {
	svc, err := h.svc.GetService(c.Param("id"))
	if err != nil {
		notFound(c, err.Error())
		return
	}
	ok(c, mapper.ToServiceResponse(svc))
}

func (h *PermissionHandler) UpdateService(c *gin.Context) {
	id := c.Param("id")
	var req request.UpdateService
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}

	svc, err := h.svc.GetService(id)
	if err != nil {
		notFound(c, err.Error())
		return
	}

	userID, _ := c.Get("user_id")
	mapper.ApplyServiceUpdate(&req, svc, toString(userID))

	if err := h.svc.UpdateService(svc); err != nil {
		internalError(c, err.Error())
		return
	}

	ok(c, mapper.ToServiceResponse(svc))
}

func (h *PermissionHandler) DeleteService(c *gin.Context) {
	if err := h.svc.DeleteService(c.Param("id")); err != nil {
		notFound(c, err.Error())
		return
	}
	okMsg(c, "service deleted")
}

func (h *PermissionHandler) ActivateService(c *gin.Context) {
	if err := h.svc.ActivateService(c.Param("id")); err != nil {
		notFound(c, err.Error())
		return
	}
	okMsg(c, "service activated")
}

func (h *PermissionHandler) DeactivateService(c *gin.Context) {
	if err := h.svc.DeactivateService(c.Param("id")); err != nil {
		notFound(c, err.Error())
		return
	}
	okMsg(c, "service deactivated")
}

// ==================== PERMISSION ENDPOINTS ====================

func (h *PermissionHandler) CreatePermission(c *gin.Context) {
	serviceID := c.Param("id")
	var req request.CreatePermission
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}

	userID, _ := c.Get("user_id")
	perm := &entity.ServicePermission{
		ServiceID:   serviceID,
		Permission:  req.Permission,
		Description: req.Description,
		CreatedBy:   toString(userID),
	}

	if err := h.svc.CreatePermission(perm); err != nil {
		if err == service.ErrServiceNotFound {
			notFound(c, err.Error())
			return
		}
		internalError(c, err.Error())
		return
	}

	created(c, mapper.ToPermissionResponse(perm))
}

func (h *PermissionHandler) ListPermissions(c *gin.Context) {
	perms, err := h.svc.ListPermissions(c.Param("id"))
	if err != nil {
		notFound(c, err.Error())
		return
	}
	ok(c, mapper.ToPermissionList(perms))
}

func (h *PermissionHandler) DeletePermission(c *gin.Context) {
	if err := h.svc.DeletePermission(c.Param("id")); err != nil {
		notFound(c, err.Error())
		return
	}
	okMsg(c, "permission deleted")
}

func (h *PermissionHandler) SeedPermissions(c *gin.Context) {
	serviceID := c.Param("id")
	var req request.SeedPermissions
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}

	userID, _ := c.Get("user_id")
	if err := h.svc.SeedServicePermissions(serviceID, toString(userID), req.Permissions); err != nil {
		internalError(c, err.Error())
		return
	}

	okMsg(c, "permissions seeded")
}

// ==================== ACCESS ENDPOINTS ====================

func (h *PermissionHandler) GrantAccess(c *gin.Context) {
	serviceID := c.Param("id")
	var req request.GrantAccess
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}

	userID, _ := c.Get("user_id")
	access := &entity.ServiceAccess{
		UserID:    req.UserID,
		ServiceID: serviceID,
		GrantedBy: toString(userID),
		IsActive:  true,
	}

	if err := h.svc.GrantAccess(access); err != nil {
		if err == service.ErrServiceNotFound {
			notFound(c, err.Error())
			return
		}
		internalError(c, err.Error())
		return
	}

	created(c, mapper.ToAccessResponse(access))
}

func (h *PermissionHandler) RevokeAccess(c *gin.Context) {
	userID, _ := c.Get("user_id")
	if err := h.svc.RevokeAccess(c.Param("id"), toString(userID)); err != nil {
		notFound(c, err.Error())
		return
	}
	okMsg(c, "access revoked")
}

func (h *PermissionHandler) GetUserServices(c *gin.Context) {
	accesses, err := h.svc.GetUserServices(c.Param("user_id"))
	if err != nil {
		internalError(c, err.Error())
		return
	}
	ok(c, mapper.ToAccessList(accesses))
}

func (h *PermissionHandler) CheckAccess(c *gin.Context) {
	hasAccess, err := h.svc.CheckAccess(c.Param("user_id"), c.Param("service_id"))
	if err != nil {
		internalError(c, err.Error())
		return
	}
	ok(c, response.AccessCheck{
		HasAccess: hasAccess,
		UserID:    c.Param("user_id"),
		ServiceID: c.Param("service_id"),
	})
}

// ==================== USER PERMISSION ENDPOINTS ====================

func (h *PermissionHandler) GrantUserPermission(c *gin.Context) {
	userID := c.Param("user_id")
	var req request.GrantUserPermission
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, err.Error())
		return
	}

	grantedBy, _ := c.Get("user_id")
	perm := &entity.UserPermission{
		UserID:     userID,
		Permission: req.Permission,
		ServiceID:  req.ServiceID,
		GrantedBy:  toString(grantedBy),
		IsActive:   true,
	}

	if err := h.svc.GrantUserPermission(perm); err != nil {
		if err == service.ErrNoAccess || err == service.ErrServiceNotFound {
			badRequest(c, err.Error())
			return
		}
		internalError(c, err.Error())
		return
	}

	created(c, mapper.ToUserPermissionResponse(perm))
}

func (h *PermissionHandler) RevokeUserPermission(c *gin.Context) {
	userID, _ := c.Get("user_id")
	if err := h.svc.RevokeUserPermission(c.Param("id"), toString(userID)); err != nil {
		notFound(c, err.Error())
		return
	}
	okMsg(c, "permission revoked")
}

func (h *PermissionHandler) GetUserPermissions(c *gin.Context) {
	perms, err := h.svc.GetUserPermissions(c.Param("user_id"))
	if err != nil {
		internalError(c, err.Error())
		return
	}
	ok(c, mapper.ToUserPermissionList(perms))
}

func (h *PermissionHandler) GetUserPermissionsByService(c *gin.Context) {
	perms, err := h.svc.GetUserPermissionsByService(c.Param("user_id"), c.Param("service_id"))
	if err != nil {
		internalError(c, err.Error())
		return
	}
	ok(c, mapper.ToUserPermissionList(perms))
}

func (h *PermissionHandler) HasPermission(c *gin.Context) {
	has, err := h.svc.HasPermission(
		c.Param("user_id"),
		c.Query("permission"),
		c.Query("service_id"),
	)
	if err != nil {
		internalError(c, err.Error())
		return
	}
	ok(c, response.PermissionCheck{
		HasPermission: has,
		Permission:    c.Query("permission"),
		ServiceID:     c.Query("service_id"),
	})
}

func (h *PermissionHandler) GetUserMenu(c *gin.Context) {
	menu, err := h.svc.GetUserMenu(c.Param("user_id"))
	if err != nil {
		internalError(c, err.Error())
		return
	}
	ok(c, menu)
}

// HasPermissionBatch checks multiple permissions in one call.
// Body: {"service_id": "...", "permissions": ["a", "b"]}
func (h *PermissionHandler) HasPermissionBatch(c *gin.Context) {
	var req struct {
		ServiceID   string   `json:"service_id" binding:"required"`
		Permissions []string `json:"permissions" binding:"required,min=1,max=100"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	results, err := h.svc.HasPermissionsBatch(c.Param("user_id"), req.ServiceID, req.Permissions)
	if err != nil {
		internalError(c, err.Error())
		return
	}
	ok(c, gin.H{"service_id": req.ServiceID, "results": results})
}

// ListAuditLogs lists assignment audit trail with filters
func (h *PermissionHandler) ListAuditLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	logs, total, err := h.svc.ListAuditLogs(postgres.AuditFilter{
		UserID:    c.Query("user_id"),
		Action:    c.Query("action"),
		ServiceID: c.Query("service_id"),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		internalError(c, err.Error())
		return
	}
	page := 1
	pageSize := limit
	if pageSize > 0 && offset > 0 {
		page = offset/pageSize + 1
	}
	okWithMeta(c, logs, &response.Pagination{Page: page, PageSize: pageSize, Total: total})
}

// SelfSync menerima deklarasi manifest dari service itu sendiri.
// Hanya untuk service token: service_name di JWT harus sama dengan service.name di body.
func (h *PermissionHandler) SelfSync(c *gin.Context) {
	isSvc, _ := c.Get("is_service_token")
	if ok, _ := isSvc.(bool); !ok {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"message": "self-sync requires service token"}})
		return
	}

	var req service.ManifestInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}

	// identity check: JWT service_name == manifest service.name
	jwtSvcName := ""
	if v, exists := c.Get("service_name"); exists {
		jwtSvcName, _ = v.(string)
	}
	if jwtSvcName == "" || !strings.EqualFold(strings.TrimSpace(jwtSvcName), strings.TrimSpace(req.Service.Name)) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{
			"message": fmt.Sprintf("service token (%q) cannot sync manifest of %q", jwtSvcName, req.Service.Name),
		}})
		return
	}

	summary, err := h.svc.SyncManifest(&req)
	if err != nil {
		internalError(c, err.Error())
		return
	}
	ok(c, gin.H{"service": req.Service.Name, "summary": summary})
}

// ==================== HELPER ====================

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// ==================== ROLE HANDLERS ====================

// CreateRoleRequest represents a role creation request
type CreateRoleRequest struct {
	Name        string   `json:"name" binding:"required"`
	ServiceID   string   `json:"service_id" binding:"required"`
	Description string   `json:"description"`
	IsDefault   bool     `json:"is_default"`
	Permissions []string `json:"permission_ids"` // permission IDs to attach
}

// UpdateRoleRequest represents a role update request
type UpdateRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	IsDefault   bool     `json:"is_default"`
	Permissions []string `json:"permission_ids"`
}

// AssignRoleRequest represents a role assignment request
type AssignRoleRequest struct {
	UserID string `json:"user_id" binding:"required"`
	RoleID string `json:"role_id" binding:"required"`
}

// CreateRole creates a new role
func (h *PermissionHandler) CreateRole(c *gin.Context) {
	var req CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}

	userID := getUserIDVal(c)
	role := &entity.Role{
		Name:        req.Name,
		ServiceID:   req.ServiceID,
		Description: req.Description,
		IsDefault:   req.IsDefault,
		CreatedBy:   userID,
		UpdatedBy:   userID,
	}

	if err := h.svc.CreateRole(role); err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}

	// Set permissions if provided
	if len(req.Permissions) > 0 {
		_ = h.svc.SetRolePermissions(role.ID, req.Permissions)
	}

	// Reload with relations
	role, _ = h.svc.GetRole(role.ID)
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": role})
}

// GetRole gets a role by ID
func (h *PermissionHandler) GetRole(c *gin.Context) {
	id := c.Param("id")
	role, err := h.svc.GetRole(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"message": "role not found"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": role})
}

// ListRoles lists all roles
func (h *PermissionHandler) ListRoles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// Filter by service_id if provided
	if serviceID := c.Query("service_id"); serviceID != "" {
		roles, err := h.svc.ListRolesByService(serviceID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": roles})
		return
	}

	roles, total, err := h.svc.ListRoles(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":     roles,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// UpdateRole updates a role
func (h *PermissionHandler) UpdateRole(c *gin.Context) {
	id := c.Param("id")
	var req UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}

	role, err := h.svc.GetRole(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"message": "role not found"}})
		return
	}

	if req.Name != "" {
		role.Name = req.Name
	}
	role.Description = req.Description
	role.IsDefault = req.IsDefault
	role.UpdatedBy = getUserIDVal(c)

	if err := h.svc.UpdateRole(role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}

	// Update permissions if provided
	if req.Permissions != nil {
		_ = h.svc.SetRolePermissions(role.ID, req.Permissions)
	}

	role, _ = h.svc.GetRole(role.ID)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": role})
}

// DeleteRole deletes a role
func (h *PermissionHandler) DeleteRole(c *gin.Context) {
	id := c.Param("id")
	if err := h.svc.DeleteRole(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"message": "role not found"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "role deleted"}})
}

// AssignRole assigns a role to a user
func (h *PermissionHandler) AssignRole(c *gin.Context) {
	var req AssignRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}

	userID := getUserIDVal(c)
	if err := h.svc.AssignRoleToUser(req.UserID, req.RoleID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "role assigned"}})
}

// RevokeRole removes a role from a user
func (h *PermissionHandler) RevokeRole(c *gin.Context) {
	userID := c.Param("user_id")
	roleID := c.Param("role_id")
	actor := getUserIDVal(c)
	if err := h.svc.RevokeRoleFromUser(userID, roleID, actor); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "role revoked"}})
}

// GetUserRoles gets all roles for a user
func (h *PermissionHandler) GetUserRoles(c *gin.Context) {
	userID := c.Param("user_id")
	roles, err := h.svc.GetUserRoles(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": roles})
}

// getUserIDVal extracts user_id from gin context
func getUserIDVal(c *gin.Context) string {
	if v, ok := c.Get("user_id"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ==================== MENU ITEMS ====================

// CreateMenuItem godoc
// @Summary Tambah menu item untuk service
func (h *PermissionHandler) CreateMenuItem(c *gin.Context) {
	var item entity.MenuItem
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if item.ServiceID == "" || item.Label == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "service_id dan label wajib"})
		return
	}
	if err := h.svc.CreateMenuItem(&item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": item})
}

// ListMenuItems godoc — by service_id query, atau semua
func (h *PermissionHandler) ListMenuItems(c *gin.Context) {
	serviceID := c.Query("service_id")
	var (
		items []entity.MenuItem
		err   error
	)
	if serviceID != "" {
		items, err = h.svc.ListMenuItemsByService(serviceID)
	} else {
		items, err = h.svc.ListAllMenuItems()
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

// UpdateMenuItem godoc
func (h *PermissionHandler) UpdateMenuItem(c *gin.Context) {
	var item entity.MenuItem
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item.ID = c.Param("id")
	if err := h.svc.UpdateMenuItem(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

// DeleteMenuItem godoc
func (h *PermissionHandler) DeleteMenuItem(c *gin.Context) {
	if err := h.svc.DeleteMenuItem(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
