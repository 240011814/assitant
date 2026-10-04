package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	einomcp "github.com/cloudwego/eino-ext/components/tool/mcp"
	mcpclient "github.com/mark3labs/mcp-go/client"
	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	mcp "github.com/mark3labs/mcp-go/mcp"

	"backend/model"
	"backend/service/tools"
)

// MCP 服务动态注册: mcp_servers 表存连接配置 (管理页 CRUD), 运行时连接 -> 发现工具 ->
// 以 mcp_<server>_<tool> 前缀注册进工具注册表。ai_tools 里的 MCP 工具行由用户手动创建/启用
// (工具不写迁移/不自动种); server 删除时自动删其工具行, 禁用时自动禁行。
// 状态/生命周期全部运行时管理, 变更后 ClearRunnerCache 让 Agent 重建工具描述
const (
	mcpToolPrefix       = "mcp_"
	mcpConnectTimeout   = 60 * time.Second
	mcpDefaultTimeout   = 30
	mcpTimeoutMin       = 5
	mcpTimeoutMax       = 300
)

var mcpNameSanitizer = regexp.MustCompile(`[^a-z0-9]+`)

// sanitizeMCPServerName server 名转前缀片段: 小写, 非字母数字折叠为 _
func sanitizeMCPServerName(name string) string {
	return strings.Trim(mcpNameSanitizer.ReplaceAllString(strings.ToLower(name), "_"), "_")
}

// mcpToolRegistryName 工具在注册表中的唯一名: mcp_<server>_<tool>
func mcpToolRegistryName(serverName, toolName string) string {
	return mcpToolPrefix + sanitizeMCPServerName(serverName) + "_" + toolName
}

// mcpToolLikePattern 该 server 工具行的 LIKE 匹配模式 (LIKE 通配符转义, MySQL 默认转义符为反斜杠;
// 前缀里的下划线同样是通配符, 一并转义)
func mcpToolLikePattern(serverName string) string {
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(sanitizeMCPServerName(serverName))
	return `mcp\_` + esc + `\_`
}

// ============ 服务 ============

type MCPService struct {
	mu       sync.Mutex
	entries  map[string]*mcpEntry // server name -> 连接与已注册工具
	agentSvc *AIAgentService
}

type mcpEntry struct {
	server    *model.MCPServer
	cli       *mcpclient.Client
	toolNames []string
}

func NewMCPService() *MCPService {
	return &MCPService{entries: map[string]*mcpEntry{}}
}

// SetAgentService 注入 Agent 服务 (注册/注销工具后清 runner/tool 缓存)
func (s *MCPService) SetAgentService(svc *AIAgentService) { s.agentSvc = svc }

func (s *MCPService) invalidate() {
	if s.agentSvc != nil {
		s.agentSvc.ClearRunnerCache()
	}
}

// Init 启动时连接全部启用的 server (并发, 状态回写 DB, 失败不阻断启动)
func (s *MCPService) Init() {
	var servers []model.MCPServer
	if err := DB.Where("enabled = ?", true).Find(&servers).Error; err != nil {
		log.Printf("[MCP] 加载服务配置失败: %v", err)
		return
	}
	for i := range servers {
		srv := servers[i]
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), mcpConnectTimeout)
			defer cancel()
			if err := s.connectServer(ctx, &srv); err != nil {
				log.Printf("[MCP] 服务 %s 启动连接失败: %v", srv.Name, err)
			}
		}()
	}
	if len(servers) > 0 {
		log.Printf("[MCP] 开始连接 %d 个已启用的服务", len(servers))
	}
}

// ============ 连接与注册 ============

