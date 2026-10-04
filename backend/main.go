package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"backend/api"
	"backend/config"
	"backend/model"
	"backend/service"
	"backend/service/tools"

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Printf("Warning: Failed to load config.yaml: %v", err)
		cfg = &config.Config{}
	}

	// JWT secret 为空或仍是默认值时 fail-fast: 空 secret 意味着任何空 key 签的 token 都能通过认证
	if cfg.Auth.JWTSecret == "" || cfg.Auth.JWTSecret == "soybean-admin-secret" {
		log.Fatal("JWT secret 未配置或仍为默认值, 请在 config.yaml 的 auth.jwt_secret 中设置强随机密钥")
	}

	_, err = service.InitDB(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize Database: %v", err)
	}

	// 操作审计日志异步写入 (需在 DB 初始化后启动)
	api.InitAuditLogger()

	// ClickHouse (分析库, 可选): 初始化失败不阻断启动, 相关同步自动跳过
	if err := service.InitClickHouse(cfg); err != nil {
		log.Printf("Warning: ClickHouse 初始化失败(将跳过 CH 同步): %v", err)
	}

	tools.SetDB(service.DB)

	systemConfigService := service.NewSystemConfigService()

	authService := service.NewAuthService(cfg)
	adminService := service.NewAdminService()

	vocabService := service.NewVocabularyService()
	vocabHandler := api.NewVocabularyHandler(vocabService)

	noteService := service.NewNoteService()
	noteHandler := api.NewNoteHandler(noteService)

	promptService := service.NewPromptService(service.DB, nil)

	aiAgentService, err := service.NewAIAgentService(cfg.AI.TimeoutMinutes, systemConfigService)
	if err != nil {
		log.Fatalf("Failed to initialize AI Agent Service: %v", err)
	}
	promptService = service.NewPromptService(service.DB, aiAgentService)
	aiAgentHandler := api.NewAIAgentHandler(aiAgentService)

	// 用户画像/经历服务 (与 mem0 并存)
	userMemoryService := service.NewUserMemoryService(aiAgentService, systemConfigService)
	aiAgentService.SetMemoryService(userMemoryService)
	tools.SetUserMemoryService(userMemoryService)
	userPortraitHandler := api.NewUserPortraitHandler(userMemoryService)

	// mem0 从 ai_tools 表读取配置
	timeoutConfig := systemConfigService.GetTimeoutConfig()
	var mem0Timeout time.Duration
	if timeoutConfig.HTTPTimeout > 0 {
		mem0Timeout = time.Duration(timeoutConfig.HTTPTimeout) * time.Second
	}
	mem0Svc := service.NewMem0Service(mem0Timeout, service.DB)
	mem0Svc.LoadFromTool()
	tools.SetMem0Service(mem0Svc)

	// 用户文档 (S3 存储, 配置在系统配置页 s3_* 键): 上传管理 + AI 文档读取工具
	// RAG 文档语义检索 (向量存储当前用 ClickHouse, 换后端只需换 NewXxxVectorStore + OpenAI 兼容嵌入, 本地 Ollama):
	// 配置在系统配置页 rag_* 键
	ragService := service.NewRagService(systemConfigService, service.NewCHVectorStore())
	userDocumentService := service.NewUserDocumentService(systemConfigService, ragService)
	tools.SetUserDocumentStore(userDocumentService)
	tools.SetDocumentSearcher(userDocumentService)
	userDocumentHandler := api.NewUserDocumentHandler(userDocumentService)

	// MCP 服务动态注册 (配置在 mcp_servers 表, 管理页 CRUD):
	// 连接 -> 发现工具 -> 注册 mcp_<server>_<tool>; ai_tools 行用户手动管理, 删服务同步删行/禁用同步禁行
	mcpService := service.NewMCPService()
	mcpService.SetAgentService(aiAgentService)
	mcpService.Init()
	mcpHandler := api.NewMCPHandler(mcpService)

	adminHandler := api.NewAdminHandler(adminService, aiAgentService, authService, mem0Svc)

	dashboardService := service.NewDashboardService()
	dashboardHandler := api.NewDashboardHandler(dashboardService)

	cutService := service.NewCutService()
	cutHandler := api.NewCutHandler(cutService)

	promptHandler := api.NewPromptHandler(promptService)

	mem0Handler := api.NewMem0Handler(mem0Svc)

	// Telegram Bot
	telegramService := service.NewTelegramService(systemConfigService)
	telegramHandler := api.NewTelegramHandler(telegramService, systemConfigService)

	// Notification
	emailNotifier := service.NewEmailNotifier(systemConfigService)
	tools.SetEmailNotifier(emailNotifier)

	// Job Scheduler
	jobAlertService := service.NewJobAlertService(emailNotifier)
	jobScheduler, err := service.NewJobScheduler(jobAlertService)
	if err != nil {
		log.Printf("Warning: Failed to create job scheduler: %v", err)
	}

	// Reminder Service
	reminderService := service.NewReminderService(jobScheduler, emailNotifier, telegramService)
	reminderHandler := api.NewReminderHandler(reminderService)
	tools.SetReminderService(reminderService)

	systemConfigHandler := api.NewSystemConfigHandler(systemConfigService, telegramService, emailNotifier, aiAgentService)
	systemConfigHandler.SetUserDocumentService(userDocumentService)
	systemConfigHandler.SetRAGService(ragService)

	userPrefService := service.NewUserPreferenceService()
	userPrefHandler := api.NewUserPreferenceHandler(userPrefService)

	historyService := service.NewHistoryService(mem0Svc)
	historyHandler := api.NewHistoryHandler(historyService)

	lotteryService := service.NewLotteryService()
	lotteryHandler := api.NewLotteryHandler(lotteryService)

	modelScenarioService := service.NewModelScenarioService()
	modelScenarioHandler := api.NewModelScenarioHandler(modelScenarioService)

	skillService := service.NewAISkillService(aiAgentService)
	skillHandler := api.NewSkillHandler(skillService)

	orchestrationService := service.NewAIOrchestrationService(aiAgentService, promptService, cfg.AI.TimeoutMinutes)
	orchestrationHandler := api.NewAIOrchestrationHandler(orchestrationService, historyService)

	courseService := service.NewCourseService()
	courseHandler := api.NewCourseHandler(courseService)

	errorBookService := service.NewErrorBookService()
	errorBookHandler := api.NewErrorBookHandler(errorBookService)

	stockService := service.NewStockService()
	stockHandler := api.NewStockHandler(stockService, cfg.Baostock.URL)

	// 自选股预警 (基于最新收盘评估, Telegram/邮件推送)
	stockAlertService := service.NewStockAlertService(emailNotifier, telegramService)
	stockAlertHandler := api.NewStockAlertHandler(stockAlertService)

	// 策略回测 (ClickHouse 只读副本)
	backtestHandler := api.NewBacktestHandler(service.NewBacktestService())

	// Job Handler + 注册后台可调度的定时任务
	jobHandler := api.NewJobHandler(jobScheduler)
	if jobScheduler != nil {
		stockHandler.RegisterCronTasks(jobScheduler)
		userMemoryService.RegisterCronTasks(jobScheduler)
		jobScheduler.RegisterTask("stock.evaluate_alerts", "评估自选股预警规则并推送 (建议每个交易日收盘后)", json.RawMessage(`{}`), func(_ *model.JobDefinition, _ json.RawMessage) error {
			_, err := stockAlertService.EvaluateAll()
			return err
		})
		// 操作审计日志清理 (默认种子任务: 每日 4 点, 保留 90 天, 可在任务管理页调整)
		jobScheduler.RegisterTask("audit.cleanup", "清理过期的操作审计日志 (参数 retentionDays 为保留天数)", json.RawMessage(`{"retentionDays":90}`), func(_ *model.JobDefinition, params json.RawMessage) error {
			return service.CleanupAuditLogs(params)
		})
	}

	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "AI English Learning Backend is running",
		})
	})

	// Telegram Webhook (公开接口，无需认证)
	r.POST("/api/telegram/webhook", telegramHandler.HandleWebhook)

	authGroup := r.Group("/auth")
	{
		authGroup.POST("/login", api.HandleLogin(authService))
		authGroup.POST("/register", api.HandleRegister(authService, systemConfigService))
		authGroup.GET("/getUserInfo", api.HandleGetUserInfo(authService, cfg.Auth.JWTSecret))
		authGroup.POST("/refreshToken", api.HandleRefreshToken(authService))
		authGroup.GET("/register-status", systemConfigHandler.GetRegisterStatus)

		// 2FA endpoints (temp token in Authorization header, no full auth required)
		authGroup.POST("/2fa/setup", api.Handle2FASetup(authService))
		authGroup.POST("/2fa/verify", api.Handle2FAVerify(authService))
	}

	apiGroup := r.Group("/api")
	apiGroup.Use(api.AuthMiddleware(cfg.Auth.JWTSecret))
	apiGroup.Use(api.AuditMiddleware())
	{
		// User Profile APIs
		apiGroup.GET("/user/profile", api.HandleGetUserProfile(authService))
		apiGroup.PUT("/user/profile", api.HandleUpdateProfile(authService))
		apiGroup.PUT("/user/password", api.HandleChangePassword(authService))

		// User Preferences
		apiGroup.GET("/user/preferences/theme", userPrefHandler.GetThemePreference)
		apiGroup.PUT("/user/preferences/theme", userPrefHandler.SaveThemePreference)
		apiGroup.GET("/user/preferences/notification", userPrefHandler.GetNotificationPreference)
		apiGroup.PUT("/user/preferences/notification", userPrefHandler.SaveNotificationPreference)

		// Telegram Binding
		apiGroup.GET("/telegram/config", telegramHandler.HandleGetTelegramConfig)
		apiGroup.GET("/telegram/status", telegramHandler.HandleGetTelegramStatus)
		apiGroup.POST("/telegram/bind-code", telegramHandler.HandleGenerateBindCode)
		apiGroup.POST("/telegram/unbind", telegramHandler.HandleUnbindTelegram)

		// Reminders
		apiGroup.GET("/reminders", reminderHandler.List)
		apiGroup.POST("/reminders", reminderHandler.Create)
		apiGroup.PUT("/reminders/:id", reminderHandler.Update)
		apiGroup.DELETE("/reminders/:id", reminderHandler.Delete)

		apiGroup.GET("/dashboard/stats", dashboardHandler.GetStats)
		apiGroup.GET("/token-usages/my", api.HandleMyTokenUsage)
		apiGroup.GET("/ai/models", api.RequirePermission("ai:model:view"), api.HandleListModels(aiAgentService))
		apiGroup.POST("/chat", api.RequirePermission("ai:chat:send"), api.HandleChatStream(aiAgentService, historyService))
		apiGroup.POST("/chat/tool-approval", api.RequirePermission("ai:chat:send"), api.HandleToolApproval(aiAgentService, historyService))

		// User specific AI prompt management
		promptGroup := apiGroup.Group("/user-prompts")
		{
			promptGroup.GET("/:agentId", api.RequirePermission("ai:prompt:view"), promptHandler.GetUserPrompt)
			promptGroup.POST("/:agentId", api.RequirePermission("ai:prompt:save"), promptHandler.SaveUserPrompt)
			promptGroup.PUT("/:agentId/switch", api.RequirePermission("ai:prompt:switch"), promptHandler.SwitchUserPrompt)
			promptGroup.DELETE("/:agentId/versions/:versionId", api.RequirePermission("ai:prompt:delete"), promptHandler.HandleDeleteVersion)
			promptGroup.DELETE("/:agentId", api.RequirePermission("ai:prompt:reset"), promptHandler.ResetUserPrompt)
		}

		vocabGroup := apiGroup.Group("/vocabulary")
		vocabGroup.Use(api.RequirePermission("ai:vocabulary:view"))
		{
			vocabGroup.POST("", api.RequirePermission("ai:vocabulary:add"), vocabHandler.HandleAddWord)
			vocabGroup.GET("", vocabHandler.HandleListWords)
			vocabGroup.GET("/random", vocabHandler.HandleGetRandomWords)
			vocabGroup.GET("/review/due", vocabHandler.HandleGetDueWords)
			vocabGroup.GET("/review/stats", vocabHandler.HandleGetReviewStats)
			vocabGroup.POST("/review/:id", api.RequirePermission("ai:vocabulary:edit"), vocabHandler.HandleSubmitReview)
			vocabGroup.PUT("/:id", api.RequirePermission("ai:vocabulary:edit"), vocabHandler.HandleUpdateWord)
			vocabGroup.DELETE("/:id", api.RequirePermission("ai:vocabulary:delete"), vocabHandler.HandleDeleteWord)
		}

		noteGroup := apiGroup.Group("/notes")
		noteGroup.Use(api.RequirePermission("ai:note:view"))
		{
			noteGroup.POST("", api.RequirePermission("ai:note:create"), noteHandler.HandleCreateNote)
			noteGroup.GET("", noteHandler.HandleListNotes)
			noteGroup.PUT("/:id", api.RequirePermission("ai:note:edit"), noteHandler.HandleUpdateNote)
			noteGroup.DELETE("/:id", api.RequirePermission("ai:note:delete"), noteHandler.HandleDeleteNote)
		}

		historyGroup := apiGroup.Group("/histories")
		historyGroup.Use(api.RequirePermission("ai:history:view"))
		{
			historyGroup.GET("", historyHandler.ListHistory)
			historyGroup.GET("/:id", historyHandler.GetHistory)
			historyGroup.PUT("/:id/favorite", api.RequirePermission("ai:history:favorite"), historyHandler.UpdateFavorite)
			historyGroup.PUT("/:id/title", api.RequirePermission("ai:history:edit"), historyHandler.UpdateTitle)
			historyGroup.DELETE("/:id", api.RequirePermission("ai:history:delete"), historyHandler.DeleteHistory)
			historyGroup.POST("/:id/share", api.RequirePermission("ai:history:edit"), historyHandler.GenerateShare)
			historyGroup.DELETE("/:id/share", api.RequirePermission("ai:history:edit"), historyHandler.RevokeShare)
		}

		aiAgentGroup := apiGroup.Group("/ai-agents")
		{
			aiAgentGroup.GET("", aiAgentHandler.ListAvailableAgents)
			aiAgentGroup.GET("/:id", aiAgentHandler.GetAIAgent)
			aiAgentGroup.POST("", api.RequirePermission("ai:custom-training:create"), aiAgentHandler.CreateAIAgent)
			aiAgentGroup.PUT("/:id", api.RequirePermission("ai:custom-training:edit"), aiAgentHandler.UpdateAIAgent)
			aiAgentGroup.DELETE("/:id", api.RequirePermission("ai:custom-training:delete"), aiAgentHandler.DeleteAIAgent)
		}

		// 训练中心「编排对话」: 复用 Agent Studio 编排运行时, 面向前台训练用户
		// (列表/详情仅要求登录; 对话要求 ai:chat:send, 与 /chat 一致)
		aiOrchestrationGroup := apiGroup.Group("/ai-orchestrations")
		{
			aiOrchestrationGroup.GET("", orchestrationHandler.HandleChatList)
			aiOrchestrationGroup.GET("/:id", orchestrationHandler.HandleChatGet)
			aiOrchestrationGroup.POST("/:id/chat", api.RequirePermission("ai:chat:send"), orchestrationHandler.HandleChatRun)
			// 编排工具审批: 一次运行的审批请求/决定靠 run_id 配对, 仅要求登录
			aiOrchestrationGroup.POST("/approvals/resolve", orchestrationHandler.HandleResolveApproval)
		}

		// Memory APIs (mem0)
		memoryGroup := apiGroup.Group("/memories")
		{
			memoryGroup.GET("/status", mem0Handler.HandleStatus)
			memoryGroup.GET("", mem0Handler.HandleListMemories)
			memoryGroup.POST("", mem0Handler.HandleAddMemory)
			memoryGroup.POST("/search", mem0Handler.HandleSearchMemories)
			memoryGroup.DELETE("/:id", mem0Handler.HandleDeleteMemory)
		}

		// 用户文档管理 (S3 存储; AI 经 search_user_documents/read_document/list_user_documents 工具使用)
		docGroup := apiGroup.Group("/documents")
		{
			docGroup.GET("", api.RequirePermission("document:view"), userDocumentHandler.HandleList)
			docGroup.GET("/:id/text", api.RequirePermission("document:view"), userDocumentHandler.HandleText)
			docGroup.GET("/:id/download", api.RequirePermission("document:view"), userDocumentHandler.HandleDownload)
			docGroup.POST("/upload", api.RequirePermission("document:upload"), userDocumentHandler.HandleUpload)
			docGroup.POST("/:id/reindex", api.RequirePermission("document:upload"), userDocumentHandler.HandleReindex)
			docGroup.DELETE("/:id", api.RequirePermission("document:delete"), userDocumentHandler.HandleDelete)
		}

		// 用户画像与经历 (本地, 与 mem0 并存; 本人数据仅登录+归属校验)
		userMemoryGroup := apiGroup.Group("/user")
		{
			userMemoryGroup.GET("/portrait", userPortraitHandler.GetPortrait)
			userMemoryGroup.PUT("/portrait", userPortraitHandler.UpdatePortrait)
			userMemoryGroup.POST("/portrait/extract", userPortraitHandler.TriggerExtract)
			userMemoryGroup.GET("/experiences", userPortraitHandler.ListExperiences)
			userMemoryGroup.POST("/experiences", userPortraitHandler.CreateExperience)
			userMemoryGroup.PUT("/experiences/:id", userPortraitHandler.UpdateExperience)
			userMemoryGroup.DELETE("/experiences/:id", userPortraitHandler.DeleteExperience)
		}

		// Cut APIs
		cutGroup := apiGroup.Group("/cut")
		cutGroup.Use(api.RequirePermission("cut:menu:view"))
		{
			cutGroup.POST("/bar", api.RequirePermission("cut:bar:compute"), cutHandler.HandleBarCut)
			cutGroup.POST("/plane", api.RequirePermission("cut:plane:compute"), cutHandler.HandlePlaneCut)
			cutGroup.GET("/scraps", api.RequirePermission("cut:record:view"), cutHandler.HandleListScraps)
			cutGroup.POST("/scraps", api.RequirePermission("cut:record:create"), cutHandler.HandleAddScraps)
			cutGroup.POST("/scraps/consume", api.RequirePermission("cut:record:create"), cutHandler.HandleConsumeScraps)
			cutGroup.DELETE("/scraps/:id", api.RequirePermission("cut:record:delete"), cutHandler.HandleDeleteScrap)
		}

		cutRecordGroup := apiGroup.Group("/cutRecord")
		cutRecordGroup.Use(api.RequirePermission("cut:menu:view"))
		{
			cutRecordGroup.POST("/add", api.RequirePermission("cut:record:create"), cutHandler.HandleAddRecord)
			cutRecordGroup.GET("/list", api.RequirePermission("cut:record:view"), cutHandler.HandleListRecords)
			cutRecordGroup.POST("/delete/:id", api.RequirePermission("cut:record:delete"), cutHandler.HandleDeleteRecord)
		}

		// 模型和场景 APIs
		modelScenarioGroup := apiGroup.Group("/model-scenario")
		modelScenarioGroup.Use(api.RequirePermission("model_scenario:view"))
		{
			modelScenarioGroup.GET("", modelScenarioHandler.HandleList)
			modelScenarioGroup.POST("", api.RequirePermission("model_scenario:create"), modelScenarioHandler.HandleCreate)
			modelScenarioGroup.PUT("/:id", api.RequirePermission("model_scenario:update"), modelScenarioHandler.HandleUpdate)
			modelScenarioGroup.DELETE("/:id", api.RequirePermission("model_scenario:delete"), modelScenarioHandler.HandleDelete)
		}

		// Skill APIs (Eino Skill Middleware 动态加载)
		skillGroup := apiGroup.Group("/skills")
		skillGroup.Use(api.RequirePermission("system:skill:view"))
		{
			skillGroup.GET("", skillHandler.HandleList)
			skillGroup.GET("/discover/github/cache", skillHandler.HandleGetCachedDiscovery)
			skillGroup.POST("/discover/github", skillHandler.HandleDiscoverGitHub)
			skillGroup.POST("", api.RequirePermission("system:skill:create"), skillHandler.HandleCreate)
			skillGroup.PUT("/:id", api.RequirePermission("system:skill:update"), skillHandler.HandleUpdate)
			skillGroup.DELETE("/:id", api.RequirePermission("system:skill:delete"), skillHandler.HandleDelete)
		}

		// Agent Studio 编排 APIs (Eino compose Chain/Graph/Workflow 可视化编排)
		orchestrationGroup := apiGroup.Group("/agent-studio")
		orchestrationGroup.Use(api.RequirePermission("system:orchestration:view"))
		{
			orchestrationGroup.GET("/orchestrations", orchestrationHandler.HandleList)
			orchestrationGroup.GET("/orchestrations/:id", orchestrationHandler.HandleGet)
			orchestrationGroup.POST("/orchestrations", api.RequirePermission("system:orchestration:create"), orchestrationHandler.HandleCreate)
			orchestrationGroup.PUT("/orchestrations/:id", api.RequirePermission("system:orchestration:update"), orchestrationHandler.HandleUpdate)
			orchestrationGroup.DELETE("/orchestrations/:id", api.RequirePermission("system:orchestration:delete"), orchestrationHandler.HandleDelete)
			orchestrationGroup.POST("/orchestrations/validate", orchestrationHandler.HandleValidate)
			orchestrationGroup.GET("/resources", orchestrationHandler.HandleResources)
			orchestrationGroup.POST("/debug", api.RequirePermission("system:orchestration:debug"), orchestrationHandler.HandleDebugRun)
		}

		// Course APIs
		courseGroup := apiGroup.Group("/courses")
		courseGroup.Use(api.RequirePermission("ai:course:view"))
		{
			courseGroup.GET("", courseHandler.ListCourses)
			courseGroup.GET("/:id", courseHandler.GetCourse)
			courseGroup.POST("", api.RequirePermission("ai:course:create"), courseHandler.CreateCourse)
			courseGroup.PUT("/:id", api.RequirePermission("ai:course:edit"), courseHandler.UpdateCourse)
			courseGroup.DELETE("/:id", api.RequirePermission("ai:course:delete"), courseHandler.DeleteCourse)
			courseGroup.GET("/:id/items", courseHandler.GetCourseItems)
			courseGroup.POST("/:id/items", api.RequirePermission("ai:course:edit"), courseHandler.CreateCourseItem)
			courseGroup.POST("/:id/items/batch", api.RequirePermission("ai:course:edit"), courseHandler.BatchCreateCourseItems)
			courseGroup.DELETE("/:id/items/batch", api.RequirePermission("ai:course:delete"), courseHandler.BatchDeleteCourseItems)
			courseGroup.PUT("/:id/items/:itemId", api.RequirePermission("ai:course:edit"), courseHandler.UpdateCourseItem)
			courseGroup.DELETE("/:id/items/:itemId", api.RequirePermission("ai:course:edit"), courseHandler.DeleteCourseItem)
			courseGroup.GET("/:id/training", courseHandler.GetTrainingStatus)
			courseGroup.PUT("/:id/training", courseHandler.UpdateTrainingStatus)
			courseGroup.POST("/:id/training/increment", courseHandler.IncrementTrainingCount)
		}

		// Error Book APIs
		errorBookGroup := apiGroup.Group("/error-book")
		errorBookGroup.Use(api.RequirePermission("ai:error-book:view"))
		{
			errorBookGroup.POST("", api.RequirePermission("ai:error-book:add"), errorBookHandler.HandleAddErrorBook)
			errorBookGroup.GET("", errorBookHandler.HandleListErrorBooks)
			errorBookGroup.GET("/practice", api.RequirePermission("ai:error-book:practice"), errorBookHandler.HandleGetErrorBookForPractice)
			errorBookGroup.GET("/random", api.RequirePermission("ai:error-book:practice"), errorBookHandler.HandleGetRandomErrorBooks)
			errorBookGroup.GET("/stats", errorBookHandler.HandleGetErrorBookStats)
			errorBookGroup.PUT("/:id", api.RequirePermission("ai:error-book:edit"), errorBookHandler.HandleUpdateErrorBook)
			errorBookGroup.DELETE("/:id", api.RequirePermission("ai:error-book:delete"), errorBookHandler.HandleDeleteErrorBook)
		}

		// Stock APIs
		stockGroup := apiGroup.Group("/stock")
		stockGroup.Use(api.RequirePermission("stock:menu:view"))
		{
			stockGroup.POST("/screen", api.RequirePermission("stock:screen:view"), stockHandler.HandleScreen)
			stockGroup.GET("/industries", stockHandler.HandleGetIndustries)
			stockGroup.GET("/concepts", stockHandler.HandleGetConcepts)
			stockGroup.GET("/:code", stockHandler.HandleGetDetail)
			stockGroup.GET("/:code/kline", stockHandler.HandleGetKline)
			stockGroup.GET("/:code/finance-history", stockHandler.HandleGetFinanceHistory)
			stockGroup.GET("/:code/sync-state", stockHandler.HandleGetSyncState)

			// 宏观经济数据
			stockGroup.GET("/macro/deposit-rate", api.RequirePermission("stock:macro:view"), stockHandler.HandleGetMacroDepositRates)
			stockGroup.GET("/macro/loan-rate", api.RequirePermission("stock:macro:view"), stockHandler.HandleGetMacroLoanRates)
			stockGroup.GET("/macro/reserve-ratio", api.RequirePermission("stock:macro:view"), stockHandler.HandleGetMacroReserveRatios)
			stockGroup.GET("/macro/money-supply-month", api.RequirePermission("stock:macro:view"), stockHandler.HandleGetMacroMoneySupplyMonth)
			stockGroup.GET("/macro/money-supply-year", api.RequirePermission("stock:macro:view"), stockHandler.HandleGetMacroMoneySupplyYear)
			stockGroup.GET("/macro/lpr", api.RequirePermission("stock:macro:view"), stockHandler.HandleGetMacroLPR)
			stockGroup.GET("/macro/gdp", api.RequirePermission("stock:macro:view"), stockHandler.HandleGetMacroGDP)
			stockGroup.GET("/macro/cpi", api.RequirePermission("stock:macro:view"), stockHandler.HandleGetMacroCPI)
			stockGroup.GET("/macro/pmi", api.RequirePermission("stock:macro:view"), stockHandler.HandleGetMacroPMI)
			stockGroup.GET("/macro/ppi", api.RequirePermission("stock:macro:view"), stockHandler.HandleGetMacroPPI)
			stockGroup.POST("/macro/sync", api.RequirePermission("stock:sync:execute"), stockHandler.HandleSyncMacro)

			// 筛选条件管理
			stockGroup.POST("/filters", api.RequirePermission("stock:screen:save"), stockHandler.HandleSaveFilterCondition)
			stockGroup.GET("/filters", stockHandler.HandleListFilterConditions)
			stockGroup.DELETE("/filters/:id", stockHandler.HandleDeleteFilterCondition)

			// 自选股管理
			stockGroup.POST("/watchlist", api.RequirePermission("stock:watchlist:edit"), stockHandler.HandleAddWatchlist)
			stockGroup.GET("/watchlist", api.RequirePermission("stock:watchlist:view"), stockHandler.HandleListWatchlist)
			stockGroup.GET("/watchlist/groups", api.RequirePermission("stock:watchlist:view"), stockHandler.HandleListWatchlistGroups)
			stockGroup.GET("/watchlist/codes", api.RequirePermission("stock:watchlist:view"), stockHandler.HandleListWatchlistCodes)
			stockGroup.DELETE("/watchlist/:id", api.RequirePermission("stock:watchlist:edit"), stockHandler.HandleDeleteWatchlist)

			// 自选股预警 (复用 watchlist 权限)
			stockGroup.GET("/alerts", api.RequirePermission("stock:watchlist:view"), stockAlertHandler.HandleListAlerts)
			stockGroup.POST("/alerts", api.RequirePermission("stock:watchlist:edit"), stockAlertHandler.HandleCreateAlert)
			stockGroup.PUT("/alerts/:id", api.RequirePermission("stock:watchlist:edit"), stockAlertHandler.HandleUpdateAlert)
			stockGroup.DELETE("/alerts/:id", api.RequirePermission("stock:watchlist:edit"), stockAlertHandler.HandleDeleteAlert)

			// 策略回测 (ClickHouse 只读副本)
			stockGroup.POST("/backtest", api.RequirePermission("stock:screen:view"), backtestHandler.HandleRun)

			// 数据同步(管理员)
			stockGroup.POST("/sync/stock-list", api.RequirePermission("stock:sync:execute"), stockHandler.HandleSyncStockList)
			stockGroup.POST("/sync/daily-quotes", api.RequirePermission("stock:sync:execute"), stockHandler.HandleSyncDailyQuotes)
			stockGroup.POST("/sync/single", api.RequirePermission("stock:sync:execute"), stockHandler.HandleSyncSingleStock)
			stockGroup.POST("/sync/finance", api.RequirePermission("stock:sync:execute"), stockHandler.HandleSyncFinanceData)
			stockGroup.POST("/sync/finance-all", api.RequirePermission("stock:sync:execute"), stockHandler.HandleSyncAllFinance)
			stockGroup.GET("/sync/status", stockHandler.HandleSyncStatus)
		}

		// Lottery Admin APIs (需要登录+权限)
		lotteryGroup := apiGroup.Group("/lottery")
		lotteryGroup.Use(api.RequirePermission("lottery:menu:view"))
		{
			// 活动管理
			lotteryGroup.POST("/activities", api.RequirePermission("lottery:activity:create"), lotteryHandler.HandleCreateActivity)
			lotteryGroup.PUT("/activities/:id", api.RequirePermission("lottery:activity:update"), lotteryHandler.HandleUpdateActivity)
			lotteryGroup.DELETE("/activities/:id", api.RequirePermission("lottery:activity:delete"), lotteryHandler.HandleDeleteActivity)
			lotteryGroup.DELETE("/activities/:id/records", api.RequirePermission("lottery:record:delete"), lotteryHandler.HandleDeleteRecordsByActivityID)

			// 奖品管理
			lotteryGroup.POST("/activities/:id/prizes", api.RequirePermission("lottery:prize:create"), lotteryHandler.HandleCreatePrize)
			lotteryGroup.PUT("/prizes/:id", api.RequirePermission("lottery:prize:update"), lotteryHandler.HandleUpdatePrize)
			lotteryGroup.DELETE("/prizes/:id", api.RequirePermission("lottery:prize:delete"), lotteryHandler.HandleDeletePrize)

			// 记录管理
			lotteryGroup.DELETE("/records/:id", api.RequirePermission("lottery:record:delete"), lotteryHandler.HandleDeleteRecord)
		}

		adminGroup := apiGroup.Group("/admin")
		{
			// User Management
			adminGroup.GET("/users", api.RequirePermission("system:user:list"), adminHandler.HandleListUsers)
			adminGroup.POST("/users", api.RequirePermission("system:user:create"), adminHandler.HandleCreateUser)
			adminGroup.PUT("/users/:id", api.RequirePermission("system:user:update"), adminHandler.HandleUpdateUser)
			adminGroup.DELETE("/users/:id", api.RequirePermission("system:user:delete"), adminHandler.HandleDeleteUser)
			adminGroup.POST("/users/:id/proxy-login", api.RequireRole("R_SUPER"), adminHandler.HandleProxyLogin)

			// Role Management
			adminGroup.GET("/roles", api.RequirePermission("system:role:list"), adminHandler.HandleListRoles)
			adminGroup.POST("/roles", api.RequirePermission("system:role:create"), adminHandler.HandleCreateRole)
			adminGroup.DELETE("/roles/:roleCode", api.RequirePermission("system:role:delete"), adminHandler.HandleDeleteRole)

			// Permission Management
			adminGroup.GET("/permissions", api.RequirePermission("system:permission:view"), adminHandler.HandleListPermissions)
			adminGroup.POST("/permissions", api.RequirePermission("system:permission:create"), adminHandler.HandleCreatePermission)
			adminGroup.PUT("/permissions/:id", api.RequirePermission("system:permission:update"), adminHandler.HandleUpdatePermission)
			adminGroup.DELETE("/permissions/:id", api.RequirePermission("system:permission:delete"), adminHandler.HandleDeletePermission)

			// Role Permission Management
			adminGroup.GET("/roles/:roleCode/permissions", api.RequirePermission("system:role:permission:view"), adminHandler.HandleGetRolePermissions)
			adminGroup.PUT("/roles/:roleCode/permissions", api.RequirePermission("system:role:permission:update"), adminHandler.HandleUpdateRolePermissions)

			// AI Config Management
			adminGroup.GET("/ai-providers", api.RequirePermission("system:ai-provider:view"), adminHandler.HandleListAIProviders)
			adminGroup.POST("/ai-providers", api.RequirePermission("system:ai-provider:create"), adminHandler.HandleCreateAIProvider)
			adminGroup.PUT("/ai-providers/:id", api.RequirePermission("system:ai-provider:update"), adminHandler.HandleUpdateAIProvider)
			adminGroup.DELETE("/ai-providers/:id", api.RequirePermission("system:ai-provider:delete"), adminHandler.HandleDeleteAIProvider)

			adminGroup.GET("/ai-models", api.RequirePermission("system:ai-model:view"), adminHandler.HandleListAIModels)
			adminGroup.POST("/ai-models", api.RequirePermission("system:ai-model:create"), adminHandler.HandleCreateAIModel)
			adminGroup.PUT("/ai-models/:id", api.RequirePermission("system:ai-model:update"), adminHandler.HandleUpdateAIModel)
			adminGroup.DELETE("/ai-models/:id", api.RequirePermission("system:ai-model:delete"), adminHandler.HandleDeleteAIModel)
			adminGroup.POST("/ai-test", api.RequirePermission("system:ai-provider:view"), adminHandler.HandleTestAIConnection)

			adminGroup.GET("/ai-tools", api.RequirePermission("system:ai-tool:view"), adminHandler.HandleListAITools)
			adminGroup.GET("/ai-tools/meta", api.RequirePermission("system:ai-tool:view"), adminHandler.HandleListAIToolMetas)
			adminGroup.GET("/ai-tools/:id", api.RequirePermission("system:ai-tool:view"), adminHandler.HandleGetAITool)
			adminGroup.POST("/ai-tools", api.RequirePermission("system:ai-tool:create"), adminHandler.HandleCreateAITool)
			adminGroup.PUT("/ai-tools/:id", api.RequirePermission("system:ai-tool:update"), adminHandler.HandleUpdateAITool)
			adminGroup.DELETE("/ai-tools/:id", api.RequirePermission("system:ai-tool:delete"), adminHandler.HandleDeleteAITool)

			// MCP 服务管理 (动态注册/发现工具)
			mcpGroup := adminGroup.Group("/mcp-servers")
			mcpGroup.Use(api.RequirePermission("system:mcp:manage"))
			{
				mcpGroup.GET("", mcpHandler.HandleList)
				mcpGroup.POST("", mcpHandler.HandleCreate)
				mcpGroup.PUT("/:id", mcpHandler.HandleUpdate)
				mcpGroup.POST("/:id/connect", mcpHandler.HandleConnect)
				mcpGroup.DELETE("/:id", mcpHandler.HandleDelete)
			}

			// System Config (R_SUPER only)
			configGroup := adminGroup.Group("/system-config")
			configGroup.Use(api.RequireRole("R_SUPER"))
			{
				configGroup.GET("", systemConfigHandler.GetAll)
				configGroup.PUT("", systemConfigHandler.Update)
				configGroup.POST("/test-email", systemConfigHandler.SendTestEmail)
				configGroup.POST("/test-embedding", systemConfigHandler.HandleTestEmbedding)
			}

			// Job Management (定时任务后台管理)
			jobGroup := adminGroup.Group("/jobs")
			{
				jobGroup.GET("/tasks", api.RequirePermission("job:manage"), jobHandler.HandleListTaskRegistry)
				jobGroup.GET("", api.RequirePermission("job:manage"), jobHandler.HandleListJobs)
				jobGroup.POST("", api.RequirePermission("job:manage"), api.RequirePermission("job:edit"), jobHandler.HandleCreateJob)
				jobGroup.PUT("/:id", api.RequirePermission("job:manage"), api.RequirePermission("job:edit"), jobHandler.HandleUpdateJob)
				jobGroup.DELETE("/:id", api.RequirePermission("job:manage"), api.RequirePermission("job:edit"), jobHandler.HandleDeleteJob)
				jobGroup.POST("/:id/run", api.RequirePermission("job:manage"), api.RequirePermission("job:edit"), jobHandler.HandleRunJob)
				jobGroup.GET("/:id/runs", api.RequirePermission("job:manage"), jobHandler.HandleListJobRuns)
			}

			// 操作审计日志 (只读查询)
			adminGroup.GET("/audit-logs", api.RequirePermission("system:audit:view"), api.HandleListAuditLogs)

			// 系统概览 (管理员视角统计)
			adminGroup.GET("/dashboard", api.RequirePermission("system:dashboard:view"), dashboardHandler.GetAdminStats)

			// Token 用量统计 (管理员; 已并入系统概览页 Tab, 与概览共用一个权限)
			adminGroup.GET("/token-usages", api.RequirePermission("system:dashboard:view"), api.HandleTokenUsageList)
			adminGroup.GET("/token-usages/stats", api.RequirePermission("system:dashboard:view"), api.HandleTokenUsageStats)
		}
	}

	// Lottery Public APIs (无需登录)
	lotteryPublicGroup := r.Group("/api/lottery")
	{
		lotteryPublicGroup.GET("/activities", lotteryHandler.HandleListActivities)
		lotteryPublicGroup.GET("/activities/:id", lotteryHandler.HandleGetActivity)
		lotteryPublicGroup.GET("/activities/:id/prizes", lotteryHandler.HandleListPrizes)
		lotteryPublicGroup.GET("/activities/:id/limits", lotteryHandler.HandleGetDrawLimits)
		lotteryPublicGroup.POST("/draw/:activityId", lotteryHandler.HandleDraw)
		lotteryPublicGroup.GET("/records", lotteryHandler.HandleListRecords)
		lotteryPublicGroup.GET("/winners", lotteryHandler.HandleListWinners)
	}

	// Public share route (no auth required, under /api for reverse proxy compatibility)
	r.GET("/api/share/:token", api.HandleGetSharedHistory(historyService))

	// Start Telegram Bot
	go func() {
		if err := telegramService.StartBot(); err != nil {
			log.Printf("Warning: Failed to start Telegram Bot: %v", err)
		}
	}()

	// Start Scheduler
	if jobScheduler != nil {
		jobScheduler.Start()
		jobScheduler.LoadCronDefinitions()
	}

	// 优雅停机: SIGINT/SIGTERM 后停止调度器、等在途请求完成、排空审计日志队列再退出
	srv := &http.Server{Addr: ":8080", Handler: r}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	if jobScheduler != nil {
		if err := jobScheduler.Shutdown(); err != nil {
			log.Printf("job scheduler shutdown: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}

	// srv.Shutdown 返回即所有在途请求已结束, 不会再有审计入队, 可安全排空
	api.ShutdownAuditLogger()
	log.Println("Server exited")
}
