package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/rama/b-wise/scheduling/internal/domain/entity"
)

// CalendarHandler — daftar hari libur kampus (calendar_events kind=holiday)
// utk konsumen lintas service (helpdesk SLA, dsb.). Service-token only.
type CalendarHandler struct{ db *gorm.DB }

func NewCalendarHandler(db *gorm.DB) *CalendarHandler { return &CalendarHandler{db: db} }

// Holidays GET /api/calendars/holidays?from=&to= — tanggal libur (YYYY-MM-DD).
func (h *CalendarHandler) Holidays(c *gin.Context) {
	q := h.db.Model(&entity.CalendarEvent{}).Where("kind = ?", "holiday").Order("date asc")
	if from := c.Query("from"); from != "" {
		q = q.Where("date >= ?", from)
	}
	if to := c.Query("to"); to != "" {
		q = q.Where("date <= ?", to)
	}
	var rows []entity.CalendarEvent
	if err := q.Limit(500).Find(&rows).Error; err != nil {
		errJSON(c, err)
		return
	}
	type item struct {
		Date string `json:"date"`
		Name string `json:"name"`
		Term string `json:"term_id"`
	}
	out := make([]item, 0, len(rows))
	for _, r := range rows {
		d := r.Date
		if len(d) > 10 {
			d = d[:10] // date PG bisa terbaca time — pastikan YYYY-MM-DD
		}
		out = append(out, item{Date: d, Name: r.Name, Term: r.TermID})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": out}})
}
