package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	service "github.com/rama/b-wise/scheduling/internal/domain/service"
)

// ==================== SOLVE JOB HANDLER (F1) ====================

type SolveHandler struct{ svc *service.SolveService }

func NewSolveHandler(svc *service.SolveService) *SolveHandler { return &SolveHandler{svc: svc} }

// DayView — jadwal efektif satu tanggal (pola + overrides + event).
func (h *SolveHandler) DayView(c *gin.Context) {
	data, err := h.svc.DayView(c.Query("date"), c.Query("term_id"))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// CreateOverride — pengecualian manual per tanggal.
func (h *SolveHandler) CreateOverride(c *gin.Context) {
	var req struct {
		TermID  string `json:"term_id"`
		Date    string `json:"date" binding:"required"`
		EntryID string `json:"entry_id" binding:"required"`
		Kind    string `json:"kind" binding:"required,oneof=moved cancelled"`
		SlotID  string `json:"slot_id"`
		RoomID  string `json:"room_id"`
		Reason  string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errJSON(c, err)
		return
	}
	ov, err := h.svc.CreateOverride(req.TermID, req.Date, req.EntryID, req.Kind, req.SlotID, req.RoomID, req.Reason)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "Pengecualian dibuat", "data": ov})
}

// DeleteOverride — hapus pengecualian.
func (h *SolveHandler) DeleteOverride(c *gin.Context) {
	if err := h.svc.DeleteOverride(c.Param("id")); err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Pengecualian dihapus — kembali ke pola normal"})
}

// ProposeAdjustment — susun usulan penyesuaian dari ketersediaan ruang (Lapis 2).
func (h *SolveHandler) ProposeAdjustment(c *gin.Context) {
	var req struct {
		RoomAvailabilityID string `json:"room_availability_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errJSON(c, err)
		return
	}
	actor := c.GetString("user_id")
	p, err := h.svc.ProposeAdjustment(req.RoomAvailabilityID, actor)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "Proposal penyesuaian disusun", "data": p})
}

// ListAdjustments — daftar proposal penyesuaian.
func (h *SolveHandler) ListAdjustments(c *gin.Context) {
	items, err := h.svc.ListAdjustments(c.DefaultQuery("status", "pending"))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// DecideAdjustment — setujui/tolak proposal.
func (h *SolveHandler) DecideAdjustment(c *gin.Context) {
	var req struct {
		Approve bool `json:"approve"`
	}
	_ = c.ShouldBindJSON(&req)
	actor := c.GetString("user_id")
	p, err := h.svc.DecideAdjustment(c.Param("id"), req.Approve, actor)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Proposal diputuskan", "data": p})
}

// MoveEntry — geser 1 sesi draft ke slot/ruang baru (Penyesuaian Jadwal Lapis 1).
// Validasi keras di service: bentrok ruang/rombel/dosen + kapasitas + tipe ruang + ketersediaan dosen.
func (h *SolveHandler) MoveEntry(c *gin.Context) {
	var req struct {
		EntryID string `json:"entry_id" binding:"required"`
		SlotID  string `json:"slot_id" binding:"required"`
		RoomID  string `json:"room_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errJSON(c, err)
		return
	}
	entry, err := h.svc.MoveEntry(req.EntryID, req.SlotID, req.RoomID)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Sesi dipindahkan", "data": entry})
}

type startSolveRequest struct {
	TermID    string `json:"term_id" binding:"required"`
	TimeLimit int    `json:"time_limit_seconds"`
}

// Start POST /api/solve-jobs — jalankan penjadwalan (async).
func (h *SolveHandler) Start(c *gin.Context) {
	var req startSolveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "term_id wajib"}})
		return
	}
	actor := c.GetString("user_id")
	if actor == "" {
		actor = c.GetHeader("X-Request-ID")
	}
	job, err := h.svc.Start(req.TermID, actor, req.TimeLimit)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": job})
}

// ProposeManualAdjustment POST /api/timetable/adjustments/manual — usulan pindah manual satu sesi.
func (h *SolveHandler) ProposeManualAdjustment(c *gin.Context) {
	var req struct {
		EntryID  string `json:"entry_id" binding:"required"`
		ToSlotID string `json:"to_slot_id" binding:"required"`
		ToRoomID string `json:"to_room_id"`
		Reason   string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "entry_id, to_slot_id, dan reason wajib"}})
		return
	}
	p, err := h.svc.ProposeManualAdjustment(req.EntryID, req.ToSlotID, req.ToRoomID, req.Reason, c.GetString("user_id"))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": p})
}

// Get GET /api/solve-jobs/:id — status + progress (dipoll UI).
func (h *SolveHandler) Get(c *gin.Context) {
	job, err := h.svc.Get(c.Param("id"))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": job})
}

// List GET /api/solve-jobs?term_id=
func (h *SolveHandler) List(c *gin.Context) {
	list, err := h.svc.List(c.Query("term_id"))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

// Cancel POST /api/solve-jobs/:id/cancel
func (h *SolveHandler) Cancel(c *gin.Context) {
	if err := h.svc.Cancel(c.Param("id")); err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Entries GET /api/timetable?term_id= — hasil jadwal (draft).
func (h *SolveHandler) Entries(c *gin.Context) {
	termID := c.Query("term_id")
	if termID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "term_id wajib"}})
		return
	}
	list, err := h.svc.Entries(termID)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

