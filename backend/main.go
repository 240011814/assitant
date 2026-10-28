package main

import (
	"backend/service/ai/agent"
	"backend/service/ai/agenttask"
	"backend/service/ai/history"
	"backend/service/ai/mcp"
	"backend/service/ai/mem0"
	"backend/service/ai/modelscenario"
	"backend/service/ai/orchestration"
	"backend/service/ai/prompt"
	"backend/service/ai/rag"
	"backend/service/ai/skill"
	"backend/service/ai/tokenusage"
	"backend/service/ai/tts"
	"backend/service/ai/userdoc"
	"backend/service/ai/usermemory"
	"backend/service/jobs"
	"backend/service/kernel"
	"backend/service/learning"
	"backend/service/notify/notifier"
	"backend/service/notify/reminder"
	"backend/service/stock"
	"backend/service/sys/admin"
	"backend/service/sys/asynctask"
	"backend/service/sys/auth"
	"backend/service/sys/dashboard"
	"backend/service/sys/systemconfig"
	"backend/service/sys/userpref"
	"backend/service/tool/mihoyo"
	"backend/service/tool/wol"
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"backend/api"
	"backend/config"
	"backend/service"
	"backend/service/aitools"
	"backend/service/cut"
	"backend/service/tool/lottery"

	"github.com/gin-gonic/gin"
)

