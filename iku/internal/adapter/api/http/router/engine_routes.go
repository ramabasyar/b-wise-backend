package router

import (
	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/iku/internal/adapter/api/http/middleware"
)

// mountEngineRoutes — route F1 (Indicator Engine): formulas, targets, PK-lite, units.
// Dipanggil dari SetupWithLogger hanya jika Engine handler tersedia.
func (h *Handlers) mountEngineRoutes(api *gin.RouterGroup, jwksAuth *middleware.JWKSAuth, permCheck *middleware.PermissionCheck) {
	ind := api.Group("/indicators")
	ind.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
	{
		ind.GET("/:id/formulas", permCheck.RequirePermission("formulas.read"), h.Engine.ListFormulas)
		ind.GET("/:id/formulas/active", permCheck.RequirePermission("formulas.read"), h.Engine.GetActiveFormula)
	}

	formulas := api.Group("/formulas")
	formulas.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
	{
		formulas.POST("", permCheck.RequirePermission("formulas.write"), h.Engine.CreateFormula)
		formulas.POST("/test", permCheck.RequirePermission("formulas.write"), h.Engine.TestFormula) // sandbox — tanpa persist
		formulas.POST("/:id/activate", permCheck.RequirePermission("formulas.write"), h.Engine.ActivateFormula)
		formulas.POST("/:id/retire", permCheck.RequirePermission("formulas.write"), h.Engine.RetireFormula)
	}

	targets := api.Group("/targets")
	targets.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
	{
		targets.GET("", permCheck.RequirePermission("targets.read"), h.Engine.ListTargets)
		targets.POST("", permCheck.RequirePermission("targets.write"), h.Engine.UpsertTarget)
		targets.POST("/:id/approve", permCheck.RequirePermission("targets.write"), h.Engine.ApproveTarget)
	}

	pk := api.Group("/pk-documents")
	pk.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
	{
		pk.GET("", permCheck.RequirePermission("targets.read"), h.Engine.ListPKDocuments)
		pk.POST("", permCheck.RequirePermission("targets.write"), h.Engine.CreatePKDocument)
	}

	// ============ DASHBOARD (F3) ============
	dash := api.Group("/dashboard")
	dash.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
	{
		dash.GET("", permCheck.RequirePermission("dashboard.read"), h.Engine.Dashboard)
		dash.GET("/trend", permCheck.RequirePermission("dashboard.read"), h.Engine.Trend)
		dash.GET("/units", permCheck.RequirePermission("dashboard.read"), h.Engine.UnitComparison)
	}

	// ============ COMPLIANCE ENGINE (F6: change workflow + impact + monitor) ============
	if h.Compliance != nil {
		rc := api.Group("/regulation-changes")
		rc.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			rc.GET("", permCheck.RequirePermission("compliance.read"), h.Compliance.ListChanges)
			rc.POST("", permCheck.RequirePermission("compliance.write"), h.Compliance.CreateChange)
			rc.GET("/:id", permCheck.RequirePermission("compliance.read"), h.Compliance.GetChange)
			rc.POST("/:id/transition", permCheck.RequirePermission("compliance.write"), h.Compliance.TransitionChange)
			rc.GET("/:id/impact", permCheck.RequirePermission("compliance.read"), h.Compliance.AnalyzeImpact)
		}
		mon := api.Group("/compliance")
		mon.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			mon.GET("", permCheck.RequirePermission("compliance.read"), h.Compliance.Monitor)
		}
	}

	// ============ F7: EXPORTS + NOTIFICATIONS + METRICS ============
	if h.Export != nil {
		ex := api.Group("/exports")
		ex.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			ex.GET("/dashboard.xlsx", permCheck.RequirePermission("exports.read"), h.Export.DashboardExcel)
			ex.GET("/achievements.xlsx", permCheck.RequirePermission("exports.read"), h.Export.AchievementsExcel)
			ex.GET("/comparison.xlsx", permCheck.RequirePermission("exports.read"), h.Export.ComparisonExcel) // F10 YoY
			ex.GET("/report.pdf", permCheck.RequirePermission("exports.read"), h.Export.ReportPDF)
		}
	}
	if h.Notification != nil {
		nt := api.Group("/notifications")
		nt.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			nt.GET("", permCheck.RequirePermission("notifications.read"), h.Notification.List)
			nt.POST("/read-all", permCheck.RequirePermission("notifications.read"), h.Notification.ReadAll)
			nt.POST("/:id/read", permCheck.RequirePermission("notifications.read"), h.Notification.ReadOne)
		}
	}
	if h.Metrics != nil {
		api.GET("/metrics", jwksAuth.RequireAuth(), permCheck.CheckAccess(), permCheck.RequirePermission("indicators.read"), h.Metrics.Get)
	}

	// ============ DATA BROKER (F5: connectors + import terkelola) ============
	if h.Broker != nil {
		h.mountBrokerRoutes(api, jwksAuth, permCheck)
	}

	// ============ ACTION PLANS (F4) ============
	apGroup := api.Group("/action-plans")
	apGroup.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
	{
		apGroup.GET("", permCheck.RequirePermission("actionplans.read"), h.Engine.ListActionPlans)
		apGroup.POST("", permCheck.RequirePermission("actionplans.write"), h.Engine.CreateActionPlan)
		apGroup.GET("/suggestions", permCheck.RequirePermission("actionplans.read"), h.Engine.ActionPlanSuggestions)
		apGroup.PUT("/:id", permCheck.RequirePermission("actionplans.write"), h.Engine.UpdateActionPlan)
		apGroup.POST("/:id/escalate", permCheck.RequirePermission("actionplans.write"), h.Engine.EscalateActionPlan)
	}

	units := api.Group("/units")
	units.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
	{
		units.GET("", permCheck.RequirePermission("indicators.read"), h.Engine.ListUnits)
	}
}

