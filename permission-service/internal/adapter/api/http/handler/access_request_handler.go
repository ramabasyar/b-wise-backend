package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/permission-service/internal/domain/entity"
	"gorm.io/gorm"
)

// AccessRequestHandler — Fase B: pengajuan & approval akses.
// Approver di-resolve via HC Org API (forward JWT user — token RS256 sama valid di HC).
type AccessRequestHandler struct {
	db    *gorm.DB
	svc   PermissionServiceIface // subset: AssignRoleToUser + audit
	hcURL string
	httpC *http.Client
}

// PermissionServiceIface — subset PermissionService yang dibutuhkan (hindari import cycle penuh).
type PermissionServiceIface interface {
	AssignRoleToUser(userID, roleID, grantedBy string) error
}

func NewAccessRequestHandler(db *gorm.DB, svc PermissionServiceIface, hcURL string) *AccessRequestHandler {
	return &AccessRequestHandler{db: db, svc: svc, hcURL: hcURL, httpC: &http.Client{Timeout: 10 * time.Second}}
}

type createAccessRequestReq struct {
	UserID        string `json:"user_id" binding:"omitempty"` // kosong = diri sendiri
	RoleID        string `json:"role_id" binding:"required"`
	Justification string `json:"justification" binding:"required,min=5"`
}

// resolveApprover — panggil HC org API dgn mem-forward JWT caller.
func (h *AccessRequestHandler) resolveApprover(authHeader, userID, roleServiceID string) (string, error) {
	url := fmt.Sprintf("%s/api/org/resolve-approver?user_id=%s&service_id=%s", h.hcURL, userID, roleServiceID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", authHeader)
	resp, err := h.httpC.Do(req)
	if err != nil {
		return "", fmt.Errorf("HC org API: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HC org API HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 120)]))
	}
	var out struct {
		Success bool `json:"success"`
		Data struct {
			ApproverUserID *string `json:"approver_user_id"`
			Reason         string  `json:"reason"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if out.Data.ApproverUserID == nil {
		return "", nil // tanpa approver → pending, admin HC putuskan
	}
	return *out.Data.ApproverUserID, nil
}

// Create POST /api/access-requests
func (h *AccessRequestHandler) Create(c *gin.Context) {
	var req createAccessRequestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	caller := getUserIDVal(c)
	target := req.UserID
	if target == "" {
		target = caller
	}

	// role harus valid & aktif
	var role entity.Role
	if err := h.db.Preload("Service").Where("id = ?", req.RoleID).First(&role).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "role tidak ditemukan/tidak aktif"}})
		return
	}
	// jangan dobel: masih pending utk user+role sama?
	var dup int64
	h.db.Model(&entity.AccessRequest{}).Where("user_id = ? AND role_id = ? AND status = ?", target, req.RoleID, "pending").Count(&dup)
	if dup > 0 {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{"message": "sudah ada pengajuan pending untuk role ini"}})
		return
	}

	approver, rErr := h.resolveApprover(c.GetHeader("Authorization"), target, role.ServiceID)
	if rErr != nil {
		// jangan gagalkan pengajuan karena resolusi error — catat pending tanpa approver
		approver = ""
	}

	ar := &entity.AccessRequest{
		UserID: target, RequestedBy: caller, RoleID: req.RoleID,
		Justification: req.Justification, Status: "pending",
	}
	if approver != "" {
		ar.ApproverID = &approver
	}
	if err := h.db.Create(ar).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	msg := "pengajuan dibuat"
	if approver == "" {
		msg = "pengajuan dibuat — menunggu admin HC (belum ada approver otomatis utk unit ini)"
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"id": ar.ID, "message": msg, "approver_id": ar.ApproverID}})
}

// Mine GET /api/access-requests/mine — pengajuan yang kulakukan / utk-ku.
func (h *AccessRequestHandler) Mine(c *gin.Context) {
	me := getUserIDVal(c)
	var rows []entity.AccessRequest
	h.db.Where("requested_by = ? OR user_id = ?", me, me).Order("created_at DESC").Limit(100).Find(&rows)
	h.decorate(c, rows)
}

// Pending GET /api/access-requests/pending — antrean approval utk caller.
func (h *AccessRequestHandler) Pending(c *gin.Context) {
	me := getUserIDVal(c)
	var rows []entity.AccessRequest
	// approver = saya; ATAU tanpa approver (admin HC dgn permission decide-any boleh lihat)
	h.db.Where("status = ? AND (approver_id = ? OR approver_id IS NULL)", "pending", me).
		Order("created_at ASC").Limit(100).Find(&rows)
	h.decorate(c, rows)
}

// decorate — tambah info role & nama service utk display.
func (h *AccessRequestHandler) decorate(c *gin.Context, rows []entity.AccessRequest) {
	type item struct {
		entity.AccessRequest
		RoleName    string `json:"role_name"`
		ServiceName string `json:"service_name"`
	}
	out := make([]item, 0, len(rows))
	for _, r := range rows {
		it := item{AccessRequest: r}
		var role entity.Role
		if err := h.db.Preload("Service").Where("id = ?", r.RoleID).First(&role).Error; err == nil {
			it.RoleName = role.Name
			if role.Service != nil {
				it.ServiceName = role.Service.Name
			}
		}
		out = append(out, it)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

type decideReq struct {
	Action string `json:"action" binding:"required,oneof=approve reject"`
	Notes  string `json:"notes"`
}

// Decide POST /api/access-requests/:id/decide — approver (atau admin decide-any) memutuskan.
func (h *AccessRequestHandler) Decide(c *gin.Context) {
	id := c.Param("id")
	me := getUserIDVal(c)
	var req decideReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}

	var ar entity.AccessRequest
	if err := h.db.Where("id = ?", id).First(&ar).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"message": "pengajuan tidak ditemukan"}})
		return
	}
	if ar.Status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{"message": "pengajuan sudah diputuskan"}})
		return
	}
	// otorisasi: approver resolved, ATAU tanpa approver & caller adalah pengaju HC admin
	// (decide-any enforcement via permission middleware di route — lihat router)
	isApprover := ar.ApproverID != nil && *ar.ApproverID == me
	if !isApprover && ar.ApproverID != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"message": "bukan approver pengajuan ini"}})
		return
	}

	now := time.Now()
	ar.Status = map[string]string{"approve": "approved", "reject": "rejected"}[req.Action]
	ar.DecidedBy = &me
	ar.DecidedAt = &now
	ar.Notes = req.Notes
	if err := h.db.Save(&ar).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}

	if req.Action == "approve" {
		if err := h.svc.AssignRoleToUser(ar.UserID, ar.RoleID, me); err != nil {
			// role gagal di-assign — pengajuan tetap approved utk audit; beri info
			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "disetujui, TAPI assign role gagal: " + err.Error()}})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "ok"}})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
