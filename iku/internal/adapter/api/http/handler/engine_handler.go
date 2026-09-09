package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/iku/internal/adapter/api/http/middleware"
	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	service "github.com/rama/b-wise/iku/internal/domain/service"
	"github.com/rama/b-wise/iku/internal/service/hcclient"
)

// EngineHandler — F1: formula (versioned + sandbox), targets, PK-lite, units proxy.
type EngineHandler struct {
	formula *service.FormulaService
	target  *service.TargetService
	dash    *service.DashboardService
	plans   *service.ActionPlanService
	hc      *hcclient.Client
}

func NewEngineHandler(f *service.FormulaService, t *service.TargetService, d *service.DashboardService, pl *service.ActionPlanService, hc *hcclient.Client) *EngineHandler {
	return &EngineHandler{formula: f, target: t, dash: d, plans: pl, hc: hc}
}

func actorOf(c *gin.Context) string {
	uid, _ := middleware.GetUserID(c)
	return uid
}

// ==================== FORMULAS ====================

// ListFormulas GET /api/indicators/:id/formulas?include_retired=
func (h *EngineHandler) ListFormulas(c *gin.Context) {
	list, err := h.formula.ListByIndicator(c.Param("id"), c.Query("include_retired") == "true")
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

// GetActiveFormula GET /api/indicators/:id/formulas/active
func (h *EngineHandler) GetActiveFormula(c *gin.Context) {
	f, err := h.formula.GetActive(c.Param("id"))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": f})
}

type CreateFormulaRequest struct {
	IndicatorID     string                   `json:"indicator_id" binding:"required"`
	Expression      string                   `json:"expression" binding:"required"`
	InputVariables  []entity.FormulaInputVar `json:"input_variables" binding:"required"`
	RoundingRule    string                   `json:"rounding_rule"`
	ValidationRules string                   `json:"validation_rules"`
	Notes           string                   `json:"notes"`
}

// CreateFormula POST /api/formulas
func (h *EngineHandler) CreateFormula(c *gin.Context) {
	var req CreateFormulaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	f, err := h.formula.Create(service.CreateFormulaInput(req), actorOf(c))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": f})
}

type TestFormulaRequest struct {
	Expression      string                   `json:"expression" binding:"required"`
	InputVariables  []entity.FormulaInputVar `json:"input_variables" binding:"required"`
	Inputs          map[string]float64       `json:"inputs"`
	RoundingRule    string                   `json:"rounding_rule"`
	ValidationRules string                   `json:"validation_rules"`
}

// TestFormula POST /api/formulas/test — sandbox, tanpa persist
func (h *EngineHandler) TestFormula(c *gin.Context) {
	var req TestFormulaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	res, err := h.formula.SandboxTest(req.Expression, req.InputVariables, req.Inputs, req.RoundingRule, req.ValidationRules)
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": res})
}

// ActivateFormula POST /api/formulas/:id/activate
func (h *EngineHandler) ActivateFormula(c *gin.Context) {
	f, err := h.formula.Activate(c.Param("id"), actorOf(c))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": f})
}

// RetireFormula POST /api/formulas/:id/retire
func (h *EngineHandler) RetireFormula(c *gin.Context) {
	if err := h.formula.Retire(c.Param("id")); err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "formula retired"})
}

// ==================== TARGETS ====================

// ListTargets GET /api/targets?indicator_id=&unit_id=&period_id=&status=
func (h *EngineHandler) ListTargets(c *gin.Context) {
	list, err := h.target.List(service.TargetFilter{
		IndicatorID: c.Query("indicator_id"),
		UnitID:      c.Query("unit_id"),
		PeriodID:    c.Query("period_id"),
		Status:      c.Query("status"),
	})
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list, "meta": gin.H{"total": len(list)}})
}

type UpsertTargetRequest struct {
	IndicatorID     string  `json:"indicator_id" binding:"required"`
	UnitID          string  `json:"unit_id" binding:"required"`
	UnitType        string  `json:"unit_type"`
	PeriodID        string  `json:"period_id" binding:"required"`
	TargetValue     float64 `json:"target_value"`
	AggregationRule string  `json:"aggregation_rule"`
	PKRefID         *string `json:"pk_ref_id"`
	Notes           string  `json:"notes"`
}

// UpsertTarget POST /api/targets
func (h *EngineHandler) UpsertTarget(c *gin.Context) {
	var req UpsertTargetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	t, err := h.target.Upsert(service.UpsertTargetInput(req), actorOf(c))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

// ApproveTarget POST /api/targets/:id/approve
func (h *EngineHandler) ApproveTarget(c *gin.Context) {
	if err := h.target.Approve(c.Param("id"), actorOf(c)); err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "target disetujui"})
}

// ==================== PK-LITE ====================

