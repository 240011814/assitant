-- +goose Up

-- +goose StatementBegin
-- 待切割窗户单: 一单可含多种类型的多樘窗, spec 存 JSON (窗型key/尺寸/分格/材料类型/樘数), 供窗户管理页维护与一键去裁剪
CREATE TABLE IF NOT EXISTS `cut_window` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned NOT NULL COMMENT '所属用户',
  `name` varchar(100) NOT NULL DEFAULT '' COMMENT '单据名称',
  `spec` text COMMENT '窗户单内容 JSON: {"windows":[{type,width,height,frameWidth,materialType,count,grid:{cols,rows,cells,sliding}}]}',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_window_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='待切割窗户单';
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS `cut_window`;
-- +goose StatementEnd
