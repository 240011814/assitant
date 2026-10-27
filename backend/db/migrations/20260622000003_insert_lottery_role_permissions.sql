-- +goose Up
-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_SUPER', 'lottery:menu:view'),
('R_SUPER', 'lottery:activity:view'),
('R_SUPER', 'lottery:activity:create'),
('R_SUPER', 'lottery:activity:update'),
('R_SUPER', 'lottery:activity:delete'),
('R_SUPER', 'lottery:prize:view'),
('R_SUPER', 'lottery:prize:create'),
('R_SUPER', 'lottery:prize:update'),
('R_SUPER', 'lottery:prize:delete'),
('R_SUPER', 'lottery:draw:execute'),
('R_SUPER', 'lottery:record:view'),
('R_SUPER', 'lottery:record:delete'),
('R_SUPER', 'lottery:winner:view'),
('R_ADMIN', 'lottery:menu:view'),
('R_ADMIN', 'lottery:activity:view'),
('R_ADMIN', 'lottery:activity:create'),
('R_ADMIN', 'lottery:activity:update'),
('R_ADMIN', 'lottery:activity:delete'),
('R_ADMIN', 'lottery:prize:view'),
('R_ADMIN', 'lottery:prize:create'),
('R_ADMIN', 'lottery:prize:update'),
('R_ADMIN', 'lottery:prize:delete'),
('R_ADMIN', 'lottery:draw:execute'),
('R_ADMIN', 'lottery:record:view'),
('R_ADMIN', 'lottery:record:delete'),
('R_ADMIN', 'lottery:winner:view')
ON DUPLICATE KEY UPDATE `role_code` = VALUES(`role_code`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` IN (
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
