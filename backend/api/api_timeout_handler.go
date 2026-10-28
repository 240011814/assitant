package api

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"backend/service/sys/systemconfig"

	"github.com/gin-gonic/gin"
)

// 前端接口请求超时配置: 默认值 + 按路由覆盖项存 system_config (api_timeout_default /
// api_timeout_overrides, 只存非默认项), 全量路由清单由 gin 运行时枚举实时生成 (新增
// 接口自动出现, 删除不留脏数据)。前端登录后拉一次, 在 axios onRequest 钩子按
// method+path 匹配应用; 优先级: 单请求显式 timeout > 路由覆盖 > 默认。
// 流式接口 (SSE) 走 fetch 不经 axios, 不受此配置约束, 清单里仅标注。

const (
	apiTimeoutDefaultKey   = "api_timeout_default"
	apiTimeoutOverridesKey = "api_timeout_overrides"
	// apiTimeoutFallbackMs 配置缺省时的兜底默认值 (与 @sa/axios 包内置默认一致)
	apiTimeoutFallbackMs = 10000
	// 超时上下限: 下限防手滑 0 造成请求永不完成, 上限给慢接口留空间 (skill 发现需数分钟)
	apiTimeoutMinMs = 1000
	apiTimeoutMaxMs = 600000
)

// apiRouteOverride 单条路由超时覆盖
type apiRouteOverride struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	TimeoutMs int    `json:"timeoutMs"`
}

// ApiTimeoutHandler 接口超时配置端点
type ApiTimeoutHandler struct {
	configSvc   *systemconfig.SystemConfigService
	routeLister func() gin.RoutesInfo
}

func NewApiTimeoutHandler(configSvc *systemconfig.SystemConfigService) *ApiTimeoutHandler {
	return &ApiTimeoutHandler{configSvc: configSvc}
}

// SetRouteLister 注入路由枚举器 (setupRouter 里闭包 gin.Engine, 请求时才求值以拿到全量路由)
func (h *ApiTimeoutHandler) SetRouteLister(fn func() gin.RoutesInfo) {
	h.routeLister = fn
}

// streamingRoutes 流式 (SSE) 接口清单: 前端走 fetch 不经 axios, 超时配置对它们无效
var streamingRoutes = map[string]bool{
	"POST /api/chat":                       true,
	"POST /api/ai-orchestrations/:id/chat": true,
	"POST /api/agent-studio/debug":         true,
	"POST /api/agent-studio/debug-node":    true,
}

// readDefaultMs 读默认超时 (毫秒), 未配置/非法回落 10000
func (h *ApiTimeoutHandler) readDefaultMs() int {
	v, err := h.configSvc.GetValue(apiTimeoutDefaultKey)
	if err != nil {
		return apiTimeoutFallbackMs
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < apiTimeoutMinMs || n > apiTimeoutMaxMs {
		return apiTimeoutFallbackMs
	}
	return n
}

// readOverrides 读覆盖项 (未配置/损坏返回空切片)
func (h *ApiTimeoutHandler) readOverrides() []apiRouteOverride {
	v, err := h.configSvc.GetValue(apiTimeoutOverridesKey)
	if err != nil || strings.TrimSpace(v) == "" {
		return []apiRouteOverride{}
	}
	var list []apiRouteOverride
	if err := json.Unmarshal([]byte(v), &list); err != nil {
		return []apiRouteOverride{}
	}
	return list
}

// HandleGetTimeouts 下发超时配置给前端 (仅需登录; 前端登录后拉一次做请求期匹配)
func (h *ApiTimeoutHandler) HandleGetTimeouts(c *gin.Context) {
	SendSuccess(c, gin.H{
		"defaultMs": h.readDefaultMs(),
		"overrides": h.readOverrides(),
	})
}

// apiRouteTimeout 管理端清单条目
type apiRouteTimeout struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	TimeoutMs int    `json:"timeoutMs"`
	Custom    bool   `json:"custom"`
	Streaming bool   `json:"streaming"`
}

