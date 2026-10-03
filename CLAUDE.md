# CLAUDE.md

# 提示词优化规则

**规则说明：**  
在执行任何操作之前，第一步始终是对用户的提示词进行优化，使其更加清晰和有效。

**优化原则：**

- 保持用户原始意图不变
- 仅优化表达清晰度、结构性和可执行性
- 不改变业务逻辑、技术选型或具体需求
- 优化程度：微调为主，避免大幅重写

**执行流程：**

1. **第一步：判断是否需要优化**

   - 如果用户明确要求"不优化"、"直接执行"等，跳过优化步骤
   - 如果提示词已经非常清晰具体，可以跳过优化
   - 其他情况进入优化流程

2. **第二步：优化提示词**

   - 分析用户原始提示词的核心意图
   - 识别模糊、不清晰或可能产生歧义的部分
   - 生成优化后的提示词（更清晰、更具体、更易执行）
   - 记录优化理由和主要改动点
   - 如果是需求开发类的功能，看看有没有比用户所想要的更好的解决方案，如果有列出来，并说明

3. **第三步：展示 A/B/C 选项（仅展示，不执行）**

   - **A：原始提示词** - 使用用户原始提示词执行
   - **B：优化后的提示词** - 使用优化后的提示词执行
   - **C：重新优化** - 根据反馈重新优化提示词，然后再次展示选项
   - **必须完整展示提示词内容**，不要只显示一行摘要或简化版本
   - 使用代码块或引用格式完整展示，确保用户能看到完整的提示词内容
   - **必须说明优化理由**：简要说明为什么这样优化，主要改进了哪些方面

4. **第四步：等待用户选择**

   - **必须等待用户明确选择 A、B 或 C**
   - 如果用户选择 C：
     - 询问用户希望如何调整（更具体、更简洁、关注不同方面等），或根据用户的反馈意见
     - 根据用户反馈重新优化提示词
     - 返回第三步，再次展示 A/B/C 选项
     - 可以多次迭代，直到用户选择 A 或 B
   - 在用户选择之前，**不得执行任何后续操作**
   - 不得进行代码分析、文件读取、工具调用等操作
   - 只能展示选项，等待用户交互

5. **第五步：执行操作**
   - 用户选择 A 或 B 后，使用选定的提示词执行后续操作

**重要约束：**

- ❌ 禁止在展示选项后立即继续执行
- ❌ 禁止在用户未选择时进行任何分析或工具调用
- ❌ 禁止只展示一行摘要或简化版本的提示词
- ❌ 禁止改变用户原始意图和核心需求
- ❌ 禁止过度优化导致提示词面目全非
- ✅ 必须完整展示提示词内容
- ✅ 必须说明优化理由
- ✅ 只能展示选项并明确说明"请选择 A、B 或 C"
- ✅ 用户选择后再继续执行
- ✅ 如果用户选择 C，根据反馈重新优化并再次展示选项

**用户反馈处理：**

- 如果用户选择 C（重新优化），AI 应该：

  1. 询问用户希望如何调整（更具体、更简洁、关注不同方面等），或根据用户提供的具体反馈
  2. 根据用户反馈重新优化提示词
  3. 返回第三步，再次展示 A/B/C 选项供用户选择
  4. 可以多次迭代，直到用户选择 A 或 B 为止

- ✅ 只能展示选项并明确说明"请选择 A 或 B"
- ✅ 用户选择后再继续执行

## 项目概述

多模块的 AI 应用平台 (Admin 后台)，基于 SoybeanAdmin (Vue 3) 模板构建，涵盖：

- **AI 与英语学习**: AI 对话/Agent、课程、词汇、笔记、错题本、训练、历史
- **切割优化**: 切割方案与记录管理
- **彩票 / 工具箱**: 彩票数据、A 股行情与选股 (`tool/stockdetail`、`tool/stockscreen`)
- **系统管理**: 用户、角色权限 (RBAC)、AI 模型/场景配置、定时任务与提醒
- **能力集成**: OpenAI API + Mem0 记忆服务、Telegram 通知、邮件通知

技术栈:

- **前端**: Vue 3 + TypeScript + Naive UI + UnoCSS + Pinia (端口 9527)
- **后端**: Go + Gin + GORM + MySQL (端口 8080)
- **后端架构**: `api` (领域处理器) → `service` (业务逻辑) → `model` (GORM 模型) 三层; 全局 DB 在 `service/db.go`, 迁移用 Goose (`//go:embed`), 中间件 `AuthMiddleware` (JWT) + `RequirePermission` (RBAC)
- **A 股数据源**: `baostock/` Python FastAPI 代理 (端口 3002), 封装 baostock SDK, Go 后端串行调用做增量同步

## 常用命令

### 前端 (在 `frontend/` 目录下)

```bash
pnpm dev          # 启动开发服务器
pnpm build        # 生产构建
pnpm lint         # OxLint + ESLint 检查并自动修复
pnpm typecheck    # TypeScript 类型检查
```

### 后端 (在 `backend/` 目录下)

```bash
go run .          # 启动服务器
go build ./...    # 编译检查
```

## 项目结构

```
backend/
  api/            # HTTP 处理器 (按领域一个文件)
  model/          # GORM 数据模型
  service/        # 业务逻辑 + 数据库初始化
    db/migrations/ # Goose SQL 迁移文件
  config/         # 配置结构体和加载器
  main.go         # 入口、路由注册

frontend/
  src/
    service/api/  # API 调用层 (按领域一个文件)
    store/        # Pinia 状态管理 (auth, app, theme, route, tab)
    views/        # 页面组件
      ai/         # AI 功能 (聊天、词汇、笔记、训练等)
      cut/        # 切割优化功能
      system/     # 系统管理 (用户、权限、AI 配置)
    hooks/        # Vue 组合式函数
    layouts/      # 布局组件
    locales/      # 国际化 (zh-CN, en-US)
    router/       # 路由 (Elegant Router 自动生成)
  packages/       # 内部共享包 (@sa/axios, @sa/hooks 等)
```

## 代码规范

### 后端

- 三层架构: `api` (处理器) → `service` (业务逻辑) → `model` (数据模型)
- 命名: `HandleXxx` 处理器, `NewXxxService()` 构造函数
- 全局数据库: `var DB *gorm.DB` 在 `service/db.go` 初始化
- 迁移: `//go:embed` 嵌入, Goose 启动时自动执行
- 中间件: `AuthMiddleware` (JWT) → `RequirePermission("resource:action")` (RBAC)
- 响应: `SendSuccess(c, data)` / `SendError(c, code, msg)`
- 文件命名: `snake_case.go`
- baostock api 代码要符合flake8规范

### 前端

- API 层: `src/service/api/` 下按领域导出类型化函数
- 状态管理: Pinia store 按模块组织
- 路由: `route.json` 文件配合 Elegant Router 自动生成
- 文件命名: Vue 组件 `kebab-case.vue`, API 文件 `kebab-case.ts`
- 格式化: 单引号, 无尾逗号, 打印宽度 120, 箭头函数无括号
- 前端路由手动编写，不要用生成的

## 配置文件

- `backend/config.yaml` — 数据库、mem0 API 密钥、baostock API 地址 (环境变量优先)
- `frontend/.env` — 默认环境变量
- `frontend/.env.test` / `.env.prod` — 环境覆盖
- `frontend/.oxfmtrc.json` — OxFmt 格式化规则
- `frontend/.oxlintrc.json` — OxLint 规则

## 注意事项

- Vue 模板中的 IDE 诊断误报可忽略 (TypeScript 检查正常即可)
- 后端 `config.yaml` 不要提交到 git (包含敏感信息)
- 数据库迁移文件按时间戳排序命名: `YYYYMMDDHHMMSS_description.sql`
- 前端 `packages/` 下的内部包通过 pnpm workspace 链接

## 开发注意事项

- 新增接口需要配置对应的权限，后端和前端都需要权限控制
- 数据库有变化，需要编写迁移文件
- 开发中遇到的过的问题和错误以及一些经验，写入到CLAUDE.MD 开发经验中

## 开发经验

MySQL 不支持在一个查询中执行多条 SQL 语句。需要将建表语句拆分成独立的 -- +goose StatementBegin 块

