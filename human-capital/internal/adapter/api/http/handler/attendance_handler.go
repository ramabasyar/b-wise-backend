package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	service "github.com/rama/b-wise/human-capital/internal/domain/service"
	"github.com/rama/b-wise/human-capital/pkg/response"
)

// AttendanceHandler — presensi: check-in/out mandiri + admin (input manual, impor, rekap).
type AttendanceHandler struct {
	svc *service.AttendanceService
}

func NewAttendanceHandler(svc *service.AttendanceService) *AttendanceHandler {
	return &AttendanceHandler{svc: svc}
}

func attErrStatus(c *gin.Context, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, service.ErrAttNoProfile):
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	case errors.Is(err, service.ErrAttCheckedIn), errors.Is(err, service.ErrAttCheckedOut), errors.Is(err, service.ErrAttNoCheckIn):
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	}
}

// CheckIn POST /attendance/check-in
func (h *AttendanceHandler) CheckIn(c *gin.Context) {
	rec, err := h.svc.CheckIn(actor(c))
	if err != nil {
		attErrStatus(c, err)
		return
	}
	response.Success(c, rec)
}

// CheckOut POST /attendance/check-out
func (h *AttendanceHandler) CheckOut(c *gin.Context) {
	rec, err := h.svc.CheckOut(actor(c))
	if err != nil {
		attErrStatus(c, err)
		return
	}
	response.Success(c, rec)
}

// MyRecords GET /attendance/my?month=2026-11
func (h *AttendanceHandler) MyRecords(c *gin.Context) {
	list, err := h.svc.MyRecords(actor(c), c.Query("month"))
	if err != nil {
		attErrStatus(c, err)
		return
	}
	response.Success(c, list)
}

// ListRecords GET /attendance/records (admin)
func (h *AttendanceHandler) ListRecords(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	f := service.AttendanceFilter{
		EmployeeID: c.Query("employee_id"), From: c.Query("from"), To: c.Query("to"),
		Status: c.Query("status"), Page: page, Size: size,
	}
	list, total, err := h.svc.ListRecords(f)
	if err != nil {
		attErrStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list, "meta": gin.H{"page": page, "page_size": size, "total": total}})
}

type attManualDTO struct {
	EmployeeID string `json:"employee_id" binding:"required"`
	Date       string `json:"date" binding:"required"`
	CheckIn    string `json:"check_in"`
	CheckOut   string `json:"check_out"`
	Status     string `json:"status"`
	Notes      string `json:"notes"`
}

// UpsertManual POST /attendance/records (admin)
func (h *AttendanceHandler) UpsertManual(c *gin.Context) {
	var in attManualDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	rec, err := h.svc.UpsertManual(in.EmployeeID, in.Date, in.CheckIn, in.CheckOut, in.Status, in.Notes, actor(c))
	if err != nil {
		attErrStatus(c, err)
		return
	}
	response.Success(c, rec)
}

type attImportDTO struct {
	Rows []service.AttendanceImportRow `json:"rows" binding:"required"`
}

// Import POST /attendance/import (admin) — cocokkan pegawai via NIP.
func (h *AttendanceHandler) Import(c *gin.Context) {
	var in attImportDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	ok, fail, firstErr := h.svc.ImportRows(in.Rows)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"ok": ok, "fail": fail, "first_error": firstErr}})
}

// Summary GET /attendance/summary?month=2026-11 (admin)
func (h *AttendanceHandler) Summary(c *gin.Context) {
	list, err := h.svc.MonthlySummary(c.Query("month"))
	if err != nil {
		attErrStatus(c, err)
		return
	}
	response.Success(c, list)
}
