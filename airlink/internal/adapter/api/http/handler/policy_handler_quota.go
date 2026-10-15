package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// SetQuota - atur kuota pemakaian harian/bulanan user (MB, null = tanpa batas).
func (h *PolicyHandler) SetQuota(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req struct {
		QuotaDailyMB   *int64 `json:"quota_daily_mb"`
		QuotaMonthlyMB *int64 `json:"quota_monthly_mb"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid"})
		return
	}
	if err := h.policy.SetQuota(c.Param("username"), req.QuotaDailyMB, req.QuotaMonthlyMB, actorID(c)); err != nil {
		c.JSON(policyErrStatus(err), gin.H{"error": err.Error()})
		return
	}
	p, _ := h.policy.GetUser(c.Param("username"))
	c.JSON(http.StatusOK, gin.H{"data": p})
}

// SetSchedule - atur jadwal akses user (jam HH:MM + hari ISODOW).
func (h *PolicyHandler) SetSchedule(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req struct {
		AccessStart *string `json:"access_start"`
		AccessEnd   *string `json:"access_end"`
		AccessDays  string  `json:"access_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid"})
		return
	}
	if req.AccessStart != nil && *req.AccessStart == "" {
		req.AccessStart = nil
	}
	if req.AccessEnd != nil && *req.AccessEnd == "" {
		req.AccessEnd = nil
	}
	if err := h.policy.SetSchedule(c.Param("username"), req.AccessStart, req.AccessEnd, req.AccessDays, actorID(c)); err != nil {
		c.JSON(policyErrStatus(err), gin.H{"error": err.Error()})
		return
	}
	p, _ := h.policy.GetUser(c.Param("username"))
	c.JSON(http.StatusOK, gin.H{"data": p})
}