- baostock 代理 (baostock/): 底层 socket 无超时, 服务端偶发卡顿会永久阻塞并占住全局锁导致连锁超时, 已通过 `socket.setdefaulttimeout(60)` 兜底; 登录会话已做复用 (shared.py 包装 login/logout), 查询异常时 force_disconnect 强制重连
- baostock 服务端会不定期重置长连接 ([Errno 104] Connection reset / [Errno 32] Broken pipe, SDK 打印"接收数据异常"): 会话复用下死连接上首个请求必 502, 且 SDK 是混淆闭源 (源文件 %TSD-Header 加密) 内部 socket 状态动不了, 重置后首次重新登录还可能"服务器连接失败"; 代理侧治法 = main.py `_execute_with_retry` 查询异常即 force_disconnect 并自动重试整个请求 (MAX_QUERY_ATTEMPTS=3 + 退避 0.5s/1s; 不重试: ValueError 本地参数错误 + BaostockQueryError 服务端 error_code 非0 的确定性错误; 其余瞬时错误才重连重试), 瞬时断连在代理内消化不再抛 502 打断 Go 同步; 用量计数仍按 HTTP 请求计, 重试多耗的调用靠 45k<50k 余量吸收
- baostock 代理曾用 baostock_lock + \_session_lock 两把锁: force_disconnect 在持 \_session_lock 时做真实网络 logout, 而等 login 的线程在持 baostock_lock 时等 \_session_lock, logout 卡住即锁序倒置导致全服务无响应(同步 def handler 共享 anyio 40 线程池, 池耗尽后 /health 也挂); 已合并为单一可重入锁 baostock_lock(login 复用同一把锁), 消除死锁; 遗留: SDK 调用本身卡死仍会占住该锁, 靠健康检查看门狗自愈
- DLP 加密陷阱: 本机装有文件透明加密(天锐/TSD), 凡 `python.exe`(含 .venv)写入仓库内文件会被加密成 `%TSD-Header-###%`, git/diff 视为二进制、Docker COPY 也会带密文; git.exe/powershell/编辑器写入则保持明文。批量改 .py 后务必用 `git diff --numstat`(出现 `- -` 即中招)或看文件头校验, 中招后 `git checkout -- <file>` 还原再用受信工具重写; 读源码时 baostock SDK 本身的 %TSD-Header 加密属正常
- baostock 周线/月线只返回已完成周期 (周线日期=周最后交易日, 月线=月末交易日), 当前周期不出 K 线; 股票代码与指数代码有重叠 (如 sz.000003 退市股 vs sh.000003 上证B指), 必须结合市场标识区分, 指数走 query_history_index_k_data_plus
- baostock 财务数据从 2007Q1 开始; 每日调用限额默认 5 万次 (BAOSTOCK_API_DAILY_LIMIT), 同步必须增量+串行, 429 限额错误不要重试
- 股票同步水位: 日K按日全量走全局水位 sync_watermark(kline_daily, 只推进到完整成功的一天, 断点续跑); stock_sync_state (每股一行) 保留周/月K与财务水位+状态, 个股日K列由按日同步顺带推进; 原则: 数据表(stock_daily/stock_finance)是真相, 状态/水位表只是缓存+观测, 漂移时最多多拉一次数据(upsert 幂等兜底); 同步成功/失败都要更新状态(失败记录原因, 便于续跑与排查); 迁移里有存量数据回填
- baostock 新版 SDK (≥00.9.30) 提供按日全量接口: query_daily_history_k_AStock(date) 1次调用返回全市场个股日K(含 preclose/tradestatus/peTTM/pbMRQ/psTTM/pcfNcfTTM/isST), 另有 query_daily_history_k_ETF / query_daily_adjust_factor; 实测返回不含指数(sh.000xxx/sz.399xxx 仍走 query_history_index_k_data_plus)和北交所, 历史深度可到 1990 年代; 代理已加 /query_daily_history_k_astock 等端点, 部署需 pip install -U baostock 并跑 baostock/verify_daily_updates.py 验证
- 新版 SDK 移除了 bs.query_history_index_k_data_plus, 但 query_history_k_data_plus 直接支持指数代码(代理 /query_history_index_k_data_plus 内部本就走它, 无需改); 服务端按"证券类型×频率"严格校验 fields: preclose/tradestatus/peTTM 等仅日线有效, 周/月线传了报 error_code 10004012 无效参数(代理 502), Go 侧已按 freq×isIndex 选字段; 直连 baostock 测试时 socket 无超时会被服务端卡死, 务必参考代理 shared.py 的 setdefaulttimeout 兜底
- 日K按日全量同步流程: 交易日历(1990起, 1次调用, 含未来交易日) → 逐日拉取全市场日K → 交易日返回0行=当日行情未发布(约17:30后), 本轮停住等下轮; 首次运行从 stock_daily 存量 MAX(trade_date) 初始化水位, force 才清零重放; 周/月K仍逐股增量(不做本地聚合), 指数随逐股路径补日K
- SyncStockList 走 query_stock_basic(空参, 1次调用, 全量证券含退市): is_active 由 status 派生(停牌不再像 query_all_stock 的 trade_status 那样误标 inactive, 退市股能正确下线), list_date 取 ipoDate, type 保留 1股票/2指数/5ETF(ETF 按需求归入股票 type=1 处理, isETFCode 按代码前缀 sh.5\*/sz.15x/16x/18x 识别, 其日K不在按日全量接口返回中必须走逐股), 其它/可转债在 stock_info 标记下线(清理旧版无过滤导入的遗留行, 避免 ETF/可转债被当股票展示或重复同步); stock_daily 已加 preclose/trade_status/pe_ttm/pb_mrq/ps_ttm/pcf_ncf_ttm 列(按日全量与逐股路径共用 stockDailyUpsertCols 填充), is_st 由按日同步的官方逐日 isST 每日更新; Screen 的 PE/PB 仍是 close/eps 现算, 后续可切官方 pe_ttm/pb_mrq
- 定时任务统一存于 job_definitions/job_runs (TaskRegistry 注册, 方法名是 key 不做反射); 备忘= task_name=reminder.notify + user_id + once/repeat 调度, 系统任务= cron + user_id IS NULL; 重复备忘按旧链式逻辑: 每条定义=一次执行, 执行完由调度器生成下一条 (chain_id 分组, 过期不补发循环补齐), 日历只查定义行不做 run 映射; 手动触发与重试不生成下一条; 管理接口不展示用户备忘
- 财务同步增量按 finance_sources 标记判断 (P盈利G成长O营运C现金流B偿债), 不是看行是否存在; 标记以数据列补齐(只增不减)——旧数据/被覆盖的标记(如利润列有值但标记缺P)按实际数据推断, 避免重复拉取, 标记齐全但列全空的来源不重拉(baostock 本身无值); 写标记必须与库中原有标记合并——只写本次来源会把旧行覆盖成 "G"/"PB" 乒乓, 每轮重复拉接口; 旧部署(9/12~9/14)曾把 O/C 数据跨股票/季度写串(600071/600072 互串), 已迁移清空 O/C 六列并剔除标记强制重拉; 所有百分比列统一 ×100 口径: roe/gross_margin/net_margin/debt_ratio 已从 baostock 原始小数改为百分比存储(迁移 ×100, 带 ABS<5 防重守卫), 与成长/营收同比列一致, 前端按百分比直接渲染; 周转率/流动比率/cfo_to_or 等倍数类列保持原始值不加 100; stock_finance 比率/同比列 decimal(8,4) 放宽为 (12,4) (小基数下同比可达上万, 1264 out of range), 比率列统一 (12,4) 治本
- GORM 零值陷阱: bool 字段带 `gorm:"default:true"` 时, Create/upsert 写入 false 会被零值替换成 true (INSERT 与 ON DUPLICATE KEY UPDATE 双双失效), 曾导致全部 7674 只证券 is_active=1、退市股/停牌股全进列表页; 修复=去掉 default 标签(临时表实测验证), 需要 DB 默认值与零值语义不一致的字段要么用指针要么不设 default 标签
- 证券代码全链路存完整格式 sz.000003/sh.600000/bj.430047 (9/14 定稿): 裸 6 位码曾使 sz.000003(退市股)与 sh.000003(上证B指)在 stock_info 唯一约束下互相覆盖、身份随同步翻转、stock_daily 混写双方K线; 对比过 code+type 复合键/指数分表/混合键等方案, 最终选完整码根治——唯一键不变无需复合, isIndexCode(code)=sh.000*/sz.399* 前缀判断(不再需要 market 参数), convertToBaostockCode 原样返回, resolveMarket 按前缀; 迁移(20260914163000): 000% 歧义K线/状态无法按行甄别直接删除(sh.000xxx 与 sz.000xxx 历史混写), 其余按 market(stock_info)/首数字(6=沪 0/3=深 4/8/9=北)回填前缀, 五表 code 加宽 varchar(16); type 列(1股票/2指数)保留用于 Screen 的 securityTypes 过滤; 前端展示剥前缀(列表代码列/详情页/东财实时行情链接); 部署后必须: ①同步股票列表 ②行情同步 force=1 跑一次(重建被删的 000% 日K历史, 约8800次调用), 周/月K与指数K线因状态已删自动全量重拉
- 小时线(60min)并入逐股K线同步(9/15): stock_daily.trade_date 由 date 放宽为 datetime(迁移 20260915000001; date→datetime 属重建表的类型转换, 大表需低峰/停机), 唯一键 uk_code_freq_date 不变——日/周/月线存交易日 00:00:00, 小时线存 bar 时间(10:30/11:30/14:00/15:00); **大坑**: DSN 为 parseTime=True&loc=Local, go-sql-driver 写 time.Time 前会先 .In(Local), 若日K仍用 time.Parse(得到 UTC 零点)解析, 新写日线会落成 08:00 而迁移后的老行是 00:00, 唯一键视作不同行→同一交易日重复写入且 upsert 失效; 治法=K线日期统一 time.ParseInLocation(..., time.Local)(parseTradeDay), 与迁移后 00:00 对齐; 小时线目标日 LatestCompletedTradeDay: 收盘(15:00, marketCloseHour)后取最新交易日, 盘中回退上一交易日(end_date 同步钳到该日, 避免把当日未完成 bar 写入), freqIsCurrent 要求已存到 15:00 收盘 bar(可自愈收盘后延迟发布最后一根); 小时线 fields 仅 date,time,open,high,low,close,volume,amount(无 preclose/tradestatus/估值/换手, 传了报 10004012 无效参数), time 字段格式 YYYYMMDDHHmmssSSS; stock_sync_state 增 kline_hourly_to(datetime, 迁移 20260915000002); 指数小时线不同步(9/29 实测: baostock 对指数 60min 返回 error_code=0 但 0 行, 近期/历史均无数据; ETF 60min 正常返回且同步代码已覆盖), 首次小时线全量约 5400 次调用, 勿与日K force 重放排同一天
- MySQL->ClickHouse 行情/财务复制(9/15): CH 只做下游只读副本(回测/分析), MySQL 仍是唯一真相源, 现有读写链路一行不动; 增量按 updated_at 时刻水位(sync_watermark 新增 last_time 列; stock_daily 为此新增 updated_at DEFAULT CURRENT_TIMESTAMP ON UPDATE, **GORM 模型刻意不加该字段**——加了会进入现有 upsert 的 INSERT/SET, 风险大且无必要, CH 侧用局部结构体读); CH 表用 ReplacingMergeTree(updated_at) 以 updated_at 为版本列, ORDER BY (code,frequency,trade_date) / (code,report_date), 重复/重放写入天然幂等(读侧要带 FINAL 或 argMax 才即时去重); CH 表结构在 service/db/ch_migrations/\*.sql(与 MySQL 的 Goose 迁移相互独立, 启动时按 CH 内 schema_migrations 表应用, 表结构变更请新增文件); 首次/修复用定时任务 stock.sync_clickhouse 的参数 {"full": true} (TRUNCATE+全量), 日常走同任务默认增量(无水位时等价全量); 无 HTTP 接口, 水位名 clickhouse_stock_daily / clickhouse_stock_finance; 配置 config.clickhouse(enabled 默认 false, 支持 CH_ENABLED/CH_HOST/CH_PORT/CH_USER/CH_PASSWORD/CH_DATABASE 环境变量), 未启用或连不上只告警不阻断启动; 日期列用 DateTime('Asia/Shanghai') 与 MySQL 的 loc=Local 对齐(小时内 bar 时间不被时区平移); 坑: 水位为空时不要写零值时间(datetime 下限 1000-01-01 会报错, 已跳过); 限制: 行删除/TRUNCATE MySQL 不会自动反映到 CH(需 full 重建), 同秒并发更新可能靠 ReplacingMergeTree 收敛
- baostock 代理死循环卡死真因(9/16): 之前在 socket 加 `setdefaulttimeout(60)` 并以为能兜底, 但 SDK `socketutil.send_msg` 的收包循环是 `while True: recv(); receive += recv; if receive[-13:]==b"<![CDATA[]]>\n": break`, 对端断开/半关闭(FIN)时 recv() 返回 b"" 且此后一直秒返回 b"", receive 永不增长→**单核 100% 空转(4 核即 25% CPU)**, 且调用方持有 baostock_lock→锁永不释放, 全服务连锁挂死, 只有不取锁的 /health 还能 200; setdefaulttimeout 无效因为 EOF 后 socket 立即可读返回 0 字节, recv 根本不阻塞(25% CPU+请求无响应=此症); 治法=shared.py 里 monkeypatch `baostock.util.socketutil.send_msg`(所有 SDK 模块都以 `sock.send_msg` 属性访问, 故模块属性替换即全局生效), 补 EOF 检测(recv 为 b"" 即 raise ConnectionError)+总超时 60s(每次 recv 前 settimeout(剩余))并**把异常抛出而非 SDK 那样静默返回 None**, 这样 main.py `_execute_with_retry` 能走 force_disconnect+重试; 附带好处: 之前 send_msg 返回 None 会让 history 返回 error_code=BSERR_RECVSOCK_FAIL, 被 endpoint 当确定性错误(BaostockQueryError)不重试而直接 502, 现在改抛异常后可重试
- 验证 baostock 代理修改: 用 `baostock\.venv\Scripts\python.exe -c "import main"`(确认路由/补丁生效)+ 伪造 socket(recv 返回 b""/完整报文)直接调用 `bs_socketutil.send_msg` 验证 EOF 抛错与正常解析, 无需真实联网
- baostock 重连"服务器连接失败"连环 502(9/16, 死循环修好后的第二个坑): 症状=服务端重置长连接后, 每个请求都 `[Errno 32] Broken pipe`, 且日志里每次重试前 SDK 打印"服务器连接失败, 请稍后再试"(来自 `SocketUtil.connect` 的 except), 连续 502; 真因=SDK logout 在 send/recv 失败时**不会执行它内部的 `socket.close()`**, 旧连接停在 CLOSE_WAIT, 而 `login()` 的 `connect()` 是"先 connect 新 socket 再覆盖 `context.default_socket`", 于是新连接握手时旧会话仍在→服务端因同账号旧连接未释放拒绝新建; 治法=shared.py 增 `_close_socket()`(close 并置 `context.default_socket=None`), `_patched_login` 在 `_real_login()` 前先关旧 socket 且 login 失败/异常时也清掉, `force_disconnect` 的 finally 里兜底关闭; 另把 main.py 退避 `RETRY_BACKOFF_SECONDS` 由 (0.5,1.0) 调成 (1.0,3.0)—服务端释放旧会话要一点时间, 退避太短会连续被拒; 排查手法: `Set-Content`/直接 close `context.default_socket` 模拟重置后验证 force_disconnect→login 能恢复
- baostock 重连仍失败的第三坑(9/17): 上述 `_close_socket()` 修了客户端的 socket 泄漏, 但 `force_disconnect` 关掉旧 socket 后立刻 `connect()` 新 socket, baostock 服务端还没来得及处理旧连接的 FIN 就收到新 SYN→同账号旧会话仍在→拒绝新建; 治法=`force_disconnect` 的 finally 里 `_close_socket()` 后加 `time.sleep(2)`, 给服务端时间释放旧会话; 2s + main.py 退避(3s/6s) = 共 5~8s 间隔; Go 侧 httpGet 重试退避也改为指数(3s, 6s)
- baostock "用户未登录" 死循环(9/18): session 过期后 SDK 内部 `context.is_login` 被置 False, 但 `_patched_login` 缓存 `_logged_in=True` 不调 `_real_login` 直接返回旧结果, SDK query 检查自身状态报"用户未登录"; endpoint 抛 `BaostockQueryError`, 而 `_execute_with_retry` 把 BaostockQueryError 当确定性错误直接 raise 不重连→重试时同样死循环; 修法=在 `_execute_with_retry` 里判断 BaostockQueryError 是否含"未登录", 是则走 force_disconnect+重试路径
- baostock 代理日志每条打印两遍(9/16): 自定义 `log_config` 里 `uvicorn.access` 只配了 handler 没设 `propagate`, 默认 True 会把 access 记录再传给 root, 于是同一行被 access handler(`INFO:    ...|`) 和 root(`basicConfig` 的 `...[baostock-api]` 前缀)各打一次; 治法=给 `uvicorn.access` 加 `"propagate": False`(uvicorn 默认配置本就有)
- 宏观经济数据同步(9/17, tool/macro 页): 存款利率/贷款利率/存款准备金率/货币供应量月度/年度五接口代理已内置, 各1次调用无参全量+upsert幂等(利率/准备金率约43~47条, 货币供应量月约400条); 坑: ①准备金率传年份区间(如 2020-01-01~2025-12-31)返回 0 条, 一律不带日期全量拉(无参返回1999起47条), 利率/货币供应量同理直接无参 ②1990年代早期行多数字段为空串( parseFloatPtr→NULL, 别存0污染数据) ③字段名: 准备金率 bigInstitutionsRatioPre/After + mediumInstitutionsRatioPre/After; 月度 statYear/statMonth + m{0,1,2}Month/m...YOY/m...ChainRelative; 年度 m{0,1,2}Year/m...YearYOY; 存款利率 demandDepositRate/fixedDepositRate{3Month,6Month,1Year,2Year,3Year,5Year}/installmentFixedDepositRate{1Year,3Year,5Year}; 贷款利率 loanRate{6Month,6MonthTo1Year,1YearTo3Year,3YearTo5Year,Above5Year}/mortgateRateBelow5Year/mortgateRateAbove5Year(baostock 把 mortgage 拼成 mortgate, 源码如此) ④余额单位亿元、利率/比率单位%按原值直存 ⑤GORM 数字驼峰(Fixed3Month→fixed_deposit_rate3_month 歧义)用显式 column tag(fixed_3m/loan_6m_1y/mortgage_below_5y)治 ⑥SDK 源码字段可确认: python.exe 走 DLP 透明解密, `.venv\Scripts\python.exe -c "print(open(<sdk .py>, encoding='utf-8').read())"` 或 inspect.getsource 可读出明文
- elegant-router 增量生成坑(9/17): dev 服务器运行中新增路由时, 插件对 routes.ts 的增量再生成只保留 title/i18nKey(丢 icon/order/permissions); 手写路由的正确模式=不放 route.json, meta 直接维护在 src/router/elegant/routes.ts(先例: cut_bar/system_user/ai_training 均无 route.json), `pnpm build` 全量重生成时会保留 routes.ts 里已有 meta; route.json 的 permissions 字段本来就不会被合并进生成文件(tool_stockscreen 先例), 页面权限实际靠后端 API RequirePermission 兜底
- Eino Skill Middleware + Skill 管理页(9/23): `github.com/cloudwego/eino/adk/middlewares/skill` 的 `Backend` 接口(`List` 返回 `[]FrontMatter`, `Get(name)` 返回 `Skill{FrontMatter,Content,BaseDirectory}`)做成 DB 版即实现动态加载——`ai_skills` 表存 name/description/context/agent/model/content/enabled, `dbSkillBackend` 每次 List/Get 实时查 `enabled=1`, 工具描述与内容即时生效(但 runner 有缓存, 变更后仍需 `AIAgentService.ClearRunnerCache()` 让工具描述重建, 故 skill CRUD service 持 agentService 引用在增删改后清缓存); context 模式 inline(默认)/fork/fork_with_context, fork 需配 AgentHub、model 字段需配 ModelHub, 二者分别用 `AIAgentService.getModel` 实现(子 Agent 用 `adk.NewChatModelAgent` 现建); 中间件在 `getOrCreateRunner` 里 append 到 Handlers, 构建失败只告警不阻断; **坑**: `adk/middlewares/skill` 包会编译 `filesystem_backend.go` 进而 import `adk/filesystem`, 后者需要 `github.com/bmatcuk/doublestar/v4` 的 go.sum 条目(此前未引入), 直接 build 报 "missing go.sum entry", 执行 `go get github.com/cloudwego/eino/adk/filesystem@v0.9.20` 即可(只加一行 indirect); 前端手写路由需同步改 5 处: routes.ts / imports.ts / transform.ts(routeMap) / typings/elegant-router.d.ts(RouteMap+LastLevelRouteKey) / locales(zh-cn+en-us)
- Eino patchtoolcalls 接入(9/23): `getOrCreateRunner` 的 Handlers 现为 logging → approval → skill → patchtoolcalls; patchtoolcalls 修补"有 tool_calls 但缺 tool result"的悬空调用(默认占位文案, `New(ctx,nil)` 即可, 构建失败只告警不阻断); **summarization 一度接入又移除**: 它会在 BeforeModelRewriteState 用摘要替换 state.Messages, 而框架会把改写后的状态写回图状态并作为中断 checkpoint 保存(恢复的是"摘要后"历史), 本项目每轮 ChatStream 都把前端完整历史重新 Run、SaveConversation 也落完整原文, 导致 ①摘要不跨轮持久化、每轮重复摘要(额外模型调用+结果抖动) ②续跑时模型上下文与前端/DB 完整历史分叉 ③ResumeToolApproval 固定 getOrCreateRunner("") 使续跑模型/摘要模型与原对话 modelOverride 不一致; 若将来要接, 需先把摘要跨轮持久化(下一轮传"摘要+最近消息")并让续跑带上原 modelOverride
- GitHub 仓库导入 Skill(9/23): 不落库, `POST /api/skills/discover/github`(权限 system:skill:view) 只负责扫描并返回解析后的 skill 列表, 前端点击「导入」把 name/description/context/agent/model/content 预填到「新增 Skill」表单, 由用户补充后再走原 create 落库(避免重名直接失败); 扫描流程=解析仓库地址(owner/repo 或完整 URL, 支持 `/tree/<branch>/<subpath>`)→ 无分支时 `GET /repos/{o}/{r}` 取 default_branch → `GET /repos/{o}/{r}/git/trees/{ref}?recursive=1` 递归列文件 → 过滤 `SKILL.md`(不区分大小写, 可按子目录前缀过滤) → 逐个 `raw.githubusercontent.com` 拉正文 → 复用 splitFrontmatter+yaml 解析; 未鉴权 GitHub API 限流 60/h, 支持环境变量 `GITHUB_TOKEN` 提额(设置了才加 Authorization 头), 命中 403/429 返回"限流"提示; 解析失败/单文件拉取失败跳过不阻断; 扫描结果按 `owner/repo@ref:subpath` 内存缓存(`skill_github.go` 包级 map+读写锁), 同仓库仅"再次扫描成功"才替换, 失败时回退返回旧缓存并带 warning(前端显示"缓存结果"标记); 返回结构 `{repo,total,cached,warning,scanned_at,skills}`; 另提供 `GET /api/skills/discover/github/cache`(只读内存缓存, 不触发扫描), 前端打开弹窗时自动加载缓存, 避免第二次打开为空; 前端默认仓库/分支为常量 DEFAULT_DISCOVER_REPO/DEFAULT_DISCOVER_REF; SKILL.md 用 12 协程并发拉取(原串行 864 个要 2 分多钟); **坑**: 曾设 `maxSkillFiles=200` 上限截断, 导致 awesome-claude-skills(master, 实测 864 个 SKILL.md) "总数不对", 已移除上限; GitHub tree `truncated` 为 true 时结果不完整, 返回 warning
- Agent Studio 可视化编排(9/30): 编排 DSL(nodes/edges JSON) 后端编译为 Eino compose 三种形态——线性走 Chain、含分支走 Graph+GraphBranch(branch 节点本身是直通 lambda, GraphBranch 挂它上面路由)、含合流走 Workflow+AddInput(MapFields("Content", 来源key)); 实测要点: ①Graph/Chain/Workflow 构造函数不能收 WithGraphName(那是 GraphCompileOption, 编译时传) ②Graph/Workflow 必须显式 AddEdge(compose.START, 入口)/wf.End().AddInput(出口), 否则 "start node not set" ③Graph 多条入边到同一节点编译报类型不匹配, 合流只能用 Workflow; MapFields 后目标 lambda 收到的值是 string(Content 已提取) 不是 *schema.Message ④Workflow 多入口合法(各自 AddInput(compose.START)), 图模式分支的多个目标可汇合到同一节点(运行时只走一条) ⑤react.Agent 用 compose.AnyLambda 包成节点(方法签名 Generate/Stream([]*schema.Message)), react 内部子节点名用 ModelNodeName/ToolsNodeName 定成 "<key>.model"/"<key>.tools"、GraphName 设 "<key>.react"(不能与编排节点 key 同名, 否则图级 end 事件与节点 end 事件混淆导致内容重复) ⑥WithGraphName/WithNodeName 归属调试事件; 回调归属用 ctx span 栈(OnStart push/OnEnd pop, 官方推荐的同 Handler 时机间 ctx 传值模式), token 只在与 ChatModel comp 相等的 span 合并(否则下游透传节点继承用量), MS 只在顶层 span 赋值 ⑦ark 流式的 handler 拷贝里分片含完整消息副本, 节点内容按 schema.ConcatMessages 合并后再做 X+X 精确重复去重 ⑧前端画布 @vue-flow/core + background/minimap(background 包无 style.css 导出, import 会构建失败); defineModel 双向绑 nodes/edges; 布局变化(调试面板开合)后要手动 fitView; NDynamicInput 的 v-model 不能绑函数调用, 用 computed get/set ⑨手写路由: 在 src/views 建目录后 elegant-router 自动生成 4 处(routes.ts 骨架/imports/transform/d.ts), 只需补 routes.ts 的 meta(icon/permissions/order) + locales 两行; 浏览器直连 8080 会 CORS 拦截, 前端经 vite 代理(/proxy-default)访问后端 ⑩调试运行 SSE: event=start/node/delta/summary/error/done, node 事件带 kind/key/comp/owner/ms/content/tool, owner 供前端把内部事件归到编排节点; 前端节点组件化: views/system/agent-studio/nodes/ 下 registry.ts(类型->label/icon/color/defaultConfig/component 唯一事实源) + base-orch-node.vue(连接桩/头部/状态色/样式只写一次, 类型组件只填插槽) + 每类型一个组件(agent-node 等, body 各异), 画布单插槽 #node-orch 里 <component :is=registry.component> 动态渲染, 新增类型只改 registry; makeNode 的 defaultConfig 必须深拷贝(否则多节点共享 tools 数组); 链式 Agent 坑: Ark(方舟) 严格校验 messages 序列(错误码 1214 "messages 参数非法"), 上游 Agent 的 assistant 输出直接作为下一跳 Agent 输入时序列为 [system?, assistant] 被拒——agent 节点 MessageModifier 里做 orchNormalizeModelInput: 序列无 user 消息时把首条非空消息复制转 user 角色(清 ToolCalls, 不改共享原消息), react 后续迭代里已有 user 则透传; 另: bash 的 cat >> heredoc 写仓库文件会触发天锐 DLP 加密(%TSD-Header, python/Write 工具此前一直安全但同样要警惕), 中招后 rm 掉用 Write 工具重写, 写完 head -c 15 验证明文 + git diff --numstat 看有无 "- -"; 画布交互坑: ⑪自定义节点必须显式渲染 <Handle>(带 id 如 "in"/"out"), 且所有边要带 sourceHandle/targetHandle 字段, 否则连线拖不出来/程序化加的边不渲染 ⑫VueFlow 的 is-valid-connection prop 对"程序化加边"也会调用, 校验函数里"已存在同边→false"会把新加的边自身判重导致全部连线被丢(包括 loadDefinition 回显), 连线校验放 @connect 回调里做 ⑬Vue 组件自定义事件上不能用 .stop/.prevent 修饰符(withModifiers 会对 emit payload 调 preventDefault, payload 不是原生事件直接抛错, handler 不执行), 在 handler 里手动 e.event.preventDefault()+stopPropagation() ⑭配置面板是节点配置的唯一编辑入口: 只在切换 nodeId 时从 props 深拷贝初始化本地副本, 不能 watch props.config 反向回写(回写→props 变→重置本地=下拉选择被冲掉的回声循环) ⑮VueFlow 事件句柄 e.event 是 MouseTouchEvent 联合类型, 取 clientX 要 as MouseEvent
- Agent Studio 主 Agent 委派多个子Agent(10/1): 新增 DSL 节点类型 `subagent`, 画布上"主 Agent --委派边--> 子Agent"(子Agent 无出线桩, 一个 Agent 可挂多个), 编译时把每个子Agent 建成独立 react.Agent 并包成主 Agent 的委派工具(工具名 `subagent_1..N`, 序号按子Agent 名称字典序、同名再按 id 兜底——`sort.Slice` 不稳定, 只比名称会让同名子Agent 的工具映射随机), 主 Agent 的 ReAct 循环按需 `{"task": "..."}` 调用, 子Agent 以 user 消息收任务、返回文本; 要点: ①子Agent 节点必须完全脱离主流——`flowAdj/flowIn` 只收"两端都不是子Agent"的边, 校验/拓扑/入口出口判定/Kahn 全部改用 flowInOf/flowOutOf, 否则"Agent 同时有委派边和一个主流出边"会让它被判成出口而漏接 END(workflow 里还会漏 `wf.End().AddInput`); 子Agent 有违规出边时若把它算进拓扑, 报错会变成莫名的"存在循环连线" ②Kahn 初始队列来自 map 迭代, 必须 `sort.Strings(queue)` 否则编译期构建顺序(进而模型/tool 构建顺序)随机 ③委派工具无法从 ctx 拿 handler(react/tools 节点重建的 ctx 会丢掉自定义 value, 只剩 callbacks/span 栈), 改为编译期把 handler 实例注入 orchDelegateTool, DebugRun 里先 `newOrchTraceHandler(orchNodeKeysOf(dsl), orchSubAgentKeysOf(dsl), nil)` 再编译, 编译成功后 `handler.setEmit(emit)` 注入 SSE 出口, `compiled.trace` 回带同一实例; ④子Agent 不是 compose 节点没有自己的 span, 其"已委派/任务/结果"由委派工具主动 `emitNodeEvent` 推 node 事件(key=子Agent 节点 id、owner=主 Agent id、delegated=true), `recordDelegateLocked` 填进 OrchNodeTrace(delegated/owner/task); nodeKeys 要把子Agent 也登记进去, 前端才拿得到条目; 前端 `applyNodeEvent` 必须先判 delegated 再按 owner 归并, 否则子Agent 状态会写到主 Agent 上 ⑤subAgent 的 agent_id≠0 时用被引用 Agent 的 system_prompt/description, 内联 system_prompt 被忽略
- 子Agent 内部事件"归属错"导致界面像卡死(10/1, 排查真因): 现象=委派已触发, 但被委派的 60~80s 里前端只看到主 Agent 在跑, 子Agent 节点不亮、事件流没有子Agent 的动作。**先别急着改事件通道**: 打日志发现 `sub.react`/`sub.model` 事件其实一直在推, 只是 `owner` 判成了外层主 Agent。原因=`ownerKeyFor` 用"span key 是否在 nodeKeys 里"判断顶层, 而子Agent 节点既登记进了 nodeKeys(为了摘要回显), 又用过 `<key>.react`/`<key>.model`/`<key>.tools` 作子图/子节点名 → 子Agent 的 `sub.react` 被当成"顶层节点自身", 内部 `sub.model` 再往上就落到主 Agent。治法=**层级感知归属**: handler 额外持有 `subNodes`(子Agent 节点 id 集合, 由 `orchSubAgentKeysOf(dsl)` 提供), `ownerKeyFor` 依次判断 ①span 自身是顶层节点→自身 ②span key 形如 `<subID>.xxx` 且 subID 是子Agent→归该子Agent ③否则取剩余栈中最近顶层节点(于是 `subagent_N` 委派工具那层仍归主 Agent, 主 Agent 的"工具调用"记录不受影响)。另: 子Agent 用同步 `Generate`, 期内无任何分片, 故再包一层 `orchSubAgentProgress`(10s ticker 推 `status=running` 心跳)让前端能显示"已运行多久"; 排障关键: 判定"事件没推"之前先确认 `owner`/归属, 别被"前端没显示"误导成链路断了
- 主 Agent 流式输出 + 思考过程输出(10/1): 现象=编排调试面板的"最终输出"整段一次性出现(子Agent 委派那 75s 里一个字都看不到)。**关键排障手法**: 用带时间戳的假模型(每片 sleep 300ms)做外部测试, 对比"模型发送时刻"与"前端收到增量时刻"——实测模型 `@300/607/908ms` 逐片发出, 而增量事件全在 `@908ms` 才到, 一眼定性为"被缓冲"而不是"没推"。结论: ①**eino 图节点拿到的流是"缓冲后"的副本**: 节点任务完成、流被读完才 `copyItem` 分发给下游/分支, 所以在 `StreamToolCallChecker`(分支输入)里转发增量必然是一次性事后推送 ②**只有模型节点的回调流是真·实时**: `OnEndWithStreamOutput` 里 `span.comp == "ChatModel"` 的那个 stream 逐片到达, 编排里唯一能实时转发的点就在这里 ③实现=在这里 `emitDelta(content)` + `emitReasoningDelta(reasoning_content)`(两者分开发, 前端把思考折叠展示), 并记录"已实时下发的字节数"; 图级输出到达时 `orchSkipStreamed` 按该计数跳过重复内容(多扣的部分留到下一段, 计数要 `replace` 而非累加——链式多个模型节点累加会把最后一个节点的回答整段吃掉, 有回归测试) ④子Agent 原来用 `agent.Generate` 同步取结果, 完全无增量; 改成 `agent.Stream` + `schema.ConcatMessageStream` 后, 子Agent 的思考/正文也能实时透出(比心跳更直观), 委派工具仍把拼接后的文本作为 tool result 回给主 Agent ⑤前端 debug-panel 新增 `reasoning` 事件(与 `delta` 分开累计, 每轮运行清空), 输出区上方加"思考过程"折叠块; 注意 `ComposeDeltas` 在 eino v0.9.20 不存在(用 `ConcatMessages`), 分片本身就是增量不需再取 delta
- 编排调试多轮对话 + 运行日志(10/1): ①多轮=前端 `turns[]` 记每轮 {user, assistant, reasoning, summary, error}, 追问时把之前轮次的 user/assistant 文本作为 `history` 随请求发给后端; 后端 `model.DebugRunRequest.History []ChatTurn` → `OrchChatTurnsToMessages`(只留 user/assistant 且非空的文本) → 经 `sessionVars[orchSessionVarsHistoryKey]`("chat_history") 传给编译器 → Agent 节点 `MessageModifier` 里拼成 `[system, 历史..., 本轮]`。**必须每轮按"历史+本轮"重建**: 若用 append 累加, ReAct 第二次模型调用会把历史再拼一遍(重复上下文、token 翻倍); ReAct 内部的 assistant/tool 消息不进历史(只回传最终文本), 消息序列始终干净。`sessionVars` 是个容易忽视的传参通道——历史这种"编译期就要知道、又不想改签名"的数据走它最省事 ②运行日志=`orchLog`(默认关闭, `ORCH_LOG=1`/`ORCH_DEBUG=1` 打开, `ORCH_LOG_FILE=<path>` 同时落文件), DebugRun 打开始(编排 id/用户/历史轮次/输入/定义长度)→校验失败→编译失败→编译完成(mode/节点数)→启动失败→超时/取消/出错→运行结束(总耗时/tokens); `finishSpan` 里打"工具调用完成"(名字/归属/耗时/错误)与"顶层节点完成"(耗时/tokens/输出长度); 委派工具打开始/完成(子Agent/任务/耗时/结果长度); 另外每次运行结束逐节点打摘要行(含 `工具=[subagent_3(75626ms)]`), 输出为空时额外告警"没有产出最终文本(检查入口节点/分支默认目标可达)"
- 子Agent 真凶: eino ReAct 默认 StreamToolCallChecker 漏判"先文本后 tool_calls"(10/1): 现象=主 Agent 只回一句开场白(如"让我用决策训练的方法来帮你梳理思路。")、耗时 5s、**没有 Tool 事件、摘要里没有子Agent 行**, 但直连模型 API(同 provider+model+工具schema) 明明返回 `finish_reason=tool_calls`。真因=`react.AgentConfig.StreamToolCallChecker` 缺省用 `firstChunkStreamToolCallChecker`, 它**只看第一个分片**: 首块是文本(Content 非空)就立刻 `return false` 判定无工具调用 → ReAct 一轮结束丢掉了后面分片里的 tool_calls; GLM/Claude 都是"先出开场白文本再给 tool_calls"的模型, 必踩; 治法=`react.AgentConfig` 显式传 `StreamToolCallChecker`(扫描完整流: 任一分片带 ToolCalls 即 true, 用 `errors.Is(err, io.EOF)` 判结束), 主 Agent(react.NewAgent)与子Agent(orchNewReactAgent)两条构建路径都要加; 排障要点: ①"模型没调用"与"调用了但被框架丢掉"必须分开——直连 provider 的 /chat/completions 用同一份 tools+schema 问一次最省事, 能一次定性 ②本机 PowerShell 处理含中文的 .go 文件**绝对不能** `Get-Content -Raw` + `Set-Content -Encoding UTF8` 往返: 会把 UTF-8 读成 GBK 再写回, 中文注释变成"涓庡瓙Agent"这类乱码且加 BOM(`git diff --numstat` 显示巨量行变更), 只能用 edit 工具改; 中招后 `git checkout -- <file>` 还原, 再用 edit 逐条重放(本次已发生过一次)
- 子Agent 有工具描述却"模型不主动调用"(10/1, 编排2 线上复现): 委派工具确实已进模型工具表(`WithTools` 收到 `subagent_1..N`, 单测可断言), 但主 Agent 的 ReAct 一次都没调——纯提示词问题: 主 Agent 系统提示词里完全没提子Agent, 子Agent 的「委派说明」为空时工具描述又退化成"职责由该子 Agent 的名称与系统提示词决定", 三个节点还都叫默认名「子Agent」, 模型没有依据选择。治法(主管模式必要一环)=编译期把"【子Agent 委派】"章节**追加进主 Agent 系统提示词**: 逐个列出 `子Agent「节点名(被引用Agent标题)」: 职责 (在本轮直接调用工具 subagent_N, 参数 task 写清任务; 不要只说已委派却没有调用工具)`; 职责优先级=节点委派说明 > 被引用 Agent 简介 > 被引用 Agent 标题 > 节点 id(标题必须带上, 否则同名子Agent 无法区分)。排障要点: ①"工具没注册"与"模型没调用"必须分开——用真实编排做单测断言 `WithTools` 收到的工具名最直接 ②调试摘要里只有主流节点 = 一次委派都没发生(子Agent 未被委派就不会出现在摘要里), 不是前端丢事件 ③委派说明为空要给后端 /validate 警告 + 配置面板橙色提示(被引用 Agent 也没标题/简介时)
- 子Agent 统一权限码 + 类型/委派说明字段(9/30): `ai_agents` 增 `agent_type`(chat/subagent, 迁移 20260930000002)与 `delegation_description`; 子Agent 共用一个能力权限码 `ai:subagent:manage`(常量 `model.AIAgentSubAgentPermission`, 迁移 20260930000003 seed 并授 R_SUPER): 后端 `api.HasPermission`(新增, 字段级用; `RequirePermission` 仍是整条路由拦截)只在 ai-agents 的 create/update/delete 校验"新建/改成/编辑/删除 subagent"必须拥有该码; 权限只约束管理动作, **不约束"出现"**: 编排 `Resources()` 照常返回 subagent 的 Agent 列表与 subagent 节点类型, agent-studio 画布工具栏/右键菜单始终提供 subagent 节点; 前端 training 页无权限时不显示「子 Agent」类型选项、子Agent 行的编辑/删除按钮隐藏; 注意不要混淆: `ai_agents.permission_code` 是"公共 agent 的访问可见性"码(按行配置, 前端 hasAuth 过滤), 本次的能力权限是全局统一的 `ai:subagent:manage`
- 编排并入 ai_agents + 训练中心编排对话(9/30): ①**存储合并**: 废弃独立表 `ai_orchestrations`(迁移 20260930000005: 给 `ai_agents` 加 `definition`/`version`/`enabled`/`last_debug_summary` 四列后 `DROP TABLE`), 编排行统一存 `ai_agents` 并用 `agent_type='orchestration'` 区分; `model.AIOrchestration` 退化为纯 API 视图(删掉 `TableName()` 与 gorm 标签), `AIOrchestrationService` 全部改读写 `model.AIAgent`, 用 `orchToDTO` 映射(`title↔name`)。新建编排时因复用 `ai_agents` 的非空/唯一约束列, 必须给 `code` 填内部唯一占位 `orchestration_<unixnano>` 并把 `system_prompt` 置空; 同时 `AIAgentService.ListAvailableAgents` 显式 `agent_type <> 'orchestration'`, 否则编排行会混进普通 Agent 列表(训练页会重复出现); `AIAgent` 新增的四列一律 `json:"-"`, 避免普通 Agent 接口把体积很大的 `definition` 带出去 ②**训练中心编排对话**: 新增 `/ai-orchestrations` 路由组(`GET /`、`GET /:id` 仅需登录, `POST /:id/chat` 要 `ai:chat:send`), 直接复用 `DebugRun` 运行时但置 `SkipSummary=true` 不覆盖 Agent Studio 的最近调试摘要(新增 `DebugRunRequest.SkipSummary`); 前端训练页并行拉普通 Agent 与已启用编排, 合并为统一卡片(编排打「编排」标签并路由到 `/ai/orchestration/:id`), `training-chat.vue` 增 `orchestrationId`/`title` 两个 props 支持编排模式(隐藏模型选择/收藏/分享/改标题/提示词入口, 解析 `delta`/`reasoning`/`summary`/`error` 事件); **只保留最终答案**: 编排会把"工具调用轮的前言文本 + 子Agent 输出 + 主Agent 最终答案"都 `delta` 进同一个气泡(且子Agent 文本还会被两个 emit 源重复), 故收到 `summary` 时用图级最终输出 `summary.output` **覆盖**该气泡(而非"无 delta 才补齐"); 停止生成没有 `summary` 则保留已流式内容; 后端 SSE 事件名与 Agent Studio 调试面板完全一致, 调试面板仍按节点分开显示中间文本
- 子Agent 流式叠字修复(9/30): 症状是编排对话/调试里子Agent 的思考与正文各被交错推两遍(`用户用户现在现在…`), 根因是子Agent 的模型被两条路径同时 emit——①`orchSubAgentProgress.Stream`(包装器, 负责实时透出+心跳) ②图回调 `orchTraceHandler.OnEndWithStreamOutput` 的 `directStream` 分支(`comp=ChatModel` 就直推)。注意：早期委派走 `Generate`(无分片)才需要包装器兜底, 现在 `orchDelegateTool.InvokableRun` 已改为 `agent.Stream`, 回调能看到同一模型流, 于是重复。修法: `directStream` 收紧为 `span.comp == "ChatModel" && !h.isSubAgentInternal(span.key)`——凡 key 形如 `<subID>.model`(前缀命中 `subNodes`) 的子Agent 内部模型调用一律由图回调跳过, 只留包装器一路 emit; 主 Agent 的 `<nodeID>.model` 不受影响仍走回调。主 Agent 实时增量、跨模型节点去重(图级 `orchSkipStreamed`)、心跳都不变。回归测试 `TestOrchestrationSubAgentDeltasNotDuplicated`(直接 `buildSubReactAgent` + `agent.WithComposeOptions(compose.WithCallbacks(handler))` 跑子Agent, 断言思考/正文各恰好各片一次) 修复前 2 片出 4 条事件、修复后通过; 另加端到端回归 `TestOrchestrationDelegationEmissionCount`(模板→主Agent(带子Agent)→结束, 主Agent 真发起 `subagent_1` 委派工具调用, 并复刻 DebugRun 的图级 `orchSkipStreamed`), 断言子Agent 思考/正文各 1 次、主 Agent 最终答案 1 次(修复前子Agent 内容会出 2 次)
- 编排 Agent 注入运行时上下文: 时间 + 用户画像(9/30): 现象是编排里的模型不知道当前时间(自称"假设今天是2024年5月20日左右")。根因: 普通 Agent 对话的 `Instruction` 固定带 `{user_profile}` 与 `当前时间: {current_time}`(`ai_agent_service.go`), 但编排节点的系统提示词只走 `orchRenderTemplate`, **只有显式写 `{{.current_time}}`/`{{.user_profile}}` 才渲染**——「你是我的助手」这类提示词下模型既拿不到时间也拿不到画像, 只能瞎猜年份。治法: 新增 `orchInjectRuntimeContext(prompt, vars)`, 在系统提示词渲染后统一追加「用户画像块 + 当前时间/用户ID」(与普通对话一致); 主 Agent(`buildAgentLambda`)、子Agent(`buildSubReactAgent`)都注入; 画像与时间都按 `strings.Contains` 判重(已引用则不重复追加), 提示词为空时只返回运行时上下文(避免出现空 system)。注意: 运行时时间/ID 块在提示词已渲染出 `current_time` 时整块跳过, 避免与模板重复。回归: `TestOrchInjectRuntimeContext`(时间/画像注入、各自判重、空提示词) + `TestOrchestrationAgentInjectsRuntimeContext`(编译后捕获模型入参, 断言 system 含当前时间与画像)
- 编排多轮"上下文丢失"排查(9/30): 用户反馈第二轮只发"用子agent搜索", 助手却反问"要搜索什么"。**结论: 管道没坏, 是模型没用上文**。排查手法(可复用): 后端 `HandleChatRun` 的鉴权只有 `ai:chat:send`, JWT 密钥就写在 `backend/config.yaml`(`main.go` 只拒绝空/`soybean-admin-secret`), 所以可直接用 HS256 手搓一个 `{userId:1, role:"R_SUPER", typ:"access"}` 的 token 打真实接口: ①带 `history=[{user:"口令是紫色犀牛"}]` 问"口令是什么" → 模型答"紫色犀牛", 证明历史确实注入; ②带天气历史发"你个笨蛋，用子agent搜索" → 模型思考里出现"因为之前用户天气"(说明看到了历史)但最终仍反问, 且思考严重重复循环(模型自身问题)。另注意前端逻辑也能静态确认: `training-chat.vue` 的 `buildOrchestrationHistory()` 取 `messages.slice(0,-2)` 并跳过开场白, 产出 `[{user 今天天气},{assistant 天气报告}]` 正确; 且页面上"只保留最终答案"生效说明浏览器已加载含该函数的最新模块。治法: 主 Agent 有历史时在系统提示词追加 `orchHistoryGuide()`(【多轮对话】提醒模型结合上文推断省略/指代式追问, 别因没重复主题就反问), 回归 `TestOrchestrationChatHistoryInjected` 断言含该提示、首轮无历史时不含。**编排对话已落库(9/30 追加)**: `HandleChatRun` 接收 `history_id`, 运行结束后用 `HistoryService.SaveConversation` 把「本轮 user + 图级最终答案 + 思考」写入 `training_histories`(`training_type=ai_orchestration`, `custom_training_id=编排ID`), 首轮回 `history_id` 事件; 前端 `training-chat.vue` 记录 `history_id` 到 `localStorage[ai_orchestration_history_<id>]`, 刷新/重开自动恢复(顶栏与普通对话一致, 提供收藏/分享, 不再放「+新会话」); 历史列表「继续训练」按 `training_type` 路由回 `/ai/orchestration/:id?history_id=`。即: 同页面多轮 + 刷新恢复 + 历史列表续聊都保留上下文
- 子Agent 整段重复二连发修复(10/1): 症状是编排对话里子Agent 的回答/思考**先实时流出一遍、结束时整段再来一遍**, 前端只能靠 `summary` 覆盖兜底。抓包(`/tmp/orch_sse3.log` 的 SSE 事件流)实锤: 同一段 350 字回答在 `delta` 里出现 2 次、分片长度模式完全一致, 思考 321 片 + 321 片。**根因是子Agent 的模型流存在第三条发射路径**: 上轮修复只跳过了框架代发的 `<subID>.model` span, 但 eino 对"未声明自带回调"的组件会包一层 `runWithCallbacks`, 其 `onStart` 会把节点 ctx 的 RunInfo **清空**(`internal/callbacks/inject.go` `On()` 里 `nMgr.runInfo = nil`), 于是内层 ark 的 `EnsureRunInfo` 自建了一个 **Name 为空** 的 RunInfo 并自己触发回调——这个空 key 的 ChatModel span 是**真·实时**的(ark 在返回流时立即 `OnEndWithStreamOutput`), 而 `directStream` 的 `isSubAgentInternal("")` 判否, 于是它照推; 包装器 `orchSubAgentProgress.Stream` 的转换函数要等 ReAct 图消费到流才触发(整段生成完才到, 结尾补发), 也照推 → 双发。主 Agent 没有这个问题(ark 直接作节点, 回调自启用, RunInfo 保得住, 只有 `.model` 一个 span)。**治法**: ①`orchSubAgentProgress` 实现 eino 的 `components.Checker`(`IsCallbacksEnabled() bool { return true }`)——框架跳过包装、RunInfo 保住, ark 的回调直接挂在 `<subID>.model` 名下, 与主 Agent 完全同一条实时路径 ②包装器 `Stream` 改纯直通(删 emitDelta/emitReasoningDelta/markStreamed, 心跳仍在 `Generate`) ③`directStream` 去掉 `isSubAgentInternal` 条件(函数已删, 归属仍由 `ownerKeyFor` 的 subNodes 前缀检查负责)。**测试教训**: `timedModel` 这类不自带回调的假模型走框架代发路径, 抓不住这个 bug——新增 `selfCallbackModel`(EnsureRunInfo + OnStart + 泵 goroutine + 立即 OnEndWithStreamOutput + IsCallbacksEnabled, 完整模拟 ark 协议) 用于子Agent 相关测试
- 编排对话身份前言(10/1): 现象是编排对话第一轮模型"不知道自己是谁/该干嘛"(用户: "上下文信息丢失的感觉")。根因: 编排的名称/简介只在前端欢迎气泡里(`orchestration/[id].vue` 的 initialMessage), 后端从未注入任何提示词——模型第一轮只看到画布上 Agent 节点的 system_prompt + 运行时上下文, 画布提示词写得泛(如"你是我的助手")时首轮回答自然没有身份感。治法: `DebugRunRequest` 增 `ChatMode`(仅 `HandleChatRun` 置位), `DebugRun` 里由编排记录构造 `orchChatPreamble(name, description)`(「你是「X」编排助手。你的职责: …」), 经 `compile` 新参传给编译器存 `chatPreamble` 字段, `buildAgentLambda` 里用 `orchAppendPromptSection` 追加进主 Agent 系统提示词(画布提示词之后、委派指引之前); 调试运行不置位不注入, 保持画布原样。回归 `TestOrchestrationChatPreambleInjected`(注入/调试不注入/空简介三种)
- 入口模板吞掉用户输入(10/1, 生产实锤): 生产编排对话发"今天天气", 模型却回复"收到的是通用开场"并列出四个训练方向——模型思考里写着 *"The user's message is a system-ish prompt: 你是ai助手，帮助用户解决各种问题"*, 说明**到达模型的唯一内容就是入口模板节点里的静态文案, 用户输入根本没进来**(该编排的入口模板没写 `{{.Input}}`)。这才是"编排对话读不到对话内容"的真正根因: 模板节点是消息变换器, 不引用 `{{.Input}}` 就合法地丢弃输入, 每一轮都丢。**两层治法**: ①对话模式兜底——编译器加 `chatMode` 字段(`s.compile` 新参, 仅 DebugRun 的 ChatMode 传 true, 校验/调试/子编排均 false), `buildTemplateLambda` 里 `appendUserInput := c.chatMode && c.flowInOf(n.ID)==0 && !strings.Contains(cfg.Template, ".Input")`, 命中时在渲染结果后追加「【用户消息】+本轮输入」——画布配置失误不能吞掉对话输入; ②画布校验警告——`orchCollectWarnings` 增 OrchNodeTemplate 分支: 无入边(入口)且模板不含 ".Input" 时提示"用户的输入不会进入模型, 请引用 {{.Input}}"(注意: Sprintf 里花括号不用转义, 写 `{{.Input}}` 即可)。回归: `TestOrchestrationChatEntryTemplateInputFallback`(对话模式补入/调试模式不补/已引用不重复) + `TestOrchestrationEntryTemplateInputWarning`(入口缺引用警告/已引用/非入口不警告)。**排障手法**: 模型思考里复述的"用户消息"原文是定位金钥匙——它就是真正到达模型的输入内容; 对照编排定义即可发现输入在哪一跳被丢弃
- 编排工具拿不到用户 ID(10/1, 生产): 入口模板修复上线后模型终于收到用户消息并发起工具调用, 随即报 `[NodeRunError] failed to stream tool call ...: 无法获取用户 ID`。根因: `user_info_query/user_info_edit/mem0_memory/reminder` 这些工具都从 **ADK 会话值**取用户 ID(`adk.GetSessionValues(ctx)["user_id"]`), 而会话值只有普通 Agent 对话的 `runner.Run(WithSessionValues)` 会注入——编排运行(DebugRun)走纯 compose, ctx 里没有 ADK 会话(`runCtxKey` 未导出, 也无法手工构造), 工具一调用必挂。治法: tools 包新增 `WithRunUserID(ctx, userID)`(显式 ctx 通道), `userIDFromSession` 改为「ADK 会话优先 → ctx 显式注入兜底 → 报错」, reminder/mem0 里两份内联取值逻辑统一收敛到这个函数; `DebugRun` 在 runCtx 上 `tools.WithRunUserID(runCtx, userID)`(委派子Agent/子编排的 ctx 都从它派生, 全链路可见)。回归 `TestUserIDFromSession`(注入可用/无来源报错/ID=0 拒绝; ADK 会话路径无法单测构造, 由线上普通对话覆盖)。**教训**: 新工具凡是需要"当前用户"身份, 一律走 `userIDFromSession`, 别自己内联 `GetSessionValues`——否则编排运行必挂
- 编排对话改回"打开即新会话"(10/1): 用户反馈"每次打开新的对话都会加载上次的对话, 而不是开启新的会话"——之前 9/30 设计的 localStorage 自动恢复(`ai_orchestration_history_<orchId>`)与用户预期相反。治法: 删掉 `orchStorageKey`/`persistOrchestrationHistoryId`/`readOrchestrationHistoryId` 及 onMounted/watch 里的恢复分支, 打开编排对话一律全新会话(欢迎语+historyId=0); 继续旧对话只走历史列表「继续训练」(路由 `?history_id=` 显式回放, onMounted/onActivated 的 query 分支保留)。注意 KeepAlive 语义不变: onActivated 里同 tab 切回仍保留内存中的消息, 只有重新挂载才是新会话; 设计此类"自动恢复"前先问用户要不要——SPA 里 localStorage 恢复会让"打开"和"刷新"无法区分
