package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"airlink/internal/domain/service"
	"github.com/gin-gonic/gin"
)

// PolicyHandler - manajemen akses user WiFi: suspend/unsuspend, kick, bandwidth.
type PolicyHandler struct {
	policy *service.PolicyService
	radius *service.RadiusService
}

func NewPolicyHandler(p *service.PolicyService, r *service.RadiusService) *PolicyHandler {
	return &PolicyHandler{policy: p, radius: r}
}

func (h *PolicyHandler) unavailable(c *gin.Context) bool {
	if h.policy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "policy service unavailable"})
		return true
	}
	return false
}

func policyErrStatus(err error) int {
	if errors.Is(err, sql.ErrNoRows) {
		return http.StatusNotFound
	}
	msg := err.Error()
	for _, m := range []string{
		"username tidak valid", "format rate", "nama profil tidak valid",
		"profil masih dipakai", "profil bandwidth tidak ditemukan",
		"kuota", "format jam", "format hari",
	} {
		if strings.Contains(msg, m) {
			return http.StatusBadRequest
		}
	}
	return http.StatusInternalServerError
}

func (h *PolicyHandler) ListUsers(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	rows, err := h.policy.ListUsers(c.Query("q"), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca daftar user"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

func (h *PolicyHandler) GetUser(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	username := c.Param("username")
	p, err := h.policy.GetUser(username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca policy user"})
		return
	}
	events, _ := h.policy.GetEvents(username, 50)
	var sessions interface{}
	if h.radius != nil {
		if s, _, err := h.radius.GetActiveSessions(username, 50, 0); err == nil {
			sessions = s
		}
	}
	today, month, _ := h.policy.UserUsage(username)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"policy": p, "events": events, "active_sessions": sessions, "today_bytes": today, "month_bytes": month}})
}

type suspendReq struct {
	Reason string     `json:"reason"`
	Until  *time.Time `json:"until"`
}

func (h *PolicyHandler) Suspend(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req suspendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid"})
		return
	}
	p, kickErrs, err := h.policy.SuspendUser(c.Param("username"), req.Reason, req.Until, actorID(c))
	if err != nil {
		c.JSON(policyErrStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": p, "kick_errors": kickErrs})
}

func (h *PolicyHandler) Unsuspend(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	p, err := h.policy.UnsuspendUser(c.Param("username"), actorID(c))
	if err != nil {
		c.JSON(policyErrStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": p})
}

func (h *PolicyHandler) Kick(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	kickErrs, err := h.policy.KickUser(c.Param("username"), actorID(c))
	if err != nil {
		c.JSON(policyErrStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"kicked": len(kickErrs) == 0}, "kick_errors": kickErrs})
}

type bandwidthReq struct {
	ProfileID *int64 `json:"profile_id"`
	RateUp    string `json:"rate_up"`
	RateDown  string `json:"rate_down"`
}

func (h *PolicyHandler) SetBandwidth(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req bandwidthReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid"})
		return
	}
	p, err := h.policy.SetBandwidth(c.Param("username"), req.ProfileID, req.RateUp, req.RateDown, actorID(c))
	if err != nil {
		c.JSON(policyErrStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": p})
}

func (h *PolicyHandler) ListProfiles(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	rows, err := h.policy.ListProfiles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca profil bandwidth"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

type profileReq struct {
	Name     string `json:"name"`
	RateDown string `json:"rate_down"`
	RateUp   string `json:"rate_up"`
	Descr    string `json:"descr"`
}

func (h *PolicyHandler) CreateProfile(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req profileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid"})
		return
	}
	p, err := h.policy.CreateProfile(req.Name, req.RateDown, req.RateUp, req.Descr)
	if err != nil {
		c.JSON(policyErrStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": p})
}

func (h *PolicyHandler) UpdateProfile(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}
	var req profileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid"})
		return
	}
	if err := h.policy.UpdateProfile(id, req.Name, req.RateDown, req.RateUp, req.Descr); err != nil {
		c.JSON(policyErrStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": id}})
}

func (h *PolicyHandler) DeleteProfile(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}
	if err := h.policy.DeleteProfile(id); err != nil {
		c.JSON(policyErrStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"deleted": id}})
}
