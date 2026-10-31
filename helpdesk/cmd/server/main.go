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

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/helpdesk/internal/adapter/api/http/handler"
	"github.com/rama/b-wise/helpdesk/internal/adapter/api/http/router"
	"github.com/rama/b-wise/helpdesk/internal/adapter/cache"
	"github.com/rama/b-wise/helpdesk/internal/adapter/config"
	"github.com/rama/b-wise/helpdesk/internal/adapter/database"
	"github.com/rama/b-wise/helpdesk/internal/adapter/logger"
	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"github.com/rama/b-wise/helpdesk/internal/migrations"
	"github.com/rama/b-wise/helpdesk/internal/service/notifsvc"
	"github.com/rama/b-wise/helpdesk/internal/service/permissionsync"
	"github.com/rama/b-wise/helpdesk/internal/service/slasvc"
	"github.com/rama/b-wise/helpdesk/internal/service/teamsync"
	"github.com/rama/b-wise/helpdesk/internal/service/ticketsvc"
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

			// Auto-migrate entities Pusat Layanan
			if err := database.AutoMigrate(db,
				&entity.Team{}, &entity.TeamMember{},
				&entity.ServiceCategory{}, &entity.Service{}, &entity.ServiceForm{},
				&entity.SLAPolicy{}, &entity.BusinessCalendar{},
				&entity.Ticket{}, &entity.TicketEvent{}, &entity.TicketAttachment{},
				&entity.TicketApproval{}, &entity.CannedResponse{}, &entity.CSATResponse{}, &entity.NotificationOutbox{},
				&entity.TicketSequence{}, &entity.KBArticle{}, &entity.KBArticleFeedback{},
			); err != nil {
				zapLogger.Info(fmt.Sprintf("[startup] auto-migrate error: %v", err))
			} else {
				zapLogger.Info("[startup] auto-migrate teams ok")
				if err := migrations.Run(db, migrations.AllMigrations()); err != nil {
					log.Printf("[startup] default data migration failed: %v", err)
				} else {
					log.Printf("[startup] default SLA data ready")
				}

				// Sinkron tim otomatis dari unit HC (D-15, blueprint v1.1)
				// Pemantau SLA berkala (SL-04; blueprint 9.5: job tiap 1 menit)
				go slasvc.NewWorker(db, log.Printf).RunInBackground(nil, time.Minute)
				go notifsvc.NewWorker(db, log.Printf).RunInBackground(nil, 30*time.Second)

				ts := teamsync.New(db, cfg.HC, cfg.SSO, log.Printf)
				if ts.Enabled() {
					go ts.RunInBackground(nil, cfg.HC.SyncInterval)
				} else {
					log.Printf("[team-sync] dinonaktifkan (HC_URL / kredensial service belum lengkap)")
				}
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
	if db != nil {
		ticketSvc := ticketsvc.New(db)
		ticketSvc.SetResolver(cfg.HC.URL, cfg.SSO.URL, cfg.SSO.ServiceClientID, cfg.SSO.ServiceClientSecret)
		handlers.Ticket = handler.NewTicketHandler(ticketSvc)
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
