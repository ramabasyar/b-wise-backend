package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RecurringProblemsH - LP-08: pola tiket berulang (agent/admin).
func (h *TicketHandler) RecurringProblemsH(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "60"))
	minCount, _ := strconv.Atoi(c.DefaultQuery("min", "3"))
	rows, err := h.svc.RecurringProblems(days, minCount)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}
