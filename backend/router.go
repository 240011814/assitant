package main

import (
	"backend/api"
	"backend/service/ai/agent"
	"backend/service/ai/history"
	"backend/service/sys/auth"
	"backend/service/sys/systemconfig"

	"github.com/gin-gonic/gin"
)

// appDeps 路由装配所需的全部依赖 (main.go 负责构造, setupRouter 只做挂载)
type appDeps struct {
	jwtSecret            string
	authService          *auth.AuthService
	systemConfigService  *systemconfig.SystemConfigService
	aiAgentService       *agent.AIAgentService
	historyService       *history.HistoryService
	historyHandler       *api.HistoryHandler
	adminHandler         *api.AdminHandler
	agentTaskHandler     *api.AgentTaskHandler
	aiAgentHandler       *api.AIAgentHandler
	orchestrationHandler *api.AIOrchestrationHandler
	mem0Handler          *api.Mem0Handler
	userDocumentHandler  *api.UserDocumentHandler
	userPortraitHandler  *api.UserPortraitHandler
	cutHandler           *api.CutHandler
	modelScenarioHandler *api.ModelScenarioHandler
	skillHandler         *api.SkillHandler
	courseHandler        *api.CourseHandler
	errorBookHandler     *api.ErrorBookHandler
	stockHandler         *api.StockHandler
	stockAlertHandler    *api.StockAlertHandler
	backtestHandler      *api.BacktestHandler
	lotteryHandler       *api.LotteryHandler
	mcpHandler           *api.MCPHandler
	jobHandler           *api.JobHandler
	dashboardHandler     *api.DashboardHandler
	promptHandler        *api.PromptHandler
	vocabHandler         *api.VocabularyHandler
	quizHandler          *api.QuizHandler
	readingHandler       *api.ReadingHandler
	rssHandler           *api.RSSHandler
	noteHandler          *api.NoteHandler
	reminderHandler      *api.ReminderHandler
	asyncJobHandler      *api.AsyncJobHandler
	apiTimeoutHandler    *api.ApiTimeoutHandler
	systemConfigHandler  *api.SystemConfigHandler
	ttsHandler           *api.TtsHandler
	telegramHandler      *api.TelegramHandler
	userPrefHandler      *api.UserPreferenceHandler
	wolMqttHandler       *api.WolMqttHandler
	mihoyoHandler        *api.MihoyoHandler
}

