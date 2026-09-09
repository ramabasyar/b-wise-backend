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
	"github.com/rama/b-wise/iku/internal/adapter/api/http/handler"
	"github.com/rama/b-wise/iku/internal/adapter/api/http/router"
	"github.com/rama/b-wise/iku/internal/adapter/cache"
	"github.com/rama/b-wise/iku/internal/adapter/config"
	"github.com/rama/b-wise/iku/internal/adapter/database"
	"github.com/rama/b-wise/iku/internal/adapter/logger"
	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	domainsvc "github.com/rama/b-wise/iku/internal/domain/service"
	"github.com/rama/b-wise/iku/internal/service/hcclient"
	"github.com/rama/b-wise/iku/internal/service/permclient"
	"github.com/rama/b-wise/iku/internal/service/permissionsync"
	ssoclient "github.com/rama/b-wise/iku/internal/service/ssoclient"
	"github.com/rama/b-wise/iku/internal/service/storage"
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

	ssoIKUClient := ssoclient.New(cfg.SSO.URL, cfg.SSO.ServiceClientID, cfg.SSO.ServiceClientSecret)

	var ikuHandler *handler.IkuHandler
	var engineHandler *handler.EngineHandler
	var achHandler *handler.AchievementHandler
	var planSvc *domainsvc.ActionPlanService
	var brokerHandler *handler.BrokerHandler
	var complianceHandler *handler.ComplianceHandler
	var notificationHandler *handler.NotificationHandler
	var exportHandler *handler.ExportHandler
	var metricsHandler *handler.MetricsHandler
	var periodSvc *domainsvc.PeriodService
	var periodHandler *handler.PeriodHandler
	var comparisonHandler *handler.ComparisonHandler
	var stopPeriodScheduler func()

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

			// F0: regulatory registry + indicator definitions + periods
			err = database.AutoMigrate(db,
				&entity.RegulatoryVersion{},
				&entity.IndicatorDefinition{},
				&entity.Period{},
				&entity.FormulaVersion{},
				&entity.IndicatorThreshold{},
				&entity.PKDocument{},
				&entity.PerformanceTarget{},
				&entity.ActionPlan{},
				&entity.DataSourceConnector{},
				&entity.ImportBatch{},
				&entity.RegulationChange{},
				&entity.RegulationChangeItem{},
				&entity.AchievementRecord{},
				&entity.EvidenceDocument{},
				&entity.WorkflowLog{},
				&entity.Notification{},
			)
			if err != nil {
				zapLogger.Info(fmt.Sprintf("[startup] auto-migrate error: %v", err))
			} else {
				zapLogger.Info("[startup] database migrated")

				ikuSvc := domainsvc.NewIkuService(db)
				formulaSvc := domainsvc.NewFormulaService(db)
				targetSvc := domainsvc.NewTargetService(db)
				if err := ikuSvc.SeedReference(); err != nil {
					zapLogger.Warn(fmt.Sprintf("[startup] seed gagal: %v", err))
				} else {
					zapLogger.Info("[startup] seed reference OK (12 IKU + periode)")
					seedExampleFormula(formulaSvc, ikuSvc, zapLogger)
				}
				ikuHandler = handler.NewIkuHandler(ikuSvc)

				// HC internal client (unit kerja) via service token
				hcCl := hcclient.New(cfg.SSO.HumanCapitalURL, ssoIKUClient.GetServiceToken)
				dashSvc := domainsvc.NewDashboardService(db)
				planSvc = domainsvc.NewActionPlanService(db)
				engineHandler = handler.NewEngineHandler(formulaSvc, targetSvc, dashSvc, planSvc, hcCl)

				// Evidence storage: S3/MinIO bila terkonfigurasi, fallback local filesystem
				evStorage := initEvidenceStorage(cfg, zapLogger)
				achSvc := domainsvc.NewAchievementService(db, formulaSvc)
				permCl := permclient.New(cfg.SSO.PermissionServiceURL, ssoIKUClient.GetServiceToken)
				achHandler = handler.NewAchievementHandler(achSvc, evStorage, handler.WithUnitScope(hcCl, permCl))
				brokerSvc := domainsvc.NewBrokerService(db)
				brokerHandler = handler.NewBrokerHandler(brokerSvc, achSvc)
				cmpSvc := domainsvc.NewComplianceService(db, formulaSvc)
				complianceHandler = handler.NewComplianceHandler(cmpSvc)

				// F7: notifier (in-app + email/WA env-gated) + export + metrics
				notifier := domainsvc.NewNotifier(db)
				achSvc.Notifier = notifier
				planSvc.Notifier = notifier
				cmpSvc.Notifier = notifier
				// F9: period lifecycle service + handler
				periodSvc = domainsvc.NewPeriodService(db, notifier)
				periodHandler = handler.NewPeriodHandler(periodSvc)
				yoySvc := domainsvc.NewComparisonService(db)
				comparisonHandler = handler.NewComparisonHandler(yoySvc)
				exportSvc := domainsvc.NewExportService(db, dashSvc, yoySvc)
				notificationHandler = handler.NewNotificationHandler(db)
				exportHandler = handler.NewExportHandler(exportSvc)
				metricsHandler = handler.NewMetricsHandler(db)
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
	handlers := &router.Handlers{Iku: ikuHandler, Engine: engineHandler, Achievement: achHandler, Broker: brokerHandler,
		Compliance: complianceHandler, Notification: notificationHandler, Export: exportHandler, Metrics: metricsHandler,
		Period: periodHandler, Comparison: comparisonHandler}

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
	// F4: eskalasi otomatis action plan yang lewat tenggat (batch saat boot)
	if planSvc != nil {
		if n, err := planSvc.EscalateOverdue(); err == nil && n > 0 {
			zapLogger.Info(fmt.Sprintf("[startup] %d action plan dieskalasi (overdue)", n))
		}
	}

	// F9: period lifecycle — tick saat boot (grace/auto-close/provision) + scheduler background
	if periodSvc != nil {
		if gn, cn, err := periodSvc.RunLifecycleTick(); err != nil {
			zapLogger.Warn(fmt.Sprintf("[startup] period lifecycle tick error: %v", err))
		} else if gn > 0 || cn > 0 {
			zapLogger.Info(fmt.Sprintf("[startup] period lifecycle: %d periode -> grace, %d auto-close", gn, cn))
		}
		stopPeriodScheduler = periodSvc.StartScheduler(15 * time.Minute)
		defer stopPeriodScheduler()
	}

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

// seedExampleFormula — formula contoh IKU-2 (DATA konfigurasi, boleh diubah admin).
// Prinsip: 12 IKU = data, bukan kode — formula pun demikian (ini cuma seed awal).
func seedExampleFormula(fs *domainsvc.FormulaService, is *domainsvc.IkuService, lg *logger.Logger) {
	inds, err := is.ListIndicators(domainsvc.IndicatorFilter{ActiveOnly: true})
	if err != nil {
		return
	}
	for _, ind := range inds {
		if ind.IkuCode != "IKU-2" {
			continue
		}
		if f, err := fs.GetActive(ind.ID); err == nil && f != nil {
			return // sudah ada
		}
		vars := []entity.FormulaInputVar{
			{Name: "count_kerja", Label: "Lulusan langsung bekerja", Unit: "orang", SourceHint: "Tracer Study / manual", Required: true},
			{Name: "count_wirausaha", Label: "Lulusan berwirausaha", Unit: "orang", SourceHint: "Tracer Study / manual", Required: true},
			{Name: "count_lanjut", Label: "Lulusan lanjut studi", Unit: "orang", SourceHint: "Tracer Study / manual", Required: true},
			{Name: "count_total", Label: "Total lulusan periode", Unit: "orang", SourceHint: "SIAKAD / import / manual", Required: true},
		}
		f, err := fs.Create(domainsvc.CreateFormulaInput{
			IndicatorID:     ind.ID,
			Expression:      "(count_kerja + count_wirausaha + count_lanjut) / count_total * 100",
			InputVariables:  vars,
			RoundingRule:    "2dp",
			ValidationRules: `{"min":0,"max":100,"warn_above":95}`,
			Notes:           "Seed contoh IKU-2 (358/2025) — sesuaikan dengan Juknis resmi.",
		}, "system")
		if err != nil {
			lg.Warn(fmt.Sprintf("[seed] formula IKU-2 gagal: %v", err))
			return
		}
		if _, err := fs.Activate(f.ID, "system"); err != nil {
			lg.Warn(fmt.Sprintf("[seed] aktivasi formula IKU-2 gagal: %v", err))
			return
		}
		lg.Info("[seed] formula IKU-2 v1 aktif (contoh, editable admin)")
		return
	}
}

// initEvidenceStorage — S3/MinIO jika env lengkap, selain itu local ./data/evidence.
// PRODUCTION: arahkan S3_ENDPOINT ke server MinIO khusus (tanpa ubah kode).
func initEvidenceStorage(cfg *config.Config, lg *logger.Logger) storage.Storage {
	s3cfg := storage.S3Config{
		Endpoint:  os.Getenv("S3_ENDPOINT"),
		AccessKey: os.Getenv("S3_ACCESS_KEY"),
		SecretKey: os.Getenv("S3_SECRET_KEY"),
		Bucket:    os.Getenv("S3_BUCKET"),
		Region:    os.Getenv("S3_REGION"),
		UseSSL:    os.Getenv("S3_USE_SSL") == "true",
	}
	if s3cfg.Endpoint != "" && s3cfg.AccessKey != "" && s3cfg.Bucket != "" {
		st, err := storage.NewS3Storage(context.Background(), s3cfg)
		if err != nil {
			lg.Warn(fmt.Sprintf("[startup] S3 storage gagal (%v) — fallback local", err))
		} else {
			lg.Info(fmt.Sprintf("[startup] evidence storage: s3://%s (%s)", s3cfg.Bucket, s3cfg.Endpoint))
			return st
		}
	}
	st, err := storage.NewLocalStorage(os.Getenv("EVIDENCE_LOCAL_DIR"))
	if err != nil {
		lg.Warn(fmt.Sprintf("[startup] local storage gagal: %v", err))
		return nil
	}
	lg.Info("[startup] evidence storage: local ./data/evidence (S3 belum terkonfigurasi)")
	return st
}
