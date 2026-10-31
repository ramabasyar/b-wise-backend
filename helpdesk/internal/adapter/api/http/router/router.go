package router

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/helpdesk/internal/adapter/api/http/handler"
	"github.com/rama/b-wise/helpdesk/internal/adapter/api/http/middleware"
	"go.uber.org/zap"
)

// Setup initializes all routes and returns a Gin engine
func Setup() *gin.Engine {
	r := gin.New()

	// Apply global middlewares
	r.Use(gin.Recovery())
	r.Use(middleware.NewCORSWithOrigins([]string{"*"}).Handle())
	r.Use(middleware.NewSecurityMiddleware(middleware.DefaultSecurityConfig()).Security())

	// Health check endpoint (public)
	healthHandler := handler.NewHealthHandler()
	r.GET("/health", healthHandler.GetHealth)

	// API routes (add your service-specific routes here)
	api := r.Group("/api")

	// Permission middleware - siap pakai (contoh wiring; versi aktif ada di SetupWithLogger).
	_ = api

	return r
}

// SetupWithLogger initializes routes with a zap logger and full middleware chain
// corsOrigins: allowed CORS origins from config (empty = wildcard *)
// ssoURL: SSO service URL for JWKS key fetching
// jwtSecret: HMAC fallback secret (must match SSO)
// permissionURL: Permission Service URL for access/permission checks
// serviceName: this service's registered name in Permission Service
// serviceClientID: OAuth2 client ID for service-to-service auth
// serviceClientSecret: OAuth2 client secret for service-to-service auth
func SetupWithLogger(
	logger *zap.Logger,
	handlers *Handlers,
	corsOrigins []string,
	ssoURL, jwtSecret, permissionURL, serviceName, serviceClientID, serviceClientSecret string,
) *gin.Engine {
	r := gin.New()

	// 1. Recovery (first — catch panics)
	r.Use(gin.Recovery())

	// 2. CORS from config
	if len(corsOrigins) == 0 {
		corsOrigins = []string{"*"}
	}
	r.Use(middleware.NewCORSWithOrigins(corsOrigins).Handle())

	// 3. Security headers
	r.Use(middleware.NewSecurityMiddleware(middleware.DefaultSecurityConfig()).Security())

	// 4. Request ID
	r.Use(func(c *gin.Context) {
		if id := c.GetHeader("X-Request-ID"); id != "" {
			middleware.SetRequestID(c, id)
		} else {
			middleware.SetRequestID(c, fmt.Sprintf("%d", time.Now().UnixNano()))
		}
		c.Writer.Header().Set("X-Request-ID", middleware.GetRequestID(c))
		c.Next()
	})

	// 5. Logging (with request ID)
	r.Use(middleware.NewLogger(logger).Log())

	// 6. JWKS Auth — validates both user JWT and service JWT via SSO
	jwksAuth := middleware.NewJWKSAuth(ssoURL)
	_ = jwtSecret // HMAC secret available if needed for local validation
	r.Use(jwksAuth.Middleware())

	// Health check (public — no auth needed)
	r.GET("/health", handler.NewHealthHandler().GetHealth)

	// API routes (protected by JWKS auth)

	api := r.Group("/api")
	api.Use(jwksAuth.RequireUser())

	// Permission middleware - selaras Permission Service Tahap 1-3.
	permCheck := middleware.NewPermissionCheck(permissionURL, serviceName).
		WithSSO(ssoURL, serviceClientID, serviceClientSecret)

	if handlers.Ticket != nil {
		v1 := api.Group("/v1")

		// Pemohon - self-service TANPA CheckAccess (pola endpoint cuti; blueprint 4.2).
		tk := v1.Group("/tickets")
		{
			tk.POST("", handlers.Ticket.Create)
			tk.GET("", handlers.Ticket.Mine) // Tiket Saya
			tk.GET("/:id", handlers.Ticket.Detail)
			tk.POST("/:id/events", handlers.Ticket.AddEvent)
			tk.POST("/:id/status", handlers.Ticket.ChangeStatus)

			// Petugas - butuh permission helpdesk.tickets.work.
			tk.POST("/:id/assign", permCheck.RequirePermission("helpdesk.tickets.work"), handlers.Ticket.Assign)
			tk.POST("/:id/take", permCheck.RequirePermission("helpdesk.tickets.work"), handlers.Ticket.Take)
			tk.POST("/:id/transfer", permCheck.RequirePermission("helpdesk.tickets.work"), handlers.Ticket.Transfer)
		}

		// Antrean tim petugas - helpdesk.queue.read.
		v1.GET("/queue", permCheck.RequirePermission("helpdesk.queue.read"), handlers.Ticket.Queue)

		// Katalog layanan — semua akun login (PM-01/PM-02, tanpa CheckAccess).
		v1.GET("/catalog", handlers.Ticket.Catalog)

		// Petugas: daftar tim (transfer) & anggota (penugasan).
		v1.GET("/teams", permCheck.RequirePermission("helpdesk.tickets.work"), handlers.Ticket.Teams)
		v1.GET("/teams/:id/members", permCheck.RequirePermission("helpdesk.tickets.work"), handlers.Ticket.TeamMembers)

		// Kotak approval + keputusan (self-service; otorisasi via approver).
		v1.GET("/approvals/inbox", handlers.Ticket.ApprovalsInbox)
		v1.POST("/approvals/decide", handlers.Ticket.ApprovalDecide)

		// Notifikasi in-app (self-service; PM-07).
		v1.GET("/notifications", handlers.Ticket.Notifications)
		v1.POST("/notifications/read", handlers.Ticket.NotificationsRead)

		// Admin unit (KT-03/KT-04) - ber-permission.
		v1.GET("/services", permCheck.RequirePermission("helpdesk.catalog.manage"), handlers.Ticket.MyServices)
		v1.POST("/services", permCheck.RequirePermission("helpdesk.catalog.manage"), handlers.Ticket.CreateService)
		v1.PUT("/services/:id", permCheck.RequirePermission("helpdesk.catalog.manage"), handlers.Ticket.UpdateService)
		v1.GET("/categories", handlers.Ticket.Categories)
		v1.POST("/categories", permCheck.RequirePermission("helpdesk.catalog.manage"), handlers.Ticket.CreateCategory)
		v1.POST("/teams/:id/members", permCheck.RequirePermission("helpdesk.teams.manage"), handlers.Ticket.AddMember)
		v1.DELETE("/teams/:id/members/:userId", permCheck.RequirePermission("helpdesk.teams.manage"), handlers.Ticket.RemoveMember)

		// Front office & survei & laporan.
		v1.POST("/tickets/:id/classify", permCheck.RequirePermission("helpdesk.tickets.work"), handlers.Ticket.Classify)
		v1.POST("/tickets/:id/csat", handlers.Ticket.SubmitCSAT)
		v1.GET("/reports/summary", permCheck.RequirePermission("helpdesk.reports.read"), handlers.Ticket.ReportSummary)
		v1.GET("/reports/export", permCheck.RequirePermission("helpdesk.reports.read"), handlers.Ticket.ExportCSV)

		// Basis pengetahuan (blueprint 6.9): baca = semua akun login; kelola = kb.manage.
		kb := v1.Group("/kb")
		{
			kb.GET("", handlers.Ticket.KBSearch)
			kb.GET("/manage", permCheck.RequirePermission("helpdesk.kb.manage"), handlers.Ticket.KBManageList)
			kb.POST("", permCheck.RequirePermission("helpdesk.kb.manage"), handlers.Ticket.KBCreate)
			kb.POST("/from-ticket", permCheck.RequirePermission("helpdesk.tickets.work"), handlers.Ticket.KBFromTicket)
			kb.GET("/:id", handlers.Ticket.KBGet)
			kb.PUT("/:id", permCheck.RequirePermission("helpdesk.kb.manage"), handlers.Ticket.KBUpdate)
			kb.POST("/:id/feedback", handlers.Ticket.KBFeedback)
		}

		// Formulir layanan (KT-05/KT-06): terbit = semua akun; kelola = catalog.manage.
		v1.GET("/services/:id/form", handlers.Ticket.FormGet)
		v1.GET("/services/:id/form/draft", permCheck.RequirePermission("helpdesk.catalog.manage"), handlers.Ticket.FormDraftGet)
		v1.PUT("/services/:id/form", permCheck.RequirePermission("helpdesk.catalog.manage"), handlers.Ticket.FormSave)
		v1.POST("/services/:id/form/publish", permCheck.RequirePermission("helpdesk.catalog.manage"), handlers.Ticket.FormPublish)

		// Sub-tugas lintas unit (AG-09/SL-06): baca = viewer tiket; kerja = tickets.work.
		v1.GET("/tickets/:id/tasks", handlers.Ticket.TasksGet)
		v1.POST("/tickets/:id/tasks", permCheck.RequirePermission("helpdesk.tickets.work"), handlers.Ticket.TaskAdd)
		v1.POST("/tickets/:id/tasks/:taskId/done", permCheck.RequirePermission("helpdesk.tickets.work"), handlers.Ticket.TaskDone)

		// Laporan lanjutan (LP-03..LP-06; blueprint 6.10).
		v1.GET("/reports/csat-lkps", permCheck.RequirePermission("helpdesk.reports.read"), handlers.Ticket.ReportCSATLKPS)
		v1.GET("/reports/services-performance", permCheck.RequirePermission("helpdesk.reports.read"), handlers.Ticket.ReportServicesPerformance)
		v1.GET("/reports/rooms", permCheck.RequirePermission("helpdesk.reports.read"), handlers.Ticket.ReportRooms)
		v1.GET("/reports/overview", permCheck.RequirePermission("helpdesk.reports.read"), handlers.Ticket.ReportOverview)

		// Kanal rahasia & PPKPT (RH-02..08, blueprint 6.3).
		v1.POST("/secret/reports", handlers.Ticket.SecretReport)
		v1.GET("/secret/track/:code", handlers.Ticket.SecretTrack)
		v1.POST("/tickets/:id/stage", permCheck.RequirePermission("helpdesk.tickets.work"), handlers.Ticket.SetStage)
		v1.GET("/tickets/:id/access-logs", permCheck.RequirePermission("helpdesk.tickets.work"), handlers.Ticket.AccessLogs)
		v1.GET("/reports/secret-aggregate", permCheck.RequirePermission("helpdesk.reports.read"), handlers.Ticket.SecretAggregate)

		// Otomatisasi tim & insiden massal (KT-10/AG-12).
		v1.GET("/automation-rules", permCheck.RequirePermission("helpdesk.teams.manage"), handlers.Ticket.AutomationRules)
		v1.POST("/automation-rules", permCheck.RequirePermission("helpdesk.teams.manage"), handlers.Ticket.AutomationRuleSave)
		v1.DELETE("/automation-rules/:id", permCheck.RequirePermission("helpdesk.teams.manage"), handlers.Ticket.AutomationRuleDelete)
		v1.POST("/tickets/:id/make-incident", permCheck.RequirePermission("helpdesk.tickets.work"), handlers.Ticket.MakeIncidentH)
		v1.GET("/tickets/:id/children", handlers.Ticket.IncidentChildrenH)
		v1.POST("/tickets/:id/children", permCheck.RequirePermission("helpdesk.tickets.work"), handlers.Ticket.AttachChildH)

		// API internal service-to-service (KN-08): token service, TANPA RequireUser.
		internalGrp := r.Group("/api/v1/internal")
		internalGrp.POST("/tickets", handlers.Ticket.InternalCreate)
	}
	_ = permCheck

	return r
}

// Handlers holds all route handlers for the service.
type Handlers struct {
	Ticket *handler.TicketHandler
}
