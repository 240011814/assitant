package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/document/parser"
	einoHTML "github.com/cloudwego/eino-ext/components/document/parser/html"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// 网页阅读工具: 抓取 URL 并抽取正文文本, 与 read_document 同款 rune 级分页。
// SSRF 防护: 仅 http/https; 主机解析出的 IP 一律拒绝环回/内网/链路本地地址;
// 重定向逐跳复检 (防公网 URL 302 跳内网/云元数据地址)。
// 注: DNS 解析与实际拨号存在 TOCTOU 窗口 (DNS rebinding 未完全防护), 个人平台可接受

const (
	fetchMaxBytes     = 8 << 20 // 单次抓取响应体上限 8MB
	fetchMaxChars     = 200000  // 正文抽取后的字符上限 (超出截断, 分页仍可读前段)
	fetchDefaultChunk = 6000    // 单次返回字符数默认值
	fetchMaxChunk     = 20000   // 单次返回字符数上限
	fetchTimeout      = 25 * time.Second
)

type fetchURLConfig struct{} // 无配置项

type fetchURLTool struct {
	allowLoopback bool // 仅测试用: 放行环回地址 (httptest 服务在 127.0.0.1)
	htmlOnce      sync.Once
	htmlParser    parser.Parser
	htmlErr       error
}

type fetchURLRequest struct {
	URL    string `json:"url" jsonschema:"description=要读取的网页地址 (http/https)"`
	Offset int    `json:"offset" jsonschema:"description=起始字符偏移, 默认 0"`
	Length int    `json:"length" jsonschema:"description=本次读取的字符数, 默认 6000, 上限 20000"`
}

func init() {
	Register("fetch_url", "网页阅读",
		"抓取指定 URL 的网页并返回正文文本 (支持分页)。web_search 找到相关链接后用本工具读取页面全文, 也用于读取用户提供的网页链接。仅支持 http/https 公网地址, 不接受内网/环回目标。",
		fetchURLConfig{},
		func(config map[string]any) (tool.BaseTool, error) {
			return &fetchURLTool{}, nil
		})
}

func (t *fetchURLTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "fetch_url",
		Desc: "抓取 URL 网页并返回正文文本 (rune 级分页, 返回带 total_chars 与 next_offset)",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"url": {
				Type:     schema.String,
				Desc:     "要读取的网页地址 (http/https)",
				Required: true,
			},
			"offset": {
				Type: schema.Integer,
				Desc: "起始字符偏移, 默认 0",
			},
			"length": {
				Type: schema.Integer,
				Desc: "本次读取的字符数, 默认 6000, 上限 20000",
			},
		}),
	}, nil
}

// ============ SSRF 防护 ============

func rejectPrivateIP(ip net.IP) bool {
	return ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func checkFetchIP(ip net.IP, allowLoopback bool) error {
	if allowLoopback && (ip.IsLoopback() || ip.IsUnspecified()) {
		return nil
	}
	if rejectPrivateIP(ip) {
		return fmt.Errorf("禁止访问内网/环回地址 (%s)", ip)
	}
	return nil
}

// validateFetchURL 校验 scheme 与主机解析后的 IP, 拒绝内网/环回目标
func validateFetchURL(raw string, allowLoopback bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("URL 解析失败: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("仅支持 http/https 协议")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL 缺少主机名")
	}
	if ip := net.ParseIP(host); ip != nil {
		return checkFetchIP(ip, allowLoopback)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("域名解析失败: %w", err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("域名无解析结果: %s", host)
	}
	for _, ip := range ips {
		if err := checkFetchIP(ip, allowLoopback); err != nil {
			return err
		}
	}
	return nil
}

func (t *fetchURLTool) httpClient() *http.Client {
	return &http.Client{
		Timeout: fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("重定向次数过多")
			}
			// 每一跳都复检目标地址
			return validateFetchURL(req.URL.String(), t.allowLoopback)
		},
	}
}

