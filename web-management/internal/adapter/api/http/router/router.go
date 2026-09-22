package router

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/web-management/internal/adapter/api/http/handler"
	"github.com/rama/b-wise/web-management/internal/adapter/api/http/middleware"
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
	h := handlers
	permCheck := middleware.NewPermissionCheck(permissionURL, serviceName).
		WithSSO(ssoURL, serviceClientID, serviceClientSecret)


	// ===== BWM: API PUBLIK (read-only, tanpa auth; cache+ETag) =====
	if h.Public != nil {
		pub := r.Group("/api/v1/public")
		pub.GET("/posts", h.Public.Posts)
		pub.GET("/posts/:slug", h.Public.PostBySlug)
		pub.GET("/banners", h.Public.Banners)
		pub.GET("/events", h.Public.Events)
		pub.GET("/pages/:slug", h.Public.PageBySlug)
		pub.GET("/documents", h.Public.Documents)
	}

	// ===== BWM: ADMIN (auth JWKS + permission contents.*) =====
	if h.Content != nil {
		posts := api.Group("/posts")
		posts.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			posts.GET("", permCheck.RequirePermission("contents.read"), h.Content.ListPosts)
			posts.GET("/:id", permCheck.RequirePermission("contents.read"), h.Content.GetPost)
			posts.POST("", permCheck.RequirePermission("contents.write"), h.Content.CreatePost)
			posts.PUT("/:id", permCheck.RequirePermission("contents.write"), h.Content.UpdatePost)
			posts.DELETE("/:id", permCheck.RequirePermission("contents.write"), h.Content.DeletePost)
			posts.POST("/:id/publish", permCheck.RequirePermission("contents.write"), h.Content.PublishPost)
			posts.POST("/:id/archive", permCheck.RequirePermission("contents.write"), h.Content.ArchivePost)
		}

		events := api.Group("/events")
		events.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			events.GET("", permCheck.RequirePermission("contents.read"), h.Content.ListEvents)
			events.GET("/:id", permCheck.RequirePermission("contents.read"), h.Content.GetEvent)
			events.POST("", permCheck.RequirePermission("contents.write"), h.Content.CreateEvent)
			events.PUT("/:id", permCheck.RequirePermission("contents.write"), h.Content.UpdateEvent)
			events.DELETE("/:id", permCheck.RequirePermission("contents.write"), h.Content.DeleteEvent)
			events.POST("/:id/publish", permCheck.RequirePermission("contents.write"), h.Content.PublishEvent)
			events.POST("/:id/archive", permCheck.RequirePermission("contents.write"), h.Content.ArchiveEvent)
		}

		banners := api.Group("/banners")
		banners.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			banners.GET("", permCheck.RequirePermission("contents.read"), h.Content.ListBanners)
			banners.POST("", permCheck.RequirePermission("contents.write"), h.Content.CreateBanner)
			banners.PUT("/:id", permCheck.RequirePermission("contents.write"), h.Content.UpdateBanner)
			banners.DELETE("/:id", permCheck.RequirePermission("contents.write"), h.Content.DeleteBanner)
		}

		pages := api.Group("/pages")
		pages.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			pages.GET("", permCheck.RequirePermission("contents.read"), h.Content.ListPages)
			pages.GET("/:id", permCheck.RequirePermission("contents.read"), h.Content.GetPage)
			pages.POST("", permCheck.RequirePermission("contents.write"), h.Content.CreatePage)
			pages.PUT("/:id", permCheck.RequirePermission("contents.write"), h.Content.UpdatePage)
			pages.DELETE("/:id", permCheck.RequirePermission("contents.write"), h.Content.DeletePage)
			pages.POST("/:id/publish", permCheck.RequirePermission("contents.write"), h.Content.PublishPage)
		}

		documents := api.Group("/documents")
		documents.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			documents.GET("", permCheck.RequirePermission("contents.read"), h.Content.ListDocuments)
			documents.POST("", permCheck.RequirePermission("contents.write"), h.Content.CreateDocument)
			documents.PUT("/:id", permCheck.RequirePermission("contents.write"), h.Content.UpdateDocument)
			documents.DELETE("/:id", permCheck.RequirePermission("contents.write"), h.Content.DeleteDocument)
		}
	}


	return r
}

// Handlers holds all route handlers for the service.
type Handlers struct {
	Content *handler.ContentHandler // admin BWM (contents.*)
	Public  *handler.PublicHandler  // API publik read-only
}
