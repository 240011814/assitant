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

	aiAgentService, err := service.NewAIAgentService(cfg.AI.TimeoutMinutes, systemConfigService)
	if err != nil {
		log.Fatalf("Failed to initialize AI Agent Service: %v", err)
	}
	promptService := service.NewPromptService(service.DB, aiAgentService)
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

	cutService := service.NewCutService(cfg.Baostock.URL)
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

	// 定时 Agent 任务 (到点让 Agent/编排带着输入跑一遍, 结果落训练历史 + 可选通知)
	agentTaskService := service.NewAgentTaskService(jobScheduler, aiAgentService, orchestrationService, historyService, emailNotifier, telegramService)
	agentTaskHandler := api.NewAgentTaskHandler(agentTaskService)

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
		noteHandler:          noteHandler,
		reminderHandler:      reminderHandler,
		systemConfigHandler:  systemConfigHandler,
		telegramHandler:      telegramHandler,
		userPrefHandler:      userPrefHandler,
	})

	// Start Telegram Bot
	go func() {
		if err := telegramService.StartBot(); err != nil {
			log.Printf("Warning: Failed to start Telegram Bot: %v", err)
		}
	}()

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
