-- ClickHouse 行情/财务复制表 (MySQL -> ClickHouse 下游只读副本)
-- ReplacingMergeTree(updated_at): updated_at 作为版本列, 同主键(ORDER BY)重复写入保留最新版本, 幂等
-- 日期列显式指定 Asia/Shanghai, 与 MySQL 侧 DSN loc=Local 对齐(小时线 15:00 不被时区平移)
-- 注意: 语句以分号分隔; 表结构变更请新增迁移文件, 不要改历史文件

CREATE TABLE IF NOT EXISTS stock_daily (
	code LowCardinality(String),
	frequency LowCardinality(String),
	trade_date DateTime('Asia/Shanghai'),
	open Nullable(Float64),
	high Nullable(Float64),
	low Nullable(Float64),
	close Nullable(Float64),
	preclose Nullable(Float64),
	volume Nullable(Float64),
	amount Nullable(Float64),
	turnover_rate Nullable(Float64),
	change_pct Nullable(Float64),
	trade_status Nullable(Int8),
	pe_ttm Nullable(Float64),
	pb_mrq Nullable(Float64),
	ps_ttm Nullable(Float64),
	pcf_ncf_ttm Nullable(Float64),
	amplitude Nullable(Float64),
	updated_at DateTime
) ENGINE = ReplacingMergeTree(updated_at)
PARTITION BY toYYYYMM(trade_date)
ORDER BY (code, frequency, trade_date);

CREATE TABLE IF NOT EXISTS stock_finance (
	code LowCardinality(String),
	report_date DateTime('Asia/Shanghai'),
	report_type String,
	pe_ttm Nullable(Float64),
	pb Nullable(Float64),
	ps_ttm Nullable(Float64),
	roe Nullable(Float64),
	roa Nullable(Float64),
	gross_margin Nullable(Float64),
	net_margin Nullable(Float64),
	revenue Nullable(Float64),
	revenue_yoy Nullable(Float64),
	net_profit Nullable(Float64),
	net_profit_yoy Nullable(Float64),
	debt_ratio Nullable(Float64),
	current_ratio Nullable(Float64),
	quick_ratio Nullable(Float64),
	cash_ratio Nullable(Float64),
	nr_turn_ratio Nullable(Float64),
	inv_turn_ratio Nullable(Float64),
	ca_turn_ratio Nullable(Float64),
	asset_turn_ratio Nullable(Float64),
	yoy_equity Nullable(Float64),
	yoy_asset Nullable(Float64),
	yoy_eps Nullable(Float64),
	cfo_to_or Nullable(Float64),
	cfo_to_np Nullable(Float64),
	eps Nullable(Float64),
	eps_deducted Nullable(Float64),
	bps Nullable(Float64),
	ocf_per_share Nullable(Float64),
	finance_sources String,
	updated_at DateTime
) ENGINE = ReplacingMergeTree(updated_at)
PARTITION BY toYYYYMM(report_date)
ORDER BY (code, report_date);