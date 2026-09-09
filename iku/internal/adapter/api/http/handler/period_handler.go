package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	"github.com/rama/b-wise/iku/internal/domain/service"
)

// ==================== PERIOD LIFECYCLE HANDLER (F9) ====================

type PeriodHandler struct {
	svc *service.PeriodService
}

func NewPeriodHandler(svc *service.PeriodService) *PeriodHandler {
	return &PeriodHandler{svc: svc}
}

// PeriodDTO — periode + info turunan untuk UI.
type PeriodDTO struct {
	entity.Period
	CapaianCounts map[string]int `json:"capaian_counts"`
	GraceEnd      *time.Time     `json:"grace_end,omitempty"`
	IsCurrent     bool           `json:"is_current"` // periode berjalan (hari ini di range start-end)
}

func toPeriodDTO(p entity.Period, counts map[string]map[string]int, now time.Time) PeriodDTO {
	d := PeriodDTO{Period: p, CapaianCounts: map[string]int{}, IsCurrent: now.After(p.StartDate) && now.Before(p.EndDate.AddDate(0, 0, 1))}
	if c, ok := counts[p.ID]; ok {
		d.CapaianCounts = c
	}
	if p.Status == "grace" || p.Status == "open" {
		ge := p.GraceEnd()
		d.GraceEnd = &ge
	}
	return d
}

// List GET /api/periods?year=&type=
func (h *PeriodHandler) List(c *gin.Context) {
	year, _ := strconv.Atoi(c.Query("year"))
	ptype := c.Query("type")
	list, err := h.svc.List(year, ptype)
	if err != nil {
		errStatus(c, err)
		return
	}
	counts, _ := h.svc.CountStats()
	now := time.Now()
	out := make([]PeriodDTO, 0, len(list))
	for _, p := range list {
		out = append(out, toPeriodDTO(p, counts, now))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// Get GET /api/periods/:id — detail + counts (checklist sebelum tutup).
func (h *PeriodHandler) Get(c *gin.Context) {
	p, _, err := h.svc.Get(c.Param("id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	counts, _ := h.svc.CountStats()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": toPeriodDTO(*p, counts, time.Now())})
}

type periodNoteRequest struct {
	Note string `json:"note"`
}

// Open POST /api/periods/:id/open — buka (provisioned) / reopen (closed, note wajib).
func (h *PeriodHandler) Open(c *gin.Context) {
	var req periodNoteRequest
	_ = c.ShouldBindJSON(&req) // note opsional kecuali reopen (divalidasi service)
	p, err := h.svc.OpenPeriod(c.Param("id"), actorOf(c), req.Note)
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": p})
}

// Close POST /api/periods/:id/close — tutup periode (open|grace). Reviewer/admin.
func (h *PeriodHandler) Close(c *gin.Context) {
	var req periodNoteRequest
	_ = c.ShouldBindJSON(&req)
	summary, err := h.svc.ClosePeriod(c.Param("id"), actorOf(c), req.Note, false)
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

// Rollup POST /api/periods/:id/rollup — generate ulang rollup tahunan (annual, stale).
func (h *PeriodHandler) Rollup(c *gin.Context) {
	p, _, err := h.svc.Get(c.Param("id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	if p.Type != "annual" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "rollup hanya untuk periode tahunan"}})
		return
	}
	n, err := h.svc.TryRollup(p.Year)
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"generated": n, "year": p.Year}})
}
