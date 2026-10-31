package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/helpdesk/internal/service/ticketsvc"
)

// ===== Laporan lanjutan (LP-03..LP-06) =====

// ReportCSATLKPS - kepuasan 5 aspek format LKPS (LP-04); format=csv utk ekspor.
func (h *TicketHandler) ReportCSATLKPS(c *gin.Context) {
	from, to := reportRange(c)
	teamID := ""
	if code := c.Query("team"); code != "" {
		t, err := h.svc.TeamByCode(code)
		if err != nil {
			fail(c, ticketsvc.ErrNotFound)
			return
		}
		teamID = t.ID
	}
	rows, err := h.svc.CSATLKPS(from, to, teamID, c.Query("requester_unit"))
	if err != nil {
		fail(c, err)
		return
	}
	if c.Query("format") == "csv" {
		c.Header("Content-Disposition", "attachment; filename=lkps-kepuasan.csv")
		out := []string{}
		for _, r := range ticketsvc.CSVLKPS(rows) {
			out = append(out, strings.Join(r, ","))
		}
		c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte(strings.Join(out, "\n")))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

// ReportServicesPerformance - standar vs realisasi per layanan tim user (LP-05).
func (h *TicketHandler) ReportServicesPerformance(c *gin.Context) {
	from, to := reportRange(c)
	team, err := h.svc.TeamOf(ctxUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	rows, err := h.svc.ServicesPerformance(team.ID, from, to)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": rows}})
}

// ReportRooms - kerusakan/permintaan berulang per ruang (LP-06).
func (h *TicketHandler) ReportRooms(c *gin.Context) {
	from, to := reportRange(c)
	rows, err := h.svc.RoomReport(from, to)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": rows}})
}

// ReportOverview - lintas unit: beban tiap tim + layanan teratas (LP-03).
func (h *TicketHandler) ReportOverview(c *gin.Context) {
	from, to := reportRange(c)
	ov, err := h.svc.Overview(from, to)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": ov})
}
