package handler

import (
	"net/http"
	"os"
	"strconv"
	"strings"

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
	reports  *service.ReportService
}

func NewKioskHandler(cs *service.CheckinService, k repository.KioskRepository, p repository.VisitPurposeRepository, r *service.ReportService) *KioskHandler {
	return &KioskHandler{checkin: cs, kiosks: k, purposes: p, reports: r}
}

func kioskFromContext(c *gin.Context) *entity.LibraryKiosk {
	return &entity.LibraryKiosk{
		Base:       entity.Base{ID: ctxString(c, "kiosk_id")},
		Code:       ctxString(c, "kiosk_code"),
		LocationID: ctxString(c, "location_id"),
	}
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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
		AppVersion string "json:\"app_version\""
	}
	_ = c.ShouldBindJSON(&body)
	k := kioskFromContext(c)
	if err := h.kiosks.Touch(k.ID, body.AppVersion); err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}

// Stats — GET /api/library/kiosk/v1/stats
// Ringkasan hari ini untuk layar kiosk (auth device kiosk).
func (h *KioskHandler) Stats(c *gin.Context) {
	out, err := h.reports.KioskDailyReport()
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}

// Config — GET /api/library/kiosk/v1/config
func (h *KioskHandler) Config(c *gin.Context) {
	purposes, err := h.purposes.ListActive()
	if err != nil {
		response.Error(c, err)
		return
	}
	type purposeOut struct {
		Code string "json:\"code\""
		Name string "json:\"name\""
	}
	out := make([]purposeOut, 0, len(purposes))
	for _, p := range purposes {
		out = append(out, purposeOut{Code: p.Code, Name: p.Name})
	}
	response.Success(c, gin.H{
		"kiosk_code":  ctxString(c, "kiosk_code"),
		"location_id": ctxString(c, "location_id"),
		"purposes":    out,
		"info": gin.H{
			"open_hours":   strings.Split(envDefault("LIBRARY_OPEN_HOURS", "Senin – Jumat: 08.00 – 16.00|Sabtu: 08.00 – 12.00"), "|"),
			"announcement": envDefault("LIBRARY_ANNOUNCEMENT", "Pendaftaran anggota baru dibuka sepanjang bulan Oktober."),
		},
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
