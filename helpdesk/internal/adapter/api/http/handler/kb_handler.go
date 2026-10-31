package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// KBSearch - cari artikel terbit; q kosong = terbaru (KB-02; semua akun login).
func (h *TicketHandler) KBSearch(c *gin.Context) {
	list, err := h.svc.KBSearch(c.Query("q"), c.Query("team"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": list}})
}

// KBGet - baca artikel (terbit utk semua; draft utk anggota tim pemilik).
func (h *TicketHandler) KBGet(c *gin.Context) {
	a, err := h.svc.KBGetForViewer(c.Param("id"), ctxUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": a})
}

// KBManageList - artikel tim user termasuk draft (KT-09).
func (h *TicketHandler) KBManageList(c *gin.Context) {
	list, err := h.svc.KBListTeam(ctxUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": list}})
}

// KBCreate - draft artikel baru (KT-09).
func (h *TicketHandler) KBCreate(c *gin.Context) {
	var req struct {
		Title string `json:"title" binding:"required"`
		Body  string `json:"body"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	a, err := h.svc.KBCreate(ctxUserID(c), req.Title, req.Body)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": a})
}

// KBUpdate - ubah/terbitkan artikel (field whitelist di service).
func (h *TicketHandler) KBUpdate(c *gin.Context) {
	var fields map[string]interface{}
	if err := c.ShouldBindJSON(&fields); err != nil {
		fail(c, err)
		return
	}
	a, err := h.svc.KBUpdate(ctxUserID(c), c.Param("id"), fields)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": a})
}

// KBFeedback - kirim umpan balik membantu/tidak (KB-05).
func (h *TicketHandler) KBFeedback(c *gin.Context) {
	var req struct {
		Helpful *bool `json:"helpful" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	a, err := h.svc.KBFeedback(ctxUserID(c), c.Param("id"), *req.Helpful)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": a})
}

// KBFromTicket - jawaban tiket dijadikan draft artikel (KB-04).
func (h *TicketHandler) KBFromTicket(c *gin.Context) {
	var req struct {
		TicketID string `json:"ticket_id" binding:"required"`
		EventID  string `json:"event_id" binding:"required"`
		Title    string `json:"title"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	a, err := h.svc.KBFromTicket(ctxUserID(c), req.TicketID, req.EventID, req.Title)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": a})
}
