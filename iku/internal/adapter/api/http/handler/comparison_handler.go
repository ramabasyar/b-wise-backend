package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	service "github.com/rama/b-wise/iku/internal/domain/service"
)

// ComparisonHandler — F10: perbandingan YoY (read-only).
type ComparisonHandler struct{ cmp *service.ComparisonService }

func NewComparisonHandler(cmp *service.ComparisonService) *ComparisonHandler {
	return &ComparisonHandler{cmp: cmp}
}

// List GET /api/comparisons?year=&unit_id=
func (h *ComparisonHandler) List(c *gin.Context) {
	year, _ := strconv.Atoi(c.Query("year"))
	if year == 0 {
		year = time.Now().Year()
	}
	rows, err := h.cmp.CompareYoY(year, c.Query("unit_id"), c.Query("indicator_id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}
