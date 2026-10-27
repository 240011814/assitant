package aitools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type WebSearchConfig struct {
	APIKey string `json:"api_key" description:"Tavily API Key" required:"true"`
}

type webSearchTool struct {
	config WebSearchConfig
	client *http.Client
}

type searchRequest struct {
	Query string `json:"query" jsonschema:"description=搜索关键词"`
}

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

type tavilyRequest struct {
	Query         string `json:"query"`
	IncludeAnswer bool   `json:"include_answer"`
	SearchDepth   string `json:"search_depth"`
}

type tavilyResponse struct {
	Answer            string `json:"answer"`
	FollowUpQuestions any    `json:"follow_up_questions"`
	Results           []struct {
		Title      string  `json:"title"`
		URL        string  `json:"url"`
		Content    string  `json:"content"`
		Score      float64 `json:"score"`
		RawContent any     `json:"raw_content"`
		Favicon    string  `json:"favicon"`
		ID         string  `json:"id"`
	} `json:"results"`
	Images       []any   `json:"images"`
	ResponseTime float64 `json:"response_time"`
	RequestID    string  `json:"request_id"`
}

func NewWebSearchTool(config WebSearchConfig) (tool.InvokableTool, error) {
	t := &webSearchTool{
		config: config,
		client: &http.Client{Timeout: 30 * time.Second},
	}
	return t, nil
}

func init() {
	Register("web_search", "联网搜索", "联网搜索工具，用于获取互联网上的实时信息。当需要查找最新新闻、实时数据、或本地知识库中没有的信息时使用此工具。", WebSearchConfig{}, func(config map[string]any) (tool.BaseTool, error) {
		apiKey, _ := config["api_key"].(string)
		return NewWebSearchTool(WebSearchConfig{APIKey: apiKey})
	})
}

func (t *webSearchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "web_search",
		Desc: "联网搜索工具，用于获取互联网上的实时信息。当需要查找最新新闻、实时数据、或本地知识库中没有的信息时使用此工具。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "搜索关键词",
				Required: true,
			},
		}),
	}, nil
}

func (t *webSearchTool) InvokableRun(ctx context.Context, arguments string, _ ...tool.Option) (string, error) {
	var req searchRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("解析搜索参数失败: %w", err)
	}
	if req.Query == "" {
		return "", fmt.Errorf("搜索关键词不能为空")
	}

	results, answer, err := t.search(ctx, req.Query)
	if err != nil {
		return "", fmt.Errorf("搜索失败: %w", err)
	}

	if len(results) == 0 && answer == "" {
		return "未找到相关搜索结果", nil
	}

	var output struct {
		Answer  string         `json:"answer,omitempty"`
		Results []searchResult `json:"results"`
	}
	output.Answer = answer
	output.Results = results

	data, _ := json.Marshal(output)
	return string(data), nil
}

func (t *webSearchTool) search(ctx context.Context, query string) ([]searchResult, string, error) {
	body, _ := json.Marshal(tavilyRequest{
		Query:         query,
		IncludeAnswer: true,
		SearchDepth:   "advanced",
	})

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.tavily.com/search", bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.config.APIKey)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("搜索API返回错误: %s", string(respBody))
	}

	var tavilyResp tavilyResponse
	if err := json.Unmarshal(respBody, &tavilyResp); err != nil {
		return nil, "", err
	}

	var results []searchResult
	for _, r := range tavilyResp.Results {
		results = append(results, searchResult{
			Title:   r.Title,
			URL:     r.URL,
			Content: r.Content,
		})
	}
	return results, tavilyResp.Answer, nil
}