// buildClient 按传输方式构建客户端
func (s *MCPService) buildClient(srv *model.MCPServer) (*mcpclient.Client, error) {
	headers := map[string]string{}
	if strings.TrimSpace(srv.Headers) != "" {
		if err := json.Unmarshal([]byte(srv.Headers), &headers); err != nil {
			return nil, fmt.Errorf("headers 不是合法的 JSON 对象: %w", err)
		}
	}
	switch srv.Transport {
	case "stdio":
		var args []string
		if strings.TrimSpace(srv.Args) != "" {
			if err := json.Unmarshal([]byte(srv.Args), &args); err != nil {
				return nil, fmt.Errorf("args 不是合法的 JSON 数组: %w", err)
			}
		}
		envMap := map[string]string{}
		if strings.TrimSpace(srv.Env) != "" {
			if err := json.Unmarshal([]byte(srv.Env), &envMap); err != nil {
				return nil, fmt.Errorf("env 不是合法的 JSON 对象: %w", err)
			}
		}
		env := make([]string, 0, len(envMap))
		for k, v := range envMap {
			env = append(env, k+"="+v)
		}
		return mcpclient.NewStdioMCPClient(srv.Command, env, args...)
	case "sse":
		return mcpclient.NewSSEMCPClient(srv.URL, mcptransport.WithHeaders(headers))
	case "http":
		return mcpclient.NewStreamableHttpClient(srv.URL, mcptransport.WithHTTPHeaders(headers))
	default:
		return nil, fmt.Errorf("不支持的传输方式: %s (stdio/sse/http)", srv.Transport)
	}
}

// connectServer 连接 -> 初始化 -> 发现并注册工具 (状态回写 DB)
func (s *MCPService) connectServer(ctx context.Context, srv *model.MCPServer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.entries[srv.Name]; ok {
		tools.Unregister(old.toolNames...)
		_ = old.cli.Close()
		delete(s.entries, srv.Name)
	}
	cli, err := s.buildClient(srv)
	if err != nil {
		s.markStatus(srv.ID, "failed", err.Error(), 0)
		return err
	}
	if err := cli.Start(ctx); err != nil {
		_ = cli.Close()
		s.markStatus(srv.ID, "failed", err.Error(), 0)
		return fmt.Errorf("启动连接失败: %w", err)
	}
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "assistant-admin", Version: "1.0"}
	if _, err := cli.Initialize(ctx, initReq); err != nil {
		_ = cli.Close()
		s.markStatus(srv.ID, "failed", err.Error(), 0)
		return fmt.Errorf("初始化失败: %w", err)
	}
	eTools, err := einomcp.GetTools(ctx, &einomcp.Config{Cli: cli})
	if err != nil {
		_ = cli.Close()
		s.markStatus(srv.ID, "failed", err.Error(), 0)
		return fmt.Errorf("发现工具失败: %w", err)
	}
	names := s.registerToolsLocked(srv, eTools, cli)
	s.markStatus(srv.ID, "ok", "", len(names))
	log.Printf("[MCP] 服务 %s 已连接, 注册 %d 个工具: %s", srv.Name, len(names), strings.Join(names, ", "))
	return nil
}

// registerToolsLocked 把发现的工具以前缀名注册进注册表 (持锁调用; DB/agentSvc 可为 nil, 测试用)
func (s *MCPService) registerToolsLocked(srv *model.MCPServer, eTools []tool.BaseTool, cli *mcpclient.Client) []string {
	timeout := srv.TimeoutSeconds
	if timeout <= 0 {
		timeout = mcpDefaultTimeout
	}
	names := make([]string, 0, len(eTools))
	for _, et := range eTools {
		info, err := et.Info(context.Background())
		if err != nil || info == nil || info.Name == "" {
			log.Printf("[MCP] 服务 %s 工具元数据获取失败, 跳过: %v", srv.Name, err)
			continue
		}
		regName := mcpToolRegistryName(srv.Name, info.Name)
		// 前缀折叠后可能与其它 server 撞名 (如 "Srv A"/"srv-a"): 已存在即跳过, 不覆盖别人的工具
		if _, exists := tools.GetToolMeta(regName); exists {
			log.Printf("[MCP] 服务 %s 工具 %s 与其他工具重名, 跳过", srv.Name, regName)
			continue
		}
		captured, regTimeout := et, time.Duration(timeout)*time.Second
		tools.Register(regName, info.Name, info.Desc, mcpToolConfigStub{}, func(config map[string]any) (tool.BaseTool, error) {
			return &renamedMCPTool{name: regName, inner: captured, timeout: regTimeout}, nil
		})
		names = append(names, regName)
	}
	s.entries[srv.Name] = &mcpEntry{server: srv, cli: cli, toolNames: names}
	s.invalidate()
	return names
}

