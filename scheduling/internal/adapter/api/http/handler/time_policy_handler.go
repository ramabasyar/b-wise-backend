package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/rama/b-wise/scheduling/internal/domain/entity"
)

// TimePolicyHandler — GET/PUT kebijakan durasi SKS (singleton "default").
type TimePolicyHandler struct{ db *gorm.DB }

func NewTimePolicyHandler(db *gorm.DB) *TimePolicyHandler { return &TimePolicyHandler{db: db} }

func (h *TimePolicyHandler) ensure() (*entity.TimePolicy, error) {
	var cfg entity.TimePolicy
	err := h.db.Where("code = ?", "default").First(&cfg).Error
	if err == nil {
		return &cfg, nil
	}
	cfg = entity.TimePolicy{Code: "default", SksMinutesTheory: 50, SksMinutesPractice: 170}
	if err := h.db.Create(&cfg).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (h *TimePolicyHandler) Get(c *gin.Context) {
	cfg, err := h.ensure()
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

func (h *TimePolicyHandler) Update(c *gin.Context) {
	if _, err := h.ensure(); err != nil {
		errJSON(c, err)
		return
	}
	var in struct {
		SksMinutesTheory   *int    `json:"sks_minutes_theory" binding:"omitempty,gte=10,lte=300"`
		SksMinutesPractice *int    `json:"sks_minutes_practice" binding:"omitempty,gte=10,lte=600"`
		Notes              *string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		errJSON(c, err)
		return
	}
	patch := map[string]any{}
	if in.SksMinutesTheory != nil {
		patch["sks_minutes_theory"] = *in.SksMinutesTheory
	}
	if in.SksMinutesPractice != nil {
		patch["sks_minutes_practice"] = *in.SksMinutesPractice
	}
	if in.Notes != nil {
		patch["notes"] = *in.Notes
	}
	var cfg entity.TimePolicy
	if err := h.db.Model(&entity.TimePolicy{}).Where("code = ?", "default").
		Clauses(clause.Returning{}).Updates(patch).Scan(&cfg).Error; err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}