// ============ 抓取与正文抽取 ============

// getHTMLParser 懒构建 HTML 正文抽取器 (构建失败记录错误, 调用时返回)
func (t *fetchURLTool) getHTMLParser(ctx context.Context) (parser.Parser, error) {
	t.htmlOnce.Do(func() {
		t.htmlParser, t.htmlErr = einoHTML.NewParser(ctx, &einoHTML.Config{})
	})
	return t.htmlParser, t.htmlErr
}

func (t *fetchURLTool) fetch(ctx context.Context, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.5")
	resp, err := t.httpClient().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("请求网页失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("网页返回 HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, fetchMaxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("读取网页内容失败: %w", err)
	}
	if len(body) > fetchMaxBytes {
		return nil, "", fmt.Errorf("网页内容超过 %dMB 上限", fetchMaxBytes>>20)
	}
	return body, resp.Header.Get("Content-Type"), nil
}

func (t *fetchURLTool) extractText(ctx context.Context, body []byte, contentType string) (string, error) {
	ct := strings.ToLower(contentType)
	isHTML := ct == "" || strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml")
	isText := strings.Contains(ct, "text/") || strings.Contains(ct, "application/json") ||
		strings.Contains(ct, "application/xml") || strings.Contains(ct, "javascript")
	var text string
	switch {
	case isHTML:
		p, err := t.getHTMLParser(ctx)
		if err != nil {
			return "", fmt.Errorf("HTML 解析器构建失败: %w", err)
		}
		docs, err := p.Parse(ctx, bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("HTML 解析失败: %w", err)
		}
		var sb strings.Builder
		for _, d := range docs {
			if d == nil || d.Content == "" {
				continue
			}
			if sb.Len() > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(d.Content)
		}
		text = sb.String()
	case isText:
		text = string(body)
	default:
		return "", fmt.Errorf("不支持的网页内容类型: %s (仅支持 HTML/纯文本)", contentType)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("未抽取到正文内容 (可能是纯脚本渲染的页面或空文档)")
	}
	if runes := []rune(text); len(runes) > fetchMaxChars {
		text = string(runes[:fetchMaxChars]) + "\n\n…(内容过长已截断)"
	}
	return text, nil
}

func (t *fetchURLTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var req fetchURLRequest
	if err := json.Unmarshal([]byte(argumentsInJSON), &req); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	rawURL := strings.TrimSpace(req.URL)
	if rawURL == "" {
		return "", fmt.Errorf("缺少 url 参数")
	}
	if err := validateFetchURL(rawURL, t.allowLoopback); err != nil {
		return "", err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	body, contentType, err := t.fetch(fetchCtx, rawURL)
	if err != nil {
		return "", err
	}
	text, err := t.extractText(fetchCtx, body, contentType)
	if err != nil {
		return "", err
	}

	runes := []rune(text)
	offset, length := req.Offset, req.Length
	if offset < 0 {
		offset = 0
	}
	if length <= 0 {
		length = fetchDefaultChunk
	}
	if length > fetchMaxChunk {
		length = fetchMaxChunk
	}
	if offset >= len(runes) {
		return fmt.Sprintf("网页 %s (共 %d 字符)\n[偏移 %d 已越界, 已到结尾]", rawURL, len(runes), offset), nil
	}
	end := offset + length
	if end > len(runes) {
		end = len(runes)
	}
	chunk := string(runes[offset:end])
	nextOffset := offset + len([]rune(chunk))
	if nextOffset >= len(runes) {
		return fmt.Sprintf("网页 %s (共 %d 字符)\n[偏移 %d-%d, 已到结尾]\n\n%s", rawURL, len(runes), offset, nextOffset, chunk), nil
	}
	return fmt.Sprintf("网页 %s (共 %d 字符)\n[偏移 %d-%d, 未读完; 继续读取请传 offset=%d]\n\n%s", rawURL, len(runes), offset, nextOffset, nextOffset, chunk), nil
}
