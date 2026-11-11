package main

import (
	"context"
	"fmt"
	"github.com/rama/b-wise/notify/internal/adapter/api/http/handler"
	"github.com/rama/b-wise/notify/internal/domain/entity"
	"github.com/rama/b-wise/notify/internal/service/notifsvc"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/notify/internal/adapter/api/http/router"
	"github.com/rama/b-wise/notify/internal/adapter/cache"
	"github.com/rama/b-wise/notify/internal/adapter/config"
	"github.com/rama/b-wise/notify/internal/adapter/database"
	"github.com/rama/b-wise/notify/internal/adapter/logger"
	"github.com/rama/b-wise/notify/internal/service/permissionsync"
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
		// Retry connect: host DB bisa belum siap saat semua VM boot bersamaan
		// (insiden 11 Nov 2026: service jalan degraded tanpa DB -> endpoint 404).
		// Retry sampai 60x10s; gagal terus -> Fatal (unit systemd Restart=always).
		for dbAttempt := 1; ; dbAttempt++ {
			db, err = database.New(cfg.Database)
			if err == nil {
				break
			}
			if dbAttempt >= 60 {
				zapLogger.Fatal(fmt.Sprintf("[startup] database unreachable after %d attempts: %v", dbAttempt, err))
			}
			zapLogger.Warn(fmt.Sprintf("[startup] database not ready (attempt %d/60), retrying in 10s: %v", dbAttempt, err))
			time.Sleep(10 * time.Second)
		}
		if err != nil {
			zapLogger.Info(fmt.Sprintf("[startup] database skipped: %v", err))
		} else {
			defer database.Close(db)
			zapLogger.Info("[startup] database connected")

			// Auto-migrate entitas notifikasi
			err = database.AutoMigrate(db, &entity.Outbox{}, &entity.PushSubscription{})
			if err != nil {
				zapLogger.Info(fmt.Sprintf("[startup] auto-migrate error: %v", err))
			} else {
				zapLogger.Info("[startup] auto-migrate OK")
			}
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
	// Notifikasi terpusat: service + worker outbox (fase-2 C)
	if db != nil {
		notifSvc := notifsvc.New(db)
		go notifSvc.RunInBackground(nil, 30*time.Second)
		handlers.Internal = handler.NewInternalHandler(notifSvc)
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
