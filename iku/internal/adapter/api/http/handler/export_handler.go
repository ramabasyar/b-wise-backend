package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	service "github.com/rama/b-wise/iku/internal/domain/service"
)

// ExportHandler — F7: export Excel/PDF laporan kinerja.
type ExportHandler struct{ exp *service.ExportService }

func NewExportHandler(exp *service.ExportService) *ExportHandler { return &ExportHandler{exp: exp} }

// DashboardExcel GET /api/exports/dashboard.xlsx?period_id=
func (h *ExportHandler) DashboardExcel(c *gin.Context) {
	f, err := h.exp.DashboardExcel(c.Query("period_id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	name := fmt.Sprintf("laporan-iku-%s.xlsx", time.Now().Format("20060102-1504"))
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if err := f.Write(c.Writer); err != nil {
		_ = c.Error(err)
	}
}

// AchievementsExcel GET /api/exports/achievements.xlsx?indicator_id=&period_id=
func (h *ExportHandler) AchievementsExcel(c *gin.Context) {
	f, err := h.exp.AchievementsExcel(c.Query("indicator_id"), c.Query("period_id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	name := fmt.Sprintf("capaian-iku-%s.xlsx", time.Now().Format("20060102-1504"))
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if err := f.Write(c.Writer); err != nil {
		_ = c.Error(err)
	}
}

// ComparisonExcel GET /api/exports/comparison.xlsx?year=&unit_id= (F10 YoY)
func (h *ExportHandler) ComparisonExcel(c *gin.Context) {
	year, _ := strconv.Atoi(c.Query("year"))
	if year == 0 {
		year = time.Now().Year()
	}
	f, err := h.exp.ComparisonExcel(year, c.Query("unit_id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	name := fmt.Sprintf("perbandingan-yoy-%d-%s.xlsx", year, time.Now().Format("20060102-1504"))
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if err := f.Write(c.Writer); err != nil {
		_ = c.Error(err)
	}
}

// ReportPDF GET /api/exports/report.pdf?period_id=
func (h *ExportHandler) ReportPDF(c *gin.Context) {
	b, err := h.exp.ReportPDF(c.Query("period_id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	name := fmt.Sprintf("laporan-iku-%s.pdf", time.Now().Format("20060102-1504"))
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Header("Content-Type", "application/pdf")
	c.Data(http.StatusOK, "application/pdf", b)
}