// SetLocked PATCH /api/timetable/:id {locked:bool}
func (h *SolveHandler) SetLocked(c *gin.Context) {
	var req struct {
		Locked *bool `json:"locked"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Locked == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "field locked (bool) wajib"}})
		return
	}
	e, err := h.svc.SetLocked(c.Param("id"), *req.Locked)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": e})
}

// Publish POST /api/timetable/publish {term_id, name, note}
func (h *SolveHandler) Publish(c *gin.Context) {
	var req struct {
		TermID string `json:"term_id" binding:"required"`
		Name   string `json:"name"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "term_id wajib"}})
		return
	}
	actor := c.GetString("user_id")
	v, err := h.svc.Publish(req.TermID, req.Name, req.Note, actor)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": v})
}

// Versions GET /api/timetable/versions?term_id=
func (h *SolveHandler) Versions(c *gin.Context) {
	list, err := h.svc.Versions(c.Query("term_id"))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

// RoomSchedule GET /api/consumers/rooms/:id/schedule?day=1..7 — consumer/doorlock.
func (h *SolveHandler) RoomSchedule(c *gin.Context) {
	day, _ := strconv.Atoi(c.Query("day"))
	if day == 0 {
		day = int(time.Now().Weekday())
		if day == 0 {
			day = 7
		}
	}
	out, err := h.svc.RoomScheduleDay(c.Param("id"), day)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out, "day": day})
}

// RoomNow GET /api/consumers/rooms/:id/now — okupansi realtime (doorlock).
func (h *SolveHandler) RoomNow(c *gin.Context) {
	out, err := h.svc.RoomNow(c.Param("id"))
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// ExportXLSX GET /api/timetable/export.xlsx?term_id=&view=master|dosen|rombel|ruang
func (h *SolveHandler) ExportXLSX(c *gin.Context) {
	termID := c.Query("term_id")
	if termID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "term_id wajib"}})
		return
	}
	view := c.Query("view")
	if view == "" {
		view = "master"
	}
	f, name, err := h.svc.ExportXLSX(termID, view)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if err := f.Write(c.Writer); err != nil {
		_ = c.Error(err)
	}
}

// CalendarICS GET /api/timetable/calendar.ics?term_id=&scope=room|group|lecturer&scope_id=
// — feed kalender standar (Google/Apple Calendar) utk 1 ruang / rombel / dosen.
func (h *SolveHandler) CalendarICS(c *gin.Context) {
	termID, scope, scopeID := c.Query("term_id"), c.Query("scope"), c.Query("scope_id")
	if termID == "" || scopeID == "" || scope == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "term_id, scope (room|group|lecturer), scope_id wajib"}})
		return
	}
	ics, err := h.svc.BuildICS(termID, scope, scopeID)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.Header("Content-Type", "text/calendar; charset=utf-8")
	c.Header("Content-Disposition", `inline; filename="jadwal.ics"`)
	_, _ = c.Writer.WriteString(ics)
}

// IssueCalendarToken POST /api/calendar-tokens {scope, scope_id, label}
func (h *SolveHandler) IssueCalendarToken(c *gin.Context) {
	var req struct {
		Scope   string `json:"scope" binding:"required"`
		ScopeID string `json:"scope_id" binding:"required"`
		Label   string `json:"label"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "scope & scope_id wajib"}})
		return
	}
	t, err := h.svc.IssueCalendarToken(req.Scope, req.ScopeID, req.Label)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": t})
}

// ListCalendarTokens GET /api/calendar-tokens
func (h *SolveHandler) ListCalendarTokens(c *gin.Context) {
	list, err := h.svc.ListCalendarTokens()
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

// RevokeCalendarToken DELETE /api/calendar-tokens/:id
func (h *SolveHandler) RevokeCalendarToken(c *gin.Context) {
	if err := h.svc.RevokeCalendarToken(c.Param("id")); err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// TimetableView GET /api/timetable/view?term_id=&by=lecturer|group|room&id=
func (h *SolveHandler) TimetableView(c *gin.Context) {
	termID, by, id := c.Query("term_id"), c.Query("by"), c.Query("id")
	if termID == "" || id == "" || by == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "term_id, by (lecturer|group|room), id wajib"}})
		return
	}
	out, err := h.svc.TimetableView(termID, by, id)
	if err != nil {
		errJSON(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// PublicICS GET /calendar/:token — feed ICS tanpa header auth (Google Calendar).
func (h *SolveHandler) PublicICS(c *gin.Context) {
	ics, _, err := h.svc.ICSByToken(c.Param("token"))
	if err != nil {
		c.String(http.StatusNotFound, "token tidak valid / sudah dicabut")
		return
	}
	c.Header("Content-Type", "text/calendar; charset=utf-8")
	c.Header("Content-Disposition", `inline; filename="jadwal.ics"`)
	_, _ = c.Writer.WriteString(ics)
}

var _ = strconv.Itoa
