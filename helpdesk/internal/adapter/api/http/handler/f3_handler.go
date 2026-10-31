package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"github.com/rama/b-wise/helpdesk/internal/service/ticketsvc"
)

// ===== Otomatisasi tim (KT-10) =====

// AutomationRules - daftar aturan tim user.
func (h *TicketHandler) AutomationRules(c *gin.Context) {
	list, err := h.svc.RulesOfTeam(ctxUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": list}})
}

// AutomationRuleSave - buat/ubah aturan (body = entity.AutomationRule).
func (h *TicketHandler) AutomationRuleSave(c *gin.Context) {
	var r entity.AutomationRule
	if err := c.ShouldBindJSON(&r); err != nil {
		fail(c, err)
		return
	}
	r.ID = c.Param("id")
	out, err := h.svc.SaveRule(ctxUserID(c), r)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// AutomationRuleDelete - hapus aturan.
func (h *TicketHandler) AutomationRuleDelete(c *gin.Context) {
	if err := h.svc.DeleteRule(ctxUserID(c), c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ===== Insiden massal (AG-12) =====

// MakeIncidentH - angkat tiket jadi induk insiden.
func (h *TicketHandler) MakeIncidentH(c *gin.Context) {
	t, err := h.svc.MakeIncident(c.Param("id"), ctxUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

// AttachChildH - tautkan tiket anak ke insiden.
func (h *TicketHandler) AttachChildH(c *gin.Context) {
	var req struct {
		ChildID string `json:"child_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	t, err := h.svc.AttachChild(c.Param("id"), req.ChildID, ctxUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

// IncidentChildrenH - daftar anak insiden (viewer induk).
func (h *TicketHandler) IncidentChildrenH(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.svc.GetForViewer(id, ctxUserID(c)); err != nil {
		fail(c, err)
		return
	}
	list, err := h.svc.IncidentChildren(id)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": list}})
}

// ===== API internal service-to-service (KN-08) =====

// InternalCreate - service B-Wise lain (token service) membuat tiket.
func (h *TicketHandler) InternalCreate(c *gin.Context) {
	if v, ok := c.Get("is_service_token"); !ok || v != true {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"message": "hanya token service (KN-08)"}})
		return
	}
	var req struct {
		ServiceID   string `json:"service_id"`
		Subject     string `json:"subject" binding:"required"`
		RequesterID string `json:"requester_id" binding:"required"`
		Answers     string `json:"answers"`
		RoomRef     string `json:"room_ref"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	t, err := h.svc.Create(ticketsvc.CreateParams{
		RequesterID: req.RequesterID,
		ServiceID:   req.ServiceID,
		Subject:     req.Subject,
		Answers:     req.Answers,
		Channel:     "api",
		RoomRef:     req.RoomRef,
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": t})
}
