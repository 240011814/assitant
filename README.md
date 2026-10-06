<div align="center">
	<h1>AI 助手平台</h1>
	<span>AI 训练 · 英语学习 · 股票工具 · 切割优化 的一体化后台</span>
</div>

---

## 简介

多模块 AI 应用平台，前端基于 [SoybeanAdmin](https://github.com/soybeanjs/soybean-admin) (Vue 3) 模板二次开发，后端为 Go + Gin 自研服务，配套 Python FastAPI sidecar 提供行情数据与切割精确求解。

## 功能模块

- **AI 训练中心**: 多 Agent 流式对话（思考过程展示、工具调用审批）、提示词版本管理、对话历史收藏/分享、Token 用量记账与月度限额
- **Agent 编排 (Agent Studio)**: 可视化画布编排 LLM 工作流（线性/分支/循环/并行合流/子 Agent 委派），后端编译为 Eino 的 Chain/Graph/Workflow 运行，SSE 逐节点调试，编排可直接嵌入训练中心对话
- **英语学习**: 课程、词汇 SRS 复习、笔记、错题本、用户画像/长期记忆抽取（LLM 异步流水线）
- **知识库文档**: 用户文档上传（S3 存储）、多格式解析（pdf/xlsx/docx/html/文本）、AI 工具按需分页读取、RAG 语义检索（ClickHouse 向量存储）
- **股票工具**: 自选股、选股筛选、自选股预警、策略回测（ClickHouse 只读副本）、宏观经济数据、行情增量同步（baostock）
- **切割优化**: 一维型材切割（DP 背包 + OR-Tools 列生成精确求解）、二维排样（Guillotine/MaxRects）、余料库存与切割记录
- **系统管理**: 用户/角色/权限 (RBAC)、AI Provider/模型/工具配置、MCP 服务动态注册、Skill 管理、定时任务、操作审计、系统配置热刷新
- **其他**: 备忘提醒（日历）、定时 Agent 任务、彩票活动、Telegram/邮件通知

## 技术栈

| 端 | 技术 |
|---|---|
| 前端 | Vue 3 + TypeScript + Vite + Naive UI + UnoCSS + Pinia（开发端口 9527） |
| 后端 | Go + Gin + GORM + MySQL（端口 8080）+ cloudwego/eino (ADK/compose/react) |
| Sidecar | Python FastAPI + baostock SDK + OR-Tools（端口 3002） |
| 存储 | MySQL（唯一真相源）、ClickHouse（行情/财务只读副本 + RAG 向量）、S3 兼容对象存储（文档） |

## 目录结构

```
backend/            # Go 后端
  api/              #   HTTP 处理器 (按领域一个文件)
  service/          #   业务逻辑 (三层: api -> service -> model)
    db/migrations/  #   Goose SQL 迁移 (//go:embed 启动时自动执行)
  main.go           #   依赖装配与生命周期; 路由在 router.go
frontend/           # Vue3 前端 (SoybeanAdmin 模板, pnpm monorepo)
  src/views/        #   ai/ cut/ tool/ system/ user/ 页面
  build/plugins/    #   vite 插件 (路由 meta 唯一维护点在 plugins/router.ts)
baostock/           # Python sidecar: baostock 代理 (含 SDK 猴子补丁) + 切割精确求解 /cut1d
```

## 快速开始

### 后端

```bash
cd backend
# 1. 准备 config.yaml (不提交 git): database / auth.jwt_secret (强随机) / ai / baostock / clickhouse
# 2. 启动 (Goose 迁移自动执行)
go run .
```

### 前端

```bash
cd frontend
pnpm install
pnpm dev        # http://localhost:9527, 经 vite 代理访问后端
```

### baostock sidecar（行情/切割求解，可选）

```bash
cd baostock
python -m venv .venv && .venv/Scripts/activate   # Windows
pip install -r requirements.txt
uvicorn main:app --port 3002
```

后端通过 `BAOSTOCK_API_URL` 环境变量指向 sidecar；未启动时行情同步与精确切割自动回退内置算法。

## 开发与质量门禁

```bash
# 后端 (backend/)
go build ./... && go vet ./...
golangci-lint run    # 必须 0 issues (backend/.golangci.yml)
go test ./...        # 标准库表驱动测试

# 前端 (frontend/)
pnpm typecheck       # vue-tsc, 0 错误
pnpm lint            # OxLint + ESLint
pnpm build           # 生产构建
```

约定速查（详见 CLAUDE.md）：

- 后端取当前用户一律 `currentUserID(c)`；分页用 `service.NormalizePage`；SSE 用 `setupSSE`/`newSSEEmitter`；多步写包 `DB.Transaction`
- 前端路由 meta 唯一维护点：`build/plugins/router.ts` 的 `customRouteMeta` + `src/router/elegant/routes.ts`（不放 route.json）
- 前端新增 i18n key 三处同步：`zh-cn.ts`、`en-us.ts`、`typings/app.d.ts` (App.I18n.Schema)
- 数据库变更编写 Goose 迁移文件（`backend/service/db/migrations/YYYYMMDDHHMMSS_xxx.sql`）

## 部署

三个组件均有独立 Dockerfile（backend 多阶段构建、frontend nginx 托管、baostock 非 root + HEALTHCHECK），镜像经 GitHub Actions 发布到 GHCR：

- `backend-test.yml` — 后端 build/vet/golangci-lint/test
- `backend-release.yml` / `frontend-release.yml` / `baostock-release.yml` — 镜像构建发布
- `linter.yml` — PR 到 main 的 super-linter

## 协议

[MIT](./LICENSE)（前端基于 SoybeanAdmin 模板，其版权与署名见 LICENSE）
