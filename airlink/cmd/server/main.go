package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"airlink/internal/adapter/api/http/handler"
	"airlink/internal/adapter/api/http/router"
	"airlink/internal/adapter/config"
	"airlink/internal/adapter/logger"
	"airlink/internal/domain/service"
	"airlink/internal/service/permissionsync"
	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	cfg, err := config.LoadEnv("configs/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)

	// Self-register ke Permission Service (best-effort)
	if m, err := permissionsync.LoadManifest("perm.manifest.yaml"); err != nil {
		log.Printf("[perm-sync] manifest: %v (self-sync dilewati)", err)
	} else {
		syncer := permissionsync.New(cfg.SSO, m, log.Printf)
		if syncer.Enabled() {
			syncer.RunInBackground(nil)
		} else {
			log.Printf("[perm-sync] dinonaktifkan (config SSO belum lengkap)")
		}
	}

	log.Println("Starting Airlink service...")

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

	// Connect RADIUS database (read-only)
	var radiusSvc *service.RadiusService
	if cfg.RadiusDB.DSN != "" {
		radiusSvc, err = service.NewRadiusService(cfg.RadiusDB.DSN)
		if err != nil {
			zapLogger.Info(fmt.Sprintf("[startup] RADIUS DB skipped: %v", err))
		} else {
			defer radiusSvc.Close()
			zapLogger.Info("[startup] RADIUS DB connected")
		}
	}

	// Initialize cert service (SSH to RADIUS host)
	var certSvc *service.CertService
	if cfg.RadiusSSH.Host != "" && cfg.RadiusSSH.User != "" {
		certSvc = service.NewCertService(cfg.RadiusSSH.User, cfg.RadiusSSH.Host)
		zapLogger.Info("[startup] cert service ready (SSH to " + cfg.RadiusSSH.Host + ")")
	}
	var certStore *service.CertStore
	if os.Getenv("CERT_DB_DSN") != "" && os.Getenv("AIRLINK_ENCRYPTION_KEY") != "" {
		certStore, err = service.NewCertStore(os.Getenv("CERT_DB_DSN"), os.Getenv("AIRLINK_ENCRYPTION_KEY"))
		if err != nil {
			zapLogger.Info(fmt.Sprintf("[startup] certificate store unavailable: %v", err))
		} else {
			defer certStore.Close()
			zapLogger.Info("[startup] encrypted certificate store connected")
		}
	}

	handlers := &router.Handlers{
		Airlink: handler.NewAirlinkHandler(radiusSvc),
		Cert:    handler.NewCertHandler(certSvc, certStore),
	}

	srv := router.SetupWithLogger(
		zapLogger,
		handlers,
		nil,
		cfg.SSO.URL,
		cfg.JWT.Secret,
		cfg.SSO.PermissionServiceURL,
		cfg.SSO.ServiceName,
		cfg.SSO.ServiceClientID,
		cfg.SSO.ServiceClientSecret,
	)

	port := fmt.Sprintf(":%d", cfg.Server.Port)
	if cfg.Server.Port == 0 {
		port = ":8087"
	}
	httpServer := &http.Server{
		Addr:         port,
		Handler:      srv,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		zapLogger.Info(fmt.Sprintf("[startup] Airlink listening on %s", port))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	zapLogger.Info("[shutdown] signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	zapLogger.Info("[shutdown] Airlink stopped")
}
