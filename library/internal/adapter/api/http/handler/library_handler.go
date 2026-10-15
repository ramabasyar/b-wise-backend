package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rama/b-wise/library/internal/domain/repository"
	"github.com/rama/b-wise/library/internal/service"
	"github.com/rama/b-wise/library/pkg/response"
)

// ctxString membaca string dari gin context (user_id, kiosk_id, dll).
func ctxString(c *gin.Context, key string) string {
	if v, ok := c.Get(key); ok {
		if s, ok2 := v.(string); ok2 {
			return s
		}
	}
	return ""
}

func isSentinel(err, target error) bool { return errors.Is(err, target) }

// actorOf — identitas pelaku untuk audit ("user:<id>").
func actorOf(c *gin.Context) string {
	if id := ctxString(c, "user_id"); id != "" {
		return "user:" + id
	}
	return "user:unknown"
}

// mapErr memetakan sentinel error service ke HTTP status.
func mapErr(c *gin.Context, err error) {
	switch {
	case isSentinel(err, service.ErrValidation), isSentinel(err, service.ErrBadMethod):
		response.BadRequest(c, err.Error())
	case isSentinel(err, service.ErrNotFound), isSentinel(err, service.ErrUnknownCredential), isSentinel(err, service.ErrPassInvalid):
		response.NotFound(c, err.Error())
	case isSentinel(err, service.ErrMemberExists), isSentinel(err, service.ErrCardTaken):
		response.Conflict(c, err.Error())
	case isSentinel(err, service.ErrMemberInactive), isSentinel(err, service.ErrCardNotActive):
		response.Forbidden(c, err.Error())
	default:
		response.Error(c, err)
	}
}

// LibraryHandler — endpoint operator/admin (JWT SSO + permission).
type LibraryHandler struct {
	members   *service.MemberService
	visits    *service.VisitService
	guests    *service.GuestService
	kiosks    *service.KioskAdminService
	purposes  repository.VisitPurposeRepository
	locations repository.LocationRepository
}

func NewLibraryHandler(
	m *service.MemberService, v *service.VisitService, g *service.GuestService,
	k *service.KioskAdminService, p repository.VisitPurposeRepository, l repository.LocationRepository,
) *LibraryHandler {
	return &LibraryHandler{members: m, visits: v, guests: g, kiosks: k, purposes: p, locations: l}
}

// ===== Anggota =====

// ListMembers — GET /api/library/v1/members?search=&status=&page=&limit=
func (h *LibraryHandler) ListMembers(c *gin.Context) {
	f := repository.MemberFilter{
		Search: c.Query("search"),
		Status: c.Query("status"),
		Page:   atoiDefault(c.Query("page"), 1),
		Limit:  atoiDefault(c.Query("limit"), 20),
	}
	rows, total, err := h.members.List(f)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"data": rows, "total": total, "page": f.Page, "limit": f.Limit})
}

// CreateMember — POST /api/library/v1/members
func (h *LibraryHandler) CreateMember(c *gin.Context) {
	var in service.CreateMemberInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "payload tidak valid")
		return
	}
	m, err := h.members.Create(in, actorOf(c))
	if err != nil {
		mapErr(c, err)
		return
	}
	response.Created(c, m)
}

// GetMember — GET /api/library/v1/members/:id
func (h *LibraryHandler) GetMember(c *gin.Context) {
	m, err := h.members.Get(c.Param("id"))
	if err != nil {
		mapErr(c, err)
		return
	}
	cards, _ := h.members.CardHistory(m.ID)
	response.Success(c, gin.H{"member": m, "cards": cards})
}

// UpdateMember — PATCH /api/library/v1/members/:id (termasuk perubahan status)
func (h *LibraryHandler) UpdateMember(c *gin.Context) {
	var in struct {
		service.CreateMemberInput
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "payload tidak valid")
		return
	}
	actor := actorOf(c)
	m, err := h.members.Update(c.Param("id"), in.CreateMemberInput, actor)
	if err != nil {
		mapErr(c, err)
		return
	}
	if in.Status == "active" || in.Status == "inactive" {
		if m, err = h.members.SetStatus(m.ID, in.Status, actor); err != nil {
			mapErr(c, err)
			return
		}
	}
	response.Success(c, m)
}

// CardHistory — GET /api/library/v1/members/:id/cards
func (h *LibraryHandler) CardHistory(c *gin.Context) {
	cards, err := h.members.CardHistory(c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"data": cards})
}

// EnrollCard — POST /api/library/v1/members/:id/cards
func (h *LibraryHandler) EnrollCard(c *gin.Context) {
	var in service.EnrollCardInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "payload tidak valid")
		return
	}
	card, err := h.members.EnrollCard(c.Param("id"), in, actorOf(c))
	if err != nil {
		mapErr(c, err)
		return
	}
	response.Created(c, card)
}

