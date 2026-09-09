package router

import (
	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/permission-service/internal/adapter/api/http/handler"
	"github.com/rama/b-wise/permission-service/internal/adapter/api/http/middleware"
	"go.uber.org/zap"
)

func Setup() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.NewCORSWithOrigins([]string{"*"}).Handle())
	r.Use(middleware.NewSecurityMiddleware(middleware.DefaultSecurityConfig()).Security())
	r.GET("/health", handler.NewHealthHandler().GetHealth)
	return r
}

func SetupWithLogger(logger *zap.Logger, permHandler *handler.PermissionHandler, ssoURL, jwtSecret string, corsOrigins []string, arHandler *handler.AccessRequestHandler) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	if len(corsOrigins) == 0 {
		corsOrigins = []string{"*"}
	}
	r.Use(middleware.NewCORSWithOrigins(corsOrigins).Handle())
	r.Use(middleware.NewSecurityMiddleware(middleware.DefaultSecurityConfig()).Security())
	r.Use(middleware.NewLogger(logger).Log())

	// JWKS Auth middleware - validates both user and service tokens
	jwksAuth := middleware.NewJWKSAuth(ssoURL).WithHMACSecret(jwtSecret)
	r.Use(jwksAuth.Middleware())

	// Public endpoints
	r.GET("/health", handler.NewHealthHandler().GetHealth)

	if permHandler != nil {
		v1 := r.Group("/api")

		// Write endpoints - require authentication (service token or admin user)
		services := v1.Group("/services")
		services.Use(jwksAuth.RequireAuth())
		{
			services.POST("", permHandler.CreateService)
			services.GET("", permHandler.ListServices)
			services.GET("/:id", permHandler.GetService)
			services.PUT("/:id", permHandler.UpdateService)
			services.DELETE("/:id", permHandler.DeleteService)
			// Self-sync: service mendaftarkan/mutakhirkan manifest-nya sendiri (service token only)
			services.POST("/self/sync", permHandler.SelfSync)
			services.POST("/:id/activate", permHandler.ActivateService)
			services.POST("/:id/deactivate", permHandler.DeactivateService)
			services.POST("/:id/permissions", permHandler.CreatePermission)
			services.GET("/:id/permissions", permHandler.ListPermissions)
			services.POST("/:id/permissions/seed", permHandler.SeedPermissions)
			services.POST("/:id/access", permHandler.GrantAccess)
		}

		permissions := v1.Group("/permissions")
		permissions.Use(jwksAuth.RequireAuth())
		{
			permissions.DELETE("/:id", permHandler.DeletePermission)
		}

		access := v1.Group("/access")
		access.Use(jwksAuth.RequireAuth())
		{
			access.DELETE("/:id", permHandler.RevokeAccess)
		}

		users := v1.Group("/users")
		users.Use(jwksAuth.RequireAuth())
		{
			users.GET("/:user_id/services", permHandler.GetUserServices)
			users.GET("/:user_id/services/:service_id/check", permHandler.CheckAccess)
			users.POST("/:user_id/permissions", permHandler.GrantUserPermission)
			users.GET("/:user_id/permissions", permHandler.GetUserPermissions)
			users.GET("/:user_id/permissions/service/:service_id", permHandler.GetUserPermissionsByService)
			users.GET("/:user_id/permissions/check", permHandler.HasPermission)
			users.POST("/:user_id/permissions/check-batch", permHandler.HasPermissionBatch)
			users.GET("/:user_id/menu", permHandler.GetUserMenu)
		}

		userPerms := v1.Group("/user-permissions")
		userPerms.Use(jwksAuth.RequireAuth())
		{
			userPerms.DELETE("/:id", permHandler.RevokeUserPermission)
		}

		// Role management
		roles := v1.Group("/roles")
		roles.Use(jwksAuth.RequireAuth())
		{
			roles.POST("", permHandler.CreateRole)
			roles.GET("", permHandler.ListRoles)
			roles.GET("/:id", permHandler.GetRole)
			roles.PUT("/:id", permHandler.UpdateRole)
			roles.DELETE("/:id", permHandler.DeleteRole)
			roles.POST("/assign", permHandler.AssignRole)
			roles.DELETE("/users/:user_id/roles/:role_id", permHandler.RevokeRole)

			// Access requests (Fase B — approval by position)
		ars := v1.Group("/access-requests")
		ars.Use(jwksAuth.RequireAuth())
		{
			ars.POST("", arHandler.Create)
			ars.GET("/mine", arHandler.Mine)
			ars.GET("/pending", arHandler.Pending)
			ars.POST("/:id/decide", arHandler.Decide)
		}

		// Audit trail
		audit := v1.Group("/audit-logs")
		audit.Use(jwksAuth.RequireAuth())
		{
			audit.GET("", permHandler.ListAuditLogs)
		}

		// Menu items (per-service navigation)
			menu := v1.Group("/menu-items")
			menu.Use(jwksAuth.RequireAuth())
			{
				menu.POST("", permHandler.CreateMenuItem)
				menu.GET("", permHandler.ListMenuItems)
				menu.PUT("/:id", permHandler.UpdateMenuItem)
				menu.DELETE("/:id", permHandler.DeleteMenuItem)
			}
			roles.GET("/users/:user_id", permHandler.GetUserRoles)
		}
	}

	return r
}
