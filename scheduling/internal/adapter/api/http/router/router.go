package router

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/scheduling/internal/adapter/api/http/handler"
	"github.com/rama/b-wise/scheduling/internal/adapter/api/http/middleware"
	"go.uber.org/zap"
)

// Setup initializes all routes and returns a Gin engine
func Setup() *gin.Engine {
	r := gin.New()

	// Apply global middlewares
	r.Use(gin.Recovery())
	r.Use(middleware.NewCORSWithOrigins([]string{"*"}).Handle())
	r.Use(middleware.NewSecurityMiddleware(middleware.DefaultSecurityConfig()).Security())

	// Health check endpoint (public)
	healthHandler := handler.NewHealthHandler()
	r.GET("/health", healthHandler.GetHealth)

	// API routes (add your service-specific routes here)
	api := r.Group("/api")

	_ = api
	return r
}

// mountCRUD — pasang route CRUD standar dengan permission read/write.
func mountCRUD(g *gin.RouterGroup, resource string, permCheck *middleware.PermissionCheck,
	list, get, create, update, del gin.HandlerFunc, readPerm, writePerm string) {
	r := g.Group("/" + resource)
	{
		r.GET("", permCheck.RequirePermission(readPerm), list)
		r.GET("/:id", permCheck.RequirePermission(readPerm), get)
		r.POST("", permCheck.RequirePermission(writePerm), create)
		r.PATCH("/:id", permCheck.RequirePermission(writePerm), update)
		r.DELETE("/:id", permCheck.RequirePermission(writePerm), del)
	}
}

