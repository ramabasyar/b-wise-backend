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
	"github.com/rama/b-wise/library/internal/adapter/api/http/handler"
	"github.com/rama/b-wise/library/internal/adapter/api/http/middleware"
	"github.com/rama/b-wise/library/internal/adapter/api/http/router"
	"github.com/rama/b-wise/library/internal/adapter/cache"
	"github.com/rama/b-wise/library/internal/adapter/config"
	"github.com/rama/b-wise/library/internal/adapter/database"
	"github.com/rama/b-wise/library/internal/adapter/logger"
	gormrepo "github.com/rama/b-wise/library/internal/adapter/repository/gorm"
	"github.com/rama/b-wise/library/internal/domain/entity"
	"github.com/rama/b-wise/library/internal/service"
	"github.com/rama/b-wise/library/internal/service/permissionsync"
	"github.com/rama/b-wise/library/internal/service/seed"
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

	// Self-register ke Permission Service (best-effort, background + retry).
	// Membaca perm.manifest.yaml — sumber kebenaran permissions & menu service ini.
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

	log.Println("Starting service...")

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

	// Connect database (optional — service can run without DB)
	var db *gorm.DB
	if cfg.Database.Host != "" {
		db, err = database.New(cfg.Database)
		if err != nil {
			zapLogger.Info(fmt.Sprintf("[startup] database skipped: %v", err))
		} else {
			defer database.Close(db)
			zapLogger.Info("[startup] database connected")

			err = database.AutoMigrate(db,
				&entity.LibraryLocation{},
				&entity.LibraryKiosk{},
				&entity.LibraryMember{},
				&entity.LibraryMemberCard{},
				&entity.LibraryMemberImportBatch{},
				&entity.LibraryGuest{},
				&entity.LibraryGuestPass{},
				&entity.LibraryVisitPurpose{},
				&entity.LibraryVisit{},
				&entity.LibraryAuditLog{},
			)
			if err != nil {
				log.Fatalf("auto-migrate library entities: %v", err)
			}
			zapLogger.Info("[startup] library schema migrated (10 tables)")
			seed.Defaults(db, func(f string, a ...interface{}) { zapLogger.Info(fmt.Sprintf(f, a...)) })
		}
	}

	// Initialize cache (Redis) — optional
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

	// ===== Initialize handlers =====
	// Create your handlers and pass them to the router:
	//
	// myHandler := handler.NewMyEntityHandler(myService)
	//
	// handlers := &router.Handlers{
	// 	// Item: myHandler,  // uncomment field in router.Handlers first
	// }
	handlers := &router.Handlers{}
	if db != nil {
		hmacKey := os.Getenv("LIBRARY_HMAC_KEY")
		if hmacKey == "" {
			hmacKey = cfg.JWT.Secret
			zapLogger.Info("[startup] LIBRARY_HMAC_KEY kosong — memakai JWT secret sebagai kunci HMAC kredensial")
		}
		memberRepo := gormrepo.NewMemberRepository(db)
		cardRepo := gormrepo.NewMemberCardRepository(db)
		guestRepo := gormrepo.NewGuestRepository(db)
		passRepo := gormrepo.NewGuestPassRepository(db)
		purposeRepo := gormrepo.NewVisitPurposeRepository(db)
		visitRepo := gormrepo.NewVisitRepository(db)
		locationRepo := gormrepo.NewLocationRepository(db)
		kioskRepo := gormrepo.NewKioskRepository(db)
		auditRepo := gormrepo.NewAuditLogRepository(db)
		reportSvc := service.NewReportService(db)

		handlers = &router.Handlers{
			Library: handler.NewLibraryHandler(
				service.NewMemberService(memberRepo, cardRepo, auditRepo, hmacKey),
				service.NewVisitService(visitRepo, memberRepo, purposeRepo, auditRepo),
				service.NewGuestService(guestRepo, passRepo, auditRepo, hmacKey),
				service.NewKioskAdminService(kioskRepo, auditRepo),
				purposeRepo,
				locationRepo,
				reportSvc,
			),
			Kiosk: handler.NewKioskHandler(
				service.NewCheckinService(memberRepo, cardRepo, guestRepo, passRepo, visitRepo, purposeRepo, locationRepo, auditRepo, hmacKey),
				kioskRepo,
				purposeRepo,
				reportSvc,
			),
			KioskAuth: middleware.NewKioskAuth(kioskRepo),
		}
	}

	// ===== Setup router =====
	zapGlobal := zapLogger.SugaredLogger.Desugar()
	r := router.SetupWithLogger(
		zapGlobal,
		handlers,
		cfg.CORS.AllowedOrigins,
		cfg.SSO.URL,
		cfg.JWT.Secret,
		cfg.SSO.PermissionServiceURL,
		cfg.SSO.ServiceName,
		cfg.SSO.ServiceClientID,
		cfg.SSO.ServiceClientSecret,
	)

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

	_ = db
	_ = redisCache
}
