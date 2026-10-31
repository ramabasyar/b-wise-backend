package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/helpdesk/internal/service/ticketsvc"
)

// ===== Kanal rahasia & PPKPT (RH-02..RH-08) =====

// SecretReport - buat laporan rahasia; boleh anonim (RH-03/RH-08).
// Kode lacak dikembalikan SEKALI; simpan baik-baik.
func (h *TicketHandler) SecretReport(c *gin.Context) {
	var req struct {
		ServiceID string `json:"service_id"`
		Subject   string `json:"subject" binding:"required"`
		Anonim    bool   `json:"anonim"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	t, code, err := h.svc.CreateSecretReport(ticketsvc.CreateParams{
		RequesterID: ctxUserID(c),
		ServiceID:   req.ServiceID,
		Subject:     req.Subject,
	}, req.Anonim)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"number": t.Number, "track_code": code}})
}

// SecretTrack - lacak laporan via kode: status+tahap tanpa isi (RH-05).
func (h *TicketHandler) SecretTrack(c *gin.Context) {
	v, err := h.svc.SecretTrack(c.Param("code"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": v})
}

// SetStage - transisi tahap penanganan PPKPT (RH-02; anggota tim pemilik).
func (h *TicketHandler) SetStage(c *gin.Context) {
	var req struct {
		Stage string `json:"stage" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	t, err := h.svc.SetStage(c.Param("id"), ctxUserID(c), req.Stage)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

// AccessLogs - jejak audit akses tiket (RH-06; anggota tim).
func (h *TicketHandler) AccessLogs(c *gin.Context) {
	logs, err := h.svc.AccessLogs(c.Param("id"), ctxUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": logs}})
}

// SecretAggregate - statistik rahasia tanpa identitas (RH-07).
func (h *TicketHandler) SecretAggregate(c *gin.Context) {
	from, to := reportRange(c)
	a, err := h.svc.SecretAggregate(from, to)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": a})
}
