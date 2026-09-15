package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/permission-service/internal/adapter/api/http/handler"
	"github.com/rama/b-wise/permission-service/internal/adapter/api/http/router"
	"github.com/rama/b-wise/permission-service/internal/adapter/cache"
	"github.com/rama/b-wise/permission-service/internal/adapter/config"
	"github.com/rama/b-wise/permission-service/internal/adapter/database"
	"github.com/rama/b-wise/permission-service/internal/adapter/logger"
	"github.com/rama/b-wise/permission-service/internal/adapter/persistence/postgres"
	"github.com/rama/b-wise/permission-service/internal/domain/entity"
	domainsvc "github.com/rama/b-wise/permission-service/internal/domain/service"
	"github.com/rama/b-wise/permission-service/internal/service/permissionsync"
	"gorm.io/gorm"
)

func main() {
	// Load configuration (with .env support)
	cfg, err := config.LoadEnv("configs/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	// Self-register ke Permission Service = diri sendiri (best-effort, background + retry).
	// Membaca perm.manifest.yaml — sumber kebenaran permissions & menu service ini,
	// termasuk menu admin permission-service sendiri (services/roles/users/audit/menu).
	if m, err := permissionsync.LoadManifest("perm.manifest.yaml"); err != nil {
		log.Printf("[perm-sync] manifest: %v (self-sync dilewati)", err)
	} else {
		syncer := permissionsync.New(cfg.SSO, m, log.Printf)
		if syncer.Enabled() {
			syncer.RunInBackground(nil)
		} else {
			log.Printf("[perm-sync] dinonaktifkan (manifest sync_enabled=false atau config SSO belum lengkap)")
		}
	}

	log.Println("Starting permission service...")

	// Initialize logger
	zapLogger, err := logger.New(logger.Config{
		Level:  cfg.Logger.Level,
		Format: cfg.Logger.Format,
	})
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer zapLogger.Sync()
	zapLogger.Info("[startup] configuration loaded")

	// Connect database
	var db *gorm.DB
	var permHandler *handler.PermissionHandler
	var arHandlerLocal *handler.AccessRequestHandler

	if cfg.Database.Host != "" {
		db, err = database.New(cfg.Database)
		if err != nil {
			zapLogger.Info(fmt.Sprintf("[startup] database skipped: %v", err))
		} else {
			defer database.Close(db)
			zapLogger.Info("[startup] database connected")

			// Run migrations
			if err := database.AutoMigrate(db,
				&entity.Service{},
				&entity.ServicePermission{},
				&entity.ServiceAccess{},
				&entity.UserPermission{},
				&entity.Role{},
				&entity.UserRole{},
				&entity.MenuItem{},
				&entity.AuditLog{},
				&entity.AccessRequest{},
			); err != nil {
				zapLogger.Warn(fmt.Sprintf("[startup] migration failed: %v", err))
			} else {
				zapLogger.Info("[startup] database migrated")
			}

			// Initialize repositories
			// Initialize cache (Redis) - optional, dipakai oleh PermissionService
			var redisCache *cache.Cache
			if cfg.Redis.Host != "" {
				redisCache, err = cache.New(cache.Config{
					Host:     cfg.Redis.Host,
					Port:     cfg.Redis.Port,
					Password: cfg.Redis.Password,
					DB:       cfg.Redis.DB,
				})
				if err != nil {
					zapLogger.Info(fmt.Sprintf("[startup] cache skipped: %v", err))
				} else {
					defer redisCache.Close()
					zapLogger.Info("[startup] cache connected")
				}
			}

			serviceRepo := postgres.NewServiceRepository(db)
			permRepo := postgres.NewServicePermissionRepository(db)
			accessRepo := postgres.NewServiceAccessRepository(db)
			userPermRepo := postgres.NewUserPermissionRepository(db)
			roleRepo := postgres.NewRoleRepository(db)
			userRoleRepo := postgres.NewUserRoleRepository(db)
			menuRepo := postgres.NewMenuItemRepository(db)
			auditRepo := postgres.NewAuditLogRepository(db)

			// Initialize service — SUPER_ADMIN_IDS: bypass akses/permission semua service
			permSvc := domainsvc.NewPermissionService(cfg.SSO.SuperAdminIDs, serviceRepo, permRepo, accessRepo, userPermRepo, roleRepo, userRoleRepo, menuRepo, redisCache, auditRepo)

			// Initialize handler
			permHandler = handler.NewPermissionHandler(permSvc)
			arHandlerLocal = handler.NewAccessRequestHandler(db, permSvc, cfg.SSO.HumanCapitalURL)

			zapLogger.Info("[startup] repositories and services initialized")
		}
	}

	// Setup router
	zapGlobal := zapLogger.SugaredLogger.Desugar()
	hcURL := cfg.SSO.HumanCapitalURL
	if hcURL == "" {
		hcURL = "http://127.0.0.1:8081"
	}
	if cfg.SSO.HumanCapitalURL == "" {
		cfg.SSO.HumanCapitalURL = "http://127.0.0.1:8081"
	}
	if arHandlerLocal == nil {
		arHandlerLocal = handler.NewAccessRequestHandler(db, nil, cfg.SSO.HumanCapitalURL)
	}
	r := router.SetupWithLogger(zapGlobal, permHandler, cfg.SSO.URL, cfg.JWT.Secret, cfg.CORS.AllowedOrigins, arHandlerLocal)

	// Start server
	serverAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	zapLogger.Info(fmt.Sprintf("Server starting on %s", serverAddr))

	srv := &http.Server{
		Addr:         serverAddr,
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zapLogger.Info("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}
	zapLogger.Info("Server exited")

	// redisCache ditutup via defer saat init
}
