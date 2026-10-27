-- +goose Up
-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('lottery:menu:view', '查看抽奖菜单', '抽奖管理'),
('lottery:activity:view', '查看抽奖活动', '抽奖管理'),
('lottery:activity:create', '创建抽奖活动', '抽奖管理'),
('lottery:activity:update', '更新抽奖活动', '抽奖管理'),
('lottery:activity:delete', '删除抽奖活动', '抽奖管理'),
('lottery:prize:view', '查看奖品', '抽奖管理'),
('lottery:prize:create', '创建奖品', '抽奖管理'),
('lottery:prize:update', '更新奖品', '抽奖管理'),
('lottery:prize:delete', '删除奖品', '抽奖管理'),
('lottery:draw:execute', '执行抽奖', '抽奖管理'),
('lottery:record:view', '查看抽奖记录', '抽奖管理'),
('lottery:record:delete', '删除抽奖记录', '抽奖管理'),
('lottery:winner:view', '查看中奖名单', '抽奖管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` IN (
    'lottery:menu:view',
    'lottery:activity:view',
    'lottery:activity:create',
    'lottery:activity:update',
    'lottery:activity:delete',
    'lottery:prize:view',
    'lottery:prize:create',
    'lottery:prize:update',
    'lottery:prize:delete',
    'lottery:draw:execute',
    'lottery:record:view',
    'lottery:record:delete',
    'lottery:winner:view'
);
-- +goose StatementEnd
