package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// UsageDaily - tren pemakaian harian user.
func (h *PolicyHandler) UsageDaily(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "14"))
	data, err := h.policy.DailyUsage(c.Param("username"), days)
	if err != nil {
		c.JSON(policyErrStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// UsageSummary - ringkasan pemakaian jaringan.
func (h *PolicyHandler) UsageSummary(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	data, err := h.policy.UsageSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membuat ringkasan pemakaian"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}
