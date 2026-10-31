-- +goose Up

-- 多角色改造: 用户可拥有多个角色。
-- user_roles 是角色归属的唯一真相源; users.role 列保留为"主角色"兼容字段
-- (始终与 user_roles 的第一项同步, 存量旧代码/审计场景仍可读)。

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `user_roles` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `user_id` INT NOT NULL,
    `role_code` VARCHAR(50) NOT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY `uk_user_role` (`user_id`, `role_code`),
    KEY `idx_user_roles_role` (`role_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose StatementBegin
-- 存量回填: 每个用户的单一角色迁入 user_roles
INSERT INTO `user_roles` (`user_id`, `role_code`)
SELECT `id`, `role` FROM `users`
WHERE `role` IS NOT NULL AND `role` <> ''
ON DUPLICATE KEY UPDATE `user_id` = `user_id`;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS `user_roles`;
-- +goose StatementEnd
