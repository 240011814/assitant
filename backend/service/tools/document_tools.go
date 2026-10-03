package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	interfaces "backend/interface"
)

// 用户文档工具: 读取当前用户在「文档管理」上传的文档。
// 身份走 userIDFromSession (普通对话 ADK 会话值 / 编排 WithRunUserID), 只能访问自己的文档

var userDocumentStore interfaces.UserDocumentStore

// SetUserDocumentStore 注册用户文档存取实例 (main.go 装配; 未注入时文档工具报未启用)
func SetUserDocumentStore(store interfaces.UserDocumentStore) { userDocumentStore = store }

// ============ list_user_documents ============

type listUserDocumentsTool struct{}

type listUserDocumentsRequest struct{}

func init() {
	Register("list_user_documents", "文档列表",
		"列出当前用户上传的全部文档 (ID/文件名/类型/解析状态/文本长度)。当用户提到\"我的文档/我上传的文件\"或问题可能依赖用户上传的资料时, 先调用本工具确认有哪些文档可用, 再用 read_document 按 ID 读取内容。",
		listUserDocumentsRequest{},
		func(config map[string]any) (tool.BaseTool, error) {
			return &listUserDocumentsTool{}, nil
		})
}

func (t *listUserDocumentsTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name:        "list_user_documents",
		Desc:        "列出当前用户上传的全部文档 (ID/文件名/类型/解析状态/文本长度)",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{}),
	}, nil
}

func (t *listUserDocumentsTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	if userDocumentStore == nil {
		return "", fmt.Errorf("文档功能未启用")
	}
	userID, err := userIDFromSession(ctx)
	if err != nil {
		return "", err
	}
	docs, err := userDocumentStore.ListDocuments(userID)
	if err != nil {
		return "", fmt.Errorf("获取文档列表失败: %w", err)
	}
	if len(docs) == 0 {
		return "当前用户没有上传任何文档。", nil
	}
	out, err := json.MarshalIndent(docs, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ============ read_document ============

type readDocumentTool struct{}

type readDocumentRequest struct {
	DocumentID uint `json:"document_id" jsonschema:"description=要读取的文档 ID (先经 list_user_documents 获取)"`
	Offset     int  `json:"offset" jsonschema:"description=起始字符偏移, 默认 0"`
	Length     int  `json:"length" jsonschema:"description=本次读取的字符数, 默认 6000, 上限 20000"`
}

func init() {
	Register("read_document", "文档读取",
		"按 ID 读取当前用户上传文档的解析文本 (支持分页)。用户提问与已上传文档内容相关时使用: 先用 list_user_documents 找到文档 ID, 再分段读取; 返回中带 total_chars 与 next_offset, 内容不够继续时用 offset 翻页。",
		readDocumentRequest{},
		func(config map[string]any) (tool.BaseTool, error) {
			return &readDocumentTool{}, nil
		})
}

func (t *readDocumentTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "read_document",
		Desc: "按 ID 分页读取当前用户上传文档的解析文本",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"document_id": {
				Type:     schema.Integer,
				Desc:     "要读取的文档 ID",
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

func (t *readDocumentTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	if userDocumentStore == nil {
		return "", fmt.Errorf("文档功能未启用")
	}
	userID, err := userIDFromSession(ctx)
	if err != nil {
		return "", err
	}
	var req readDocumentRequest
	if err := json.Unmarshal([]byte(argumentsInJSON), &req); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	if req.DocumentID == 0 {
		return "", fmt.Errorf("缺少 document_id 参数")
	}
	chunk, total, filename, err := userDocumentStore.ReadDocumentChunk(userID, req.DocumentID, req.Offset, req.Length)
	if err != nil {
		return "", err
	}
	nextOffset := req.Offset + len([]rune(chunk))
	if nextOffset >= total {
		return fmt.Sprintf("文档「%s」(共 %d 字符)\n[偏移 %d-%d, 已到结尾]\n\n%s", filename, total, req.Offset, nextOffset, chunk), nil
	}
	return fmt.Sprintf("文档「%s」(共 %d 字符)\n[偏移 %d-%d, 未读完; 继续读取请传 offset=%d]\n\n%s", filename, total, req.Offset, nextOffset, nextOffset, chunk), nil
}