// ListPKDocuments GET /api/pk-documents?unit_id=&year=
func (h *EngineHandler) ListPKDocuments(c *gin.Context) {
	year := 0
	if y := c.Query("year"); y != "" {
		year, _ = strconv.Atoi(y)
	}
	list, err := h.target.ListPKDocuments(c.Query("unit_id"), year)
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

// CreatePKDocument POST /api/pk-documents
func (h *EngineHandler) CreatePKDocument(c *gin.Context) {
	var pk entity.PKDocument
	if err := c.ShouldBindJSON(&pk); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	if err := h.target.CreatePKDocument(&pk, actorOf(c)); err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": pk})
}

// ==================== UNITS PROXY (HC) ====================

// ListUnits GET /api/units?type=branch|department
func (h *EngineHandler) ListUnits(c *gin.Context) {
	if h.hc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": gin.H{"message": "HC client belum terkonfigurasi"}})
		return
	}
	var (
		list []hcclient.Unit
		err  error
	)
	switch c.DefaultQuery("type", "branch") {
	case "department":
		list, err = h.hc.Departments()
	default:
		list, err = h.hc.Branches()
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	// level institusi selalu tersedia
	units := []hcclient.Unit{{ID: "institution", Name: "Institusi (Universitas Binawan)"}}
	units = append(units, list...)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": units})
}

// ==================== DASHBOARD (F3) ====================

// Dashboard GET /api/dashboard?unit_id=&period_id= — ringkasan capaian unit (periode default: terbaru)
func (h *EngineHandler) Dashboard(c *gin.Context) {
	periodID := c.Query("period_id")
	if periodID == "" {
		var per entity.Period
		if err := h.dash.LatestPeriod(&per); err != nil {
			errStatus(c, err)
			return
		}
		periodID = per.ID
	}
	unitID := c.DefaultQuery("unit_id", "institution")
	var children []string
	if unitID == "institution" && h.hc != nil {
		if list, err := h.hc.Branches(); err == nil {
			children = idsOf(list)
		}
	}
	sum, err := h.dash.Build(unitID, periodID, children)
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": sum})
}

// Trend GET /api/dashboard/trend?unit_id=&year=
func (h *EngineHandler) Trend(c *gin.Context) {
	year := 0
	if y := c.Query("year"); y != "" {
		year, _ = strconv.Atoi(y)
	}
	if year == 0 {
		year = time.Now().Year()
	}
	rows, err := h.dash.Trend(c.DefaultQuery("unit_id", "institution"), year)
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

// UnitComparison GET /api/dashboard/units?period_id=&units=a,b,c (default: periode terisi ter-advance)
func (h *EngineHandler) UnitComparison(c *gin.Context) {
	periodID := c.Query("period_id")
	if periodID == "" {
		var per entity.Period
		if err := h.dash.LatestPeriod(&per); err != nil {
			errStatus(c, err)
			return
		}
		periodID = per.ID
	}
	unitIDs := splitCSV(c.Query("units"))
	if len(unitIDs) == 0 {
		if h.hc != nil {
			if list, err := h.hc.Branches(); err == nil {
				unitIDs = append([]string{"institution"}, idsOf(list)...)
			}
		}
	}
	rows, err := h.dash.UnitComparison(periodID, unitIDs)
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

func splitCSV(s string) []string {
	var out []string
	cur := ""
	for _, ch := range s {
		if ch == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(ch)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func idsOf(us []hcclient.Unit) []string {
	ids := make([]string, 0, len(us))
	for _, u := range us {
		ids = append(ids, u.ID)
	}
	return ids
}

// ==================== ACTION PLANS (F4) ====================

// ListActionPlans GET /api/action-plans?achievement_id=&status=&overdue=
func (h *EngineHandler) ListActionPlans(c *gin.Context) {
	list, err := h.plans.List(service.ActionPlanFilter{
		AchievementID: c.Query("achievement_id"),
		IndicatorID:   c.Query("indicator_id"),
		UnitID:        c.Query("unit_id"),
		Status:        c.Query("status"),
		OnlyOverdue:   c.Query("overdue") == "true",
	})
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list, "meta": gin.H{"total": len(list)}})
}

type CreateActionPlanRequest struct {
	AchievementID    string                  `json:"achievement_id" binding:"required"`
	ProblemStatement string                  `json:"problem_statement" binding:"required"`
	Items            []entity.ActionPlanItem `json:"items" binding:"required"`
	PICUserID        *string                 `json:"pic_user_id"`
	Deadline         *time.Time              `json:"deadline"`
}

// CreateActionPlan POST /api/action-plans
func (h *EngineHandler) CreateActionPlan(c *gin.Context) {
	var req CreateActionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	ap, err := h.plans.Create(service.CreateActionPlanInput(req), actorOf(c))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": ap})
}

type UpdateActionPlanRequest struct {
	Items    []entity.ActionPlanItem `json:"items" binding:"required"`
	Deadline *time.Time              `json:"deadline"`
}

// UpdateActionPlan PUT /api/action-plans/:id (update items/progress)
func (h *EngineHandler) UpdateActionPlan(c *gin.Context) {
	var req UpdateActionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
		return
	}
	ap, err := h.plans.UpdateItems(c.Param("id"), req.Items, req.Deadline)
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": ap})
}

// EscalateActionPlan POST /api/action-plans/:id/escalate
func (h *EngineHandler) EscalateActionPlan(c *gin.Context) {
	ap, err := h.plans.Escalate(c.Param("id"), c.Query("note"))
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": ap})
}

// ActionPlanSuggestions GET /api/action-plans/suggestions?period_id=
func (h *EngineHandler) ActionPlanSuggestions(c *gin.Context) {
	periodID := c.Query("period_id")
	if periodID == "" {
		var per entity.Period
		if err := h.dash.LatestPeriod(&per); err != nil {
			errStatus(c, err)
			return
		}
		periodID = per.ID
	}
	list, err := h.plans.Suggestions(periodID)
	if err != nil {
		errStatus(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list, "meta": gin.H{"total": len(list)}})
}
