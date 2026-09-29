-- +goose Up

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS stock_alert_rules (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL COMMENT '所属用户',
    code VARCHAR(16) NOT NULL COMMENT '完整证券代码 sh.XXXXXX/sz.XXXXXX/bj.XXXXXX',
    name VARCHAR(100) NOT NULL DEFAULT '' COMMENT '规则备注名',
    rule_type VARCHAR(32) NOT NULL COMMENT 'price_above/price_below/change_pct_above/change_pct_below',
    threshold DECIMAL(16,4) NOT NULL COMMENT '触发阈值 (价格元 / 涨跌幅百分比)',
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    last_triggered_at DATETIME NULL COMMENT '最近触发时间 (当日已触发不再重复推送)',
    last_triggered_value DECIMAL(16,4) NULL COMMENT '最近触发时的观测值',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    KEY idx_alert_user (user_id),
    KEY idx_alert_enabled (enabled),
    KEY idx_alert_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='自选股预警规则 (按最新收盘价评估)';
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS stock_alert_rules;
-- +goose StatementEnd
