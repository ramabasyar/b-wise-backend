package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"airlink/internal/adapter/api/http/middleware"
	"airlink/internal/domain/service"
	"github.com/gin-gonic/gin"
)

var certIdentityClient = &http.Client{Timeout: 5 * time.Second}

// selfUsername resolves the signed-in account through SSO. Access tokens only
// carry user_id/email; trusting a username supplied by the browser would let
// a user mint or download another person's WiFi identity.
func (h *CertHandler) selfUsername(c *gin.Context) (string, bool) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "login diperlukan"})
		return "", false
	}
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, h.ssoURL+"/oauth/userinfo", nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "identitas SSO tidak dapat diverifikasi"})
		return "", false
	}
	request.Header.Set("Authorization", c.GetHeader("Authorization"))
	response, err := certIdentityClient.Do(request)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "identitas SSO tidak dapat diverifikasi"})
		return "", false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": "identitas SSO tidak dapat diverifikasi"})
		return "", false
	}
	var identity struct {
		Sub               string `json:"sub"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 8192)).Decode(&identity); err != nil || identity.Sub != userID {
		c.JSON(http.StatusBadGateway, gin.H{"error": "identitas SSO tidak cocok dengan token"})
		return "", false
	}
	username := strings.TrimSpace(identity.PreferredUsername)
	if !service.ValidCertUsername(username) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "username akun SSO tidak sesuai format sertifikat WiFi"})
		return "", false
	}
	return username, true
}

func (h *CertHandler) ListMyCerts(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	username, ok := h.selfUsername(c)
	if !ok {
		return
	}
	items, err := h.store.List(c.Request.Context(), username, 100, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat sertifikat"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *CertHandler) IssueMyCert(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	username, ok := h.selfUsername(c)
	if !ok {
		return
	}
	var request struct {
		Device string `json:"device" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nama perangkat wajib diisi"})
		return
	}
	h.issueFor(c, username, request.Device)
}

func (h *CertHandler) GetMyPackage(c *gin.Context) {
	if !h.ready(c) {
		return
	}
	username, ok := h.selfUsername(c)
	if !ok {
		return
	}
	h.packageFor(c, username, c.Param("device"))
}