// SetupWithLogger initializes routes with a zap logger and full middleware chain
// corsOrigins: allowed CORS origins from config (empty = wildcard *)
// ssoURL: SSO service URL for JWKS key fetching
// jwtSecret: HMAC fallback secret (must match SSO)
// permissionURL: Permission Service URL for access/permission checks
// serviceName: this service's registered name in Permission Service
// serviceClientID: OAuth2 client ID for service-to-service auth
// serviceClientSecret: OAuth2 client secret for service-to-service auth
func SetupWithLogger(
	logger *zap.Logger,
	handlers *Handlers,
	corsOrigins []string,
	ssoURL, jwtSecret, permissionURL, serviceName, serviceClientID, serviceClientSecret string,
) *gin.Engine {
	r := gin.New()

	// 1. Recovery (first — catch panics)
	r.Use(gin.Recovery())

	// 2. CORS from config
	if len(corsOrigins) == 0 {
		corsOrigins = []string{"*"}
	}
	r.Use(middleware.NewCORSWithOrigins(corsOrigins).Handle())

	// 3. Security headers
	r.Use(middleware.NewSecurityMiddleware(middleware.DefaultSecurityConfig()).Security())

	// 4. Request ID
	r.Use(func(c *gin.Context) {
		if id := c.GetHeader("X-Request-ID"); id != "" {
			middleware.SetRequestID(c, id)
		} else {
			middleware.SetRequestID(c, fmt.Sprintf("%d", time.Now().UnixNano()))
		}
		c.Writer.Header().Set("X-Request-ID", middleware.GetRequestID(c))
		c.Next()
	})

	// 5. Logging (with request ID)
	r.Use(middleware.NewLogger(logger).Log())

	// 6. JWKS Auth — validates both user JWT and service JWT via SSO
	jwksAuth := middleware.NewJWKSAuth(ssoURL)
	_ = jwtSecret // HMAC secret available if needed for local validation
	r.Use(jwksAuth.Middleware())

	// Health check (public — no auth needed)
	r.GET("/health", handler.NewHealthHandler().GetHealth)

	// API routes (protected by JWKS auth)

	api := r.Group("/api")

	// Permission middleware — siap pakai (selaras Permission Service Tahap 1-3:
	// wildcard '*', batch check, cache invalidation otomatis).
	// Contoh wiring entity "items":
	//
	//	permCheck := middleware.NewPermissionCheck(permissionURL, serviceName).
	//		WithSSO(ssoURL, serviceClientID, serviceClientSecret)
	//	items := api.Group("/items")
	//	items.Use(jwksAuth.RequireAuth())
	//	items.Use(permCheck.CheckAccess())
	//	{
	//		items.GET("", permCheck.RequirePermission("items.read"), h.Item.List)
	//		items.POST("", permCheck.RequirePermission("items.write"), h.Item.Create)
	//		tools.GET("", permCheck.RequireAnyPermission("items.read", "items.admin"), h.Item.List)
	//	}
	h := handlers
	permCheck := middleware.NewPermissionCheck(permissionURL, serviceName).
		WithSSO(ssoURL, serviceClientID, serviceClientSecret)

	// ============ MASTER DATA (F0 scheduling) ============
	if h.Master != nil {
		master := api.Group("/master")
		master.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		mountCRUD(master, "buildings", permCheck,
			h.Master.BuildingList(), h.Master.BuildingGet(), h.Master.BuildingCreate(), h.Master.BuildingUpdate(), h.Master.BuildingDelete(),
			"facilities.read", "facilities.write")
		mountCRUD(master, "rooms", permCheck,
			h.Master.RoomList(), h.Master.RoomGet(), h.Master.RoomCreate(), h.Master.RoomUpdate(), h.Master.RoomDelete(),
			"facilities.read", "facilities.write")
		mountCRUD(master, "room-types", permCheck,
			h.Master.RoomTypeList(), h.Master.RoomTypeGet(), h.Master.RoomTypeCreate(), h.Master.RoomTypeUpdate(), h.Master.RoomTypeDelete(),
			"facilities.read", "facilities.write")
		mountCRUD(master, "facility-types", permCheck,
			h.Master.FacilityTypeList(), h.Master.FacilityTypeGet(), h.Master.FacilityTypeCreate(), h.Master.FacilityTypeUpdate(), h.Master.FacilityTypeDelete(),
			"facilities.read", "facilities.write")
		mountCRUD(master, "course-types", permCheck,
			h.Master.CourseTypeList(), h.Master.CourseTypeGet(), h.Master.CourseTypeCreate(), h.Master.CourseTypeUpdate(), h.Master.CourseTypeDelete(),
			"academics.read", "academics.write")
		mountCRUD(master, "terms", permCheck,
			h.Master.TermList(), h.Master.TermGet(), h.Master.TermCreate(), h.Master.TermUpdate(), h.Master.TermDelete(),
			"academics.read", "academics.write")
		mountCRUD(master, "courses", permCheck,
			h.Master.CourseList(), h.Master.CourseGet(), h.Master.CourseCreate(), h.Master.CourseUpdate(), h.Master.CourseDelete(),
			"academics.read", "academics.write")
		mountCRUD(master, "lecturers", permCheck,
			h.Master.LecturerList(), h.Master.LecturerGet(), h.Master.LecturerCreate(), h.Master.LecturerUpdate(), h.Master.LecturerDelete(),
			"academics.read", "academics.write")
		mountCRUD(master, "class-groups", permCheck,
			h.Master.GroupList(), h.Master.GroupGet(), h.Master.GroupCreate(), h.Master.GroupUpdate(), h.Master.GroupDelete(),
			"academics.read", "academics.write")
		mountCRUD(master, "time-slots", permCheck,
			h.Master.SlotList(), h.Master.SlotGet(), h.Master.SlotCreate(), h.Master.SlotUpdate(), h.Master.SlotDelete(),
			"timeslots.read", "timeslots.write")
		mountCRUD(master, "lecturer-availabilities", permCheck,
			h.Master.AvailList(), h.Master.AvailGet(), h.Master.AvailCreate(), h.Master.AvailUpdate(), h.Master.AvailDelete(),
			"academics.read", "academics.write")
		mountCRUD(master, "room-availabilities", permCheck,
			h.Master.RoomAvailList(), h.Master.RoomAvailGet(), h.Master.RoomAvailCreate(), h.Master.RoomAvailUpdate(), h.Master.RoomAvailDelete(),
			"facilities.read", "facilities.write")
		mountCRUD(master, "calendar-events", permCheck,
			h.Master.CalEventList(), h.Master.CalEventGet(), h.Master.CalEventCreate(), h.Master.CalEventUpdate(), h.Master.CalEventDelete(),
			"timeslots.read", "timeslots.write")
		master.POST("/time-slots/seed-default", permCheck.RequirePermission("timeslots.write"), h.Master.SeedTimeSlots)
		mountCRUD(master, "offerings", permCheck,
			h.Master.OfferingList(), h.Master.OfferingGet(), h.Master.OfferingCreate(), h.Master.OfferingUpdate(), h.Master.OfferingDelete(),
			"academics.read", "academics.write")
	}
	// ============ SOLVE JOBS & TIMETABLE (F1) ============
	if h.SolveConfig != nil {
		sc := api.Group("/solve-config")
		sc.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			sc.GET("", permCheck.RequirePermission("solve.read"), h.SolveConfig.Get)
			sc.PUT("", permCheck.RequirePermission("solve.execute"), h.SolveConfig.Update)
		}
	}
	if h.TimePolicy != nil {
		tp := api.Group("/time-policy")
		tp.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			tp.GET("", permCheck.RequirePermission("solve.read"), h.TimePolicy.Get)
			tp.PUT("", permCheck.RequirePermission("solve.execute"), h.TimePolicy.Update)
		}
	}
	if h.Solve != nil {
		jobs := api.Group("/solve-jobs")
		jobs.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			jobs.GET("", permCheck.RequirePermission("solve.read"), h.Solve.List)
			jobs.POST("", permCheck.RequirePermission("solve.execute"), h.Solve.Start)
			jobs.GET("/:id", permCheck.RequirePermission("solve.read"), h.Solve.Get)
			jobs.POST("/:id/cancel", permCheck.RequirePermission("solve.execute"), h.Solve.Cancel)
		}
		tt := api.Group("/timetable")
		tt.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		tt.GET("", permCheck.RequirePermission("schedules.read"), h.Solve.Entries)
		tt.PATCH("/:id", permCheck.RequirePermission("schedules.write"), h.Solve.SetLocked)
		tt.POST("/publish", permCheck.RequirePermission("schedules.write"), h.Solve.Publish)
		tt.GET("/versions", permCheck.RequirePermission("schedules.read"), h.Solve.Versions)
		tt.GET("/export.xlsx", permCheck.RequirePermission("schedules.read"), h.Solve.ExportXLSX)
		tt.GET("/calendar.ics", permCheck.RequirePermission("consumers.read"), h.Solve.CalendarICS)
		tt.GET("/view", permCheck.RequirePermission("schedules.read"), h.Solve.TimetableView)
		tt.POST("/move", permCheck.RequirePermission("schedules.write"), h.Solve.MoveEntry)
		tt.GET("/adjustments", permCheck.RequirePermission("schedules.read"), h.Solve.ListAdjustments)
		tt.POST("/adjustments/propose", permCheck.RequirePermission("schedules.write"), h.Solve.ProposeAdjustment)
		tt.POST("/adjustments/:id/decide", permCheck.RequirePermission("schedules.write"), h.Solve.DecideAdjustment)
		tt.GET("/day-view", permCheck.RequirePermission("schedules.read"), h.Solve.DayView)
		tt.POST("/overrides", permCheck.RequirePermission("schedules.write"), h.Solve.CreateOverride)
		tt.DELETE("/overrides/:id", permCheck.RequirePermission("schedules.write"), h.Solve.DeleteOverride)

		// ============ CALENDAR TOKENS (F4 gap — ICS tanpa header auth) ============
		if h.Solve != nil {
			ct := api.Group("/calendar-tokens")
			ct.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
			{
				ct.GET("", permCheck.RequirePermission("schedules.read"), h.Solve.ListCalendarTokens)
				ct.POST("", permCheck.RequirePermission("schedules.write"), h.Solve.IssueCalendarToken)
				ct.DELETE("/:id", permCheck.RequirePermission("schedules.write"), h.Solve.RevokeCalendarToken)
			}
		}

		// ============ PUBLIC ICS (token di path — Google Calendar compatible) ============
		// Tanpa auth middleware — kekuatan = token acak 48 hex + revocable + read-only jadwal.
		if h.Solve != nil {
			r.GET("/calendar/:token", h.Solve.PublicICS)
		}

		// ============ CONSUMER API (F3 — doorlock & sistem eksternal) ============
		cons := api.Group("/consumers")
		cons.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			cons.GET("/rooms/:id/schedule", permCheck.RequirePermission("consumers.read"), h.Solve.RoomSchedule)
			cons.GET("/rooms/:id/now", permCheck.RequirePermission("consumers.read"), h.Solve.RoomNow)
		}
	}

	if h.Importer != nil {
		imp := api.Group("/master")
		imp.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			imp.POST("/buildings/import", permCheck.RequirePermission("imports.write"), h.Importer.Buildings)
			imp.POST("/rooms/import", permCheck.RequirePermission("imports.write"), h.Importer.Rooms)
			imp.POST("/courses/import", permCheck.RequirePermission("imports.write"), h.Importer.Courses)
			imp.POST("/lecturers/import", permCheck.RequirePermission("imports.write"), h.Importer.Lecturers)
			imp.POST("/class-groups/import", permCheck.RequirePermission("imports.write"), h.Importer.Groups)
		}
	}

	return r
}

// Handlers holds all route handlers for the service.
type Handlers struct {
	Master      *handler.MasterHandler
	Importer    *handler.ImportHandler
	Solve       *handler.SolveHandler
	SolveConfig *handler.SolveConfigHandler
	TimePolicy  *handler.TimePolicyHandler
}
