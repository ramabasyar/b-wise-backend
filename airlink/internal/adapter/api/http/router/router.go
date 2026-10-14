package router

import (
	"fmt"
	"time"

	"airlink/internal/adapter/api/http/handler"
	"airlink/internal/adapter/api/http/middleware"
	"airlink/internal/adapter/logger"
	"github.com/gin-gonic/gin"
)

type Handlers struct {
	Airlink *handler.AirlinkHandler
	Cert    *handler.CertHandler
}

func SetupWithLogger(
	log *logger.Logger,
	handlers *Handlers,
	corsOrigins []string,
	ssoURL, jwtSecret, permissionURL, serviceName, serviceClientID, serviceClientSecret string,
) *gin.Engine {
	r := gin.New()

	// 1. Recovery
	r.Use(gin.Recovery())

	// 2. CORS
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

	// 5. JWKS Auth
	jwksAuth := middleware.NewJWKSAuth(ssoURL)
	r.Use(jwksAuth.Middleware())

	// Health check (public)
	r.GET("/health", handler.NewHealthHandler().GetHealth)

	// Permission middleware
	permCheck := middleware.NewPermissionCheck(permissionURL, serviceName).
		WithSSO(ssoURL, serviceClientID, serviceClientSecret)

	// API routes
	api := r.Group("/api")
	{
		airlink := api.Group("/airlink")
		{
			// Observability (Operator + Admin)
			airlink.GET("/health", permCheck.RequirePermission("wifi.health.read"), handlers.Airlink.GetHealth)
			airlink.GET("/sessions/active", permCheck.RequirePermission("wifi.sessions.read"), handlers.Airlink.GetActiveSessions)
			airlink.GET("/sessions/history", permCheck.RequirePermission("wifi.sessions.read"), handlers.Airlink.GetSessionHistory)
			airlink.GET("/auth-logs", permCheck.RequirePermission("wifi.authlogs.read"), handlers.Airlink.GetAuthLogs)

			// IT-only: issue dan unduh ulang dengan audit; revoke menunggu CoA.
			airlink.GET("/certs", permCheck.RequirePermission("wifi.cert.download"), handlers.Cert.ListCerts)
			airlink.POST("/certs", permCheck.RequirePermission("wifi.cert.issue"), handlers.Cert.IssueCert)
			airlink.GET("/certs/:username/:device/package", permCheck.RequirePermission("wifi.cert.download"), handlers.Cert.GetPackage)
			airlink.DELETE("/certs/:username/:device", permCheck.RequirePermission("wifi.cert.revoke"), handlers.Cert.RevokeCert)
		}
	}

	return r
}
