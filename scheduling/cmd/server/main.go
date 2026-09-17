package main

import (
	"context"
	"fmt"
	"github.com/rama/b-wise/scheduling/internal/adapter/api/http/handler"
	entity "github.com/rama/b-wise/scheduling/internal/domain/entity"
	service "github.com/rama/b-wise/scheduling/internal/domain/service"
	solverclient "github.com/rama/b-wise/scheduling/internal/service/solverclient"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/scheduling/internal/adapter/api/http/router"
	"github.com/rama/b-wise/scheduling/internal/adapter/cache"
	"github.com/rama/b-wise/scheduling/internal/adapter/config"
	"github.com/rama/b-wise/scheduling/internal/adapter/database"
	"github.com/rama/b-wise/scheduling/internal/adapter/logger"
	"github.com/rama/b-wise/scheduling/internal/service/permissionsync"
	"gorm.io/gorm")

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

			// Auto-migrate your entities here:
			// AutoMigrate scheduling master data (F0)
			err = database.AutoMigrate(db,
				&entity.Building{}, &entity.Room{}, &entity.RoomType{}, &entity.FacilityType{}, &entity.CourseType{}, &entity.Term{},
				&entity.Course{}, &entity.Lecturer{}, &entity.ClassGroup{},
				&entity.TimeSlot{}, &entity.LecturerAvailability{}, &entity.Offering{}, &entity.OfferingLecturer{},
				&entity.SolveJob{}, &entity.TimetableEntry{}, &entity.TimetableVersion{}, &entity.CalendarToken{}, &entity.SolveConfig{}, &entity.TimePolicy{},
			)
			if err != nil {
				zapLogger.Info(fmt.Sprintf("[startup] auto-migrate error: %v", err))
			} else {
				zapLogger.Info("[startup] database migrated (scheduling master data)")
				seedRoomTypes(db, zapLogger)
				seedFacilityTypes(db, zapLogger)
				seedCourseTypes(db, zapLogger)
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
	masterHandler := handler.NewMasterHandlerDB(db)
	importHandler := handler.NewImportHandler(db)
	solverURL := os.Getenv("SOLVER_URL")
	if solverURL == "" {
		solverURL = "http://127.0.0.1:8086"
	}
	solveHandler := handler.NewSolveHandler(service.NewSolveService(db, solverclient.NewSolverClient(solverURL)))
	handlers := &router.Handlers{Master: masterHandler, Importer: importHandler, Solve: solveHandler, SolveConfig: handler.NewSolveConfigHandler(db), TimePolicy: handler.NewTimePolicyHandler(db)}

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

// seedCourseTypes — seed default jenis MK saat tabel kosong (idempotent by code).
// theory/practice/mixed mereplikasi perilaku solver lama.
func seedCourseTypes(db *gorm.DB, log *logger.Logger) {
	var count int64
	db.Model(&entity.CourseType{}).Count(&count)
	if count > 0 {
		return
	}
	defaults := []entity.CourseType{
		{Code: "theory", Name: "Teori", RoomNeed: "theory", SortOrder: 1},
		{Code: "practice", Name: "Praktikum", RoomNeed: "practice", SortOrder: 2},
		{Code: "mixed", Name: "Teori+Praktik", RoomNeed: "theory", SortOrder: 3},
	}
	if err := db.Create(&defaults).Error; err != nil {
		log.Info(fmt.Sprintf("[startup] seed course types error: %v", err))
	} else {
		log.Info(fmt.Sprintf("[startup] seeded %d default course types", len(defaults)))
	}
}
// seedFacilityTypes — seed default fasilitas saat tabel kosong (idempotent by code).
func seedFacilityTypes(db *gorm.DB, log *logger.Logger) {
	var count int64
	db.Model(&entity.FacilityType{}).Count(&count)
	if count > 0 {
		return
	}
	defaults := []entity.FacilityType{
		{Code: "projector", Name: "Proyektor", Kind: "boolean", SortOrder: 1},
		{Code: "ac", Name: "AC", Kind: "boolean", SortOrder: 2},
		{Code: "wifi", Name: "WiFi", Kind: "boolean", SortOrder: 3},
		{Code: "whiteboard", Name: "Papan Tulis", Kind: "boolean", SortOrder: 4},
		{Code: "pc", Name: "PC/Komputer", Kind: "number", UnitLabel: "unit", SortOrder: 5},
		{Code: "hospital_bed", Name: "Bed Pasien", Kind: "number", UnitLabel: "bed", SortOrder: 6},
	}
	if err := db.Create(&defaults).Error; err != nil {
		log.Info(fmt.Sprintf("[startup] seed facility types error: %v", err))
	} else {
		log.Info(fmt.Sprintf("[startup] seeded %d default facility types", len(defaults)))
	}
}

// seedRoomTypes — seed default tipe ruang saat tabel kosong (idempotent by code).
// Flags mereplikasi semantik solver lama: practice→lab/skill_lab persis,
// theory→theory/smart/hall; office/other tidak pernah auto-assign.
func seedRoomTypes(db *gorm.DB, log *logger.Logger) {
	var count int64
	db.Model(&entity.RoomType{}).Count(&count)
	if count > 0 {
		return
	}
	defaults := []entity.RoomType{
		{Code: "theory", Name: "Ruang Teori", ForTheory: true, SortOrder: 1},
		{Code: "lab", Name: "Laboratorium", ForPractice: true, SortOrder: 2},
		{Code: "skill_lab", Name: "Skill Lab", ForPractice: true, SortOrder: 3},
		{Code: "smart", Name: "Smart Class", ForTheory: true, SortOrder: 4},
		{Code: "hall", Name: "Aula", ForTheory: true, SortOrder: 5},
		{Code: "office", Name: "Kantor", SortOrder: 6},
		{Code: "other", Name: "Lainnya", SortOrder: 7},
	}
	if err := db.Create(&defaults).Error; err != nil {
		log.Info(fmt.Sprintf("[startup] seed room types error: %v", err))
	} else {
		log.Info(fmt.Sprintf("[startup] seeded %d default room types", len(defaults)))
	}
}
