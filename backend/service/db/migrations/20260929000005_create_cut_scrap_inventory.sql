-- +goose Up

-- +goose StatementBegin
-- 余料库存: 切割后剩余材料登记, 下次计算可从库存带入复用
CREATE TABLE IF NOT EXISTS `cut_scrap_inventory` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned NOT NULL COMMENT '所属用户',
  `scrap_type` tinyint NOT NULL DEFAULT 1 COMMENT '1=一维余料(长度) 2=二维余料(板材)',
  `label` varchar(100) NOT NULL DEFAULT '' COMMENT '余料名称/来源材料规格 (如: 长料余料)',
  `length_value` decimal(12,2) DEFAULT NULL COMMENT '一维: 长度',
  `width_value` decimal(12,2) DEFAULT NULL COMMENT '二维: 宽',
  `height_value` decimal(12,2) DEFAULT NULL COMMENT '二维: 高',
  `quantity` int NOT NULL DEFAULT 1,
  `note` varchar(200) NOT NULL DEFAULT '',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_scrap_user` (`user_id`, `scrap_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='切割余料库存';
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS `cut_scrap_inventory`;
-- +goose StatementEnd
