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
	Data           *DataHandler
	Form           *FormHandler
	Knowledge      *KnowledgeHandler

	AuthSvc    *service.AuthService
	OrgSvc     *service.OrgService
	CompanySvc *service.CompanyService

	Config *config.Config
	Redis  *redis.Client
}

// adaptReportIDParam wraps a handler to copy the :id param as :reportId.
func adaptReportIDParam(h gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "reportId", Value: c.Param("id")})
		h(c)
	}
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
		authProtected.POST("/invite", rt.Auth.InviteMember) // frontend compat
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
		reports.POST("/generate", rt.Report.Generate) // frontend compat
		reports.GET("", rt.Report.List)
		reports.POST("/calculate", rt.Report.Calculate)
		reports.GET("/supported-forms", rt.Report.SupportedForms)

		reportByID := reports.Group("/:id")
		{
			reportByID.GET("", rt.Report.Get)
			reportByID.GET("/download", rt.Report.DownloadPDF)   // frontend compat (was /pdf)
			reportByID.GET("/pdf", rt.Report.DownloadPDF)        // keep original
			reportByID.GET("/export-csv", rt.Report.DownloadCSV) // frontend compat (was /csv)
			reportByID.GET("/csv", rt.Report.DownloadCSV)        // keep original
			reportByID.DELETE("", rt.Report.Delete)
			reportByID.PATCH("/status", rt.Report.UpdateStatus)
			reportByID.PATCH("/confirm", rt.Report.Confirm)       // frontend compat
			reportByID.PATCH("/edit", rt.Report.Edit)             // frontend compat
			reportByID.PATCH("/transition", rt.Report.Transition) // frontend compat
			reportByID.POST("/recalculate", rt.Report.Recalculate)
			reportByID.POST("/overrides", rt.Report.ApplyOverrides)
			// Compliance routes nested under reports (frontend compat)
			reportByID.POST("/validate", adaptReportIDParam(rt.Compliance.Validate))
			reportByID.GET("/validation", adaptReportIDParam(rt.Compliance.GetLatest))
			reportByID.GET("/validation/history", adaptReportIDParam(rt.Compliance.ListValidations))
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

	// Reconciliation routes (canonical)
	recon := api.Group("/reconciliation")
	recon.Use(authMw)
	{
		recon.POST("/run", rt.Reconciliation.Run)
		recon.POST("/detect-format", rt.Reconciliation.DetectFormat)
		recon.POST("/match-preview", rt.Reconciliation.MatchPreview)
		recon.GET("/batches", rt.Reconciliation.List)
		recon.GET("/batches/:id", rt.Reconciliation.Get)
	}

	// Bank Reconciliation routes (frontend compat — uses /bank-recon prefix)
	bankRecon := api.Group("/bank-recon")
	bankRecon.Use(authMw)
	{
		bankRecon.POST("/process", rt.Reconciliation.Process)
		bankRecon.GET("/batches", rt.Reconciliation.List)
		bankRecon.GET("/batches/:id", rt.Reconciliation.Get)
		bankRecon.POST("/batches/:id/accept-suggestion", rt.Reconciliation.AcceptSuggestion)
		bankRecon.POST("/batches/:id/reject-suggestion", rt.Reconciliation.RejectSuggestion)
		bankRecon.POST("/batches/:id/rerun-analysis", rt.Reconciliation.RerunAnalysis)
	}

	// Compliance routes (canonical)
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
		// Frontend compat: /learning/stats and /learning/analyze sub-paths
		corrections.GET("/learning/stats", rt.Correction.LearningStats)
		corrections.POST("/learning/analyze", rt.Correction.AnalyzePatterns)
		corrections.GET("/rules", rt.Correction.ListRules)
		corrections.PATCH("/rules/:ruleId", rt.Correction.UpdateRule)
	}

	// Withholding tax routes
	withholding := api.Group("/withholding")
	withholding.Use(authMw)
	{
		withholding.POST("/classify", rt.Withholding.Classify)
		withholding.GET("/rates", rt.Withholding.Rates)
		withholding.GET("/ewt-rates", rt.Withholding.Rates)       // frontend compat
		withholding.GET("/ewt-summary", rt.Withholding.EWTSummary) // frontend compat
		withholding.POST("/certificates", rt.Withholding.CreateCertificate)
		withholding.GET("/certificates", rt.Withholding.ListCertificates)
		withholding.GET("/certificates/:id", rt.Withholding.GetCertificate)
		withholding.GET("/certificates/:id/download", rt.Withholding.DownloadCertificate) // frontend compat
		// Supplier CRUD (stubs for frontend compat)
		withholding.GET("/suppliers", rt.Withholding.ListSuppliers)
		withholding.POST("/suppliers", rt.Withholding.CreateSupplier)
		withholding.PATCH("/suppliers/:id", rt.Withholding.UpdateSupplier)
		withholding.DELETE("/suppliers/:id", rt.Withholding.DeleteSupplier)
		// SAWT (stubs for frontend compat)
		withholding.GET("/sawt", rt.Withholding.GetSAWT)
		withholding.GET("/sawt/download", rt.Withholding.DownloadSAWT)
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
		receipts.POST("/upload", rt.Receipt.Upload)          // multipart + auto-compress
		receipts.POST("/upload-json", rt.Receipt.UploadJSON) // legacy: pre-saved image paths
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

	// Form schema routes (frontend compat)
	forms := api.Group("/forms")
	forms.Use(authMw)
	{
		forms.GET("", rt.Form.List)
		forms.GET("/:formType", rt.Form.GetSchema)
	}

	// Knowledge base routes (frontend compat)
	knowledge := api.Group("/knowledge")
	knowledge.Use(authMw)
	{
		knowledge.GET("", rt.Knowledge.List)
		knowledge.GET("/stats", rt.Knowledge.Stats)
	}

	// Data upload routes
	data := api.Group("/data")
	data.Use(authMw)
	{
		data.POST("/upload", rt.Data.Upload)
		data.POST("/upload-parsed", rt.Data.UploadParsed)
		data.POST("/preview", rt.Data.Preview)
		data.POST("/mapping", rt.Data.SuggestMapping)
	}

	// Async task polling routes
	tasks := api.Group("/tasks")
	tasks.Use(authMw)
	{
		tasks.GET("", rt.Task.List)
		tasks.GET("/:id", rt.Task.Get)
	}
}
