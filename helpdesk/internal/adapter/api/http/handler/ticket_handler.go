package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"github.com/rama/b-wise/helpdesk/internal/service/ticketsvc"
)

// TicketHandler — endpoint tiket F1 (pemohon self-service + petugas).
type TicketHandler struct {
	svc *ticketsvc.Service
}

func NewTicketHandler(svc *ticketsvc.Service) *TicketHandler { return &TicketHandler{svc: svc} }

func ctxUserID(c *gin.Context) string {
	v, ok := c.Get("user_id")
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func errStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, ticketsvc.ErrValidation):
		return http.StatusBadRequest
	case errors.Is(err, ticketsvc.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ticketsvc.ErrForbidden):
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

func fail(c *gin.Context, err error) {
	status := errStatus(err)
	message := err.Error()
	if status == http.StatusInternalServerError {
		_ = c.Error(err)
		message = "terjadi kesalahan internal"
	}
	c.JSON(status, gin.H{"success": false, "error": gin.H{"message": message}})
}

// Create — pemohon membuat tiket (PM-04/PM-06; tanpa CheckAccess, pola self-service).
func (h *TicketHandler) Create(c *gin.Context) {
	var req struct {
		ServiceID string `json:"service_id"`
		Subject   string `json:"subject" binding:"required"`
		Answers   string `json:"answers"`
		Channel   string `json:"channel"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	t, err := h.svc.Create(ticketsvc.CreateParams{
		RequesterID: ctxUserID(c),
		ServiceID:   req.ServiceID,
		Subject:     req.Subject,
		Answers:     req.Answers,
		Channel:     req.Channel,
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": t})
}

// Mine — daftar Tiket Saya (PM-05).
func (h *TicketHandler) Mine(c *gin.Context) {
	list, err := h.svc.ListMine(ctxUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": list}})
}

// Detail — tiket + timeline; catatan internal hanya untuk anggota tim (§9.3).
func (h *TicketHandler) Detail(c *gin.Context) {
	id := c.Param("id")
	uid := ctxUserID(c)
	t, err := h.svc.GetForViewer(id, uid)
	if err != nil {
		fail(c, err)
		return
	}
	member := h.svc.IsMember(t.TeamID, uid)
	events, err := h.svc.Events(id, member)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"ticket": t, "events": events, "is_team_member": member}})
}

// AddEvent — balasan publik (pemohon/petugas) atau catatan internal (petugas).
func (h *TicketHandler) AddEvent(c *gin.Context) {
	id := c.Param("id")
	uid := ctxUserID(c)
	var req struct {
		Kind string `json:"kind"` // reply | note
		Body string `json:"body" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	t, err := h.svc.GetForViewer(id, uid)
	if err != nil {
		fail(c, err)
		return
	}
	kind := "reply"
	visibility := "public"
	if req.Kind == "note" {
		if !h.svc.IsMember(t.TeamID, uid) {
			fail(c, ticketsvc.ErrForbidden)
			return
		}
		kind = "note"
		visibility = "internal"
	}
	if _, err := h.svc.AddEvent(id, uid, kind, visibility, req.Body); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true})
}

