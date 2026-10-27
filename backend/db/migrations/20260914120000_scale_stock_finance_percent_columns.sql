-- +goose Up

-- roe/gross_margin/net_margin/debt_ratio 原存 baostock 原始小数(0.047=4.7%),
-- 统一为百分比口径(×100, 与营收/净利润同比等列一致, 前端按百分比直接渲染);
-- ABS < 5 守卫防止误重复执行(原始小数不可能达到 5, 已放大的百分比不会二次放大)
-- +goose StatementBegin
UPDATE `stock_finance` SET `roe` = `roe` * 100 WHERE `roe` IS NOT NULL AND ABS(`roe`) < 5;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `stock_finance` SET `gross_margin` = `gross_margin` * 100 WHERE `gross_margin` IS NOT NULL AND ABS(`gross_margin`) < 5;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `stock_finance` SET `net_margin` = `net_margin` * 100 WHERE `net_margin` IS NOT NULL AND ABS(`net_margin`) < 5;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `stock_finance` SET `debt_ratio` = `debt_ratio` * 100 WHERE `debt_ratio` IS NOT NULL AND ABS(`debt_ratio`) < 5;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
UPDATE `stock_finance` SET `roe` = `roe` / 100 WHERE `roe` IS NOT NULL AND ABS(`roe`) >= 5;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `stock_finance` SET `gross_margin` = `gross_margin` / 100 WHERE `gross_margin` IS NOT NULL AND ABS(`gross_margin`) >= 5;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `stock_finance` SET `net_margin` = `net_margin` / 100 WHERE `net_margin` IS NOT NULL AND ABS(`net_margin`) >= 5;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `stock_finance` SET `debt_ratio` = `debt_ratio` / 100 WHERE `debt_ratio` IS NOT NULL AND ABS(`debt_ratio`) >= 5;
-- +goose StatementEnd
