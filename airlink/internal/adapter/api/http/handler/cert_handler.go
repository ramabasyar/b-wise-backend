package handler

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"time"

	"airlink/internal/adapter/api/http/middleware"
	"airlink/internal/domain/service"
	"github.com/gin-gonic/gin"
)

type CertHandler struct {
	certSvc *service.CertService
	store   *service.CertStore
	radius  *service.RadiusService
}

func NewCertHandler(certSvc *service.CertService, store *service.CertStore, radius *service.RadiusService) *CertHandler {
	return &CertHandler{certSvc: certSvc, store: store, radius: radius}
}

func (h *CertHandler) ready(c *gin.Context) bool {
	if middleware.IsServiceToken(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "sertifikat hanya dapat dikelola operator IT"})
		return false
	}
	if h.certSvc == nil || h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "pengelolaan sertifikat belum dikonfigurasi"})
		return false
	}
	return true
}

func actorID(c *gin.Context) string {
	if id, ok := middleware.GetUserID(c); ok {
		return id
	}
	return "unknown"
}

func (h *CertHandler) ListCerts(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	items, err := h.store.List(c.Request.Context(), c.Query("username"), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat sertifikat"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *CertHandler) IssueCert(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	var req struct {
		Username string `json:"username" binding:"required"`
		Device   string `json:"device" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username dan device wajib diisi"})
		return
	}
	exists, err := h.store.Exists(c.Request.Context(), req.Username, req.Device)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memeriksa sertifikat"})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{"error": "sertifikat aktif untuk perangkat ini sudah ada"})
		return
	}
	result, err := h.certSvc.Issue(req.Username, req.Device)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "penerbitan sertifikat gagal"})
		return
	}
	packageData, err := h.certSvc.FetchP12(req.Username, req.Device)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "paket sertifikat tidak dapat diambil"})
		return
	}
	serial, expiresAt, err := h.certSvc.CertMetadata(req.Username, req.Device)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "metadata sertifikat tidak dapat dibaca"})
		return
	}
	id, err := h.store.Save(c.Request.Context(), req.Username, req.Device, serial, expiresAt, packageData, result.P12Pass, actorID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "sertifikat terbit tetapi pencatatan gagal; hubungi administrator RADIUS"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, gin.H{"data": gin.H{"id": id, "username": result.Username, "device_name": result.DeviceName, "cert_serial": serial, "expires_at": expiresAt, "total_certs": result.Total, "issued_by": actorID(c), "issued_at": time.Now().Format(time.RFC3339)}})
}

// GetPackage menyerahkan P12 dan password via HTTPS ke operator IT yang berizin.
func (h *CertHandler) GetPackage(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	username, device := c.Param("username"), c.Param("device")
	data, password, err := h.store.Package(c.Request.Context(), username, device, actorID(c))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "paket sertifikat tidak tersedia"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "paket sertifikat tidak dapat dibuka"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"filename": username + "-" + device + ".p12", "p12_base64": base64.StdEncoding.EncodeToString(data), "password": password}})
}

// RevokeCert DELETE /api/airlink/certs/:username/:device
// Mencabut sertifikat (CRL) + best-effort CoA disconnect + laporan status terpisah.
func (h *CertHandler) RevokeCert(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	username, device := c.Param("username"), c.Param("device")

	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)

	// 1. Revoke cert di PKI (CRL)
	if err := h.certSvc.Revoke(username, device, req.Reason); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "revoke sertifikat gagal", "details": err.Error()})
		return
	}

	// 2. Best-effort CoA: cari sesi aktif user, coba putus masing-masing
	disconnectResults := []gin.H{}
	sessions, sessErr := h.radius.GetActiveSessionsForUser(username)
	if sessErr != nil {
		disconnectResults = append(disconnectResults, gin.H{"error": "gagal mencari sesi aktif: " + sessErr.Error()})
	} else {
		for _, sess := range sessions {
			coaErr := h.certSvc.DisconnectSession(sess.UserName, sess.AcctSessionID, sess.NASIPAddress, sess.CallingStationID)
			entry := gin.H{
				"session_id":   sess.AcctSessionID,
				"nas_ip":       sess.NASIPAddress,
				"mac_address":  sess.CallingStationID,
				"disconnected": coaErr == nil,
			}
			if coaErr != nil {
				entry["error"] = coaErr.Error()
			}
			disconnectResults = append(disconnectResults, entry)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"username":     username,
			"device":       device,
			"revoked_by":   actorID(c),
			"reason":       req.Reason,
			"revoked_at":   time.Now().Format(time.RFC3339),
			"cert_revoked": true,
			"disconnects":  disconnectResults,
		},
		"message": "Sertifikat dicabut. Sesi aktif diputus best-effort — periksa status masing-masing sesi.",
	})
}
