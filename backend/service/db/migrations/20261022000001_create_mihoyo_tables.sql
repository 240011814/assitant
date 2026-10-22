-- +goose Up

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `mihoyo_credentials` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `user_id` int unsigned NOT NULL COMMENT '所属用户',
  `mihoyo_uid` varchar(32) DEFAULT '' COMMENT '米游社 aid',
  `mihoyo_mid` varchar(64) DEFAULT '' COMMENT '米游社 mid',
  `nickname` varchar(64) DEFAULT '' COMMENT '米游社昵称',
  `cookie_token` text COMMENT 'cookie_token (账号登录态, 仅内部使用)',
  `ltoken` text COMMENT 'ltoken (账号登录态, 仅内部使用)',
  `stoken` text COMMENT 'stoken v2, 用于 cookie_token/ltoken 过期自动续期',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_mihoyo_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='米哈游登录凭据 (扫码登录绑定, 每用户一条)';
-- +goose StatementEnd

-- +goose StatementBegin
-- 默认创建原神每日签到系统任务: 北京时间 8:23 (镜像 TZ=Asia/Shanghai)
INSERT IGNORE INTO `job_definitions` (`name`, `task_name`, `cron_expr`, `params`, `enabled`, `max_retries`, `remark`)
VALUES ('mihoyo-genshin-checkin', 'mihoyo.genshin_checkin', '23 8 * * *', '{}', 1, 1, '原神每日签到 (已扫码绑定的账号, 自动创建)');
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DELETE FROM `job_definitions` WHERE `name` = 'mihoyo-genshin-checkin' AND `task_name` = 'mihoyo.genshin_checkin';
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS `mihoyo_credentials`;
-- +goose StatementEnd
