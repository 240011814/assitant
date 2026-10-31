-- +goose Up

-- 多角色改造收尾: 移除 users.role 单角色残留列。
-- 角色归属的唯一真相源是 user_roles (20261031200000 已回填存量),
-- 代码层所有读取方均已切换到 user_roles, 本列不再有消费方。

-- +goose StatementBegin
ALTER TABLE `users` DROP COLUMN `role`;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
-- 无法从 user_roles 无损恢复单一列的原始值 (多角色信息有损), 回滚时按第一项近似还原
ALTER TABLE `users` ADD COLUMN `role` VARCHAR(20) NOT NULL DEFAULT 'R_USER';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `users` SET `role` = COALESCE((
    SELECT `ur`.`role_code` FROM `user_roles` `ur`
    WHERE `ur`.`user_id` = `users`.`id`
    ORDER BY `ur`.`role_code` ASC LIMIT 1
), 'R_USER');
-- +goose StatementEnd
