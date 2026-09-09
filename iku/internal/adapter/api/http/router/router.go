package router

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/iku/internal/adapter/api/http/handler"
	"github.com/rama/b-wise/iku/internal/adapter/api/http/middleware"
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

	h := handlers // alias agar blok route ringkas
	if h == nil || h.Iku == nil {
		return r
	}
	// F9: fallback — bila Period handler tidak di-wire, route period pakai Iku (list lama)
	h.periodList, h.periodGet, h.periodOpen, h.periodClose, h.periodRollup =
		func(c *gin.Context) {
			if h.Period != nil {
				h.Period.List(c)
				return
			}
			h.Iku.ListPeriods(c)
		},
		func(c *gin.Context) {
			if h.Period != nil {
				h.Period.Get(c)
				return
			}
			c.JSON(http.StatusNotImplemented, gin.H{"success": false, "error": gin.H{"message": "period handler tidak tersedia"}})
		},
		func(c *gin.Context) { h.Period.Open(c) },
		func(c *gin.Context) { h.Period.Close(c) },
		func(c *gin.Context) { h.Period.Rollup(c) }
	_ = h.Engine // boleh nil — route engine hanya terpasang jika ada

	permCheck := middleware.NewPermissionCheck(permissionURL, serviceName).
		WithSSO(ssoURL, serviceClientID, serviceClientSecret)

	// ============ REGULATIONS (Compliance Engine — registry) ============
	regs := api.Group("/regulations")
	regs.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
	{
		regs.GET("", permCheck.RequirePermission("regulations.read"), h.Iku.ListRegulations)
		regs.POST("", permCheck.RequirePermission("regulations.write"), h.Iku.CreateRegulation)
		regs.PATCH("/:id/verify", permCheck.RequirePermission("regulations.write"), h.Iku.VerifyRegulation) // F7 JDIH
	}

	// ============ INDICATORS ============
	indicators := api.Group("/indicators")
	indicators.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
	{
		indicators.GET("", permCheck.RequirePermission("indicators.read"), h.Iku.ListIndicators)
		indicators.GET("/:id", permCheck.RequirePermission("indicators.read"), h.Iku.GetIndicator)
		indicators.PATCH("/:id", permCheck.RequirePermission("indicators.write"), h.Iku.UpdateIndicator) // F10: polarity/rollup
	}

	if h.Engine != nil {
		h.mountEngineRoutes(api, jwksAuth, permCheck)
	}
	if h.Achievement != nil {
		h.mountAchievementRoutes(api, jwksAuth, permCheck)
	}
	// (definisi mountEngineRoutes ada di engine_routes.go)

	// ============ COMPARISONS (F10 YoY — read-only) ============
	if h.Comparison != nil {
		cmp := api.Group("/comparisons")
		cmp.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		cmp.GET("", permCheck.RequirePermission("dashboard.read"), h.Comparison.List)
	}

	// ============ PERIODS (F9 lifecycle) ============
	periods := api.Group("/periods")
	periods.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
	{
		periods.GET("", permCheck.RequirePermission("indicators.read"), h.periodList)
		periods.POST("", permCheck.RequirePermission("periods.write"), h.Iku.CreatePeriod)
		periods.GET("/:id", permCheck.RequirePermission("indicators.read"), h.periodGet)
		periods.POST("/:id/open", permCheck.RequirePermission("periods.write"), h.periodOpen)
		// tutup: reviewer BPM (achievements.workflow) ATAU admin (periods.write) — hybrid
		periods.POST("/:id/close", permCheck.RequireAnyPermission("periods.write", "achievements.workflow"), h.periodClose)
		periods.POST("/:id/rollup", permCheck.RequirePermission("periods.write"), h.periodRollup)
	}

	return r
}

// Handlers holds all route handlers for the service.
type Handlers struct {
	Iku          *handler.IkuHandler
	Period       *handler.PeriodHandler
	Comparison   *handler.ComparisonHandler
	periodList   gin.HandlerFunc // di-set dari SetupWithLogger (lihat helper di bawah)
	periodGet    gin.HandlerFunc
	periodOpen   gin.HandlerFunc
	periodClose  gin.HandlerFunc
	periodRollup gin.HandlerFunc
	Engine       *handler.EngineHandler
	Achievement  *handler.AchievementHandler
	Broker       *handler.BrokerHandler
	Compliance   *handler.ComplianceHandler
	Notification *handler.NotificationHandler
	Export       *handler.ExportHandler
	Metrics      *handler.MetricsHandler
}
