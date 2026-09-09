package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/rama/b-wise/scheduling/internal/domain/entity"
)

// SolveConfigHandler — GET/PUT bobot soft constraint (singleton "default").
type SolveConfigHandler struct{ db *gorm.DB }

func NewSolveConfigHandler(db *gorm.DB) *SolveConfigHandler { return &SolveConfigHandler{db: db} }

// ensure — ambil baris default; buat dgn bobot bawaan (5/2/1) bila belum ada.
func (h *SolveConfigHandler) ensure() (*entity.SolveConfig, error) {
	var cfg entity.SolveConfig
	err := h.db.Where("code = ?", "default").First(&cfg).Error
	if err == nil {
		return &cfg, nil
	}
	cfg = entity.SolveConfig{Code: "default", SpreadWeight: 5, RoomWasteWeight: 2, LastSlotWeight: 1}
	if err := h.db.Create(&cfg).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (h *SolveConfigHandler) Get(c *gin.Context) {
	cfg, err := h.ensure()
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

func (h *SolveConfigHandler) Update(c *gin.Context) {
	if _, err := h.ensure(); err != nil {
		errJSON(c, err)
		return
	}
	var in struct {
		SpreadWeight    *int    `json:"spread_weight" binding:"omitempty,gte=0,lte=100"`
		RoomWasteWeight *int    `json:"room_waste_weight" binding:"omitempty,gte=0,lte=100"`
		LastSlotWeight  *int    `json:"last_slot_weight" binding:"omitempty,gte=0,lte=100"`
		Notes           *string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		errJSON(c, err)
		return
	}
	patch := map[string]any{}
	if in.SpreadWeight != nil {
		patch["spread_weight"] = *in.SpreadWeight
	}
	if in.RoomWasteWeight != nil {
		patch["room_waste_weight"] = *in.RoomWasteWeight
	}
	if in.LastSlotWeight != nil {
		patch["last_slot_weight"] = *in.LastSlotWeight
	}
	if in.Notes != nil {
		patch["notes"] = *in.Notes
	}
	var cfg entity.SolveConfig
	if err := h.db.Model(&entity.SolveConfig{}).Where("code = ?", "default").
		Clauses(clause.Returning{}).Updates(patch).Scan(&cfg).Error; err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}
