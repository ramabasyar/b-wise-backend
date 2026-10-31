package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ===== Formulir layanan (KT-05/KT-06) =====

// FormGet - formulir versi TERBIT utk pemohon (semua akun login).
func (h *TicketHandler) FormGet(c *gin.Context) {
	f := h.svc.FormPublished(c.Param("id"))
	if f == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": f})
}

// FormDraftGet - draf terbaru (admin unit; catalog.manage).
func (h *TicketHandler) FormDraftGet(c *gin.Context) {
	f := h.svc.FormDraft(c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": f})
}

// FormSave - simpan draf = versi baru; terbit tidak tersentuh (KT-06).
func (h *TicketHandler) FormSave(c *gin.Context) {
	var req struct {
		Schema string `json:"schema"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	f, err := h.svc.SaveFormDraft(ctxUserID(c), c.Param("id"), req.Schema)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": f})
}

// FormPublish - bekukan draf sebagai versi terbit.
func (h *TicketHandler) FormPublish(c *gin.Context) {
	f, err := h.svc.PublishForm(ctxUserID(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": f})
}

// ===== Sub-tugas lintas unit (AG-09/SL-06) =====

// TasksGet - sub-tugas tiket + OLA (pemohon/petugas viewer tiket).
func (h *TicketHandler) TasksGet(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.svc.GetForViewer(id, ctxUserID(c)); err != nil {
		fail(c, err)
		return
	}
	list, err := h.svc.TasksForViewer(id, ctxUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": list}})
}

// TaskAdd - petugas tiket menambah sub-tugas manual (AG-09).
func (h *TicketHandler) TaskAdd(c *gin.Context) {
	var req struct {
		TeamCode   string `json:"team_code" binding:"required"`
		Title      string `json:"title" binding:"required"`
		OLAMinutes int    `json:"ola_minutes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	t, err := h.svc.AddTask(c.Param("id"), ctxUserID(c), req.TeamCode, req.Title, req.OLAMinutes)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": t})
}

// TaskDone - anggota tim pemilik tugas menandakan selesai.
func (h *TicketHandler) TaskDone(c *gin.Context) {
	t, err := h.svc.DoneTask(c.Param("id"), c.Param("taskId"), ctxUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}
