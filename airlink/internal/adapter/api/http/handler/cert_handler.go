package handler

import (
	"net/http"
	"time"

	"airlink/internal/domain/service"
	"github.com/gin-gonic/gin"
)

type CertHandler struct {
	certSvc *service.CertService
}

func NewCertHandler(certSvc *service.CertService) *CertHandler {
	return &CertHandler{certSvc: certSvc}
}

// IssueCert POST /api/airlink/certs
// Body: {"username": "btest", "device": "laptop"}
func (h *CertHandler) IssueCert(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Device   string `json:"device" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	// Actor = user yang login (dari JWKS auth middleware)
	actor, _ := c.Get("username")
	actorStr, _ := actor.(string)

	result, err := h.certSvc.Issue(req.Username, req.Device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "issue failed", "details": err.Error()})
		return
	}

	// Jangan expose password di response — kirim via jalur terpisah
	c.JSON(http.StatusCreated, gin.H{
		"data": gin.H{
			"username":    result.Username,
			"device_name": result.DeviceName,
			"total_certs": result.Total,
			"issued_by":   actorStr,
			"issued_at":   time.Now().Format(time.RFC3339),
		},
		"message": "Sertifikat berhasil diterbitkan. Gunakan endpoint download untuk mengambil file .p12",
	})
}

// DownloadP12 GET /api/airlink/certs/:username/:device/download
func (h *CertHandler) DownloadP12(c *gin.Context) {
	username := c.Param("username")
	device := c.Param("device")
	if username == "" || device == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username dan device required"})
		return
	}

	remotePath := "/var/lib/radius-certs/" + username + "/" + device + ".p12"
	data, err := h.certSvc.FetchP12(remotePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "certificate not found", "details": err.Error()})
		return
	}

	c.Header("Content-Disposition", "attachment; filename="+username+"-"+device+".p12")
	c.Data(http.StatusOK, "application/x-pkcs12", data)
}

// RevokeCert DELETE /api/airlink/certs/:username/:device
func (h *CertHandler) RevokeCert(c *gin.Context) {
	username := c.Param("username")
	device := c.Param("device")
	if username == "" || device == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username dan device required"})
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req) // optional body

	actor, _ := c.Get("username")
	actorStr, _ := actor.(string)

	if err := h.certSvc.Revoke(username, device, req.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "revoke failed", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"username":   username,
			"device":     device,
			"revoked_by": actorStr,
			"reason":     req.Reason,
			"revoked_at": time.Now().Format(time.RFC3339),
		},
		"message": "Sertifikat dicabut. Sesi WiFi aktif belum diputus otomatis; lakukan pemutusan melalui UniFi.",
	})
}
