package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/iku/internal/domain/service"
)

// SetupHandler — status setup IKU (panduan urutan konfigurasi user).
type SetupHandler struct{ svc *service.SetupService }

func NewSetupHandler(svc *service.SetupService) *SetupHandler {
	return &SetupHandler{svc: svc}
}

// GetStatus GET /api/setup-status — lookup agregat read-only:
// cukup auth + akses service (mengikuti preseden lookup periode).
func (h *SetupHandler) GetStatus(c *gin.Context) {
	st, err := h.svc.GetStatus()
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": st})
}