// HandleListRoutes 全量路由清单 × 生效超时 (配置页「接口超时」区块数据源)
func (h *ApiTimeoutHandler) HandleListRoutes(c *gin.Context) {
	defaultMs := h.readDefaultMs()
	overrides := h.readOverrides()
	lookup := make(map[string]int, len(overrides))
	for _, o := range overrides {
		lookup[o.Method+" "+o.Path] = o.TimeoutMs
	}

	var out []apiRouteTimeout
	if h.routeLister != nil {
		for _, r := range h.routeLister() {
			item := apiRouteTimeout{Method: r.Method, Path: r.Path, TimeoutMs: defaultMs, Streaming: streamingRoutes[r.Method+" "+r.Path]}
			if t, ok := lookup[r.Method+" "+r.Path]; ok {
				item.TimeoutMs = t
				item.Custom = true
			}
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	SendSuccess(c, gin.H{
		"defaultMs": defaultMs,
		"overrides": overrides,
		"routes":    out,
	})
}

// HandleUpdateRoute 更新单条路由超时 (timeoutMs<=0 表示清除覆盖恢复默认)
func (h *ApiTimeoutHandler) HandleUpdateRoute(c *gin.Context) {
	var req struct {
		Method    string `json:"method" binding:"required"`
		Path      string `json:"path" binding:"required"`
		TimeoutMs int    `json:"timeoutMs"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	req.Path = strings.TrimSpace(req.Path)

	// 路由必须真实存在 (防手滑写入永远不会匹配的配置)
	exists := false
	if h.routeLister != nil {
		for _, r := range h.routeLister() {
			if r.Method == req.Method && r.Path == req.Path {
				exists = true
				break
			}
		}
	}
	if !exists {
		SendError(c, "400", "路由不存在: "+req.Method+" "+req.Path)
		return
	}
	if streamingRoutes[req.Method+" "+req.Path] {
		SendError(c, "400", "流式接口不经 axios, 超时配置无效")
		return
	}

	overrides := h.readOverrides()
	kept := overrides[:0:0]
	changed := false
	for _, o := range overrides {
		if o.Method == req.Method && o.Path == req.Path {
			changed = true
			continue // 覆盖项一律先摘除: 下面按需补回 (清除或更新)
		}
		kept = append(kept, o)
	}
	if req.TimeoutMs > 0 {
		if req.TimeoutMs < apiTimeoutMinMs {
			req.TimeoutMs = apiTimeoutMinMs
		}
		if req.TimeoutMs > apiTimeoutMaxMs {
			req.TimeoutMs = apiTimeoutMaxMs
		}
		kept = append(kept, apiRouteOverride{Method: req.Method, Path: req.Path, TimeoutMs: req.TimeoutMs})
		changed = true
	}
	if !changed {
		SendSuccess(c, gin.H{"message": "配置无变化"})
		return
	}

	raw, err := json.Marshal(kept)
	if err != nil {
		SendError(c, "500", "序列化覆盖项失败: "+err.Error())
		return
	}
	if err := h.configSvc.SetValue(apiTimeoutOverridesKey, string(raw), "前端接口超时覆盖项 (method+path → 毫秒)"); err != nil {
		SendError(c, "500", "保存配置失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"message": "已保存, 前端下次进入生效"})
}

// HandleUpdateDefault 更新默认超时 (毫秒)
func (h *ApiTimeoutHandler) HandleUpdateDefault(c *gin.Context) {
	var req struct {
		TimeoutMs int `json:"timeoutMs" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	if req.TimeoutMs < apiTimeoutMinMs || req.TimeoutMs > apiTimeoutMaxMs {
		SendError(c, "400", "超时需在 1s ~ 10min 之间")
		return
	}
	if err := h.configSvc.SetValue(apiTimeoutDefaultKey, strconv.Itoa(req.TimeoutMs), "前端接口默认请求超时 (毫秒)"); err != nil {
		SendError(c, "500", "保存配置失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"message": "已保存, 前端下次进入生效"})
}
