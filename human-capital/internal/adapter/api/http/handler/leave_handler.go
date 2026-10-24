package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rama/b-wise/human-capital/internal/adapter/api/http/middleware"
	entity "github.com/rama/b-wise/human-capital/internal/domain/entity"
	service "github.com/rama/b-wise/human-capital/internal/domain/service"
	"github.com/rama/b-wise/human-capital/pkg/response"
)

// LeaveHandler — cuti self-service + approval 1 level + admin (Fase 2 ROADMAP HC).
type LeaveHandler struct {
	svc *service.LeaveService
}

func NewLeaveHandler(svc *service.LeaveService) *LeaveHandler { return &LeaveHandler{svc: svc} }

type leaveSubmitDTO struct {
	LeaveTypeID string `json:"leave_type_id" binding:"required"`
	StartDate   string `json:"start_date" binding:"required"`
	EndDate     string `json:"end_date" binding:"required"`
	Reason      string `json:"reason"`
}

type leaveDecisionDTO struct {
	Action string `json:"action" binding:"required"`
	Note   string `json:"note"`
}

// leaveErrStatus — mapping error service → HTTP status.
func leaveErrStatus(c *gin.Context, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, service.ErrLeaveNotFound),
		errors.Is(err, service.ErrLeaveNoProfile),
		errors.Is(err, service.ErrLeaveTypeNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrLeaveNotApprover), errors.Is(err, service.ErrLeaveNotOwner):
		response.Forbidden(c, err.Error())
	case errors.Is(err, service.ErrLeaveTypeInUse):
		response.Conflict(c, err.Error())
	case errors.Is(err, service.ErrLeaveNoSupervisor),
		errors.Is(err, service.ErrLeaveNotPending),
		errors.Is(err, service.ErrLeaveRange),
		errors.Is(err, service.ErrLeaveTooLong),
		errors.Is(err, service.ErrLeaveReason),
		errors.Is(err, service.ErrLeaveOverlap),
		errors.Is(err, service.ErrLeaveInsufficient),
		errors.Is(err, service.ErrLeaveTypeInactive),
		errors.Is(err, service.ErrLeaveNumberGen):
		response.BadRequest(c, err.Error())
	default:
		response.Error(c, err)
	}
}

func leavePageParams(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}
	return page, size
}

func leaveListEnvelope(c *gin.Context, rows interface{}, total int64, page, size int) {
	response.Success(c, gin.H{"items": rows, "total": total, "page": page, "page_size": size})
}

// ==================== JENIS CUTI ====================

func (h *LeaveHandler) ListTypes(c *gin.Context) {
	rows, err := h.svc.ListTypes(boolQuery(c, "active_only", false))
	if err != nil {
		leaveErrStatus(c, err)
		return
	}
	response.Success(c, rows)
}

func (h *LeaveHandler) CreateType(c *gin.Context) {
	var t entity.LeaveType
	if err := c.ShouldBindJSON(&t); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.svc.CreateType(&t); err != nil {
		leaveErrStatus(c, err)
		return
	}
	response.Created(c, t)
}

func (h *LeaveHandler) UpdateType(c *gin.Context) {
	var t entity.LeaveType
	if err := c.ShouldBindJSON(&t); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	t.ID = c.Param("id")
	if err := h.svc.UpdateType(&t); err != nil {
		leaveErrStatus(c, err)
		return
	}
	response.Success(c, t)
}

func (h *LeaveHandler) DeleteType(c *gin.Context) {
	if err := h.svc.DeleteType(c.Param("id")); err != nil {
		leaveErrStatus(c, err)
		return
	}
	response.Success(c, gin.H{"message": "jenis cuti dihapus"})
}

// ==================== CUTI SAYA ====================

func (h *LeaveHandler) MySummary(c *gin.Context) {
	out, err := h.svc.MySummary(middleware.GetUserID(c))
	if err != nil {
		leaveErrStatus(c, err)
		return
	}
	response.Success(c, out)
}

func (h *LeaveHandler) MyRequests(c *gin.Context) {
	page, size := leavePageParams(c)
	rows, total, err := h.svc.MyRequests(middleware.GetUserID(c), c.Query("status"), page, size)
	if err != nil {
		leaveErrStatus(c, err)
		return
	}
	leaveListEnvelope(c, rows, total, page, size)
}

func (h *LeaveHandler) Submit(c *gin.Context) {
	var dto leaveSubmitDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	req, err := h.svc.Submit(middleware.GetUserID(c), dto.LeaveTypeID, dto.StartDate, dto.EndDate, dto.Reason)
	if err != nil {
		leaveErrStatus(c, err)
		return
	}
	response.Created(c, req)
}

func (h *LeaveHandler) Cancel(c *gin.Context) {
	if err := h.svc.Cancel(middleware.GetUserID(c), c.Param("id")); err != nil {
		leaveErrStatus(c, err)
		return
	}
	response.Success(c, gin.H{"message": "pengajuan dibatalkan"})
}

// ==================== APPROVAL ====================

func (h *LeaveHandler) MyApprovals(c *gin.Context) {
	page, size := leavePageParams(c)
	rows, total, err := h.svc.MyApprovals(middleware.GetUserID(c), c.Query("status"), page, size)
	if err != nil {
		leaveErrStatus(c, err)
		return
	}
	leaveListEnvelope(c, rows, total, page, size)
}

func (h *LeaveHandler) Decide(c *gin.Context) {
	var dto leaveDecisionDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if dto.Action != "approve" && dto.Action != "reject" {
		response.BadRequest(c, "action harus 'approve' atau 'reject'")
		return
	}
	req, err := h.svc.Decide(middleware.GetUserID(c), c.Param("id"), dto.Action, dto.Note)
	if err != nil {
		leaveErrStatus(c, err)
		return
	}
	response.Success(c, req)
}

// ==================== ADMIN ====================

func (h *LeaveHandler) AllRequests(c *gin.Context) {
	page, size := leavePageParams(c)
	rows, total, err := h.svc.ListAllRequests(c.Query("status"), c.Query("q"), page, size)
	if err != nil {
		leaveErrStatus(c, err)
		return
	}
	leaveListEnvelope(c, rows, total, page, size)
}
