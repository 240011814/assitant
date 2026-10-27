-- +goose Up
-- system_config.value 原为 VARCHAR(500), 内置 AI 提示词配置化后 (prompt_* 键) 需要存数千字符的提示词, 扩成 TEXT
ALTER TABLE `system_config` MODIFY COLUMN `value` TEXT NOT NULL;

-- +goose Down
-- MySQL 无法直接把 TEXT 缩回 VARCHAR, 先截断超长值再改列
UPDATE `system_config` SET `value` = LEFT(`value`, 500) WHERE CHAR_LENGTH(`value`) > 500;
ALTER TABLE `system_config` MODIFY COLUMN `value` VARCHAR(500) NOT NULL DEFAULT '';