// setupRouter 把全部 HTTP 路由挂载到 engine (与原 main.go 逐行等价, 仅依赖来源改为 appDeps)
func setupRouter(r *gin.Engine, d *appDeps) {
	// 接口超时管理的路由枚举器: 闭包 engine, 请求时才求值 (此刻全部路由已注册)。
	// nil 容忍: router_test 用零值依赖只验证路由注册本身
	if d.apiTimeoutHandler != nil {
		d.apiTimeoutHandler.SetRouteLister(func() gin.RoutesInfo { return r.Routes() })
	}

	authService := d.authService
	systemConfigHandler := d.systemConfigHandler
	ttsHandler := d.ttsHandler
	telegramHandler := d.telegramHandler
	userPrefHandler := d.userPrefHandler
	reminderHandler := d.reminderHandler
	agentTaskHandler := d.agentTaskHandler
	dashboardHandler := d.dashboardHandler
	aiAgentService := d.aiAgentService
	historyService := d.historyService
	historyHandler := d.historyHandler
	promptHandler := d.promptHandler
	vocabHandler := d.vocabHandler
	quizHandler := d.quizHandler
	readingHandler := d.readingHandler
	rssHandler := d.rssHandler
	noteHandler := d.noteHandler
	aiAgentHandler := d.aiAgentHandler
	orchestrationHandler := d.orchestrationHandler
	mem0Handler := d.mem0Handler
	userDocumentHandler := d.userDocumentHandler
	userPortraitHandler := d.userPortraitHandler
	cutHandler := d.cutHandler
	modelScenarioHandler := d.modelScenarioHandler
	skillHandler := d.skillHandler
	courseHandler := d.courseHandler
	errorBookHandler := d.errorBookHandler
	stockHandler := d.stockHandler
	stockAlertHandler := d.stockAlertHandler
	backtestHandler := d.backtestHandler
	lotteryHandler := d.lotteryHandler
	adminHandler := d.adminHandler
	mcpHandler := d.mcpHandler
	jobHandler := d.jobHandler
	wolMqttHandler := d.wolMqttHandler
	mihoyoHandler := d.mihoyoHandler
	apiTimeoutHandler := d.apiTimeoutHandler
	asyncJobHandler := d.asyncJobHandler

	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "AI English Learning Backend is running",
		})
	})

	// Telegram Webhook (公开接口，无需认证)
	r.POST("/api/telegram/webhook", telegramHandler.HandleWebhook)

	authGroup := r.Group("/auth")
	{
		authGroup.POST("/login", api.HandleLogin(authService))
		authGroup.POST("/register", api.HandleRegister(authService, d.systemConfigService))
		authGroup.GET("/getUserInfo", api.HandleGetUserInfo(authService, d.jwtSecret))
		authGroup.POST("/refreshToken", api.HandleRefreshToken(authService))
		authGroup.GET("/register-status", systemConfigHandler.GetRegisterStatus)

		// 2FA verify (temp token in Authorization header, no full auth required)
		authGroup.POST("/2fa/verify", api.Handle2FAVerify(authService))
	}

	apiGroup := r.Group("/api")
	apiGroup.Use(api.AuthMiddleware(d.jwtSecret))
	apiGroup.Use(api.AuditMiddleware())
	{
		// User Profile APIs
		apiGroup.GET("/user/profile", api.HandleGetUserProfile(authService))
		apiGroup.PUT("/user/profile", api.HandleUpdateProfile(authService))
		apiGroup.PUT("/user/password", api.HandleChangePassword(authService))

		// User 2FA self-service (个人中心两步验证开关)
		apiGroup.POST("/user/2fa/setup", api.HandleUser2FASetup(authService))
		apiGroup.POST("/user/2fa/enable", api.HandleUser2FAEnable(authService))
		apiGroup.POST("/user/2fa/disable", api.HandleUser2FADisable(authService))

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

		// TTS 语音合成 (按系统配置 tts_mode 分流: 前端本地朗读 / OpenAI 兼容 / Edge 在线)
		apiGroup.GET("/tts/config", ttsHandler.HandleGetConfig)
		apiGroup.GET("/tts", ttsHandler.HandleSynthesize)

		// Reminders
		apiGroup.GET("/reminders", reminderHandler.List)
		apiGroup.POST("/reminders", reminderHandler.Create)
		apiGroup.PUT("/reminders/:id", reminderHandler.Update)

		// 定时 Agent 任务 (用户侧, 仅本人数据)
		agentTaskGroup := apiGroup.Group("/agent-tasks")
		{
			agentTaskGroup.GET("", agentTaskHandler.HandleList)
			agentTaskGroup.POST("", agentTaskHandler.HandleCreate)
			agentTaskGroup.PUT("/:id", agentTaskHandler.HandleUpdate)
			agentTaskGroup.DELETE("/:id", agentTaskHandler.HandleDelete)
			agentTaskGroup.POST("/:id/run", agentTaskHandler.HandleRunNow)
		}
		apiGroup.DELETE("/reminders/:id", reminderHandler.Delete)

		// 通用异步任务轮询 (AI 写课文/词汇量测试/RSS 生词提取/RSS 对照翻译等; 仅需登录, service 做 owner 校验)
		apiGroup.GET("/async-jobs/:id", d.asyncJobHandler.HandleGet)

		// 前端接口请求超时配置下发 (仅需登录; 前端登录后拉一次, axios onRequest 钩子按路由匹配)
		apiGroup.GET("/api-timeouts", d.apiTimeoutHandler.HandleGetTimeouts)

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
			vocabGroup.GET("/review/session", vocabHandler.HandleGetReviewSession)
			vocabGroup.GET("/review/stats", vocabHandler.HandleGetReviewStats)
			vocabGroup.GET("/review/forecast", vocabHandler.HandleGetReviewForecast)
			vocabGroup.GET("/quick-session", vocabHandler.HandleGetQuickSession)
			vocabGroup.GET("/learning-stats", vocabHandler.HandleGetLearningStats)
			vocabGroup.POST("/review/:id", api.RequirePermission("ai:vocabulary:edit"), vocabHandler.HandleSubmitReview)
			vocabGroup.GET("/:id/assist", vocabHandler.HandleAssistWord)
			vocabGroup.POST("/import", api.RequirePermission("ai:vocabulary:add"), vocabHandler.HandleImportWords)
			vocabGroup.POST("/import/document", api.RequirePermission("ai:vocabulary:add"), vocabHandler.HandleExtractFromDocument)
			vocabGroup.PUT("/:id", api.RequirePermission("ai:vocabulary:edit"), vocabHandler.HandleUpdateWord)
			vocabGroup.DELETE("/:id", api.RequirePermission("ai:vocabulary:delete"), vocabHandler.HandleDeleteWord)
		}

		// 出题练习 (挂 vocabulary 权限域)
		quizGroup := apiGroup.Group("/vocabulary/quiz")
		quizGroup.Use(api.RequirePermission("ai:vocabulary:view"))
		{
			quizGroup.POST("/sessions", quizHandler.HandleCreateQuizSession)
			quizGroup.GET("/sessions/active", quizHandler.HandleGetActiveQuizSession)
			quizGroup.GET("/sessions/:id", quizHandler.HandleGetQuizSession)
			quizGroup.POST("/sessions/:id/answer", api.RequirePermission("ai:vocabulary:edit"), quizHandler.HandleSubmitQuizAnswer)
			quizGroup.POST("/sentence-practice", quizHandler.HandleSentencePractice)
		}

		// 分级阅读 (挂 vocabulary 权限域)
		readingGroup := apiGroup.Group("/reading")
		readingGroup.Use(api.RequirePermission("ai:vocabulary:view"))
		{
			readingGroup.POST("/import", api.RequirePermission("ai:vocabulary:add"), readingHandler.HandleImportReading)
			readingGroup.GET("/docs", readingHandler.HandleListReadingDocs)
			readingGroup.GET("/docs/:id", readingHandler.HandleGetReadingDoc)
			readingGroup.DELETE("/docs/:id", api.RequirePermission("ai:vocabulary:delete"), readingHandler.HandleDeleteReadingDoc)
			readingGroup.POST("/docs/:id/annotate", readingHandler.HandleAnnotateReadingDoc)
			readingGroup.GET("/lookup", readingHandler.HandleLookupWord)
			readingGroup.POST("/generate-passage", readingHandler.HandleGeneratePassage)
			readingGroup.POST("/translate", readingHandler.HandleTranslate)
			readingGroup.POST("/heartbeat", readingHandler.HandleReadingHeartbeat)
			readingGroup.POST("/vocab-test/start", readingHandler.HandleStartVocabTest)
			readingGroup.POST("/vocab-test/submit", readingHandler.HandleSubmitVocabTest)
			readingGroup.GET("/vocab-test/latest", readingHandler.HandleLatestVocabTest)
		}

		// RSS 阅读 (挂 vocabulary 权限域)
		rssGroup := apiGroup.Group("/rss")
		rssGroup.Use(api.RequirePermission("ai:vocabulary:view"))
		{
			rssGroup.POST("/sources", api.RequirePermission("ai:vocabulary:add"), rssHandler.HandleAddRSSSource)
			rssGroup.PUT("/sources/:id", api.RequirePermission("ai:vocabulary:add"), rssHandler.HandleUpdateRSSSource)
			rssGroup.GET("/sources", rssHandler.HandleListRSSSources)
			rssGroup.DELETE("/sources/:id", api.RequirePermission("ai:vocabulary:delete"), rssHandler.HandleDeleteRSSSource)
			rssGroup.POST("/sources/:id/fetch", rssHandler.HandleFetchRSSSource)
			rssGroup.GET("/items", rssHandler.HandleListRSSItems)
			rssGroup.POST("/items/:id/read", rssHandler.HandleMarkRSSItemRead)
			rssGroup.GET("/items/:id/content", rssHandler.HandleGetRSSItemContent)
			rssGroup.POST("/items/:id/import", rssHandler.HandleImportRSSItem)
			rssGroup.POST("/items/:id/annotate", rssHandler.HandleAnnotateRSSItem)
			rssGroup.POST("/items/:id/translate", rssHandler.HandleTranslateRSSItem)
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
			docGroup.GET("/status", userDocumentHandler.HandleStatus)
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
			cutGroup.PUT("/scraps/:id", api.RequirePermission("cut:record:create"), cutHandler.HandleUpdateScrap)
			cutGroup.DELETE("/scraps/:id", api.RequirePermission("cut:record:delete"), cutHandler.HandleDeleteScrap)
			cutGroup.POST("/scraps/batch-delete", api.RequirePermission("cut:record:delete"), cutHandler.HandleBatchDeleteScraps)
			// 产品单 (待切割产品, 一单多件)
			cutGroup.GET("/products", api.RequirePermission("cut:record:view"), cutHandler.HandleListProducts)
			cutGroup.POST("/products", api.RequirePermission("cut:record:create"), cutHandler.HandleSaveProduct)
			cutGroup.PUT("/products/:id", api.RequirePermission("cut:record:create"), cutHandler.HandleSaveProduct)
			cutGroup.DELETE("/products/:id", api.RequirePermission("cut:record:delete"), cutHandler.HandleDeleteProduct)
		}

		cutRecordGroup := apiGroup.Group("/cutRecord")
		cutRecordGroup.Use(api.RequirePermission("cut:menu:view"))
		{
			cutRecordGroup.POST("/add", api.RequirePermission("cut:record:create"), cutHandler.HandleAddRecord)
			cutRecordGroup.GET("/list", api.RequirePermission("cut:record:view"), cutHandler.HandleListRecords)
			cutRecordGroup.GET("/detail/:id", api.RequirePermission("cut:record:view"), cutHandler.HandleGetRecord)
			cutRecordGroup.DELETE("/delete/:id", api.RequirePermission("cut:record:delete"), cutHandler.HandleDeleteRecord)
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
			orchestrationGroup.POST("/debug-node", api.RequirePermission("system:orchestration:debug"), orchestrationHandler.HandleDebugNode)
			orchestrationGroup.GET("/orchestrations/:id/runs", orchestrationHandler.HandleListRuns)
			orchestrationGroup.GET("/my-templates", orchestrationHandler.HandleListMyTemplates)
			orchestrationGroup.POST("/my-templates", orchestrationHandler.HandleSaveMyTemplate)
			orchestrationGroup.DELETE("/my-templates/:id", orchestrationHandler.HandleDeleteMyTemplate)
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
			courseGroup.DELETE("/:id/items/batch", api.RequirePermission("ai:course:edit"), courseHandler.BatchDeleteCourseItems)
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

			// 宏观经济数据 (统一走 MacroHandler, 按 kind 查表, 路径不变)
			macroPerm := api.RequirePermission("stock:macro:view")
			stockGroup.GET("/macro/deposit-rate", macroPerm, stockHandler.MacroHandler("deposit-rates"))
			stockGroup.GET("/macro/loan-rate", macroPerm, stockHandler.MacroHandler("loan-rates"))
			stockGroup.GET("/macro/reserve-ratio", macroPerm, stockHandler.MacroHandler("reserve-ratios"))
			stockGroup.GET("/macro/money-supply-month", macroPerm, stockHandler.MacroHandler("money-supply-month"))
			stockGroup.GET("/macro/money-supply-year", macroPerm, stockHandler.MacroHandler("money-supply-year"))
			stockGroup.GET("/macro/lpr", macroPerm, stockHandler.MacroHandler("lpr"))
			stockGroup.GET("/macro/gdp", macroPerm, stockHandler.MacroHandler("gdp"))
			stockGroup.GET("/macro/cpi", macroPerm, stockHandler.MacroHandler("cpi"))
			stockGroup.GET("/macro/pmi", macroPerm, stockHandler.MacroHandler("pmi"))
			stockGroup.GET("/macro/ppi", macroPerm, stockHandler.MacroHandler("ppi"))
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

		// 设备管理 (WOL MQTT): 远程发现设备并下发唤醒/地址簿命令
		wolGroup := apiGroup.Group("/wol")
		{
			wolGroup.GET("/config", api.RequirePermission("tool:device:view"), wolMqttHandler.GetConfig)
			wolGroup.GET("/status", api.RequirePermission("tool:device:view"), wolMqttHandler.Status)
			wolGroup.GET("/devices", api.RequirePermission("tool:device:view"), wolMqttHandler.ListDevices)
			wolGroup.GET("/devices/:id/macs", api.RequirePermission("tool:device:view"), wolMqttHandler.GetMacList)
			wolGroup.PUT("/config", api.RequirePermission("tool:device:manage"), wolMqttHandler.UpdateConfig)
			wolGroup.POST("/connect", api.RequirePermission("tool:device:manage"), wolMqttHandler.Connect)
			wolGroup.POST("/disconnect", api.RequirePermission("tool:device:manage"), wolMqttHandler.Disconnect)
			wolGroup.POST("/devices/:id/mac", api.RequirePermission("tool:device:manage"), wolMqttHandler.UpdateMac)
			wolGroup.POST("/devices/:id/wake", api.RequirePermission("tool:device:manage"), wolMqttHandler.Wake)
			wolGroup.POST("/devices/:id/ota", api.RequirePermission("tool:device:manage"), wolMqttHandler.Ota)
			wolGroup.POST("/publish", api.RequirePermission("tool:device:manage"), wolMqttHandler.Publish)
			wolGroup.DELETE("/relay", api.RequirePermission("tool:device:manage"), wolMqttHandler.ClearRelay)
			wolGroup.DELETE("/relay/:id", api.RequirePermission("tool:device:manage"), wolMqttHandler.DeleteRelay)
		}

		// 米哈游 (原神每日签到): 扫码登录绑定 + 签到/状态/解绑
		// view = 查询绑定与签到状态; manage = 扫码绑定/手动签到/解绑 (R_ADMIN 默认授予, 权限后台可调)
		mihoyoGroup := apiGroup.Group("/mihoyo")
		{
			mihoyoGroup.POST("/qr/start", api.RequirePermission("tool:mihoyo:manage"), mihoyoHandler.StartQR)
			mihoyoGroup.GET("/qr/status", api.RequirePermission("tool:mihoyo:view"), mihoyoHandler.QRStatus)
			mihoyoGroup.GET("/status", api.RequirePermission("tool:mihoyo:view"), mihoyoHandler.GetStatus)
			mihoyoGroup.GET("/checkin/status", api.RequirePermission("tool:mihoyo:view"), mihoyoHandler.CheckinStatus)
			mihoyoGroup.POST("/checkin", api.RequirePermission("tool:mihoyo:manage"), mihoyoHandler.CheckinNow)
			mihoyoGroup.DELETE("/bind", api.RequirePermission("tool:mihoyo:manage"), mihoyoHandler.Unbind)
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
			adminGroup.PUT("/users/:id/reset-password", api.RequirePermission("system:user:update"), adminHandler.HandleResetUserPassword)
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
				configGroup.GET("/prompts", systemConfigHandler.HandleListPrompts)
				configGroup.POST("/test-email", systemConfigHandler.SendTestEmail)
				configGroup.POST("/test-embedding", systemConfigHandler.HandleTestEmbedding)
				configGroup.POST("/test-s3", systemConfigHandler.HandleTestS3)
				configGroup.POST("/test-tts", ttsHandler.HandleTestTts)
				// 接口超时配置 (全量路由清单由 gin 枚举生成, 编辑写 api_timeout_* 配置键)
				configGroup.GET("/api-timeouts", apiTimeoutHandler.HandleListRoutes)
				configGroup.PUT("/api-timeouts/route", apiTimeoutHandler.HandleUpdateRoute)
				configGroup.PUT("/api-timeouts/default", apiTimeoutHandler.HandleUpdateDefault)
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

			// 通用异步任务 (只读查询, 审计日志页「异步任务」Tab; 含输入快照, 仅管理员可见)
			adminGroup.GET("/async-jobs", api.RequirePermission("system:audit:view"), asyncJobHandler.HandleAdminList)

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
}
