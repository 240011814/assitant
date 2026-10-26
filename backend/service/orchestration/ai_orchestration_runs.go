package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"backend/model"
)

// ---------- 运行历史 ----------

// recordOrchRunAsync 异步落一行编排运行历史 (摘要 JSON 供历史页回看节点 trace)
func (s *AIOrchestrationService) recordOrchRunAsync(orchID, userID uint, req *model.DebugRunRequest, summary *DebugRunResult, errMsg string) {
	source := model.OrchRunSourceDebug
	if req.ChatMode {
		source = model.OrchRunSourceChat
	}
	status := model.OrchRunStatusSuccess
	if errMsg != "" {
		status = model.OrchRunStatusError
	}
	run := model.OrchRun{
		OrchID:  orchID,
		UserID:  userID,
		Source:  source,
		Status:  status,
		Mode:    summaryModeOrEmpty(summary),
		TotalMS: summaryTotalMSOrZero(summary),
		Error:   errMsg,
	}
	SafeGo("orch:record-run", func() {
		if summary != nil {
			if data, err := json.Marshal(summary); err == nil {
				run.Summary = string(data)
			}
		}
		if err := DB.Create(&run).Error; err != nil {
			log.Printf("[orchestration] 记录运行历史失败 orch=%d: %v", orchID, err)
		}
	})
}

func summaryModeOrEmpty(s *DebugRunResult) string {
	if s == nil {
		return ""
	}
	return s.Mode
}

func summaryTotalMSOrZero(s *DebugRunResult) int64 {
	if s == nil {
		return 0
	}
	return s.TotalMS
}

// ListRuns 编排的运行历史 (新→旧)
func (s *AIOrchestrationService) ListRuns(orchID uint, limit int) ([]model.OrchRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var runs []model.OrchRun
	if err := DB.Where("orch_id = ?", orchID).Order("id DESC").Limit(limit).Find(&runs).Error; err != nil {
		return nil, err
	}
	return runs, nil
}

// ---------- 单节点试跑 ----------

// DebugNode 单节点试跑: 取画布定义中的目标节点 (连同其子Agent 委派) 构成最小图运行,
// 输入为用户给定文本。用于调试面板对单个节点验证提示词/配置, 不触发整个编排。
func (s *AIOrchestrationService) DebugNode(ctx context.Context, userID uint, definition, nodeID, input string, params map[string]string) (*DebugRunResult, error) {
	dsl, issues := validateOrchestrationDSLDetailed(definition)
	if len(issues) > 0 {
		return nil, fmt.Errorf("编排定义校验失败: %s", strings.Join(orchIssueMessages(issues), "; "))
	}
	node := cNodeByID(dsl, nodeID)
	if node == nil {
		return nil, fmt.Errorf("节点 %s 不存在", nodeID)
	}
	switch node.Type {
	case OrchNodeBranch, OrchNodeRouter, OrchNodeMerge, OrchNodeEnd:
		return nil, fmt.Errorf("节点类型 %s 不支持单独试跑 (依赖上游输入或路由结构)", node.Type)
	}

	// 最小图: 目标节点 + 它委派的子Agent (委派边不改主流拓扑)
	mini := &OrchestrationDSL{Version: 1, Params: dsl.Params}
	mini.Nodes = append(mini.Nodes, *node)
	for i := range dsl.Nodes {
		n := dsl.Nodes[i]
		if n.Type == OrchNodeSubAgent {
			mini.Nodes = append(mini.Nodes, n)
		}
	}
	for _, e := range dsl.Edges {
		if e.Source == nodeID {
			if tn := cNodeByID(dsl, e.Target); tn != nil && tn.Type == OrchNodeSubAgent {
				mini.Edges = append(mini.Edges, e)
			}
		}
	}

	handler := newOrchTraceHandler(orchNodeKeysOf(mini), orchSubAgentKeysOf(mini), nil)
	// orchID=0: 节点提示词版本不解析, 用画布内联提示词原样试跑
	compiled, err := s.compile(ctx, userID, mini, handler, nil, "", 0, nil, false, nil, params)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	stream, runErr := compiled.runnable.Stream(runCtx, schema.UserMessage(input))
	if runErr != nil {
		return nil, runErr
	}
	defer stream.Close()
	var full strings.Builder
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		full.WriteString(chunk.Content)
	}
	return &DebugRunResult{
		Mode:    "node",
		Output:  full.String(),
		Nodes:   handler.NodeTraces(),
		TotalMS: time.Since(started).Milliseconds(),
	}, nil
}

func cNodeByID(dsl *OrchestrationDSL, id string) *OrchestrationNode {
	for i := range dsl.Nodes {
		if dsl.Nodes[i].ID == id {
			return &dsl.Nodes[i]
		}
	}
	return nil
}

// ---------- 自定义模板 ----------

// ListMyTemplates 当前用户保存的编排模板 (新→旧)
func (s *AIOrchestrationService) ListMyTemplates(userID uint) ([]model.OrchTemplate, error) {
	var rows []model.OrchTemplate
	if err := DB.Where("user_id = ?", userID).Order("id DESC").Limit(50).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// SaveMyTemplate 把画布定义存为用户模板 (入库前过一遍校验, 不合法不入库)
func (s *AIOrchestrationService) SaveMyTemplate(userID uint, name, description, definition string) (*model.OrchTemplate, error) {
	definition = strings.TrimSpace(definition)
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("模板名称不能为空")
	}
	if _, issues := validateOrchestrationDSLDetailed(definition); len(issues) > 0 {
		// 子编排占位/工具占位属预期, 只拦结构性错误 (解析失败)
		return nil, fmt.Errorf("画布定义校验失败: %s", strings.Join(orchIssueMessages(issues), "; "))
	}
	row := &model.OrchTemplate{
		UserID:      userID,
		Name:        strings.TrimSpace(name),
		Description: strings.TrimSpace(description),
		Definition:  definition,
	}
	if err := DB.Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

// DeleteMyTemplate 删除自己的模板 (归属校验防越权)
func (s *AIOrchestrationService) DeleteMyTemplate(userID, id uint) error {
	var row model.OrchTemplate
	if err := DB.First(&row, id).Error; err != nil {
		return err
	}
	if row.UserID != userID {
		return fmt.Errorf("无权删除该模板")
	}
	return DB.Delete(&model.OrchTemplate{}, id).Error
}
