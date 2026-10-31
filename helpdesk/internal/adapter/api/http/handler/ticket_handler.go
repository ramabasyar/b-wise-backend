package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
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
	c.JSON(errStatus(err), gin.H{"success": false, "error": gin.H{"message": err.Error()}})
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
