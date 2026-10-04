package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	interfaces "backend/interface"
)

// 发送邮件工具: 经系统配置页 SMTP 配置真实投递邮件。
// 注入走 interfaces.Notifier (service.EmailNotifier 实现, tools 包不反向 import service)。
// ai_tools 里以 confirm_required=1 种子, 执行前走现成的工具审批 (人工确认), 防误发/滥用

const (
	emailMaxRecipients = 10
	emailMaxSubject    = 200
	emailMaxBodyChars  = 20000
)

var emailNotifier interfaces.Notifier

// SetEmailNotifier 注册邮件发送实例 (main.go 装配; 未注入时邮件工具报未启用)
func SetEmailNotifier(n interfaces.Notifier) { emailNotifier = n }

type sendEmailConfig struct{} // 无配置项

type sendEmailTool struct{}

type sendEmailRequest struct {
	To      string `json:"to" jsonschema:"description=收件人邮箱地址, 多个收件人用英文逗号分隔"`
	Subject string `json:"subject" jsonschema:"description=邮件主题"`
	Content string `json:"content" jsonschema:"description=邮件正文 (纯文本)"`
}

func init() {
	Register("send_email", "发送邮件",
		"发送真实电子邮件 (会实际投递到收件人邮箱, 且执行前需要用户人工确认)。仅当用户明确要求发送邮件时使用; 收件人、主题、正文必须与用户的要求一致, 不要替用户编造或扩展内容。",
		sendEmailConfig{},
		func(config map[string]any) (tool.BaseTool, error) {
			return &sendEmailTool{}, nil
		})
}

func (t *sendEmailTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "send_email",
		Desc: "发送真实电子邮件 (需用户确认后执行)",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"to": {
				Type:     schema.String,
				Desc:     "收件人邮箱地址, 多个收件人用英文逗号分隔 (上限 10 个)",
				Required: true,
			},
			"subject": {
				Type:     schema.String,
				Desc:     "邮件主题",
				Required: true,
			},
			"content": {
				Type:     schema.String,
				Desc:     "邮件正文 (纯文本)",
				Required: true,
			},
		}),
	}, nil
}

func (t *sendEmailTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	if emailNotifier == nil {
		return "", fmt.Errorf("邮件功能未启用 (需在系统配置中开启 SMTP)")
	}
	var req sendEmailRequest
	if err := json.Unmarshal([]byte(argumentsInJSON), &req); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	// 收件人: 逗号分隔, 去重, 校验格式与数量
	seen := map[string]bool{}
	recipients := make([]string, 0, 4)
	for _, part := range strings.Split(req.To, ",") {
		addr := strings.TrimSpace(part)
		if addr == "" {
			continue
		}
		if strings.ContainsAny(addr, " \t\r\n") || !strings.Contains(addr, "@") {
			return "", fmt.Errorf("收件人邮箱格式不正确: %q", addr)
		}
		if !seen[addr] {
			seen[addr] = true
			recipients = append(recipients, addr)
		}
	}
	if len(recipients) == 0 {
		return "", fmt.Errorf("缺少收件人邮箱 (to)")
	}
	if len(recipients) > emailMaxRecipients {
		return "", fmt.Errorf("收件人数量超过上限 %d", emailMaxRecipients)
	}
	subject := strings.TrimSpace(req.Subject)
	if subject == "" {
		return "", fmt.Errorf("邮件主题不能为空")
	}
	if len([]rune(subject)) > emailMaxSubject {
		return "", fmt.Errorf("邮件主题超过 %d 字符上限", emailMaxSubject)
	}
	content := req.Content
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("邮件正文不能为空")
	}
	if len([]rune(content)) > emailMaxBodyChars {
		return "", fmt.Errorf("邮件正文超过 %d 字符上限", emailMaxBodyChars)
	}

	// 逐个收件人发送 (Send 接口单收件人), 记录部分失败
	var sent, failed []string
	for _, addr := range recipients {
		err := emailNotifier.Send(addr, interfaces.NotifyMessage{Subject: subject, Body: content})
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s (原因: %s)", addr, err.Error()))
			continue
		}
		sent = append(sent, addr)
	}
	var sb strings.Builder
	if len(sent) > 0 {
		fmt.Fprintf(&sb, "邮件已发送至: %s", strings.Join(sent, ", "))
	}
	if len(failed) > 0 {
		if len(sent) > 0 {
			sb.WriteString("; ")
		}
		fmt.Fprintf(&sb, "发送失败: %s", strings.Join(failed, ", "))
	}
	return sb.String(), nil
}
