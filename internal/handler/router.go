package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/tonypk/aistarlight-go/internal/config"
	"github.com/tonypk/aistarlight-go/internal/handler/middleware"
	"github.com/tonypk/aistarlight-go/internal/service"
)

type Router struct {
	Auth           *AuthHandler
	Org            *OrgHandler
	Company        *CompanyHandler
	Report         *ReportHandler
	Chat           *ChatHandler
	Reconciliation *ReconciliationHandler
	Compliance     *ComplianceHandler
	Correction     *CorrectionHandler
	Withholding    *WithholdingHandler
	Dashboard      *DashboardHandler
	Receipt        *ReceiptHandler
	Audit          *AuditHandler
	Memory         *MemoryHandler
	Task           *TaskHandler

	AuthSvc    *service.AuthService
	OrgSvc     *service.OrgService
	CompanySvc *service.CompanyService

	Config *config.Config
	Redis  *redis.Client
}

func (rt *Router) Setup(r *gin.Engine) {
	api := r.Group("/api/v1")

	// Public routes (no auth)
	auth := api.Group("/auth")
	{
		auth.POST("/register", rt.Auth.Register)
		auth.POST("/login", rt.Auth.Login)
		auth.POST("/refresh", rt.Auth.Refresh)
	}

	// Auth middleware for all remaining routes
	authMw := middleware.Auth(rt.Config.JWT.Secret, rt.AuthSvc, rt.AuthSvc)

	// Authenticated auth routes
	authProtected := api.Group("/auth")
	authProtected.Use(authMw)
	{
		authProtected.POST("/logout", rt.Auth.Logout)
		authProtected.GET("/me", rt.Auth.Me)
		authProtected.POST("/api-key", rt.Auth.GenerateAPIKey)
		authProtected.GET("/companies", rt.Auth.ListCompanies)
		authProtected.POST("/switch-company", rt.Auth.SwitchCompany)
	}

	// Organization routes
	orgs := api.Group("/orgs")
	orgs.Use(authMw)
	{
		orgs.POST("", rt.Org.Create)
		orgs.GET("", rt.Org.List)

		orgByID := orgs.Group("/:orgId")
		orgByID.Use(middleware.RequireOrgRole(rt.OrgSvc, "org_member"))
		{
			orgByID.GET("", rt.Org.Get)
			orgByID.GET("/members", rt.Org.ListMembers)
			orgByID.GET("/companies", rt.Company.ListByOrg)
		}

		orgAdmin := orgs.Group("/:orgId")
		orgAdmin.Use(middleware.RequireOrgRole(rt.OrgSvc, "org_admin"))
		{
			orgAdmin.PUT("", rt.Org.Update)
			orgAdmin.POST("/members", rt.Org.AddMember)
			orgAdmin.PATCH("/members/:userId", rt.Org.UpdateMemberRole)
			orgAdmin.DELETE("/members/:userId", rt.Org.RemoveMember)
			orgAdmin.POST("/companies", rt.Company.CreateUnderOrg)
		}
	}

	// Company routes
	companies := api.Group("/companies")
	companies.Use(authMw)
	{
		companyByID := companies.Group("/:id")
		{
			companyByID.GET("", rt.Company.Get)
			companyByID.PUT("", rt.Company.Update)
			companyByID.GET("/members", rt.Company.ListMembers)
			companyByID.POST("/members", rt.Company.AddMember)
		}
	}

	// Report routes
	reports := api.Group("/reports")
	reports.Use(authMw)
	{
		reports.POST("", rt.Report.Create)
		reports.GET("", rt.Report.List)
		reports.POST("/calculate", rt.Report.Calculate)

		reportByID := reports.Group("/:id")
		{
			reportByID.GET("", rt.Report.Get)
			reportByID.DELETE("", rt.Report.Delete)
			reportByID.PATCH("/status", rt.Report.UpdateStatus)
			reportByID.POST("/recalculate", rt.Report.Recalculate)
			reportByID.POST("/overrides", rt.Report.ApplyOverrides)
		}
	}

	// Chat routes (AI agent)
	chat := api.Group("/chat")
	chat.Use(authMw)
	{
		chat.POST("/message", rt.Chat.Message)
		chat.POST("/stream", rt.Chat.Stream)
		chat.GET("/history", rt.Chat.History)
	}

	// Reconciliation routes
	recon := api.Group("/reconciliation")
	recon.Use(authMw)
	{
		recon.POST("/run", rt.Reconciliation.Run)
		recon.POST("/detect-format", rt.Reconciliation.DetectFormat)
		recon.POST("/match-preview", rt.Reconciliation.MatchPreview)
		recon.GET("/batches", rt.Reconciliation.List)
		recon.GET("/batches/:id", rt.Reconciliation.Get)
	}

	// Compliance routes
	compliance := api.Group("/compliance")
	compliance.Use(authMw)
	{
		compliance.POST("/validate/:reportId", rt.Compliance.Validate)
		compliance.GET("/reports/:reportId/latest", rt.Compliance.GetLatest)
		compliance.GET("/reports/:reportId/history", rt.Compliance.ListValidations)
		compliance.GET("/filing-calendar", rt.Compliance.FilingCalendar)
	}

	// Correction routes
	corrections := api.Group("/corrections")
	corrections.Use(authMw)
	{
		corrections.POST("", rt.Correction.Record)
		corrections.GET("", rt.Correction.List)
		corrections.GET("/stats", rt.Correction.Stats)
		corrections.GET("/entity/:type/:id", rt.Correction.GetByEntity)
		corrections.POST("/analyze", rt.Correction.AnalyzePatterns)
		corrections.POST("/persist-rules", rt.Correction.PersistRules)
		corrections.GET("/learning-stats", rt.Correction.LearningStats)
	}

	// Withholding tax routes
	withholding := api.Group("/withholding")
	withholding.Use(authMw)
	{
		withholding.POST("/classify", rt.Withholding.Classify)
		withholding.GET("/rates", rt.Withholding.Rates)
		withholding.POST("/certificates", rt.Withholding.CreateCertificate)
		withholding.GET("/certificates", rt.Withholding.ListCertificates)
		withholding.GET("/certificates/:id", rt.Withholding.GetCertificate)
	}

	// Dashboard routes
	dashboard := api.Group("/dashboard")
	dashboard.Use(authMw)
	{
		dashboard.GET("/stats", rt.Dashboard.Stats)
		dashboard.GET("/calendar", rt.Dashboard.Calendar)
		dashboard.GET("/compare", rt.Dashboard.Compare)
		dashboard.GET("/company", rt.Dashboard.CompanySettings)
	}

	// Receipt routes
	receipts := api.Group("/receipts")
	receipts.Use(authMw)
	{
		receipts.POST("/upload", rt.Receipt.Upload)
		receipts.GET("/batches", rt.Receipt.List)
		receipts.GET("/batches/:id", rt.Receipt.Get)
	}

	// Audit trail routes
	audit := api.Group("/audit")
	audit.Use(authMw)
	{
		audit.GET("", rt.Audit.List)
		audit.GET("/report/:reportId", rt.Audit.ByReport)
	}

	// Memory / preferences routes
	memory := api.Group("/memory")
	memory.Use(authMw)
	{
		memory.GET("/preferences/:reportType", rt.Memory.GetPreference)
		memory.PUT("/preferences/:reportType", rt.Memory.UpsertPreference)
	}

	// Async task polling routes
	tasks := api.Group("/tasks")
	tasks.Use(authMw)
	{
		tasks.GET("", rt.Task.List)
		tasks.GET("/:id", rt.Task.Get)
	}
}