// disconnectServerLocked 注销 server 的全部工具并断开 (持锁调用)
func (s *MCPService) disconnectServerLocked(name string) {
	entry, ok := s.entries[name]
	if !ok {
		return
	}
	tools.Unregister(entry.toolNames...)
	_ = entry.cli.Close()
	delete(s.entries, name)
	s.invalidate()
}

func (s *MCPService) markStatus(id uint, status, errMsg string, toolCount int) {
	if DB == nil {
		return
	}
	updates := map[string]any{"last_status": status, "last_error": errMsg, "tool_count": toolCount}
	if err := DB.Model(&model.MCPServer{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		log.Printf("[MCP] 更新服务状态失败 id=%d: %v", id, err)
	}
}

// RegisteredToolNames server 当前已注册的工具名 (管理页展示用)
func (s *MCPService) RegisteredToolNames(serverName string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[serverName]
	if !ok {
		return nil
	}
	out := make([]string, len(entry.toolNames))
	copy(out, entry.toolNames)
	return out
}

// ============ CRUD (配置动态注册) ============

// validateMCPServer 创建/更新前的配置校验
func validateMCPServer(srv *model.MCPServer) error {
	if strings.TrimSpace(srv.Name) == "" {
		return fmt.Errorf("服务名称不能为空")
	}
	if len([]rune(srv.Name)) > 64 {
		return fmt.Errorf("服务名称超过 64 字符")
	}
	if sanitizeMCPServerName(srv.Name) == "" {
		return fmt.Errorf("服务名称需包含字母或数字")
	}
	switch srv.Transport {
	case "sse", "http":
		u := strings.TrimSpace(srv.URL)
		if u == "" {
			return fmt.Errorf("sse/http 传输必须填写服务地址")
		}
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return fmt.Errorf("服务地址需以 http:// 或 https:// 开头")
		}
	case "stdio":
		if strings.TrimSpace(srv.Command) == "" {
			return fmt.Errorf("stdio 传输必须填写可执行命令")
		}
	default:
		return fmt.Errorf("不支持的传输方式: %s (stdio/sse/http)", srv.Transport)
	}
	if srv.TimeoutSeconds <= 0 {
		srv.TimeoutSeconds = mcpDefaultTimeout
	}
	if srv.TimeoutSeconds > mcpTimeoutMax {
		srv.TimeoutSeconds = mcpTimeoutMax
	}
	for k, v := range map[string]string{"args": srv.Args, "env": srv.Env, "headers": srv.Headers} {
		if strings.TrimSpace(v) == "" {
			continue
		}
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			return fmt.Errorf("%s 不是合法的 JSON: %w", k, err)
		}
	}
	return nil
}

// Create 新增服务并按需连接
func (s *MCPService) Create(srv *model.MCPServer) error {
	if err := validateMCPServer(srv); err != nil {
		return err
	}
	var count int64
	if err := DB.Model(&model.MCPServer{}).Where("name = ?", srv.Name).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("服务名称 %q 已存在", srv.Name)
	}
	if err := DB.Create(srv).Error; err != nil {
		return err
	}
	if srv.Enabled {
		s.asyncConnect(srv)
	}
	return nil
}

