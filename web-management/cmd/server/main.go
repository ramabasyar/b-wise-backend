package main

import (
	"strings"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/rama/b-wise/web-management/internal/adapter/api/http/handler"
	"github.com/rama/b-wise/web-management/internal/adapter/api/http/router"
	"github.com/rama/b-wise/web-management/internal/adapter/cache"
	"github.com/rama/b-wise/web-management/internal/adapter/config"
	"github.com/rama/b-wise/web-management/internal/adapter/database"
	"github.com/rama/b-wise/web-management/internal/adapter/logger"
	"github.com/rama/b-wise/web-management/internal/adapter/storage"
	"github.com/rama/b-wise/web-management/internal/domain/entity"
	"github.com/rama/b-wise/web-management/internal/domain/service"
	"github.com/rama/b-wise/web-management/internal/service/permissionsync"
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

			// Auto-migrate entitas BWM
			err = database.AutoMigrate(db,
				&entity.Post{}, &entity.Page{}, &entity.Event{},
				&entity.Banner{}, &entity.MediaAsset{}, &entity.Document{},
			)
			if err != nil {
				zapLogger.Info(fmt.Sprintf("[startup] auto-migrate error: %v", err))
			} else {
				zapLogger.Info("[startup] auto-migrate OK (6 entitas BWM)")
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
	handlers := &router.Handlers{}

	// MinIO (media library) — optional
	var minioStore *storage.MinIO
	if cfg.MinIO.Endpoint != "" && cfg.MinIO.AccessKey != "" {
		minioStore, err = storage.New(storage.Config{
			Endpoint: cfg.MinIO.Endpoint, AccessKey: cfg.MinIO.AccessKey, SecretKey: cfg.MinIO.SecretKey,
			Bucket: cfg.MinIO.Bucket, UseSSL: cfg.MinIO.UseSSL, PublicBase: cfg.MinIO.PublicBase,
		})
		if err != nil {
			zapLogger.Info(fmt.Sprintf("[startup] minio skipped: %v", err))
			minioStore = nil
		} else if err := minioStore.EnsureBucket(context.Background()); err != nil {
			zapLogger.Info(fmt.Sprintf("[startup] minio ensure-bucket gagal: %v", err))
		} else {
			zapLogger.Info(fmt.Sprintf("[startup] minio OK (bucket %s)", cfg.MinIO.Bucket))
		}
	}

	// Webhook publish — dari env WEBHOOK_URLS (csv)
	var webhookURLs []string
	for _, u := range strings.Split(os.Getenv("WEBHOOK_URLS"), ",") {
		if u = strings.TrimSpace(u); u != "" {
			webhookURLs = append(webhookURLs, u)
		}
	}

	if db != nil {
		var redisClient *redis.Client
		if redisCache != nil {
			redisClient = redisCache.Client()
		}
		contentSvc := service.NewContentService(db, redisClient).WithWebhooks(webhookURLs)
		handlers = &router.Handlers{
			Content: handler.NewContentHandler(contentSvc),
			Public:  handler.NewPublicHandler(contentSvc, redisClient),
			Media:   handler.NewMediaHandler(service.NewMediaService(db, minioStore)),
		}
		zapLogger.Info("[startup] BWM content service ready (admin + public API)")
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
