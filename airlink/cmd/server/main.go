package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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
		certSvc.SetCoaSecret(os.Getenv("RADIUS_COA_SECRET"))
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
	var dnsSvc *service.DNSService
	if dsn := os.Getenv("DNS_DB_DSN"); dsn != "" {
		sshTarget := ""
		if cfg.RadiusSSH.Host != "" && cfg.RadiusSSH.User != "" {
			sshTarget = cfg.RadiusSSH.User + "@" + cfg.RadiusSSH.Host
		}
		dnsSvc, err = service.NewDNSService(dsn, sshTarget)
		if err != nil {
			zapLogger.Info(fmt.Sprintf("[startup] DNS DB unavailable: %v", err))
		} else {
			defer dnsSvc.Close()
			zapLogger.Info("[startup] DNS DB connected")
		}
	}

	// Policy service (suspend + bandwidth) & reconciler queue MikroTik
	var policySvc *service.PolicyService
	if cfg.RadiusDB.DSN != "" {
		sshTarget := ""
		if cfg.RadiusSSH.Host != "" && cfg.RadiusSSH.User != "" {
			sshTarget = cfg.RadiusSSH.User + "@" + cfg.RadiusSSH.Host
		}
		policySvc, err = service.NewPolicyService(cfg.RadiusDB.DSN, sshTarget, os.Getenv("RADIUS_COA_SECRET"))
		if err != nil {
			zapLogger.Info(fmt.Sprintf("[startup] policy service skipped: %v", err))
		} else {
			defer policySvc.Close()
			zapLogger.Info("[startup] policy service ready (suspend + bandwidth)")
		}
	}
	if policySvc != nil {
		qi := 5 * time.Minute
		if v, perr := strconv.Atoi(os.Getenv("AIRLINK_QUOTA_INTERVAL_SECONDS")); perr == nil && v >= 60 && v <= 86400 {
			qi = time.Duration(v) * time.Second
		}
		go policySvc.RunQuotaReconciler(context.Background(), qi, func(f string, a ...interface{}) {
			zapLogger.Info("[quota] " + fmt.Sprintf(f, a...))
		})
		zapLogger.Info("[startup] quota reconciler aktif")
	}
	var queueCancel context.CancelFunc
	if policySvc != nil && os.Getenv("MIKROTIK_URL") != "" && os.Getenv("MIKROTIK_USER") != "" {
		interval := 20 * time.Second
		if v, perr := strconv.Atoi(os.Getenv("MIKROTIK_INTERVAL_SECONDS")); perr == nil && v >= 5 && v <= 3600 {
			interval = time.Duration(v) * time.Second
		}
		qcfg := service.QueueConfig{
			BaseURL:            os.Getenv("MIKROTIK_URL"),
			User:               os.Getenv("MIKROTIK_USER"),
			Pass:               os.Getenv("MIKROTIK_PASS"),
			InsecureSkipVerify: os.Getenv("MIKROTIK_INSECURE") != "0",
			DefaultUp:          os.Getenv("AIRLINK_DEFAULT_UP"),
			DefaultDown:        os.Getenv("AIRLINK_DEFAULT_DOWN"),
			Interval:           interval,
		}
		var qctx context.Context
		qctx, queueCancel = context.WithCancel(context.Background())
		defer queueCancel()
		qs := service.NewQueueService(policySvc, qcfg, func(f string, a ...interface{}) {
			zapLogger.Info("[queue] " + fmt.Sprintf(f, a...))
		})
		go qs.Run(qctx)
		zapLogger.Info("[startup] queue reconciler MikroTik aktif")
	} else {
		zapLogger.Info("[startup] queue reconciler MikroTik nonaktif (MIKROTIK_URL/USER belum diisi)")
	}

	handlers := &router.Handlers{
		Airlink: handler.NewAirlinkHandler(radiusSvc),
		Cert:    handler.NewCertHandler(certSvc, certStore, radiusSvc),
		DNS:     handler.NewDNSHandler(dnsSvc, radiusSvc),
		Policy:  handler.NewPolicyHandler(policySvc, radiusSvc),
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

	if queueCancel != nil {
		queueCancel()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	zapLogger.Info("[shutdown] Airlink stopped")
}
