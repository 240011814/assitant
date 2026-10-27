-- +goose Up

CREATE TABLE IF NOT EXISTS `sync_watermark` (
  `name` varchar(50) NOT NULL COMMENT '水位名称: kline_daily=日K按日全量同步',
  `last_date` date DEFAULT NULL COMMENT '已完整同步到的日期',
  `status` varchar(20) NOT NULL DEFAULT 'pending' COMMENT '同步状态 pending/ok/failed',
  `error` varchar(255) DEFAULT NULL COMMENT '失败原因',
  `synced_at` datetime DEFAULT NULL COMMENT '最后同步时间',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='批量同步水位断点(数据表是真相, 水位只推进到完整成功的一天)';

-- +goose Down

DROP TABLE IF EXISTS `sync_watermark`;
