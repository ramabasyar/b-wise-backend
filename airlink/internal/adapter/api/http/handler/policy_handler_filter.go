package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type filterReq struct {
	Name    string   `json:"name"`
	Mode    string   `json:"mode"`
	Domains []string `json:"domains"`
	Descr   string   `json:"descr"`
}

func (h *PolicyHandler) ListFilters(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	data, err := h.policy.ListFilters()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca profil filter"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

func (h *PolicyHandler) CreateFilter(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req filterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid"})
		return
	}
	f, err := h.policy.CreateFilter(req.Name, req.Mode, req.Domains, req.Descr)
	if err != nil {
		c.JSON(policyErrStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": f})
}

func (h *PolicyHandler) UpdateFilter(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}
	var req filterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid"})
		return
	}
	if err := h.policy.UpdateFilter(id, req.Name, req.Mode, req.Domains, req.Descr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "profil filter tidak ditemukan"})
			return
		}
		c.JSON(policyErrStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": id}})
}

func (h *PolicyHandler) DeleteFilter(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}
	if err := h.policy.DeleteFilter(id); err != nil {
		c.JSON(policyErrStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"deleted": id}})
}

func (h *PolicyHandler) SetUserFilter(c *gin.Context) {
	if h.unavailable(c) {
		return
	}
	var req struct {
		FilterProfileID *int64 `json:"filter_profile_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid"})
		return
	}
	p, err := h.policy.SetUserFilter(c.Param("username"), req.FilterProfileID, actorID(c))
	if err != nil {
		c.JSON(policyErrStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": p})
}
