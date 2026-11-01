package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ==================== AG-13 + OT-06: Asisten AI (F4) ====================
// Saran semata — selalu dikonfirmasi petugas (disclaimer disertakan respons).
// Binding via map (tanpa struct tag) — bebas jebakan kutip lintas-lapis.

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

// AITriageH - saran layanan/tim/prioritas utk tiket baru (OT-06, utk FO).
func (h *TicketHandler) AITriageH(c *gin.Context) {
	var body map[string]any
	_ = c.ShouldBindJSON(&body)
	subject, _ := body["subject"].(string)
	answers, _ := body["answers"].(string)
	out, err := h.svc.AITriage(subject, answers)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}
