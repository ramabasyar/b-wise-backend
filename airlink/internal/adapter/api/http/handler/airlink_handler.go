package handler

import (
	"net/http"
	"strconv"
	"time"

	"airlink/internal/domain/service"
	"github.com/gin-gonic/gin"
)

type AirlinkHandler struct {
	radius *service.RadiusService
}

func NewAirlinkHandler(radius *service.RadiusService) *AirlinkHandler {
	return &AirlinkHandler{radius: radius}
}

// GetHealth GET /api/airlink/health
func (h *AirlinkHandler) GetHealth(c *gin.Context) {
	if h.radius == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "RADIUS database unavailable"})
		return
	}
	health := h.radius.GetHealth()
	c.JSON(http.StatusOK, gin.H{"data": health})
}

// GetActiveSessions GET /api/airlink/sessions/active?username=&limit=&offset=
func (h *AirlinkHandler) GetActiveSessions(c *gin.Context) {
	if h.radius == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "RADIUS database unavailable"})
		return
	}
	username := c.Query("username")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	sessions, total, err := h.radius.GetActiveSessions(username, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query sessions", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sessions, "total": total, "limit": limit, "offset": offset})
}

// GetSessionHistory GET /api/airlink/sessions/history?username=&start=&end=&limit=&offset=
func (h *AirlinkHandler) GetSessionHistory(c *gin.Context) {
	if h.radius == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "RADIUS database unavailable"})
		return
	}
	username := c.Query("username")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	var startTime, endTime time.Time
	if s := c.Query("start"); s != "" {
		startTime, _ = time.Parse(time.RFC3339, s)
	}
	if e := c.Query("end"); e != "" {
		endTime, _ = time.Parse(time.RFC3339, e)
	}

	sessions, total, err := h.radius.GetSessionHistory(username, startTime, endTime, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query history", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sessions, "total": total, "limit": limit, "offset": offset})
}

// GetAuthLogs GET /api/airlink/auth-logs?username=&limit=&offset=
func (h *AirlinkHandler) GetAuthLogs(c *gin.Context) {
	if h.radius == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "RADIUS database unavailable"})
		return
	}
	username := c.Query("username")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	logs, total, err := h.radius.GetAuthLogs(username, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query auth logs", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": logs, "total": total, "limit": limit, "offset": offset})
}

// GetUsageSummary GET /api/airlink/usage?start=&end=&limit=
func (h *AirlinkHandler) GetUsageSummary(c *gin.Context) {
	if h.radius == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "RADIUS database unavailable"})
		return
	}
	var startTime, endTime time.Time
	if s := c.Query("start"); s != "" {
		startTime, _ = time.Parse(time.RFC3339, s)
	}
	if e := c.Query("end"); e != "" {
		endTime, _ = time.Parse(time.RFC3339, e)
	}
	if endTime.IsZero() {
		endTime = time.Now()
	}
	if startTime.IsZero() {
		startTime = endTime.AddDate(0, 0, -30)
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	data, err := h.radius.GetUsageSummary(startTime, endTime, limit)
	// Enrich dengan peak speed per user
	for i := range data {
		if peak, peakErr := h.radius.GetPeakSpeed(data[i].UserName, startTime, endTime); peakErr == nil {
			data[i].PeakSpeedMbps = peak
		}
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query usage", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data, "start": startTime.Format(time.RFC3339), "end": endTime.Format(time.RFC3339)})
}

// GetUserUsageSessions GET /api/airlink/usage/:username/sessions?start=&end=
func (h *AirlinkHandler) GetUserUsageSessions(c *gin.Context) {
	if h.radius == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "RADIUS database unavailable"})
		return
	}
	username := c.Param("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username required"})
		return
	}
	var startTime, endTime time.Time
	if s := c.Query("start"); s != "" {
		startTime, _ = time.Parse(time.RFC3339, s)
	}
	if e := c.Query("end"); e != "" {
		endTime, _ = time.Parse(time.RFC3339, e)
	}
	if endTime.IsZero() {
		endTime = time.Now()
	}
	if startTime.IsZero() {
		startTime = endTime.AddDate(0, 0, -7)
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	data, err := h.radius.GetUserUsageSessions(username, startTime, endTime, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query usage sessions", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data, "username": username})
}
