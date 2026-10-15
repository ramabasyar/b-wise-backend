package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rama/b-wise/library/internal/domain/entity"
	"github.com/rama/b-wise/library/internal/domain/repository"
	"github.com/rama/b-wise/library/internal/service"
	"github.com/rama/b-wise/library/pkg/response"
)

// KioskHandler — endpoint perangkat kiosk (device credential).
type KioskHandler struct {
	checkin  *service.CheckinService
	kiosks   repository.KioskRepository
	purposes repository.VisitPurposeRepository
}

func NewKioskHandler(cs *service.CheckinService, k repository.KioskRepository, p repository.VisitPurposeRepository) *KioskHandler {
	return &KioskHandler{checkin: cs, kiosks: k, purposes: p}
}

func kioskFromContext(c *gin.Context) *entity.LibraryKiosk {
	return &entity.LibraryKiosk{
		Base:       entity.Base{ID: ctxString(c, "kiosk_id")},
		Code:       ctxString(c, "kiosk_code"),
		LocationID: ctxString(c, "location_id"),
	}
}

// CheckIn — POST /api/library/kiosk/v1/check-ins
func (h *KioskHandler) CheckIn(c *gin.Context) {
	var in service.CheckinInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.BadRequest(c, "payload tidak valid")
		return
	}
	res, err := h.checkin.CheckIn(in, kioskFromContext(c))
	if err != nil {
		switch {
		case isSentinel(err, service.ErrRateLimited):
			c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "error": err.Error()})
		case isSentinel(err, service.ErrUnknownCredential), isSentinel(err, service.ErrPassInvalid):
			response.NotFound(c, err.Error())
		case isSentinel(err, service.ErrBadMethod), isSentinel(err, service.ErrValidation):
			response.BadRequest(c, err.Error())
		case isSentinel(err, service.ErrCardNotActive), isSentinel(err, service.ErrMemberInactive):
			response.Forbidden(c, err.Error())
		default:
			response.Error(c, err)
		}
		return
	}
	response.Success(c, res)
}

// Heartbeat — POST /api/library/kiosk/v1/heartbeat
func (h *KioskHandler) Heartbeat(c *gin.Context) {
	var body struct {
		AppVersion string `json:"app_version"`
	}
	_ = c.ShouldBindJSON(&body)
	k := kioskFromContext(c)
	if err := h.kiosks.Touch(k.ID, body.AppVersion); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}

// Config — GET /api/library/kiosk/v1/config
func (h *KioskHandler) Config(c *gin.Context) {
	purposes, err := h.purposes.ListActive()
	if err != nil {
		response.Error(c, err)
		return
	}
	type purposeOut struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	out := make([]purposeOut, 0, len(purposes))
	for _, p := range purposes {
		out = append(out, purposeOut{Code: p.Code, Name: p.Name})
	}
	response.Success(c, gin.H{
		"kiosk_code":  ctxString(c, "kiosk_code"),
		"location_id": ctxString(c, "location_id"),
		"purposes":    out,
	})
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return def
	}
	return n
}
