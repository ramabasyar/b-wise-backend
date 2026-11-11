package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	entity "github.com/rama/b-wise/human-capital/internal/domain/entity"
	service "github.com/rama/b-wise/human-capital/internal/domain/service"
	"github.com/rama/b-wise/human-capital/pkg/response"
)

// LeaveAdminHandler — kebijakan cuti, saldo (alokasi tahunan), tanggal blokir.
type LeaveAdminHandler struct {
	svc *service.LeaveAdminService
}

func NewLeaveAdminHandler(svc *service.LeaveAdminService) *LeaveAdminHandler {
	return &LeaveAdminHandler{svc: svc}
}

func adminLeaveErrStatus(c *gin.Context, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, service.ErrPolicyNotFound), errors.Is(err, service.ErrBlockDateInUse), errors.Is(err, service.ErrBalanceNotFound):
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	}
}

// ---------- Kebijakan ----------

func (h *LeaveAdminHandler) ListPolicies(c *gin.Context) {
	list, err := h.svc.ListPolicies()
	if err != nil {
		adminLeaveErrStatus(c, err)
		return
	}
	response.Success(c, list)
}

type policyUpsertDTO struct {
	Name              string   `json:"name" binding:"required"`
	EmploymentTypeID  *string  `json:"employment_type_id"`
	LeaveTypeID       string   `json:"leave_type_id" binding:"required"`
	AnnualDays        float64  `json:"annual_days"`
	CarryOverMax      float64  `json:"carry_over_max"`
	CarryExpiryMonths int      `json:"carry_expiry_months"`
	AllowHalfDay      bool     `json:"allow_half_day"`
	Active            bool     `json:"active"`
}

func (h *LeaveAdminHandler) UpsertPolicy(c *gin.Context) {
	var in policyUpsertDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	p := &entity.LeavePolicy{
		Name: in.Name, EmploymentTypeID: in.EmploymentTypeID, LeaveTypeID: in.LeaveTypeID,
		AnnualDays: in.AnnualDays, CarryOverMax: in.CarryOverMax, CarryExpiryMonths: in.CarryExpiryMonths,
		AllowHalfDay: in.AllowHalfDay, Active: in.Active,
	}
	if err := h.svc.UpsertPolicy(p); err != nil {
		adminLeaveErrStatus(c, err)
		return
	}
	response.Success(c, p)
}

func (h *LeaveAdminHandler) DeletePolicy(c *gin.Context) {
	if err := h.svc.DeletePolicy(c.Param("id")); err != nil {
		adminLeaveErrStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "kebijakan dihapus"})
}

// ---------- Tanggal blokir ----------

func (h *LeaveAdminHandler) ListBlockDates(c *gin.Context) {
	year, _ := strconv.Atoi(c.Query("year"))
	list, err := h.svc.ListBlockDates(year)
	if err != nil {
		adminLeaveErrStatus(c, err)
		return
	}
	response.Success(c, list)
}

type blockDateDTO struct {
	Date   string `json:"date" binding:"required"`
	Reason string `json:"reason" binding:"required"`
}

func (h *LeaveAdminHandler) CreateBlockDate(c *gin.Context) {
	var in blockDateDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	d, err := time.Parse("2006-01-02", in.Date)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "format tanggal harus YYYY-MM-DD"}})
		return
	}
	b := &entity.LeaveBlockDate{Date: d, Reason: in.Reason, Active: true}
	if err := h.svc.CreateBlockDate(b); err != nil {
		adminLeaveErrStatus(c, err)
		return
	}
	response.Success(c, b)
}

func (h *LeaveAdminHandler) DeleteBlockDate(c *gin.Context) {
	if err := h.svc.DeleteBlockDate(c.Param("id")); err != nil {
		adminLeaveErrStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "tanggal blokir dihapus"})
}

// ---------- Saldo / alokasi ----------

func (h *LeaveAdminHandler) ListBalances(c *gin.Context) {
	year, _ := strconv.Atoi(c.DefaultQuery("year", strconv.Itoa(time.Now().Year())))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	list, total, err := h.svc.ListBalances(year, c.Query("q"), page, size)
	if err != nil {
		adminLeaveErrStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list, "meta": gin.H{"page": page, "page_size": size, "total": total, "year": year}})
}

type balanceAdjustDTO struct {
	Entitled    *float64 `json:"entitled"`
	CarriedOver *float64 `json:"carried_over"`
}

func (h *LeaveAdminHandler) AdjustBalance(c *gin.Context) {
	var in balanceAdjustDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	if err := h.svc.AdjustBalance(c.Param("id"), in.Entitled, in.CarriedOver); err != nil {
		adminLeaveErrStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "saldo diperbarui"})
}

type generateDTO struct {
	Year int `json:"year" binding:"required"`
}

func (h *LeaveAdminHandler) GenerateBalances(c *gin.Context) {
	var in generateDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	created, err := h.svc.GenerateBalances(in.Year)
	if err != nil {
		adminLeaveErrStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "alokasi saldo dibuat", "data": gin.H{"created": created, "year": in.Year}})
}
