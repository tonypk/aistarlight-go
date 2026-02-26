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
	Session        *SessionHandler
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
	Account        *AccountHandler
	Journal        *JournalHandler
	Period         *AccountingPeriodHandler
	GL             *GLHandler
	QBO            *QBOHandler
	Settings       *SettingsHandler
	Telegram       *TelegramHandler
	Notification   *NotificationHandler
	// Pipeline bridge handlers
	ReceiptBridge  *ReceiptBridgeHandler
	JournalBridge  *JournalBridgeHandler
	FinStatement   *FinancialStatementHandler
	TaxBridge      *TaxBridgeHandler

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
			reportByID.GET("/excel", rt.Report.DownloadExcel)
			reportByID.GET("/export-excel", rt.Report.DownloadExcel) // frontend compat
			reportByID.GET("/export-csv", rt.Report.DownloadCSV)    // frontend compat (was /csv)
			reportByID.GET("/csv", rt.Report.DownloadCSV)        // keep original
			reportByID.DELETE("", rt.Report.Delete)
			reportByID.PATCH("/status", rt.Report.UpdateStatus)
			reportByID.PATCH("/confirm", rt.Report.Confirm)       // frontend compat
			reportByID.PATCH("/edit", rt.Report.Edit)             // frontend compat
			reportByID.PATCH("/transition", rt.Report.Transition) // frontend compat
			reportByID.POST("/recalculate", rt.Report.Recalculate)
			reportByID.POST("/overrides", rt.Report.ApplyOverrides)
			reportByID.GET("/approvals", rt.Report.ListApprovals)
			reportByID.POST("/amend", rt.Report.Amend)
			reportByID.GET("/amendments", rt.Report.ListAmendments)
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

	// Reconciliation Session routes
	reconSessions := recon.Group("/sessions")
	{
		reconSessions.POST("", rt.Session.CreateSession)
		reconSessions.GET("", rt.Session.ListSessions)

		reconSessionByID := reconSessions.Group("/:id")
		{
			reconSessionByID.GET("", rt.Session.GetSession)
			reconSessionByID.DELETE("", rt.Session.DeleteSession)
			reconSessionByID.POST("/files", rt.Session.AddFile)
			reconSessionByID.POST("/classify", rt.Session.Classify)
			reconSessionByID.GET("/transactions", rt.Session.ListTransactions)
			reconSessionByID.PATCH("/transactions/bulk", rt.Session.BulkUpdateTransactions)
			reconSessionByID.PATCH("/transactions/:txnId", rt.Session.UpdateTransaction)
			reconSessionByID.POST("/detect-anomalies", rt.Session.DetectAnomalies)
			reconSessionByID.GET("/anomalies", rt.Session.ListAnomalies)
			reconSessionByID.PATCH("/anomalies/:anomalyId", rt.Session.ResolveAnomaly)
			reconSessionByID.GET("/summary", rt.Session.GetSummary)
			reconSessionByID.POST("/reconcile", rt.Session.Reconcile)
			reconSessionByID.POST("/generate-report", rt.Session.GenerateReport)
			reconSessionByID.GET("/export-pdf", rt.Session.ExportPDF)
			reconSessionByID.GET("/export-excel", rt.Session.ExportExcel)
			reconSessionByID.GET("/export", rt.Session.Export)
		}
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
		compliance.GET("/reports/:reportId/suggest-fixes", rt.Compliance.SuggestFixes)
		compliance.POST("/reports/:reportId/auto-fix", rt.Compliance.AutoFix)
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
		memory.GET("/preferences", rt.Memory.ListPreferences)
		memory.GET("/preferences/:reportType", rt.Memory.GetPreference)
		memory.PUT("/preferences/:reportType", rt.Memory.UpsertPreference)
		memory.DELETE("/preferences/:reportType", rt.Memory.DeletePreference)
		memory.GET("/corrections", rt.Memory.ListCorrections)
	}

	// Notification routes
	notifications := api.Group("/notifications")
	notifications.Use(authMw)
	{
		notifications.GET("", rt.Notification.List)
		notifications.GET("/unread-count", rt.Notification.UnreadCount)
		notifications.PATCH("/:id/read", rt.Notification.MarkRead)
		notifications.POST("/mark-all-read", rt.Notification.MarkAllRead)
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

	// Settings routes
	settings := api.Group("/settings")
	settings.Use(authMw)
	{
		settings.GET("/company", rt.Settings.GetCompanySettings)
		settings.PUT("/company", rt.Settings.UpdateCompanySettings)
		settings.GET("/team", rt.Settings.ListTeam)
		settings.PATCH("/team/:userId/role", rt.Settings.UpdateMemberRole)
	}

	// Telegram routes
	if rt.Telegram != nil {
		tg := api.Group("/telegram")
		tg.Use(authMw)
		{
			tg.POST("/link-token", rt.Telegram.GenerateLinkToken)
		}
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

	// Chart of Accounts routes
	accounts := api.Group("/accounts")
	accounts.Use(authMw)
	{
		accounts.POST("", rt.Account.Create)
		accounts.GET("", rt.Account.List)
		accounts.POST("/seed", rt.Account.Seed)
		accountByID := accounts.Group("/:id")
		{
			accountByID.GET("", rt.Account.Get)
			accountByID.PUT("", rt.Account.Update)
			accountByID.DELETE("", rt.Account.Delete)
			accountByID.GET("/balance", rt.Account.Balance)
		}
	}

	// Journal Entry routes
	journalEntries := api.Group("/journal-entries")
	journalEntries.Use(authMw)
	{
		journalEntries.POST("", rt.Journal.Create)
		journalEntries.GET("", rt.Journal.List)
		jeByID := journalEntries.Group("/:id")
		{
			jeByID.GET("", rt.Journal.Get)
			jeByID.POST("/post", rt.Journal.Post)
			jeByID.POST("/reverse", rt.Journal.Reverse)
		}
	}

	// Accounting Period routes
	periods := api.Group("/accounting-periods")
	periods.Use(authMw)
	{
		periods.POST("", rt.Period.Create)
		periods.GET("", rt.Period.List)
		periods.POST("/generate", rt.Period.Generate)
		periodByID := periods.Group("/:id")
		{
			periodByID.POST("/close", rt.Period.Close)
			periodByID.POST("/reopen", rt.Period.Reopen)
		}
	}

	// General Ledger routes
	gl := api.Group("/gl")
	gl.Use(authMw)
	{
		gl.GET("/trial-balance", rt.GL.TrialBalance)
		gl.GET("/account/:id/ledger", rt.GL.AccountLedger)
	}

	// ---- Pipeline Bridge Routes ----

	// Receipt-to-Transaction bridge
	receipts.POST("/batches/:id/convert", rt.ReceiptBridge.Convert)

	// Journal entry generation bridge
	journalBridge := api.Group("/journals")
	journalBridge.Use(authMw)
	{
		journalBridge.POST("/generate", rt.JournalBridge.Generate)
	}

	// Financial statements
	statements := api.Group("/statements")
	statements.Use(authMw)
	{
		statements.GET("/balance-sheet", rt.FinStatement.BalanceSheet)
		statements.GET("/income-statement", rt.FinStatement.IncomeStatement)
	}

	// Tax bridge (GL → Tax Engine + eBIRForms export)
	tax := api.Group("/tax")
	tax.Use(authMw)
	{
		tax.POST("/calculate", rt.TaxBridge.Calculate)
		tax.GET("/export", rt.TaxBridge.Export)
	}

	// QuickBooks Online routes
	if rt.QBO != nil {
		qboGroup := api.Group("/qbo")
		qboGroup.Use(authMw)
		{
			qboGroup.GET("/auth-url", rt.QBO.AuthURL)
			qboGroup.GET("/callback", rt.QBO.Callback)
			qboGroup.GET("/status", rt.QBO.Status)
			qboGroup.POST("/disconnect", rt.QBO.Disconnect)
			qboGroup.POST("/sync/accounts", rt.QBO.SyncAccounts)
			qboGroup.GET("/sync/logs", rt.QBO.SyncLogs)
		}
	}
}
