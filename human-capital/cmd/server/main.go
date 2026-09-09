package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"gorm.io/gorm"

	"github.com/rama/b-wise/human-capital/internal/adapter/api/http/handler"
	"github.com/rama/b-wise/human-capital/internal/adapter/api/http/router"
	"github.com/rama/b-wise/human-capital/internal/adapter/cache"
	"github.com/rama/b-wise/human-capital/internal/adapter/config"
	"github.com/rama/b-wise/human-capital/internal/adapter/database"
	"github.com/rama/b-wise/human-capital/internal/adapter/logger"
	"github.com/rama/b-wise/human-capital/internal/adapter/persistence/postgres"
	"github.com/rama/b-wise/human-capital/internal/domain/entity"
	domainsvc "github.com/rama/b-wise/human-capital/internal/domain/service"
	"github.com/rama/b-wise/human-capital/internal/service/onboarding"
	perm "github.com/rama/b-wise/human-capital/internal/service/permission"
	"github.com/rama/b-wise/human-capital/internal/service/permissionsync"
	"github.com/rama/b-wise/human-capital/internal/service/sso"
)

func main() {
	cfg, err := config.LoadEnv("configs/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Println("Starting human capital service...")

	zapLogger, err := logger.New(logger.Config{Level: cfg.Logger.Level, Format: cfg.Logger.Format})
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer zapLogger.Sync()
	zapLogger.Info("[startup] configuration loaded")

	var empHandler *handler.EmployeeHandler
	var orgHandler *handler.OrgHandler
	var dictHandler *handler.DictionaryHandler
	var db *gorm.DB

	if cfg.Database.Host != "" {
		db, err = database.New(cfg.Database)
		if err != nil {
			zapLogger.Info(fmt.Sprintf("[startup] database skipped: %v", err))
		} else {
			defer database.Close(db)
			zapLogger.Info("[startup] database connected")

			if err := database.AutoMigrate(db,
				&entity.Employee{},
				&entity.EmployeeEducation{},
				&entity.Department{},
				&entity.Designation{},
				&entity.EmploymentType{},
				&entity.Grade{},
				&entity.Branch{},
				&entity.EmployeeMovement{},
				&entity.MovementDetail{},
				&entity.DepartmentService{},
				&entity.EmployeeType{},
				&entity.OrgUnitType{},
				&entity.EmploymentLevel{},
				&entity.AcademicRank{},
				&entity.StructuralPosition{},
				&entity.EmployeeUnitAssignment{},
			); err != nil {
				zapLogger.Warn(fmt.Sprintf("[startup] migration failed: %v", err))
			} else {
				zapLogger.Info("[startup] database migrated")
			}

			// Repositories
			empRepo := postgres.NewEmployeeRepository(db)
			deptRepo := postgres.NewDepartmentRepository(db)
			desigRepo := postgres.NewDesignationRepository(db)
			empTypeRepo := postgres.NewEmploymentTypeRepository(db)
			gradeRepo := postgres.NewGradeRepository(db)
			branchRepo := postgres.NewBranchRepository(db)
			movRepo := postgres.NewMovementRepository(db)

			// Services
			empSvc := domainsvc.NewEmployeeService(empRepo, movRepo)
			orgSvc := domainsvc.NewOrgService(deptRepo, desigRepo, empTypeRepo, gradeRepo, branchRepo)
			movSvc := domainsvc.NewMovementService(movRepo, empRepo, db)

			// SSO + Permission clients (dipakai onboarding & perm middleware)
			ssoClient := sso.NewSSOClient(cfg.SSO.URL, cfg.SSO.ServiceClientID, cfg.SSO.ServiceClientSecret)
			permClient := perm.NewPermissionClient(cfg.SSO.PermissionServiceURL, ssoClient.GetServiceToken)
			obs := onboarding.NewOnboardingService(empSvc, ssoClient, permClient, db)

			// Handlers
			empHandler = handler.NewEmployeeHandler(empSvc, movSvc)
			empHandler.SetOnboardingService(obs)
			empHandler.SetDB(db) // utk mapping department_services (onboard org-driven)
			orgHandler = handler.NewOrgHandler(orgSvc)
			dictHandler = handler.NewDictionaryHandler(db)

			// Seed master reference (idempoten — tanpa data dummy)
			seedReference(db, zapLogger)
			seedDictionaries(db, zapLogger)

			zapLogger.Info("[startup] repositories and services initialized")
		}
	}

	var redisCache *cache.Cache
	if cfg.Redis.Host != "" {
		redisCache, err = cache.New(cache.Config{
			Host: cfg.Redis.Host, Port: cfg.Redis.Port,
			Password: cfg.Redis.Password, DB: cfg.Redis.DB,
		})
		if err != nil {
			zapLogger.Info(fmt.Sprintf("[startup] cache skipped: %v", err))
		} else {
			defer redisCache.Close()
			zapLogger.Info("[startup] cache connected")
		}
	}

	// Self-register permissions & menu (perm.manifest.yaml) — best effort, retry backoff
	stopSync := make(chan struct{})
	if m, err := permissionsync.LoadManifest("perm.manifest.yaml"); err != nil {
		zapLogger.Warn(fmt.Sprintf("[perm-sync] manifest tidak terbaca: %v", err))
	} else {
		syncer := permissionsync.New(cfg.SSO, m, func(f string, a ...interface{}) {
			zapLogger.Info(fmt.Sprintf(f, a...))
		})
		if syncer.Enabled() {
			syncer.RunInBackground(stopSync)
		}
	}

	zapGlobal := zapLogger.SugaredLogger.Desugar()
	r := router.SetupWithLogger(router.SetupDeps{
		Logger:               zapGlobal,
		EmployeeHandler:      empHandler,
		OrgHandler:           orgHandler,
		DictionaryHandler:    dictHandler,
		SSOURL:               cfg.SSO.URL,
		PermissionServiceURL: cfg.SSO.PermissionServiceURL,
		ServiceName:          cfg.SSO.ServiceName,
		ServiceClientID:      cfg.SSO.ServiceClientID,
		ServiceClientSecret:  cfg.SSO.ServiceClientSecret,
		JWTSecret:            cfg.JWT.Secret,
		CORSOrigins:          cfg.CORS.AllowedOrigins,
	})

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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zapLogger.Info("Shutting down...")
	close(stopSync)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}
	zapLogger.Info("Server exited")

	_ = redisCache
}

// seedReference — (Fase B) dikosongkan: semua master data organisasi dikelola admin via UI.
// Seed sistem (kamus + org tree awal) ada di seed_dictionaries.go.
func seedReference(db *gorm.DB, lg *logger.Logger) {
	_ = db
	_ = lg
}
