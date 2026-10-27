-- +goose Up

-- 旧部署(9/12~9/14上午)同步的营运/现金流数据与 baostock 接口实际返回不符
-- (跨股票/季度错乱, 实测 600071/600072 互相串写, 如 2026Q2 的周转率被写到 2007Q1 行),
-- 清空 O(周转率)/C(现金流) 六列并从 finance_sources 剔除 O/C 标记, 让下轮同步重新拉取;
-- P/G/B 数据已逐一与接口比对一致, 不动
-- +goose StatementBegin
UPDATE `stock_finance`
SET `nr_turn_ratio` = NULL, `inv_turn_ratio` = NULL, `ca_turn_ratio` = NULL, `asset_turn_ratio` = NULL,
    `cfo_to_or` = NULL, `cfo_to_np` = NULL
WHERE `nr_turn_ratio` IS NOT NULL OR `inv_turn_ratio` IS NOT NULL OR `ca_turn_ratio` IS NOT NULL
   OR `asset_turn_ratio` IS NOT NULL OR `cfo_to_or` IS NOT NULL OR `cfo_to_np` IS NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `stock_finance`
SET `finance_sources` = REPLACE(REPLACE(`finance_sources`, 'O', ''), 'C', '')
WHERE `finance_sources` LIKE '%O%' OR `finance_sources` LIKE '%C%';
-- +goose StatementEnd

-- +goose Down

-- 清空的错乱值与被剔除的标记无法还原, Down 保持空操作
-- +goose StatementBegin
SELECT 1;
-- +goose StatementEnd
