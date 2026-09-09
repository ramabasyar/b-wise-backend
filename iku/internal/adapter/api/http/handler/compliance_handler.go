package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	service "github.com/rama/b-wise/iku/internal/domain/service"
)

// ComplianceHandler — F6: regulation change workflow + impact analyzer + monitor.
type ComplianceHandler struct {
	cmp *service.ComplianceService
}

func NewComplianceHandler(c *service.ComplianceService) *ComplianceHandler {
	return &ComplianceHandler{cmp: c}
}

// ListChanges GET /api/regulation-changes?include_inactive=
func (h *ComplianceHandler) ListChanges(c *gin.Context) {
	list, err := h.cmp.ListChanges(c.Query("include_inactive") == "true")
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

// GetChange GET /api/regulation-changes/:id
func (h *ComplianceHandler) GetChange(c *gin.Context) {
	rc, err := h.cmp.GetChange(c.Param("id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rc})
}

type CreateChangeRequest struct {
	Title         string `json:"title" binding:"required"`
	NewRegCode    string `json:"new_reg_code"`
	NewRegTitle   string `json:"new_reg_title"`
	EffectiveDate string `json:"effective_date" binding:"required"` // YYYY-MM-DD
	Notes         string `json:"notes"`
	Items         []struct {
		IndicatorID       string                   `json:"indicator_id" binding:"required"`
		NewExpression     string                   `json:"new_expression"`
		NewInputVariables []entity.FormulaInputVar `json:"new_input_variables"`
		Notes             string                   `json:"notes"`
	} `json:"items" binding:"required,min=1"`
}

// CreateChange POST /api/regulation-changes
func (h *ComplianceHandler) CreateChange(c *gin.Context) {
	var req CreateChangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	eff, err := time.Parse("2006-01-02", req.EffectiveDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "effective_date format YYYY-MM-DD"}})
		return
	}
	in := service.CreateChangeInput{
		Title: req.Title, NewRegCode: req.NewRegCode, NewRegTitle: req.NewRegTitle,
		EffectiveDate: eff, Notes: req.Notes,
	}
	for _, it := range req.Items {
		in.Items = append(in.Items, service.RegulationChangeItemInput{
			IndicatorID: it.IndicatorID, NewExpression: it.NewExpression,
			NewInputVariables: it.NewInputVariables, Notes: it.Notes,
		})
	}
	rc, err := h.cmp.CreateChange(in, actorOf(c))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": rc})
}

type ChangeTransitionRequest struct {
	To    string `json:"to" binding:"required"`
	Notes string `json:"notes"`
}

// TransitionChange POST /api/regulation-changes/:id/transition (in_review|approved|active|rejected)
func (h *ComplianceHandler) TransitionChange(c *gin.Context) {
	var req ChangeTransitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	rc, err := h.cmp.TransitionChange(c.Param("id"), req.To, actorOf(c), req.Notes)
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rc})
}

// AnalyzeImpact GET /api/regulation-changes/:id/impact — simulasi (tanpa menyimpan)
func (h *ComplianceHandler) AnalyzeImpact(c *gin.Context) {
	rep, err := h.cmp.AnalyzeImpact(c.Param("id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rep})
}

// Monitor GET /api/compliance?period_id= — IKU wajib: formula/target/capaian/published
func (h *ComplianceHandler) Monitor(c *gin.Context) {
	rep, err := h.cmp.Monitor(c.Query("period_id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rep})
}
