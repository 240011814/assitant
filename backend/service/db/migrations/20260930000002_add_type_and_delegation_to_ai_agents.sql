-- +goose Up
-- 为 ai_agents 增加两个与 Agent Studio 编排相关的公共字段:
--   agent_type              Agent 类型, 决定它能出现在哪里 (chat=普通对话, subagent=可作为编排子Agent 被委派)
--   delegation_description  委派说明, 主 Agent 判断"何时该委派给它"的依据 (编译期写进主 Agent 提示词的委派指引)

-- +goose StatementBegin
ALTER TABLE `ai_agents`
  ADD COLUMN `agent_type` VARCHAR(20) NOT NULL DEFAULT 'chat'
    COMMENT 'Agent 类型: chat=对话 / subagent=可作为编排子Agent 被委派';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE `ai_agents`
  ADD COLUMN `delegation_description` VARCHAR(500) NOT NULL DEFAULT ''
    COMMENT '委派说明: 供主 Agent 判断何时委派 (仅 subagent 类型有意义)';
-- +goose StatementEnd

-- 存量数据: 全部保持默认 'chat', 不改变现有行为 (编排页的子Agent 下拉只列出
-- agent_type='subagent' 或 delegation_description 非空的 Agent, 见 AIOrchestrationService.Resources)。
-- 把某个 Agent 开放给编排, 前端"AI 训练"页编辑里选"子Agent"类型并填委派说明即可;
-- 若要批量处理, 自行执行 (示例, 按标识码挑选):
--   UPDATE `ai_agents` SET `agent_type` = 'subagent' WHERE `code` IN ('decision', 'social');

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `ai_agents` DROP COLUMN `delegation_description`;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE `ai_agents` DROP COLUMN `agent_type`;
-- +goose StatementEnd
