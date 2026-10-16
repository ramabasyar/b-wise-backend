package router

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/library/internal/adapter/api/http/handler"
	"github.com/rama/b-wise/library/internal/adapter/api/http/middleware"
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

	// Permission middleware — siap pakai (selaras Permission Service Tahap 1-3:
	// wildcard '*', batch check, cache invalidation otomatis).
	// Contoh wiring entity "items":
	//
	//	permCheck := middleware.NewPermissionCheck(permissionURL, serviceName).
	//		WithSSO(ssoURL, serviceClientID, serviceClientSecret)
	//	items := api.Group("/items")
	//	items.Use(jwksAuth.RequireAuth())
	//	items.Use(permCheck.CheckAccess())
	//	{
	//		items.GET("", permCheck.RequirePermission("items.read"), h.Item.List)
	//		items.POST("", permCheck.RequirePermission("items.write"), h.Item.Create)
	//		tools.GET("", permCheck.RequireAnyPermission("items.read", "items.admin"), h.Item.List)
	//	}
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

	// Permission middleware — siap pakai (selaras Permission Service Tahap 1-3:
	// wildcard '*', batch check, cache invalidation otomatis).
	// Contoh wiring entity "items":
	//
	//	permCheck := middleware.NewPermissionCheck(permissionURL, serviceName).
	//		WithSSO(ssoURL, serviceClientID, serviceClientSecret)
	//	items := api.Group("/items")
	//	items.Use(jwksAuth.RequireAuth())
	//	items.Use(permCheck.CheckAccess())
	//	{
	//		items.GET("", permCheck.RequirePermission("items.read"), h.Item.List)
	//		items.POST("", permCheck.RequirePermission("items.write"), h.Item.Create)
	//		tools.GET("", permCheck.RequireAnyPermission("items.read", "items.admin"), h.Item.List)
	//	}
	_ = api

	// ===== Binawan Library: operator/admin (JWT SSO + permission) =====
	libJWKS := middleware.NewJWKSAuth(ssoURL)
	libPerm := middleware.NewPermissionCheck(permissionURL, serviceName).
		WithSSO(ssoURL, serviceClientID, serviceClientSecret)
	if handlers != nil && handlers.Library != nil {
		h := handlers.Library
		api := r.Group("/api/library/v1")
		api.Use(libJWKS.RequireAuth())
		api.Use(libPerm.CheckAccess())
		{
			api.GET("/members", libPerm.RequirePermission("library.members.read"), h.ListMembers)
			api.POST("/members", libPerm.RequirePermission("library.members.create"), h.CreateMember)
			api.GET("/members/:id", libPerm.RequirePermission("library.members.read"), h.GetMember)
			api.PATCH("/members/:id", libPerm.RequirePermission("library.members.update"), h.UpdateMember)
			api.GET("/members/:id/cards", libPerm.RequirePermission("library.members.read"), h.CardHistory)
			api.POST("/members/:id/cards", libPerm.RequirePermission("library.cards.enroll"), h.EnrollCard)
			api.POST("/cards/:id/revoke", libPerm.RequirePermission("library.cards.revoke"), h.RevokeCard)
			api.GET("/visits", libPerm.RequirePermission("library.visits.read"), h.ListVisits)
			api.POST("/visits/manual", libPerm.RequirePermission("library.visits.create"), h.ManualVisit)
			api.PATCH("/visits/:id", libPerm.RequirePermission("library.visits.correct"), h.CorrectVisit)
			api.POST("/visits/:id/void", libPerm.RequirePermission("library.visits.void"), h.VoidVisit)
			api.GET("/guests", libPerm.RequirePermission("library.visits.read"), h.ListGuests)
			api.POST("/guests", libPerm.RequirePermission("library.guest_passes.issue"), h.CreateGuestWithPass)
			api.POST("/guests/:id/passes", libPerm.RequirePermission("library.guest_passes.issue"), h.IssuePass)
			api.POST("/guest-passes/:id/revoke", libPerm.RequirePermission("library.guest_passes.revoke"), h.RevokePass)
			api.GET("/purposes", libPerm.RequirePermission("library.visits.read"), h.ListPurposes)
			api.GET("/locations", libPerm.RequirePermission("library.locations.manage"), h.ListLocations)
			api.GET("/reports/summary", libPerm.RequirePermission("library.reports.read"), h.ReportSummary)
			api.GET("/reports/timeseries", libPerm.RequirePermission("library.reports.read"), h.ReportTimeseries)
			api.GET("/reports/breakdown", libPerm.RequirePermission("library.reports.read"), h.ReportBreakdown)
			api.GET("/reports/export.csv", libPerm.RequirePermission("library.reports.export"), h.ReportExport)
			api.POST("/kiosks", libPerm.RequirePermission("library.kiosks.manage"), h.ProvisionKiosk)
			api.GET("/kiosks", libPerm.RequirePermission("library.kiosks.manage"), h.ListKiosks)
			api.GET("/downloads/visitor-app", libPerm.RequirePermission("library.kiosks.manage"), h.DownloadVisitorApp)
		}
	}

	// ===== Binawan Library: kiosk (device credential) =====
	if handlers != nil && handlers.Kiosk != nil && handlers.KioskAuth != nil {
		k := r.Group("/api/library/kiosk/v1")
		k.Use(handlers.KioskAuth.Require())
		{
			k.POST("/check-ins", handlers.Kiosk.CheckIn)
			k.POST("/heartbeat", handlers.Kiosk.Heartbeat)
			k.GET("/config", handlers.Kiosk.Config)
		}
	}

	return r
}

// Handlers holds all route handlers for the service.
// Add your handler fields here when creating new entities.
type Handlers struct {
	// Item  *handler.ItemHandler  // sample
	_         struct{} // prevent empty struct warning
	Library   *handler.LibraryHandler
	Kiosk     *handler.KioskHandler
	KioskAuth *middleware.KioskAuth
}