// ChangeStatus — pemohon: batalkan / konfirmasi selesai; petugas: semua transisi.
func (h *TicketHandler) ChangeStatus(c *gin.Context) {
	id := c.Param("id")
	uid := ctxUserID(c)
	var req struct {
		Status string `json:"status" binding:"required"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	t, err := h.svc.GetForViewer(id, uid)
	if err != nil {
		fail(c, err)
		return
	}
	if t.RequesterUserID == uid && !h.svc.IsMember(t.TeamID, uid) {
		// pemohon murni: hanya batalkan (belum selesai) atau konfirmasi tutup
		allowed := (req.Status == "dibatalkan") || (req.Status == "ditutup" && t.Status == "selesai")
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"message": "pemohon hanya bisa membatalkan atau mengonfirmasi selesai"}})
			return
		}
	}
	t, err = h.svc.ChangeStatus(id, uid, req.Status, req.Note)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

// Assign — koordinator menugaskan petugas (AG-03).
func (h *TicketHandler) Assign(c *gin.Context) {
	var req struct {
		AssigneeID string `json:"assignee_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	t, err := h.svc.Assign(c.Param("id"), ctxUserID(c), req.AssigneeID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

// Take — petugas mengambil tiket (AG-03).
func (h *TicketHandler) Take(c *gin.Context) {
	t, err := h.svc.Take(c.Param("id"), ctxUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

// Transfer — pindah tim dengan alasan; SLA tidak di-reset (AG-04/D-11).
func (h *TicketHandler) Transfer(c *gin.Context) {
	var req struct {
		ToTeamID string `json:"to_team_id" binding:"required"`
		Reason   string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	t, err := h.svc.Transfer(c.Param("id"), ctxUserID(c), req.ToTeamID, req.Reason)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

// Queue — antrean tim petugas (AG-01); tim dari keanggotaan user.
func (h *TicketHandler) Queue(c *gin.Context) {
	uid := ctxUserID(c)
	team, err := h.svc.TeamOf(uid)
	if err != nil {
		fail(c, err)
		return
	}
	filter := c.DefaultQuery("filter", "all")
	list, err := h.svc.Queue(team.ID, filter, uid)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"team": team.Code, "tickets": list}})
}

// Catalog — katalog layanan untuk semua akun login (PM-01/PM-02).
func (h *TicketHandler) Catalog(c *gin.Context) {
	list, err := h.svc.Catalog()
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": list}})
}

// Notifications - daftar notifikasi in-app user + jumlah belum dibaca (PM-07).
func (h *TicketHandler) Notifications(c *gin.Context) {
	list, unread, err := h.svc.Notifications(ctxUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": list, "unread": unread}})
}

// NotificationsRead - tandai notifikasi dibaca (ids kosong = semua).
func (h *TicketHandler) NotificationsRead(c *gin.Context) {
	var req struct {
		IDs []string `json:"ids"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.NotificationsRead(ctxUserID(c), req.IDs); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Teams - daftar tim aktif (pilihan transfer; semua akun login).
func (h *TicketHandler) Teams(c *gin.Context) {
	list, err := h.svc.Teams()
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": list}})
}

// TeamMembers - anggota aktif tim (pilihan penugasan; petugas).
func (h *TicketHandler) TeamMembers(c *gin.Context) {
	list, err := h.svc.TeamMembers(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": list}})
}

// ApprovalsInbox - kotak approval user (AP-03; self-service tanpa permission).
func (h *TicketHandler) ApprovalsInbox(c *gin.Context) {
	list, err := h.svc.ApprovalInbox(ctxUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": list}})
}

// ApprovalDecide - keputusan approver (AP-02).
func (h *TicketHandler) ApprovalDecide(c *gin.Context) {
	var req struct {
		TicketID string `json:"ticket_id" binding:"required"`
		Decision string `json:"decision" binding:"required"`
		Comment  string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	t, err := h.svc.Decide(req.TicketID, ctxUserID(c), req.Decision, req.Comment)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

// MyServices - layanan tim user utk admin unit (KT-04; termasuk draft).
func (h *TicketHandler) MyServices(c *gin.Context) {
	list, err := h.svc.MyServices(ctxUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": list}})
}

// CreateService - layanan baru (KT-04).
func (h *TicketHandler) CreateService(c *gin.Context) {
	var p entity.Service
	if err := c.ShouldBindJSON(&p); err != nil {
		fail(c, err)
		return
	}
	p.ID = ""
	svc, err := h.svc.CreateService(ctxUserID(c), p)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": svc})
}

// UpdateService - ubah layanan (field whitelist di service).
func (h *TicketHandler) UpdateService(c *gin.Context) {
	var fields map[string]interface{}
	if err := c.ShouldBindJSON(&fields); err != nil {
		fail(c, err)
		return
	}
	svc, err := h.svc.UpdateService(ctxUserID(c), c.Param("id"), fields)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": svc})
}

// Categories - daftar kategori katalog.
func (h *TicketHandler) Categories(c *gin.Context) {
	list, err := h.svc.Categories()
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"data": list}})
}

// CreateCategory - kategori baru (admin).
func (h *TicketHandler) CreateCategory(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
		Icon string `json:"icon"`
		Sort int    `json:"sort"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	cat, err := h.svc.CreateCategory(req.Name, req.Icon, req.Sort)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": cat})
}

// AddMember - tambah/aktifkan anggota tim (KT-03).
func (h *TicketHandler) AddMember(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id" binding:"required"`
		Role   string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	m, err := h.svc.AddMember(c.Param("id"), ctxUserID(c), req.UserID, req.Role)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": m})
}

// RemoveMember - nonaktifkan anggota tim (KT-03).
func (h *TicketHandler) RemoveMember(c *gin.Context) {
	if err := h.svc.RemoveMember(c.Param("id"), ctxUserID(c), c.Param("userId")); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Classify - front office menetapkan layanan tiket (FO-02).
func (h *TicketHandler) Classify(c *gin.Context) {
	var req struct {
		ServiceID string `json:"service_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	t, err := h.svc.Classify(c.Param("id"), ctxUserID(c), req.ServiceID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

// SubmitCSAT - pemohon mengisi survei kepuasan (PM-08).
func (h *TicketHandler) SubmitCSAT(c *gin.Context) {
	var p entity.CSATResponse
	if err := c.ShouldBindJSON(&p); err != nil {
		fail(c, err)
		return
	}
	p.ID = ""
	res, err := h.svc.SubmitCSAT(c.Param("id"), ctxUserID(c), p)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": res})
}

// ReportSummary - dashboard unit (LP-01).
func (h *TicketHandler) ReportSummary(c *gin.Context) {
	from, to := reportRange(c)
	r, err := h.svc.ReportSummaryFor(ctxUserID(c), from, to)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": r})
}

// ExportCSV - unduh CSV tiket periode (LP-02).
func (h *TicketHandler) ExportCSV(c *gin.Context) {
	from, to := reportRange(c)
	rows, err := h.svc.ExportCSV(ctxUserID(c), from, to)
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=tiket-pusat-layanan.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte(strings.Join(rows, "\n")))
}

func reportRange(c *gin.Context) (time.Time, time.Time) {
	to := time.Now()
	from := to.AddDate(0, 0, -30)
	if v := c.Query("from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			from = t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			to = t.Add(24 * time.Hour)
		}
	}
	return from, to
}