// main 只做依赖装配与进程生命周期; HTTP 路由全部在 router.go 的 setupRouter
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

	db, err := service.InitDB(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize Database: %v", err)
	}

	// 业务子包各自的 DB 接线: 子包声明 var DB, 这里统一注入 (子包内部 DB. 调用点无需改)
	lottery.DB = db
	cut.DB = db
	prompt.DB = db
	learning.DB = db
	systemconfig.DB = db
	auth.DB = db
	admin.DB = db
	dashboard.DB = db
	modelscenario.DB = db
	userpref.DB = db
	tokenusage.DB = db
	mem0.DB = db
	history.DB = db
	notifier.DB = db
	jobs.DB = db
	reminder.DB = db
	usermemory.DB = db
	agent.DB = db
	skill.DB = db
	mcp.DB = db
	orchestration.DB = db
	agenttask.DB = db
	stock.DB = db
	userdoc.DB = db
	mihoyo.DB = db
	asynctask.DB = db

	// 操作审计日志异步写入 (需在 DB 初始化后启动)
	api.InitAuditLogger()

	// 异步任务重启清扫: 旧进程遗留的 running 任务不可能再有结果, 置 failed 供重新触发
	asynctask.SweepInterrupted()
	// 词汇量测试题面下发前剥离正确答案 (async_jobs 通用轮询端点按 kind 调用)
	asynctask.RegisterRedactor(asynctask.KindVocabTest, learning.RedactVocabTestResult)

	// ClickHouse (分析库, 可选): 初始化失败不阻断启动, 相关同步自动跳过
	if err := service.InitClickHouse(cfg); err != nil {
		log.Printf("Warning: ClickHouse 初始化失败(将跳过 CH 同步): %v", err)
	}
	// CH 连接注入给需要 ClickHouse 的子包 (未启用时为 nil, 各子包自行降级)
	stock.CH = service.CH
	rag.CH = service.CH

	aitools.SetDB(service.DB)

	systemConfigService := systemconfig.NewSystemConfigService()

	authService := auth.NewAuthService(cfg)
	adminService := admin.NewAdminService()

	noteService := learning.NewNoteService()
	noteHandler := api.NewNoteHandler(noteService)

	aiAgentService, err := agent.NewAIAgentService(cfg.AI.TimeoutMinutes, systemConfigService)
	if err != nil {
		log.Fatalf("Failed to initialize AI Agent Service: %v", err)
	}
	promptService := prompt.NewPromptService(service.DB, aiAgentService)
	aiAgentHandler := api.NewAIAgentHandler(aiAgentService)

	// 用户画像/经历服务 (与 mem0 并存)
	userMemoryService := usermemory.NewUserMemoryService(aiAgentService, systemConfigService)
	aiAgentService.SetMemoryService(userMemoryService)
	aitools.SetUserMemoryService(userMemoryService)
	userPortraitHandler := api.NewUserPortraitHandler(userMemoryService)

	// mem0 从 ai_tools 表读取配置
	timeoutConfig := systemConfigService.GetTimeoutConfig()
	var mem0Timeout time.Duration
	if timeoutConfig.HTTPTimeout > 0 {
		mem0Timeout = time.Duration(timeoutConfig.HTTPTimeout) * time.Second
	}
	mem0Svc := mem0.NewMem0Service(mem0Timeout, service.DB)
	mem0Svc.LoadFromTool()
	aitools.SetMem0Service(mem0Svc)

	// 用户文档 (S3 存储, 配置在系统配置页 s3_* 键): 上传管理 + AI 文档读取工具
	// RAG 文档语义检索 (向量存储当前用 ClickHouse, 换后端只需换 NewXxxVectorStore + OpenAI 兼容嵌入, 本地 Ollama):
	// 配置在系统配置页 rag_* 键
	ragService := rag.NewRagService(systemConfigService, rag.NewCHVectorStore())
	userDocumentService := userdoc.NewUserDocumentService(systemConfigService, ragService)
	aitools.SetUserDocumentStore(userDocumentService)
	aitools.SetDocumentSearcher(userDocumentService)
	userDocumentHandler := api.NewUserDocumentHandler(userDocumentService)

	// MCP 服务动态注册 (配置在 mcp_servers 表, 管理页 CRUD):
	// 连接 -> 发现工具 -> 注册 mcp_<server>_<tool>; ai_tools 行用户手动管理, 删服务同步删行/禁用同步禁行
	mcpService := mcp.NewMCPService()
	mcpService.SetAgentService(aiAgentService)
	mcpService.Init()
	mcpHandler := api.NewMCPHandler(mcpService)

	adminHandler := api.NewAdminHandler(adminService, aiAgentService, authService, mem0Svc)

	dashboardService := dashboard.NewDashboardService()
	dashboardHandler := api.NewDashboardHandler(dashboardService)

	cutService := cut.NewCutService(cfg.Baostock.URL)
	cutHandler := api.NewCutHandler(cutService)

	promptHandler := api.NewPromptHandler(promptService)

	mem0Handler := api.NewMem0Handler(mem0Svc)

	// Telegram Bot
	telegramService := notifier.NewTelegramService(systemConfigService)
	telegramHandler := api.NewTelegramHandler(telegramService, systemConfigService)

	// Notification
	emailNotifier := notifier.NewEmailNotifier(systemConfigService)
	aitools.SetEmailNotifier(emailNotifier)

	// Job Scheduler
	jobAlertService := jobs.NewJobAlertService(emailNotifier)
	jobScheduler, err := jobs.NewJobScheduler(jobAlertService)
	if err != nil {
		log.Printf("Warning: Failed to create job scheduler: %v", err)
	}

	// Reminder Service
	reminderService := reminder.NewReminderService(jobScheduler, emailNotifier, telegramService)
	reminderHandler := api.NewReminderHandler(reminderService)
	aitools.SetReminderService(reminderService)

	// TTS 语音合成 (tts_mode: frontend/openai/edge, 配置页保存后热刷新;
	// openai 模式复用 AI 对话默认激活的 Provider, 不单独维护 key)
	ttsService := tts.NewTtsService(systemConfigService, aiAgentService)
	ttsHandler := api.NewTtsHandler(ttsService)

	systemConfigHandler := api.NewSystemConfigHandler(systemConfigService, emailNotifier)
	systemConfigHandler.SetRAGService(ragService)

	userPrefService := userpref.NewUserPreferenceService()
	userPrefHandler := api.NewUserPreferenceHandler(userPrefService)

	historyService := history.NewHistoryService(mem0Svc)
	historyHandler := api.NewHistoryHandler(historyService)

	lotteryService := lottery.NewLotteryService()
	lotteryHandler := api.NewLotteryHandler(lotteryService)

	modelScenarioService := modelscenario.NewModelScenarioService()
	modelScenarioHandler := api.NewModelScenarioHandler(modelScenarioService)

	skillService := skill.NewAISkillService(aiAgentService)
	skillHandler := api.NewSkillHandler(skillService)

	orchestrationService := orchestration.NewAIOrchestrationService(aiAgentService, promptService, cfg.AI.TimeoutMinutes)
	orchestrationHandler := api.NewAIOrchestrationHandler(orchestrationService, historyService)

	// 定时 Agent 任务 (到点让 Agent/编排带着输入跑一遍, 结果落训练历史 + 可选通知)
	agentTaskService := agenttask.NewAgentTaskService(jobScheduler, aiAgentService, orchestrationService, historyService, emailNotifier, telegramService)
	agentTaskHandler := api.NewAgentTaskHandler(agentTaskService)

	courseService := learning.NewCourseService()
	courseHandler := api.NewCourseHandler(courseService)

	errorBookService := learning.NewErrorBookService()
	errorBookHandler := api.NewErrorBookHandler(errorBookService)

	// 词汇 FSRS 复习 / 出题练习 / 分级阅读 (依赖 AI 服务、文档服务与课程服务, 故在三者之后装配)
	vocabService := learning.NewVocabularyService(aiAgentService, userDocumentService, userPrefService)
	vocabHandler := api.NewVocabularyHandler(vocabService)
	quizService := learning.NewQuizService(aiAgentService, vocabService, systemConfigService)
	quizHandler := api.NewQuizHandler(quizService)
	readingService := learning.NewReadingService(aiAgentService, courseService, systemConfigService)
	readingHandler := api.NewReadingHandler(readingService)
	rssService := learning.NewRSSService(readingService)
	rssHandler := api.NewRSSHandler(rssService)

	// 通用异步任务轮询 (owner 校验与结果脱敏在 service 内)
	asyncJobHandler := api.NewAsyncJobHandler()

	// 接口超时配置 (默认值+覆盖项存 system_config, 全量路由清单由 gin 枚举)
	apiTimeoutHandler := api.NewApiTimeoutHandler(systemConfigService)

	stockService := stock.NewStockService()
	// baostock 同步服务: handler 的手动同步端点与 tasks 包的定时任务共用同一实例 (互斥锁同源)
	stockSyncService := stock.NewStockSyncService(cfg.Baostock.URL)
	stockChSyncService := stock.NewStockChSyncService()
	stockHandler := api.NewStockHandler(stockService, stockSyncService, stockChSyncService)

	// 自选股预警 (基于最新收盘评估, Telegram/邮件推送)
	stockAlertService := stock.NewStockAlertService(emailNotifier, telegramService)
	stockAlertHandler := api.NewStockAlertHandler(stockAlertService)

	// 策略回测 (ClickHouse 只读副本)
	backtestHandler := api.NewBacktestHandler(stock.NewBacktestService())

	// 设备管理 (WOL MQTT): 长连接会话 + 设备发现/唤醒/地址簿, 配置存 system_config
	wolMqttService := wol.NewWolMqttService(systemConfigService)
	wolMqttHandler := api.NewWolMqttHandler(wolMqttService)

	// 启动自动连接 MQTT (只尝试一次, 失败不重试; 可到设备管理页手动连接)
	kernel.SafeGo("mqtt:auto-connect", func() {
		if err := wolMqttService.Connect(); err != nil {
			log.Printf("Warning: 自动连接 MQTT 失败(不重试): %v", err)
		}
	})

	// 米哈游原神签到 (扫码登录绑定 + 每日定时签到)
	mihoyoService := mihoyo.NewMihoyoService()
	mihoyoHandler := api.NewMihoyoHandler(mihoyoService)

	// Job Handler + 注册后台可调度的定时任务 (全部注册收拢在 tasks 包, 按域一个文件)
	jobHandler := api.NewJobHandler(jobScheduler)
	jobs.RegisterAll(&jobs.Deps{
		JobScheduler:     jobScheduler,
		StockSync:        stockSyncService,
		StockCh:          stockChSyncService,
		StockAlert:       stockAlertService,
		AgentTaskHandler: agentTaskService.AgentTaskHandler,
		UserMemory:       userMemoryService,
		RSS:              rssService,
		ReminderHandler:  reminderService.ReminderTaskHandler,
		Mihoyo:           mihoyoService,
	})

	// 全部 HTTP 路由 (router.go)
	setupRouter(r, &appDeps{
		jwtSecret:            cfg.Auth.JWTSecret,
		authService:          authService,
		systemConfigService:  systemConfigService,
		aiAgentService:       aiAgentService,
		historyService:       historyService,
		historyHandler:       historyHandler,
		adminHandler:         adminHandler,
		agentTaskHandler:     agentTaskHandler,
		aiAgentHandler:       aiAgentHandler,
		orchestrationHandler: orchestrationHandler,
		mem0Handler:          mem0Handler,
		userDocumentHandler:  userDocumentHandler,
		userPortraitHandler:  userPortraitHandler,
		cutHandler:           cutHandler,
		modelScenarioHandler: modelScenarioHandler,
		skillHandler:         skillHandler,
		courseHandler:        courseHandler,
		errorBookHandler:     errorBookHandler,
		stockHandler:         stockHandler,
		stockAlertHandler:    stockAlertHandler,
		backtestHandler:      backtestHandler,
		lotteryHandler:       lotteryHandler,
		mcpHandler:           mcpHandler,
		jobHandler:           jobHandler,
		dashboardHandler:     dashboardHandler,
		promptHandler:        promptHandler,
		vocabHandler:         vocabHandler,
		quizHandler:          quizHandler,
		readingHandler:       readingHandler,
		rssHandler:           rssHandler,
		noteHandler:          noteHandler,
		reminderHandler:      reminderHandler,
		asyncJobHandler:      asyncJobHandler,
		apiTimeoutHandler:    apiTimeoutHandler,
		systemConfigHandler:  systemConfigHandler,
		ttsHandler:           ttsHandler,
		telegramHandler:      telegramHandler,
		userPrefHandler:      userPrefHandler,
		wolMqttHandler:       wolMqttHandler,
		mihoyoHandler:        mihoyoHandler,
	})

	// Start Telegram Bot
	kernel.SafeGo("telegram:start-bot", func() {
		if err := telegramService.StartBot(); err != nil {
			log.Printf("Warning: Failed to start Telegram Bot: %v", err)
		}
	})

	// Start Scheduler
	if jobScheduler != nil {
		jobScheduler.Start()
		if err := jobScheduler.LoadCronDefinitions(); err != nil {
			log.Printf("加载定时任务定义失败: %v", err)
		}
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

	// 主动停止 Telegram long-poll, 避免进程重启后旧请求仍占用 getUpdates 导致 409
	telegramService.Shutdown()

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
