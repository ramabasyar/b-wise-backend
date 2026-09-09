package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/iku/internal/adapter/api/http/middleware"
	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	service "github.com/rama/b-wise/iku/internal/domain/service"
)

type IkuHandler struct {
	svc *service.IkuService
}

func NewIkuHandler(svc *service.IkuService) *IkuHandler { return &IkuHandler{svc: svc} }

func errStatus(c *gin.Context, err error) {
	msg := err.Error()
	code := http.StatusInternalServerError
	if len(msg) >= 15 && msg[:15] == "data tidak ditem" {
		code = http.StatusNotFound
	} else if len(msg) >= 14 && msg[:14] == "validasi gagal" {
		code = http.StatusBadRequest
	}
	c.JSON(code, gin.H{"success": false, "error": gin.H{"message": msg}})
}

// ==================== REGULATIONS ====================

// ListRegulations GET /api/regulations?include_inactive=
func (h *IkuHandler) ListRegulations(c *gin.Context) {
	list, err := h.svc.ListRegulations(c.Query("include_inactive") == "true")
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

// CreateRegulation POST /api/regulations
func (h *IkuHandler) CreateRegulation(c *gin.Context) {
	var r entity.RegulatoryVersion
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	uid, _ := middleware.GetUserID(c)
	r.CreatedBy = uid
	if err := h.svc.CreateRegulation(&r); err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": r})
}

// ==================== INDICATORS ====================

// ListIndicators GET /api/indicators?reg_version_id=&period_type=&nature=&active_only=
func (h *IkuHandler) ListIndicators(c *gin.Context) {
	f := service.IndicatorFilter{
		RegVersionID: c.Query("reg_version_id"),
		PeriodType:   c.Query("period_type"),
		Nature:       c.Query("nature"),
		ActiveOnly:   c.DefaultQuery("active_only", "true") == "true",
	}
	list, err := h.svc.ListIndicators(f)
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list,
		"meta": gin.H{"total": len(list)}})
}

// GetIndicator GET /api/indicators/:id
func (h *IkuHandler) GetIndicator(c *gin.Context) {
	d, err := h.svc.GetIndicator(c.Param("id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": d})
}

// ==================== PERIODS ====================

// ListPeriods GET /api/periods?year=&type=
func (h *IkuHandler) ListPeriods(c *gin.Context) {
	year, _ := strconv.Atoi(c.Query("year"))
	list, err := h.svc.ListPeriods(year, c.Query("type"))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

// CreatePeriod POST /api/periods
func (h *IkuHandler) CreatePeriod(c *gin.Context) {
	var p entity.Period
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	if err := h.svc.CreatePeriod(&p); err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": p})
}

var _ = time.Now

// UpdateIndicator PATCH /api/indicators/:id — atur polarity & rollup_rule (F10).
type UpdateIndicatorRequest struct {
	Polarity   *string `json:"polarity"`    // higher_is_better|lower_is_better
	RollupRule *string `json:"rollup_rule"` // sum|avg|last
}

func (h *IkuHandler) UpdateIndicator(c *gin.Context) {
	var req UpdateIndicatorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	patch := map[string]interface{}{}
	if req.Polarity != nil {
		if *req.Polarity != "higher_is_better" && *req.Polarity != "lower_is_better" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "polarity harus higher_is_better|lower_is_better"}})
			return
		}
		patch["polarity"] = *req.Polarity
	}
	if req.RollupRule != nil {
		if *req.RollupRule != "sum" && *req.RollupRule != "avg" && *req.RollupRule != "last" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "rollup_rule harus sum|avg|last"}})
			return
		}
		patch["rollup_rule"] = *req.RollupRule
	}
	if len(patch) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "tidak ada field yang diubah"}})
		return
	}
	var ind entity.IndicatorDefinition
	if err := h.svc.DB().Where("id = ?", c.Param("id")).First(&ind).Error; err != nil {
		errStatus(c, service.ErrNotFound)
		return
	}
	if err := h.svc.DB().Model(&ind).Updates(patch).Error; err != nil {
		errStatus(c, err)
		return
	}
	h.svc.DB().Where("id = ?", ind.ID).First(&ind)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": ind})
}

type VerifyRegulationRequest struct {
	DocumentURL string `json:"document_url"`
	Verified    *bool  `json:"verified"`
}

// VerifyRegulation PATCH /api/regulations/:id/verify (F7 JDIH)
func (h *IkuHandler) VerifyRegulation(c *gin.Context) {
	var req VerifyRegulationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	verified := true
	if req.Verified != nil {
		verified = *req.Verified
	}
	reg, err := h.svc.VerifyRegulation(c.Param("id"), req.DocumentURL, verified, actorOf(c))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": reg})
}