// RevokeCard — POST /api/library/v1/cards/:id/revoke  {status?, reason}
func (h *LibraryHandler) RevokeCard(c *gin.Context) {
	var in struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&in)
	if in.Status == "" {
		in.Status = "revoked"
	}
	card, err := h.members.RevokeCard(c.Param("id"), in.Status, in.Reason, actorOf(c))
	if err != nil {
		mapErr(c, err)
		return
	}
	response.Success(c, card)
}

// ===== Kunjungan =====

// ListVisits — GET /api/library/v1/visits
func (h *LibraryHandler) ListVisits(c *gin.Context) {
	f := repository.VisitFilter{
		LocationID:  c.Query("location_id"),
		VisitorType: c.Query("visitor_type"),
		Status:      c.Query("status"),
		MemberID:    c.Query("member_id"),
		Search:      c.Query("search"),
		From:        c.Query("from"),
		To:          c.Query("to"),
		Page:        atoiDefault(c.Query("page"), 1),
		Limit:       atoiDefault(c.Query("limit"), 20),
	}
	rows, total, err := h.visits.List(f)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"data": rows, "total": total, "page": f.Page, "limit": f.Limit})
}

// ManualVisit — POST /api/library/v1/visits/manual
func (h *LibraryHandler) ManualVisit(c *gin.Context) {
	var in service.ManualVisitInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "payload tidak valid")
		return
	}
	v, err := h.visits.ManualCreate(in, actorOf(c), c.Query("location_id"))
	if err != nil {
		mapErr(c, err)
		return
	}
	response.Created(c, v)
}

// CorrectVisit — PATCH /api/library/v1/visits/:id
func (h *LibraryHandler) CorrectVisit(c *gin.Context) {
	var in service.CorrectVisitInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "payload tidak valid")
		return
	}
	v, err := h.visits.Correct(c.Param("id"), in, actorOf(c))
	if err != nil {
		mapErr(c, err)
		return
	}
	response.Success(c, v)
}

// VoidVisit — POST /api/library/v1/visits/:id/void  {reason}
func (h *LibraryHandler) VoidVisit(c *gin.Context) {
	var in struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Reason == "" {
		response.BadRequest(c, "alasan pembatalan wajib diisi")
		return
	}
	v, err := h.visits.Void(c.Param("id"), in.Reason, actorOf(c))
	if err != nil {
		mapErr(c, err)
		return
	}
	response.Success(c, v)
}

// ===== Tamu & pass =====

// ListGuests — GET /api/library/v1/guests
func (h *LibraryHandler) ListGuests(c *gin.Context) {
	rows, total, err := h.guests.ListGuests(c.Query("search"), atoiDefault(c.Query("page"), 1), atoiDefault(c.Query("limit"), 20))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"data": rows, "total": total})
}

// CreateGuestWithPass — POST /api/library/v1/guests (registrasi + pass sekali jalan)
func (h *LibraryHandler) CreateGuestWithPass(c *gin.Context) {
	var in service.CreateGuestInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "payload tidak valid")
		return
	}
	g, pass, err := h.guests.CreateWithPass(in, actorOf(c))
	if err != nil {
		mapErr(c, err)
		return
	}
	response.Created(c, gin.H{"guest": g, "pass": pass})
}

// IssuePass — POST /api/library/v1/guests/:id/passes  {ttl_hours?}
func (h *LibraryHandler) IssuePass(c *gin.Context) {
	var in struct {
		TTLHours int `json:"ttl_hours"`
	}
	_ = c.ShouldBindJSON(&in)
	pass, err := h.guests.IssuePass(c.Param("id"), in.TTLHours, actorOf(c))
	if err != nil {
		mapErr(c, err)
		return
	}
	response.Created(c, pass)
}

// RevokePass — POST /api/library/v1/guest-passes/:id/revoke
func (h *LibraryHandler) RevokePass(c *gin.Context) {
	if err := h.guests.RevokePass(c.Param("id"), actorOf(c)); err != nil {
		mapErr(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}

// ===== Master & kiosk =====

// ListPurposes — GET /api/library/v1/purposes
func (h *LibraryHandler) ListPurposes(c *gin.Context) {
	rows, err := h.purposes.ListActive()
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"data": rows})
}

// ListLocations — GET /api/library/v1/locations
func (h *LibraryHandler) ListLocations(c *gin.Context) {
	rows, err := h.locations.List()
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"data": rows})
}

// ProvisionKiosk — POST /api/library/v1/kiosks  {code, name, location_id}
func (h *LibraryHandler) ProvisionKiosk(c *gin.Context) {
	var in struct {
		Code       string `json:"code"`
		Name       string `json:"name"`
		LocationID string `json:"location_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "payload tidak valid")
		return
	}
	out, err := h.kiosks.Provision(in.Code, in.Name, in.LocationID, actorOf(c))
	if err != nil {
		mapErr(c, err)
		return
	}
	response.Created(c, out) // secret hanya tampil sekali di respons ini
}

// ListKiosks — GET /api/library/v1/kiosks
func (h *LibraryHandler) ListKiosks(c *gin.Context) {
	rows, err := h.kiosks.List()
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"data": rows})
}

var _ = strconv.Itoa // jaga import strconv bila atoiDefault dipindah
