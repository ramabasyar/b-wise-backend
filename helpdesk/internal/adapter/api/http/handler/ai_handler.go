package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ==================== AG-13: Asisten AI (F4) ====================
// Saran semata — selalu dikonfirmasi petugas (disclaimer disertakan respons).

// AISummaryH - ringkasan tiket utk petugas (anggota tim pemilik).
func (h *TicketHandler) AISummaryH(c *gin.Context) {
	out, err := h.svc.AISummary(c.Param("id"), ctxUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// AISuggestReplyH - draft balasan utk pemohon (petugas konfirmasi/edit/kirim).
func (h *TicketHandler) AISuggestReplyH(c *gin.Context) {
	out, err := h.svc.AISuggestReply(c.Param("id"), ctxUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
