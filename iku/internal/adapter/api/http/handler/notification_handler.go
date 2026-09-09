package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	"gorm.io/gorm"
)

// NotificationHandler — F7: in-app notifications (list, unread count, mark read).
type NotificationHandler struct{ db *gorm.DB }

func NewNotificationHandler(db *gorm.DB) *NotificationHandler { return &NotificationHandler{db: db} }

// List GET /api/notifications?unread=1&limit=50
func (h *NotificationHandler) List(c *gin.Context) {
	uid := actorOf(c)
	q := h.db.Where("user_id = ? AND channel = 'inapp'", uid)
	if c.Query("unread") == "1" {
		q = q.Where("read_at IS NULL")
	}
	limit := 50
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	var rows []entity.Notification
	if err := q.Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	var unread int64
	h.db.Model(&entity.Notification{}).
		Where("user_id = ? AND channel = 'inapp' AND read_at IS NULL", uid).Count(&unread)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": rows, "unread": unread}})
}

// ReadAll POST /api/notifications/read-all
func (h *NotificationHandler) ReadAll(c *gin.Context) {
	uid := actorOf(c)
	now := time.Now()
	if err := h.db.Model(&entity.Notification{}).
		Where("user_id = ? AND channel = 'inapp' AND read_at IS NULL", uid).
		Update("read_at", now).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ReadOne POST /api/notifications/:id/read
func (h *NotificationHandler) ReadOne(c *gin.Context) {
	uid := actorOf(c)
	if err := h.db.Model(&entity.Notification{}).
		Where("id = ? AND user_id = ?", c.Param("id"), uid).
		Update("read_at", time.Now()).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