// Update 修改服务 (断开旧连接; 禁用时同步禁用其 ai_tools 行)
func (s *MCPService) Update(id uint, req *model.MCPServer) error {
	var old model.MCPServer
	if err := DB.Where("id = ?", id).First(&old).Error; err != nil {
		return err
	}
	var count int64
	if err := DB.Model(&model.MCPServer{}).Where("name = ? AND id <> ?", req.Name, id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("服务名称 %q 已存在", req.Name)
	}
	s.mu.Lock()
	s.disconnectServerLocked(old.Name)
	s.mu.Unlock()

	updates := map[string]any{
		"name": req.Name, "transport": req.Transport, "url": req.URL,
		"command": req.Command, "args": req.Args, "env": req.Env, "headers": req.Headers,
		"timeout_seconds": req.TimeoutSeconds, "enabled": req.Enabled,
	}
	if err := DB.Model(&model.MCPServer{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return err
	}
	// 从启用改为禁用: 自动禁用其工具行 (保留行, 重新启用后用户可再手动启用)
	if old.Enabled && !req.Enabled {
		s.disableToolRows(req.Name)
	}
	if req.Enabled {
		fresh := *req
		fresh.ID = id
		s.asyncConnect(&fresh)
	}
	return nil
}

// Delete 删除服务: 断开注销 + 自动删除其 ai_tools 工具行 + 删配置行
func (s *MCPService) Delete(id uint) error {
	var srv model.MCPServer
	if err := DB.Where("id = ?", id).First(&srv).Error; err != nil {
		return err
	}
	s.mu.Lock()
	s.disconnectServerLocked(srv.Name)
	s.mu.Unlock()
	if DB != nil {
		if err := DB.Where("name LIKE ?", mcpToolLikePattern(srv.Name)).Delete(&model.AITool{}).Error; err != nil {
			log.Printf("[MCP] 删除工具行失败 server=%s: %v", srv.Name, err)
		}
	}
	return DB.Delete(&model.MCPServer{}, id).Error
}

// ConnectNow 手动重连 (断开旧连接后重新发现注册)
func (s *MCPService) ConnectNow(id uint) error {
	var srv model.MCPServer
	if err := DB.Where("id = ?", id).First(&srv).Error; err != nil {
		return err
	}
	if !srv.Enabled {
		return fmt.Errorf("服务已禁用, 请先启用")
	}
	s.asyncConnect(&srv)
	return nil
}

func (s *MCPService) asyncConnect(srv *model.MCPServer) {
	copySrv := *srv
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), mcpConnectTimeout)
		defer cancel()
		if err := s.connectServer(ctx, &copySrv); err != nil {
			log.Printf("[MCP] 服务 %s 连接失败: %v", copySrv.Name, err)
		}
	}()
}

func (s *MCPService) disableToolRows(serverName string) {
	if DB == nil {
		return
	}
	if err := DB.Model(&model.AITool{}).Where("name LIKE ?", mcpToolLikePattern(serverName)).Update("enabled", false).Error; err != nil {
		log.Printf("[MCP] 禁用工具行失败 server=%s: %v", serverName, err)
	}
}

// ============ MCP 工具适配 ============

type mcpToolConfigStub struct{} // 注册表元数据用 (真实 schema 在 ToolInfo 里)

// renamedMCPTool 包装 eino-ext 的 MCP 工具: 改名为 mcp_<server>_<tool> 防跨 server 撞名,
// 调用时按服务配置加超时
type renamedMCPTool struct {
	name    string
	inner   tool.BaseTool
	timeout time.Duration
}

func (t *renamedMCPTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	ti, err := t.inner.Info(ctx)
	if err != nil {
		return nil, err
	}
	// 必须返回拷贝: eino-ext 组件在 InvokableRun 里直接用其内部 info.Name 构造调用请求,
	// 改写共享指针会把改名后的名字发给远端 server (server 侧报 tool not found)
	copied := *ti
	copied.Name = t.name
	return &copied, nil
}

func (t *renamedMCPTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	invokable, ok := t.inner.(tool.InvokableTool)
	if !ok {
		return "", fmt.Errorf("MCP 工具 %s 不支持同步调用", t.name)
	}
	if t.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}
	return invokable.InvokableRun(ctx, args, opts...)
}
