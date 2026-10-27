-- +goose Up
-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('cut:menu:view', '查看切割工具菜单', '切割工具'),
('cut:bar:compute', '一维切割计算', '切割工具'),
('cut:plane:compute', '二维切割计算', '切割工具'),
('cut:record:view', '查看切割记录', '切割工具'),
('cut:record:create', '创建切割记录', '切割工具'),
('cut:record:delete', '删除切割记录', '切割工具')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` IN ('cut:menu:view', 'cut:bar:compute', 'cut:plane:compute', 'cut:record:view', 'cut:record:create', 'cut:record:delete');
-- +goose StatementEnd
