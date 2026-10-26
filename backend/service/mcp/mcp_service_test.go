package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"
	einomcp "github.com/cloudwego/eino-ext/components/tool/mcp"
	mcpclient "github.com/mark3labs/mcp-go/client"
	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"backend/model"
	"backend/service/tools"
)

func TestSanitizeMCPServerName(t *testing.T) {
	cases := map[string]string{
		"My Server":   "my_server",
		"文件系统-01":    "01", // Trim 折叠首尾下划线
		"  GitHub #1": "github_1",
	}
	for in, want := range cases {
		if got := sanitizeMCPServerName(in); got != want {
			t.Fatalf("sanitize(%q)=%q, want %q", in, got, want)
		}
	}
	if got := mcpToolRegistryName("Test Srv", "echo"); got != "mcp_test_srv_echo" {
		t.Fatalf("registry name = %q", got)
	}
}

func TestMCPToolLikePatternEscapesWildcards(t *testing.T) {
	// server 名里的 _ 和 % 必须转义, 否则 LIKE 会误伤其他 server 的工具行
	if got := mcpToolLikePattern(`my_srv`); got != `mcp\_my\_srv\_` {
		t.Fatalf("pattern = %q", got)
	}
	if got := mcpToolLikePattern(`a%b`); got != `mcp\_a\_b\_` {
		t.Fatalf("pattern = %q", got)
	}
}

func TestValidateMCPServer(t *testing.T) {
	base := func() *model.MCPServer {
		return &model.MCPServer{Name: "demo", Transport: "http", URL: "https://mcp.example.com/sse"}
	}
	if err := validateMCPServer(base()); err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}
	// sse/http 缺地址
	s := base()
	s.URL = ""
	if err := validateMCPServer(s); err == nil {
		t.Fatal("http 缺地址应报错")
	}
	// stdio 缺命令
	s = base()
	s.Transport = "stdio"
	if err := validateMCPServer(s); err == nil {
		t.Fatal("stdio 缺命令应报错")
	}
	// 坏 transport
	s = base()
	s.Transport = "ws"
	if err := validateMCPServer(s); err == nil {
		t.Fatal("未知 transport 应报错")
	}
	// 坏 JSON
	s = base()
	s.Headers = "{bad"
	if err := validateMCPServer(s); err == nil {
		t.Fatal("headers 非法 JSON 应报错")
	}
	// 超时钳位
	s = base()
	s.TimeoutSeconds = 99999
	if err := validateMCPServer(s); err != nil {
		t.Fatalf("超时钳位不应报错: %v", err)
	}
	if s.TimeoutSeconds != mcpTimeoutMax {
		t.Fatalf("超时应钳位到 %d, got %d", mcpTimeoutMax, s.TimeoutSeconds)
	}
}

// TestMCPRegisterToolsEndToEnd in-process MCP server -> 发现 -> 前缀注册 -> 调用
func TestMCPRegisterToolsEndToEnd(t *testing.T) {
	srv := mcpserver.NewMCPServer("test-server", "1.0.0", mcpserver.WithToolCapabilities(false))
	echoTool := mcp.NewTool("echo",
		mcp.WithDescription("回显输入文本"),
		mcp.WithString("text", mcp.Required(), mcp.Description("要回显的文本")),
	)
	srv.AddTool(echoTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("echo:" + req.GetString("text", "")), nil
	})
	cli, err := mcpclient.NewInProcessClient(srv)
	if err != nil {
		t.Fatalf("构建 in-process 客户端失败: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "test", Version: "1.0"}
	if _, err := cli.Initialize(ctx, initReq); err != nil {
		t.Fatalf("初始化失败: %v", err)
	}
	eTools, err := einomcp.GetTools(ctx, &einomcp.Config{Cli: cli})
	if err != nil {
		t.Fatalf("发现工具失败: %v", err)
	}

	svc := NewMCPService()
	srvRow := &model.MCPServer{Name: "Test Srv", Transport: "http", TimeoutSeconds: 5}
	names := svc.registerToolsLocked(srvRow, eTools, cli)
	t.Cleanup(func() { tools.Unregister(names...) })

	if len(names) != 1 || names[0] != "mcp_test_srv_echo" {
		t.Fatalf("应注册 mcp_test_srv_echo, got %v", names)
	}
	meta, ok := tools.GetToolMeta("mcp_test_srv_echo")
	if !ok || meta.DisplayName != "echo" {
		t.Fatalf("注册表元数据不符: %+v ok=%v", meta, ok)
	}
	if got := svc.RegisteredToolNames("Test Srv"); len(got) != 1 || got[0] != "mcp_test_srv_echo" {
		t.Fatalf("RegisteredToolNames 不符: %v", got)
	}

	// 经注册表工厂路径构建并调用 (与 ai_tools 的 BuildToolByName 同路径)
	inst, err := tools.CreateTool("mcp_test_srv_echo", "{}")
	if err != nil {
		t.Fatalf("CreateTool 失败: %v", err)
	}
	inv, ok := inst.(tool.InvokableTool)
	if !ok {
		t.Fatal("工具应实现 InvokableTool")
	}
	info, err := inv.Info(ctx)
	if err != nil || info.Name != "mcp_test_srv_echo" {
		t.Fatalf("Info 名称应为前缀名, got %+v err=%v", info, err)
	}
	out, err := inv.InvokableRun(ctx, `{"text": "你好"}`)
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}
	if !strings.Contains(out, "echo:你好") {
		t.Fatalf("调用结果不符: %s", out)
	}
}

func TestMCPUnregister(t *testing.T) {
	tools.Register("mcp_test_unregister_dummy", "d", "d", struct{}{}, func(map[string]any) (tool.BaseTool, error) {
		return nil, nil
	})
	t.Cleanup(func() { tools.Unregister("mcp_test_unregister_dummy") })
	if _, ok := tools.GetToolMeta("mcp_test_unregister_dummy"); !ok {
		t.Fatal("注册后应可见")
	}
	tools.Unregister("mcp_test_unregister_dummy")
	if _, ok := tools.GetToolMeta("mcp_test_unregister_dummy"); ok {
		t.Fatal("注销后应不可见")
	}
}
