package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// MetricsHandler — F7 observability: endpoint Prometheus text-format sederhana
// (counters dari tabel — cukup utk panel/monitoring dev; tanpa dependency client lib).
type MetricsHandler struct{ db *gorm.DB }

func NewMetricsHandler(db *gorm.DB) *MetricsHandler { return &MetricsHandler{db: db} }

// Get GET /api/metrics
func (h *MetricsHandler) Get(c *gin.Context) {
	var b []byte
	b = append(b, "# B-Wise IKU service metrics\n"...)

	countBy := func(table, where, label string, out *int64) {
		q := h.db.Table(table)
		if where != "" {
			q = q.Where(where)
		}
		q.Count(out)
		b = append(b, []byte(fmt.Sprintf("%s %d\n", label, *out))...)
	}

	var v int64
	countBy("indicator_definitions", "deleted_at IS NULL", "iku_indicators_total", &v)
	for _, st := range []string{"draft", "submitted", "reviewed", "approved", "published", "rejected"} {
		countBy("achievement_records", "status = '"+st+"'", fmt.Sprintf("iku_achievements{status=%q} ", st), &v)
	}
	for _, st := range []string{"active", "retired"} {
		countBy("formula_versions", "status = '"+st+"'", fmt.Sprintf("iku_formula_versions{status=%q} ", st), &v)
	}
	countBy("performance_targets", "", "iku_targets_total", &v)
	for _, st := range []string{"open", "escalated", "done"} {
		countBy("action_plans", "status = '"+st+"'", fmt.Sprintf("iku_action_plans{status=%q} ", st), &v)
	}
	for _, st := range []string{"applied", "failed", "preview"} {
		countBy("import_batches", "status = '"+st+"'", fmt.Sprintf("iku_imports{status=%q} ", st), &v)
	}
	for _, st := range []string{"sent", "failed", "read"} {
		countBy("notifications", "status = '"+st+"'", fmt.Sprintf("iku_notifications{status=%q} ", st), &v)
	}
	countBy("regulatory_versions", "status = 'active'", "iku_regulations_active", &v)

	c.Data(http.StatusOK, "text/plain; version=0.0.4", b)
}
