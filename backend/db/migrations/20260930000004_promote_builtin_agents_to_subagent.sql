-- +goose Up
-- 把 4 个内置公共 Agent 全部升级为"子Agent", 并补齐委派说明 (delegation_description)。
-- 升级后它们会出现在 Agent Studio 编排的「引用 Agent」下拉里, 供主 Agent 判断何时委派。
-- 委派说明会写进主 Agent 的委派指引 (见 orchDelegationGuide / orchSubAgentDesc)。

-- +goose StatementBegin
UPDATE `ai_agents`
SET `agent_type` = 'subagent',
    `delegation_description` = '用户需要用英语口语练习时委派: 模拟点餐/开会/旅行等真实生活场景, 出中文句子让用户翻译并纠错点评, 提供地道表达与词汇笔记。'
WHERE `code` = 'chat';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `ai_agents`
SET `agent_type` = 'subagent',
    `delegation_description` = '用户面临选择、纠结、难以取舍时委派: 用 60+ 决策模型厘清价值观/风险/机会成本/可逆性, 给出可执行的下一步与止损条件。'
WHERE `code` = 'decision';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `ai_agents`
SET `agent_type` = 'subagent',
    `delegation_description` = '用户需要练习沟通话术时委派: 破冰、安慰、拒绝、道歉、化解冲突等 40+ 社交场景的角色扮演与回应点评。'
WHERE `code` = 'social';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `ai_agents`
SET `agent_type` = 'subagent',
    `delegation_description` = '用户想锻炼临场反应时委派: 被追问隐私、被查岗、被突然质疑等突发尴尬场景的应对策略与话术演练。'
WHERE `code` = 'emergency';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE `ai_agents`
SET `agent_type` = 'chat', `delegation_description` = ''
WHERE `code` IN ('chat', 'decision', 'social', 'emergency');
-- +goose StatementEnd