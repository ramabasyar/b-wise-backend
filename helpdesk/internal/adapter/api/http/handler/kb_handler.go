package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"github.com/rama/b-wise/helpdesk/internal/service/ticketsvc"
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
	var (
		list []entity.KBArticle
		err  error
	)
	if h.isSuper(ctxUserID(c)) {
		list, err = h.svc.KBListAll()
	} else {
		list, err = h.svc.KBListTeam(ctxUserID(c))
	}
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
		CollectionID string `json:"collection_id"`
		ParentID     string `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	a, err := h.svc.KBCreate(ctxUserID(c), req.Title, req.Body, req.CollectionID, req.ParentID)
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

// ===== Fase 2 wiki: koleksi & revisi =====

func (h *TicketHandler) KBCollectionsList(c *gin.Context) {
	teamID := ""
	if !h.isSuper(ctxUserID(c)) {
		team, err := h.svc.TeamOf(ctxUserID(c))
		if err != nil { fail(c, err); return }
		teamID = team.ID
	}
	list, err := h.svc.KBCollectionsList(teamID)
	if err != nil { fail(c, err); return }
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": list}})
}

func (h *TicketHandler) KBCollectionCreate(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil { fail(c, err); return }
	col, err := h.svc.KBCollectionCreate(ctxUserID(c), req.Name, req.Description)
	if err != nil { fail(c, err); return }
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": col})
}

func (h *TicketHandler) KBCollectionUpdate(c *gin.Context) {
	var fields map[string]interface{}
	if err := c.ShouldBindJSON(&fields); err != nil { fail(c, err); return }
	col, err := h.svc.KBCollectionUpdate(ctxUserID(c), c.Param("cid"), fields)
	if err != nil { fail(c, err); return }
	c.JSON(http.StatusOK, gin.H{"success": true, "data": col})
}

func (h *TicketHandler) KBCollectionDelete(c *gin.Context) {
	if err := h.svc.KBCollectionDelete(ctxUserID(c), c.Param("cid")); err != nil { fail(c, err); return }
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *TicketHandler) KBRevisionsList(c *gin.Context) {
	list, err := h.svc.KBRevisionsList(ctxUserID(c), c.Param("id"))
	if err != nil { fail(c, err); return }
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": list}})
}

func (h *TicketHandler) KBRevisionRestore(c *gin.Context) {
	ver := 0
	fmt.Sscanf(c.Param("version"), "%d", &ver)
	if ver <= 0 {
		fail(c, fmt.Errorf("%w: versi tidak valid", ticketsvc.ErrValidation))
		return
	}
	a, err := h.svc.KBRestoreRevision(ctxUserID(c), c.Param("id"), ver)
	if err != nil { fail(c, err); return }
	c.JSON(http.StatusOK, gin.H{"success": true, "data": a})
}
