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

func SetupWithLogger(logger *zap.Logger, permHandler *handler.PermissionHandler, ssoURL, jwtSecret string, corsOrigins []string, arHandler *handler.AccessRequestHandler, superAdminIDs []string) *gin.Engine {
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

	// AdminGuard — endpoint tulis/manajemen HANYA untuk service token (machine-to-machine)
	// atau superadmin (SUPER_ADMIN_IDS). Menutup eskalasi privilege register-publik →
	// roles/assign (audit HC+SSO 24 Okt 2026, temuan C1). Endpoint baca dan access-requests
	// (alur approval by position) tetap terbuka untuk user terautentikasi.
	guard := middleware.NewAdminGuard(superAdminIDs).RequireServiceOrSuperAdmin()

	// Public endpoints
	r.GET("/health", handler.NewHealthHandler().GetHealth)

	if permHandler != nil {
		v1 := r.Group("/api")

		// Write endpoints - require authentication (service token or admin user)
		services := v1.Group("/services")
		services.Use(jwksAuth.RequireAuth())
		{
			services.POST("", guard, permHandler.CreateService)
			services.GET("", permHandler.ListServices)
			services.GET("/:id", permHandler.GetService)
			services.PUT("/:id", guard, permHandler.UpdateService)
			services.DELETE("/:id", guard, permHandler.DeleteService)
			// Self-sync: service mendaftarkan/mutakhirkan manifest-nya sendiri (service token only — dicek di handler)
			services.POST("/self/sync", permHandler.SelfSync)
			services.POST("/:id/activate", guard, permHandler.ActivateService)
			services.POST("/:id/deactivate", guard, permHandler.DeactivateService)
			services.POST("/:id/permissions", guard, permHandler.CreatePermission)
			services.GET("/:id/permissions", permHandler.ListPermissions)
			services.POST("/:id/permissions/seed", guard, permHandler.SeedPermissions)
			services.POST("/:id/access", guard, permHandler.GrantAccess)
		}

		permissions := v1.Group("/permissions")
		permissions.Use(jwksAuth.RequireAuth())
		{
			permissions.DELETE("/:id", guard, permHandler.DeletePermission)
		}

		access := v1.Group("/access")
		access.Use(jwksAuth.RequireAuth())
		{
			access.DELETE("/:id", guard, permHandler.RevokeAccess)
		}

		users := v1.Group("/users")
		users.Use(jwksAuth.RequireAuth())
		{
			users.GET("/:user_id/services", permHandler.GetUserServices)
			users.GET("/:user_id/services/:service_id/check", permHandler.CheckAccess)
			users.POST("/:user_id/permissions", guard, permHandler.GrantUserPermission)
			users.GET("/:user_id/permissions", permHandler.GetUserPermissions)
			users.GET("/:user_id/permissions/service/:service_id", permHandler.GetUserPermissionsByService)
			users.GET("/:user_id/permissions/check", permHandler.HasPermission)
			users.POST("/:user_id/permissions/check-batch", permHandler.HasPermissionBatch)
			users.GET("/:user_id/menu", permHandler.GetUserMenu)
		}

		userPerms := v1.Group("/user-permissions")
		userPerms.Use(jwksAuth.RequireAuth())
		{
			userPerms.DELETE("/:id", guard, permHandler.RevokeUserPermission)
		}

		// Role management
		roles := v1.Group("/roles")
		roles.Use(jwksAuth.RequireAuth())
		{
			roles.POST("", guard, permHandler.CreateRole)
			roles.GET("", permHandler.ListRoles)
			roles.GET("/:id", permHandler.GetRole)
			roles.PUT("/:id", guard, permHandler.UpdateRole)
			roles.DELETE("/:id", guard, permHandler.DeleteRole)
			roles.POST("/assign", guard, permHandler.AssignRole)
			roles.DELETE("/users/:user_id/roles/:role_id", guard, permHandler.RevokeRole)

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
				audit.GET("", guard, permHandler.ListAuditLogs)
			}

			// Menu items (per-service navigation)
			menu := v1.Group("/menu-items")
			menu.Use(jwksAuth.RequireAuth())
			{
				menu.POST("", guard, permHandler.CreateMenuItem)
				menu.GET("", permHandler.ListMenuItems)
				menu.PUT("/:id", guard, permHandler.UpdateMenuItem)
				menu.DELETE("/:id", guard, permHandler.DeleteMenuItem)
			}
			roles.GET("/users/:user_id", permHandler.GetUserRoles)
		}
	}

	return r
}
