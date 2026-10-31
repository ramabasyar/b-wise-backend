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
	}
	_ = permCheck

	return r
}

// Handlers holds all route handlers for the service.
type Handlers struct {
	Ticket *handler.TicketHandler
}