// mountAchievementRoutes — F2 (capaian + workflow + evidence).
func (h *Handlers) mountAchievementRoutes(api *gin.RouterGroup, jwksAuth *middleware.JWKSAuth, permCheck *middleware.PermissionCheck) {
	ach := api.Group("/achievements")
	ach.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
	{
		ach.GET("", permCheck.RequirePermission("achievements.read"), h.Achievement.ListAchievements)
		ach.POST("", permCheck.RequirePermission("achievements.write"), h.Achievement.UpsertAchievement)
		ach.GET("/:id", permCheck.RequirePermission("achievements.read"), h.Achievement.GetAchievement)
		ach.POST("/:id/transition", permCheck.RequirePermission("achievements.workflow"), h.Achievement.TransitionAchievement)
		ach.POST("/:id/evidence", permCheck.RequirePermission("achievements.write"), h.Achievement.UploadEvidence)

		// evidence group terpisah agar :id tidak bentrok
		_ = ach
	}
	evGroup := api.Group("/evidence")
	evGroup.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
	{
		evGroup.GET("/:id", permCheck.RequirePermission("achievements.read"), h.Achievement.DownloadEvidence)
	}
}

// mountBrokerRoutes — F5: connector registry + preview/apply import.
func (h *Handlers) mountBrokerRoutes(api *gin.RouterGroup, jwksAuth *middleware.JWKSAuth, permCheck *middleware.PermissionCheck) {
	conn := api.Group("/connectors")
	conn.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
	{
		conn.GET("", permCheck.RequirePermission("connectors.read"), h.Broker.ListConnectors)
		conn.POST("", permCheck.RequirePermission("connectors.write"), h.Broker.UpsertConnector)
		conn.GET("/:id/template", permCheck.RequirePermission("connectors.read"), h.Broker.ConnectorTemplate)
		conn.POST("/:id/import/preview", permCheck.RequirePermission("imports.execute"), h.Broker.PreviewImport)
	}
	imports := api.Group("/imports")
	imports.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
	{
		imports.GET("", permCheck.RequirePermission("connectors.read"), h.Broker.ListImports)
		imports.POST("/:id/apply", permCheck.RequirePermission("imports.execute"), h.Broker.ApplyImport)
	}
}
