package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	interfaces "backend/interface"
)

// 文档语义检索工具 (RAG): 在当前用户的文档向量索引中检索相关片段。
// 与 read_document 互补: search 定位相关段落 -> read 按偏移精读上下文。
// 身份走 userIDFromSession, 只能检索当前用户自己的文档

var documentSearcher interfaces.DocumentSearcher

// SetDocumentSearcher 注册文档检索实例 (main.go 装配; 未注入时检索工具报未启用)
func SetDocumentSearcher(searcher interfaces.DocumentSearcher) { documentSearcher = searcher }

// documentSearchConfig 检索工具无配置项; Register 的参数是工具配置, 不是 LLM 请求参数 (后者在 Info 声明)
type documentSearchConfig struct{}

type searchUserDocumentsTool struct{}

type searchUserDocumentsRequest struct {
	Query       string `json:"query" jsonschema:"description=检索查询语句, 用自然语言描述要找的内容"`
	DocumentIDs string `json:"document_ids" jsonschema:"description=可选, 逗号分隔的文档 ID 列表, 限定在这些文档内检索 (来自 list_user_documents)"`
	TopK        int    `json:"top_k" jsonschema:"description=可选, 返回片段数, 默认 5, 上限 20"`
}

func init() {
	Register("search_user_documents", "文档语义检索",
		"在当前用户上传的文档中做语义检索, 返回与查询最相关的片段 (含文档名/片段序号/相似度)。用户问题可能涉及已上传文档内容、或文档较多较长时优先用本工具定位相关段落, 再配合 read_document 按 (document_id, offset) 精读上下文; 文档列表可用 list_user_documents 获取。query 用与文档内容相近的表述 (而非照抄用户口语)。",
		documentSearchConfig{},
		func(config map[string]any) (tool.BaseTool, error) {
			return &searchUserDocumentsTool{}, nil
		})
}

func (t *searchUserDocumentsTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "search_user_documents",
		Desc: "语义检索当前用户上传文档, 返回最相关的片段 (文档名/片段序号/相似度/内容)",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "检索查询语句, 用自然语言描述要找的内容",
				Required: true,
			},
			"document_ids": {
				Type: "string",
				Desc: "可选, 逗号分隔的文档 ID 列表, 限定检索范围",
			},
			"top_k": {
				Type: schema.Integer,
				Desc: "可选, 返回片段数, 默认 5, 上限 20",
			},
		}),
	}, nil
}

func (t *searchUserDocumentsTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	if documentSearcher == nil {
		return "", fmt.Errorf("文档检索功能未启用")
	}
	userID, err := userIDFromSession(ctx)
	if err != nil {
		return "", err
	}
	var req searchUserDocumentsRequest
	if err := json.Unmarshal([]byte(argumentsInJSON), &req); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return "", fmt.Errorf("缺少 query 参数")
	}
	// 注册表参数反射只支持标量类型, 文档 ID 列表以逗号分隔字符串传递
	var docIDs []uint
	for _, part := range strings.Split(req.DocumentIDs, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.ParseUint(part, 10, 32); err == nil && n > 0 {
			docIDs = append(docIDs, uint(n))
		}
	}
	hits, err := documentSearcher.SearchDocuments(userID, query, docIDs, req.TopK)
	if err != nil {
		return "", err
	}
	if len(hits) == 0 {
		return "未检索到相关片段。可能原因: 查询词与文档内容差异较大, 或文档尚未建立/已失效向量索引 (可在文档管理页对文档执行建立/重建索引)。", nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "共检索到 %d 条相关片段 (按相似度降序):\n", len(hits))
	for _, h := range hits {
		name := h.Filename
		if name == "" {
			name = "(文档已删除)"
		}
		fmt.Fprintf(&sb, "\n【%s | 文档ID=%d | 片段#%d | 相似度 %.3f】\n%s\n", name, h.DocumentID, h.ChunkIndex, h.Score, h.Content)
	}
	return sb.String(), nil
}
