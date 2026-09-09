package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthHandler handles health check requests
type HealthHandler struct{}

// NewHealthHandler creates a new health handler
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// GetHealth returns health status
func (h *HealthHandler) GetHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": "Service is running",
	})
}
