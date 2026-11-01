package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/rama/b-wise/notify/internal/service/notifsvc"
	"github.com/rama/b-wise/notify/pkg/response"
)

// InternalHandler — API notifikasi terpusat: enqueue (service-token), status,
// dan langganan push PWA (user).
type InternalHandler struct {
	svc *notifsvc.Service
}

func NewInternalHandler(svc *notifsvc.Service) *InternalHandler { return &InternalHandler{svc: svc} }

// Create POST /api/internal/notifications — service-token only.
func (h *InternalHandler) Create(c *gin.Context) {
	var req struct {
		Channel   string `json:"channel" binding:"required"`
		Recipient string `json:"recipient"`
		UserID    string `json:"user_id"`
		Topic     string `json:"topic" binding:"required"`
		Title     string `json:"title" binding:"required"`
		Body      string `json:"body"`
		Payload   string `json:"payload"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "channel, topic, dan title wajib")
		return
	}
	if req.Channel == "push" && req.Recipient == "" {
		req.Recipient = req.UserID // push: recipient = user_id
	}
	o, err := h.svc.Enqueue(req.Channel, req.Recipient, req.UserID, req.Topic, req.Title, req.Body, req.Payload)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": o})
}

// GetStatus GET /api/internal/notifications/:id — service-token only.
func (h *InternalHandler) GetStatus(c *gin.Context) {
	o, err := h.svc.Get(c.Param("id"))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "notifikasi tidak ditemukan")
			return
		}
		response.Error(c, err)
		return
	}
	response.Success(c, o)
}

// SaveSub POST /api/push/subscriptions — user auth (browser mendaftar push).
func (h *InternalHandler) SaveSub(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid, _ := userID.(string)
	if uid == "" {
		response.Unauthorized(c, "login diperlukan")
		return
	}
	var req struct {
		Endpoint string `json:"endpoint" binding:"required"`
		Keys     struct {
			P256DH string `json:"p256dh" binding:"required"`
			Auth   string `json:"auth" binding:"required"`
		} `json:"keys"`
		UserAgent string `json:"user_agent"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "endpoint + keys(p256dh,auth) wajib")
		return
	}
	if err := h.svc.SaveSubscription(uid, req.Endpoint, req.Keys.P256DH, req.Keys.Auth, req.UserAgent); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"message": "langganan push tersimpan"}})
}

// DeleteSub DELETE /api/push/subscriptions — user auth (berhenti langganan).
func (h *InternalHandler) DeleteSub(c *gin.Context) {
	var req struct {
		Endpoint string `json:"endpoint" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "endpoint wajib")
		return
	}
	if err := h.svc.DeleteSubscription(req.Endpoint); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"message": "langganan dihapus"})
}
