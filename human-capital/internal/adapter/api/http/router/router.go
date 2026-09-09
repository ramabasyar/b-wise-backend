package router

import (
	"github.com/gin-gonic/gin"
	"github.com/rama/b-wise/human-capital/internal/adapter/api/http/handler"
	"github.com/rama/b-wise/human-capital/internal/adapter/api/http/middleware"
	"go.uber.org/zap"
)

func Setup() *gin.Engine {
	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.NewCORSWithOrigins([]string{"*"}).Handle())
	r.Use(middleware.NewSecurityMiddleware(middleware.DefaultSecurityConfig()).Security())
	r.GET("/health", handler.NewHealthHandler().GetHealth)
	return r
}

type SetupDeps struct {
	Logger               *zap.Logger
	EmployeeHandler      *handler.EmployeeHandler
	OrgHandler           *handler.OrgHandler
	DictionaryHandler    *handler.DictionaryHandler
	SSOURL               string
	PermissionServiceURL string
	ServiceName          string
	ServiceClientID      string
	ServiceClientSecret  string
	JWTSecret            string
	CORSOrigins          []string
}

func SetupWithLogger(d SetupDeps) *gin.Engine {
	r := gin.New()
	r.Use(middleware.Recovery())
	if len(d.CORSOrigins) == 0 {
		d.CORSOrigins = []string{"*"}
	}
	r.Use(middleware.NewCORSWithOrigins(d.CORSOrigins).Handle())
	r.Use(middleware.NewSecurityMiddleware(middleware.DefaultSecurityConfig()).Security())
	r.Use(middleware.NewLogger(d.Logger).Log())

	// Global: validasi token (user/service) sekali, set context user_id/is_service_token
	jwksAuth := middleware.NewJWKSAuth(d.SSOURL).WithHMACSecret(d.JWTSecret)
	r.Use(jwksAuth.Middleware())

	permCheck := middleware.NewPermissionCheck(d.PermissionServiceURL, d.ServiceName).
		WithSSO(d.SSOURL, d.ServiceClientID, d.ServiceClientSecret)

	api := r.Group("/api")

	// ============ EMPLOYEES ============
	if d.EmployeeHandler != nil {
		employees := api.Group("/employees")
		employees.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			employees.GET("", permCheck.RequirePermission("employees.read"), d.EmployeeHandler.ListEmployees)
			employees.GET("/stats", permCheck.RequirePermission("employees.read"), d.EmployeeHandler.EmployeeStats)
			employees.POST("", permCheck.RequirePermission("employees.write"), d.EmployeeHandler.CreateEmployee)
			employees.GET("/:id", permCheck.RequirePermission("employees.read"), d.EmployeeHandler.GetEmployee)
			employees.PUT("/:id", permCheck.RequirePermission("employees.write"), d.EmployeeHandler.UpdateEmployee)
			employees.DELETE("/:id", permCheck.RequirePermission("employees.delete"), d.EmployeeHandler.DeleteEmployee)
			employees.PUT("/:id/educations", permCheck.RequirePermission("employees.write"), d.EmployeeHandler.SetEducations)
			employees.GET("/:id/movements", permCheck.RequirePermission("movements.read"), d.EmployeeHandler.ListEmployeeMovements)
		}

		movements := api.Group("/movements")
		movements.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			movements.GET("", permCheck.RequirePermission("movements.read"), d.EmployeeHandler.ListMovements)
			movements.POST("", permCheck.RequirePermission("movements.write"), d.EmployeeHandler.CreateMovement)
		}

		onboard := api.Group("/onboard")
		onboard.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
		{
			onboard.POST("", permCheck.RequirePermission("onboard.execute"), d.EmployeeHandler.Onboard)
		onboard.GET("/check", permCheck.RequirePermission("onboard.execute"), d.EmployeeHandler.OnboardCheck)
		onboard.GET("/department-services", permCheck.RequirePermission("onboard.execute"), d.EmployeeHandler.OnboardDeptServices)
		}
	}

	// ============ DICTIONARIES (Fase B — dynamic master data) ============
	dicts := api.Group("/dictionaries")
	dicts.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
	{
		dicts.GET("", d.DictionaryHandler.GetAll)
		dicts.POST("/:kind", permCheck.RequirePermission("dictionaries.write"), d.DictionaryHandler.Upsert)
		dicts.DELETE("/:kind/:code", permCheck.RequirePermission("dictionaries.write"), d.DictionaryHandler.Delete)
	}

	// ============ ORG INTERNAL (Fase B — approver resolution utk permission-service) ============
	orgInternal := api.Group("/org")
	orgInternal.Use(jwksAuth.RequireAuth())
	{
		orgInternal.GET("/resolve-approver", d.EmployeeHandler.ResolveApprover)
	}

	// ============ ORG MASTERS ============
	if d.OrgHandler != nil {
		orgGroup := func(path, resource string) *gin.RouterGroup {
			g := api.Group(path)
			g.Use(jwksAuth.RequireAuth(), permCheck.CheckAccess())
			g.GET("", permCheck.RequirePermission(resource+".read"), orgList(d.OrgHandler, path))
			return g
		}

		departments := orgGroup("/departments", "departments")
		departments.POST("", permCheck.RequirePermission("departments.write"), d.OrgHandler.CreateDepartment)
		departments.PUT("/:id", permCheck.RequirePermission("departments.write"), d.OrgHandler.UpdateDepartment)
		departments.DELETE("/:id", permCheck.RequirePermission("departments.delete"), d.OrgHandler.DeleteDepartment)

		designations := orgGroup("/designations", "designations")
		designations.POST("", permCheck.RequirePermission("designations.write"), d.OrgHandler.CreateDesignation)
		designations.PUT("/:id", permCheck.RequirePermission("designations.write"), d.OrgHandler.UpdateDesignation)
		designations.DELETE("/:id", permCheck.RequirePermission("designations.delete"), d.OrgHandler.DeleteDesignation)

		empTypes := orgGroup("/employment-types", "employment_types")
		empTypes.POST("", permCheck.RequirePermission("employment_types.write"), d.OrgHandler.CreateEmploymentType)
		empTypes.PUT("/:id", permCheck.RequirePermission("employment_types.write"), d.OrgHandler.UpdateEmploymentType)
		empTypes.DELETE("/:id", permCheck.RequirePermission("employment_types.write"), d.OrgHandler.DeleteEmploymentType)

		grades := orgGroup("/grades", "grades")
		grades.POST("", permCheck.RequirePermission("grades.write"), d.OrgHandler.CreateGrade)
		grades.PUT("/:id", permCheck.RequirePermission("grades.write"), d.OrgHandler.UpdateGrade)
		grades.DELETE("/:id", permCheck.RequirePermission("grades.write"), d.OrgHandler.DeleteGrade)

		branches := orgGroup("/branches", "branches")
		branches.POST("", permCheck.RequirePermission("branches.write"), d.OrgHandler.CreateBranch)
		branches.PUT("/:id", permCheck.RequirePermission("branches.write"), d.OrgHandler.UpdateBranch)
		branches.DELETE("/:id", permCheck.RequirePermission("branches.write"), d.OrgHandler.DeleteBranch)
	}

	r.GET("/health", handler.NewHealthHandler().GetHealth)
	return r
}

// orgList — dispatch GET list sesuai path
func orgList(h *handler.OrgHandler, path string) gin.HandlerFunc {
	switch path {
	case "/departments":
		return h.ListDepartments
	case "/designations":
		return h.ListDesignations
	case "/employment-types":
		return h.ListEmploymentTypes
	case "/grades":
		return h.ListGrades
	case "/branches":
		return h.ListBranches
	}
	return func(c *gin.Context) {
		c.JSON(http404, gin.H{"success": false, "error": gin.H{"message": "unknown resource"}})
	}
}

const http404 = 404
