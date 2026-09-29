# baostock-api

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
![Python 3.12+](https://img.shields.io/badge/Python-3.12%2B-3776AB?logo=python&logoColor=white)
![Docker Ready](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white)
![Version 0.1.0](https://img.shields.io/badge/version-1.0.0-2ea44f)

`baostock` HTTP 包装服务 (FastAPI + uvicorn)

## 已实现

- `GET /health`
- `GET /usage`
- `GET /query_all_stock?day=YYYY-MM-DD`
- `GET /query_adjust_factor`
- `GET /query_daily_adjust_factor?date=YYYY-MM-DD`
- `GET /query_daily_history_k_astock?date=YYYY-MM-DD`
- `GET /query_daily_history_k_etf?date=YYYY-MM-DD`
- `GET /query_balance_data`
- `GET /query_cash_flow_data`
- `GET /query_deposit_rate_data`
- `GET /query_dividend_data`
- `GET /query_dupont_data`
- `GET /query_forecast_report`
- `GET /query_growth_data`
- `GET /query_hs300_stocks`
- `GET /query_loan_rate_data`
- `GET /query_money_supply_data_month`
- `GET /query_money_supply_data_year`
- `GET /query_operation_data`
- `GET /query_performance_express_report`
- `GET /query_profit_data`
- `GET /query_required_reserve_ratio_data`
- `GET /query_history_k_data_plus`
- `GET /query_history_index_k_data_plus`
- `GET /query_stock_basic`
- `GET /query_stock_industry`
- `GET /query_sz50_stocks`
- `GET /query_trade_dates`
- `GET /query_zz500_stocks`
- `GET /query_lhb_detail?symbol=&start_date=&end_date=`
- `GET /query_fund_flow?stock=&market=`
- `GET /query_north_flow?symbol=`
- `GET /query_gdp`
- `GET /query_cpi`
- `GET /query_pmi`
- `GET /query_ppi`
- `GET /query_lpr`

以上 `query_lhb_detail`、`query_fund_flow`、`query_north_flow`、`query_gdp`、`query_cpi`、`query_pmi`、`query_ppi`、`query_lpr` 共 8 个接口基于 akshare 实现，不经过 baostock，不受每日限额与串行限制。

## 特性

- 本地调用计数持久化到 `data/usage.json`
- `usage.json` 不纳入 git；服务启动时如果文件不存在，会自动生成默认内容
- 每日自动重置
- 单日默认限制 `45000` 次

## 目录约定

- `main.py` 只负责 FastAPI 应用、路由注册、路由分发和通用错误处理
- `baostock_api/*_endpoint.py` 每个文件对应一个接口，文件内放该接口的参数类型、业务类型和执行逻辑
- `baostock_api/shared.py` 放公共运行时能力，例如 `baostock` 连接、usage 计数和通用错误响应结构
- `baostock_api/request.py` 放通用请求参数读取函数

## 安装

```bash
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt -i https://pypi.org/simple
```

## 本地运行

```bash
source .venv/bin/activate
python main.py
```

默认地址：

```text
http://127.0.0.1:3002
```

## 环境变量

```bash
BAOSTOCK_API_HOST=127.0.0.1
BAOSTOCK_API_PORT=3002
BAOSTOCK_API_DAILY_LIMIT=45000
```

## Docker 构建与运行

在当前目录构建镜像：

```bash
docker build -t baostock-api .
```

运行容器：

```bash
docker run --rm -p 3002:3002 \
  -e BAOSTOCK_API_HOST=0.0.0.0 \
  -e BAOSTOCK_API_PORT=3002 \
  -e BAOSTOCK_API_DAILY_LIMIT=45000 \
  -v "$(pwd)/data:/app/data" \
  baostock-api
```

说明：

- 容器内服务监听 `0.0.0.0`，这样宿主机才能通过映射端口访问
- `data` 目录挂载到容器内 `/app/data`，这样 `usage.json` 可以持久化

## 示例

```bash
curl "http://127.0.0.1:3002/query_all_stock?day=2024-10-25"
```

```bash
curl "http://127.0.0.1:3002/query_adjust_factor?code=sh.600000&start_date=2015-01-01&end_date=2017-12-31"
```

```bash
curl "http://127.0.0.1:3002/query_balance_data?code=sh.600000&year=2017&quarter=2"
```

```bash
curl "http://127.0.0.1:3002/query_cash_flow_data?code=sh.600000&year=2017&quarter=2"
```

```bash
curl "http://127.0.0.1:3002/query_deposit_rate_data?start_date=2015-01-01&end_date=2015-12-31"
```

```bash
curl "http://127.0.0.1:3002/query_dividend_data?code=sh.600000&year=2017&year_type=report"
```

```bash
curl "http://127.0.0.1:3002/query_dupont_data?code=sh.600000&year=2017&quarter=2"
```

```bash
curl "http://127.0.0.1:3002/query_forecast_report?code=sh.600000&start_date=2010-01-01&end_date=2017-12-31"
```

```bash
curl "http://127.0.0.1:3002/query_growth_data?code=sh.600000&year=2017&quarter=2"
```

```bash
curl "http://127.0.0.1:3002/query_hs300_stocks"
```

```bash
curl "http://127.0.0.1:3002/query_hs300_stocks?date=2018-11-26"
```

```bash
curl "http://127.0.0.1:3002/query_loan_rate_data?start_date=2010-01-01&end_date=2015-12-31"
```

```bash
curl "http://127.0.0.1:3002/query_money_supply_data_month?start_date=2010-01&end_date=2015-12"
```

```bash
curl "http://127.0.0.1:3002/query_money_supply_data_year?start_date=2010&end_date=2015"
```

```bash
curl "http://127.0.0.1:3002/query_operation_data?code=sh.600000&year=2017&quarter=2"
```

```bash
curl "http://127.0.0.1:3002/query_performance_express_report?code=sh.600000&start_date=2015-01-01&end_date=2017-12-31"
```

```bash
curl "http://127.0.0.1:3002/query_profit_data?code=sh.600000&year=2017&quarter=2"
```

```bash
curl "http://127.0.0.1:3002/query_required_reserve_ratio_data?start_date=2010-01-01&end_date=2015-12-31&year_type=0"
```

```bash
curl "http://127.0.0.1:3002/query_history_k_data_plus?code=sh.600000&fields=date,code,open,high,low,close,preclose,volume,amount,adjustflag,turn,tradestatus,pctChg,isST&start_date=2024-07-01&end_date=2024-12-31&frequency=d&adjustflag=3"
```

```bash
curl "http://127.0.0.1:3002/query_history_index_k_data_plus?code=sh.000001&fields=date,code,open,high,low,close,preclose,volume,amount,pctChg&start_date=2017-01-01&end_date=2017-06-30&frequency=d"
```

```bash
curl "http://127.0.0.1:3002/query_stock_basic?code=sh.600000"
```

```bash
curl "http://127.0.0.1:3002/query_stock_basic?code_name=%E6%B5%A6%E5%8F%91%E9%93%B6%E8%A1%8C"
```

```bash
curl "http://127.0.0.1:3002/query_stock_industry"
```

```bash
curl "http://127.0.0.1:3002/query_stock_industry?code=sh.600000&date=2018-11-26"
```

```bash
curl "http://127.0.0.1:3002/query_sz50_stocks"
```

```bash
curl "http://127.0.0.1:3002/query_sz50_stocks?date=2018-11-26"
```

```bash
curl "http://127.0.0.1:3002/query_trade_dates?start_date=2017-01-01&end_date=2017-06-30"
```

```bash
curl "http://127.0.0.1:3002/query_daily_history_k_astock?date=2026-09-10"
```

```bash
curl "http://127.0.0.1:3002/query_daily_adjust_factor?date=2026-09-10"
```

```bash
curl "http://127.0.0.1:3002/query_zz500_stocks"
```

## 每日更新接口 (需 baostock ≥ 00.9.30)

- `query_daily_history_k_astock`：指定交易日全市场个股日K，1 次调用返回全部个股；返回不含指数与北交所
- `query_daily_history_k_etf`：指定交易日全部 ETF 日K
- `query_daily_adjust_factor`：指定交易日除权股票的复权因子
- 部署/升级后运行 `python verify_daily_updates.py [date]` 验证版本与数据

```bash
curl "http://127.0.0.1:3002/query_zz500_stocks?date=2018-11-26"
```

## License

MIT. See [LICENSE](LICENSE).

## `query_adjust_factor` 参数

- `code`：必填，证券代码，例如 `sh.600000`
- `start_date`：可选，开始日期，格式 `YYYY-MM-DD`，为空时默认为 `2015-01-01`
- `end_date`：可选，结束日期，格式 `YYYY-MM-DD`，为空时默认为当前日期

## 复权因子字段

- `code`
- `dividOperateDate`
- `foreAdjustFactor`
- `backAdjustFactor`
- `adjustFactor`

## 复权因子字段说明

- `code`：证券代码
- `dividOperateDate`：除权除息日期
- `foreAdjustFactor`：向前复权因子
- `backAdjustFactor`：向后复权因子
- `adjustFactor`：本次复权因子

## `query_dividend_data` 参数

- `code`：必填，证券代码，例如 `sh.600000`
- `year`：必填，年份，例如 `2017`
- `year_type`：必填，年份类别，`report` 表示预案公告年份，`operate` 表示除权除息年份

## `query_profit_data` 参数

- `code`：必填，证券代码，例如 `sh.600000`
- `year`：可选，统计年份，默认为当前年
- `quarter`：可选，统计季度，默认为当前季度，可选值为 `1`、`2`、`3`、`4`

## `query_dupont_data` 参数

- `code`：必填，证券代码，例如 `sh.600000`
- `year`：可选，统计年份，默认为当前年
- `quarter`：可选，统计季度，默认为当前季度，可选值为 `1`、`2`、`3`、`4`

## `query_performance_express_report` 参数

- `code`：必填，证券代码，例如 `sh.600000`
- `start_date`：可选，开始日期，发布日期或更新日期在这个范围内
- `end_date`：可选，结束日期，发布日期或更新日期在这个范围内

## `query_forecast_report` 参数

- `code`：必填，证券代码，例如 `sh.600000`
- `start_date`：可选，开始日期，发布日期或更新日期在这个范围内
- `end_date`：可选，结束日期，发布日期或更新日期在这个范围内

## `query_operation_data` 参数

- `code`：必填，证券代码，例如 `sh.600000`
- `year`：可选，统计年份，默认为当前年
- `quarter`：可选，统计季度，默认为当前季度，可选值为 `1`、`2`、`3`、`4`

## `query_growth_data` 参数

- `code`：必填，证券代码，例如 `sh.600000`
- `year`：可选，统计年份，默认为当前年
- `quarter`：可选，统计季度，默认为当前季度，可选值为 `1`、`2`、`3`、`4`

## `query_balance_data` 参数

- `code`：必填，证券代码，例如 `sh.600000`
- `year`：可选，统计年份，默认为当前年
- `quarter`：可选，统计季度，默认为当前季度，可选值为 `1`、`2`、`3`、`4`

## `query_cash_flow_data` 参数

- `code`：必填，证券代码，例如 `sh.600000`
- `year`：可选，统计年份，默认为当前年
- `quarter`：可选，统计季度，默认为当前季度，可选值为 `1`、`2`、`3`、`4`

## `query_stock_basic` 参数

- `code`：可选，A 股股票代码或指数代码，例如 `sh.600000`
- `code_name`：可选，证券名称，支持模糊查询
- 当 `code` 和 `code_name` 都为空时，返回全部证券基本资料

## `query_deposit_rate_data` 参数

- `start_date`：可选，开始日期，格式 `YYYY-MM-DD`
- `end_date`：可选，结束日期，格式 `YYYY-MM-DD`

## `query_loan_rate_data` 参数

- `start_date`：可选，开始日期，格式 `YYYY-MM-DD`
- `end_date`：可选，结束日期，格式 `YYYY-MM-DD`

## `query_required_reserve_ratio_data` 参数

- `start_date`：可选，开始日期，格式 `YYYY-MM-DD`
- `end_date`：可选，结束日期，格式 `YYYY-MM-DD`
- `year_type`：可选，年份类别；`0` 表示查询公告日期，`1` 表示查询生效日期

## `query_money_supply_data_month` 参数

- `start_date`：可选，开始日期，格式 `YYYY-MM`
- `end_date`：可选，结束日期，格式 `YYYY-MM`

## `query_money_supply_data_year` 参数

- `start_date`：可选，开始日期，格式 `YYYY`
- `end_date`：可选，结束日期，格式 `YYYY`

## `query_stock_industry` 参数

- `code`：可选，A 股股票代码或指数代码，例如 `sh.600000`
- `date`：可选，查询日期，格式 `YYYY-MM-DD`，为空时默认最新日期

## `query_sz50_stocks` 参数

- `date`：可选，查询日期，格式 `YYYY-MM-DD`，为空时默认最新日期

## `query_hs300_stocks` 参数

- `date`：可选，查询日期，格式 `YYYY-MM-DD`，为空时默认最新日期

## `query_zz500_stocks` 参数

- `date`：可选，查询日期，格式 `YYYY-MM-DD`，为空时默认最新日期

## `query_trade_dates` 参数

- `start_date`：可选，开始日期，格式 `YYYY-MM-DD`
- `end_date`：可选，结束日期，格式 `YYYY-MM-DD`

## 偿债能力字段

- `code`
- `pubDate`
- `statDate`
- `currentRatio`
- `quickRatio`
- `cashRatio`
- `YOYLiability`
- `liabilityToAsset`
- `assetToEquity`

## 偿债能力字段说明

- `code`：证券代码
- `pubDate`：公司发布财报的日期
- `statDate`：财报统计季度的最后一天
- `currentRatio`：流动比率
- `quickRatio`：速动比率
- `cashRatio`：现金比率
- `YOYLiability`：总负债同比增长率
- `liabilityToAsset`：资产负债率
- `assetToEquity`：权益乘数

## 现金流量字段

- `code`
- `pubDate`
- `statDate`
- `CAToAsset`
- `NCAToAsset`
- `tangibleAssetToAsset`
- `ebitToInterest`
- `CFOToOR`
- `CFOToNP`
- `CFOToGr`

## 现金流量字段说明

- `code`：证券代码
- `pubDate`：公司发布财报的日期
- `statDate`：财报统计季度的最后一天
- `CAToAsset`：流动资产除以总资产
- `NCAToAsset`：非流动资产除以总资产
- `tangibleAssetToAsset`：有形资产除以总资产
- `ebitToInterest`：已获利息倍数
- `CFOToOR`：经营活动产生的现金流量净额除以营业收入
- `CFOToNP`：经营性现金净流量除以净利润
- `CFOToGr`：经营性现金净流量除以营业总收入

## 成长能力字段

- `code`
- `pubDate`
- `statDate`
- `YOYEquity`
- `YOYAsset`
- `YOYNI`
- `YOYEPSBasic`
- `YOYPNI`

## 成长能力字段说明

- `code`：证券代码
- `pubDate`：公司发布财报的日期
- `statDate`：财报统计季度的最后一天
- `YOYEquity`：净资产同比增长率
- `YOYAsset`：总资产同比增长率
- `YOYNI`：净利润同比增长率
- `YOYEPSBasic`：基本每股收益同比增长率
- `YOYPNI`：归属母公司股东净利润同比增长率

## 营运能力字段

- `code`
- `pubDate`
- `statDate`
- `NRTurnRatio`
- `NRTurnDays`
- `INVTurnRatio`
- `INVTurnDays`
- `CATurnRatio`
- `AssetTurnRatio`

## 营运能力字段说明

- `code`：证券代码
- `pubDate`：公司发布财报的日期
- `statDate`：财报统计季度的最后一天
- `NRTurnRatio`：应收账款周转率(次)
- `NRTurnDays`：应收账款周转天数(天)
- `INVTurnRatio`：存货周转率(次)
- `INVTurnDays`：存货周转天数(天)
- `CATurnRatio`：流动资产周转率(次)
- `AssetTurnRatio`：总资产周转率

## 盈利能力字段

- `code`
- `pubDate`
- `statDate`
- `roeAvg`
- `npMargin`
- `gpMargin`
- `netProfit`
- `epsTTM`
- `MBRevenue`
- `totalShare`
- `liqaShare`

## 盈利能力字段说明

- `code`：证券代码
- `pubDate`：公司发布财报的日期
- `statDate`：财报统计季度的最后一天
- `roeAvg`：净资产收益率(平均)(%)
- `npMargin`：销售净利率(%)
- `gpMargin`：销售毛利率(%)
- `netProfit`：净利润(元)
- `epsTTM`：每股收益
- `MBRevenue`：主营营业收入(元)
- `totalShare`：总股本
- `liqaShare`：流通股本

## 杜邦分析字段

- `code`
- `pubDate`
- `statDate`
- `dupontROE`
- `dupontAssetStoEquity`
- `dupontAssetTurn`
- `dupontPnitoni`
- `dupontNitogr`
- `dupontTaxBurden`
- `dupontIntburden`
- `dupontEbittogr`

## 杜邦分析字段说明

- `code`：证券代码
- `pubDate`：公司发布财报的日期
- `statDate`：财报统计季度的最后一天
- `dupontROE`：净资产收益率
- `dupontAssetStoEquity`：权益乘数，反映企业财务杠杆效应强弱和财务风险
- `dupontAssetTurn`：总资产周转率，反映企业资产管理效率的指标
- `dupontPnitoni`：归属母公司股东的净利润/净利润，反映母公司控股子公司百分比
- `dupontNitogr`：净利润/营业总收入，反映企业销售获利率
- `dupontTaxBurden`：净利润/利润总额，反映企业税负水平
- `dupontIntburden`：利润总额/息税前利润，反映企业利息负担
- `dupontEbittogr`：息税前利润/营业总收入，反映企业经营利润率

## 业绩快报字段

- `code`
- `performanceExpPubDate`
- `performanceExpStatDate`
- `performanceExpUpdateDate`
- `performanceExpressTotalAsset`
- `performanceExpressNetAsset`
- `performanceExpressEPSChgPct`
- `performanceExpressROEWa`
- `performanceExpressEPSDiluted`
- `performanceExpressGRYOY`
- `performanceExpressOPYOY`

## 业绩快报字段说明

- `code`：证券代码
- `performanceExpPubDate`：业绩快报披露日
- `performanceExpStatDate`：业绩快报统计日期
- `performanceExpUpdateDate`：业绩快报披露日（最新）
- `performanceExpressTotalAsset`：业绩快报总资产
- `performanceExpressNetAsset`：业绩快报净资产
- `performanceExpressEPSChgPct`：业绩每股收益增长率
- `performanceExpressROEWa`：业绩快报净资产收益率 ROE-加权
- `performanceExpressEPSDiluted`：业绩快报每股收益 EPS-摊薄
- `performanceExpressGRYOY`：业绩快报营业总收入同比
- `performanceExpressOPYOY`：业绩快报营业利润同比

## 业绩预告字段

- `code`
- `profitForcastExpPubDate`
- `profitForcastExpStatDate`
- `profitForcastType`
- `profitForcastAbstract`
- `profitForcastChgPctUp`
- `profitForcastChgPctDwn`

## 业绩预告字段说明

- `code`：证券代码
- `profitForcastExpPubDate`：业绩预告发布日期
- `profitForcastExpStatDate`：业绩预告统计日期
- `profitForcastType`：业绩预告类型
- `profitForcastAbstract`：业绩预告摘要
- `profitForcastChgPctUp`：预告归属于母公司的净利润增长上限(%)
- `profitForcastChgPctDwn`：预告归属于母公司的净利润增长下限(%)

## 除权除息字段

- `code`
- `dividPreNoticeDate`
- `dividAgmPumDate`
- `dividPlanAnnounceDate`
- `dividPlanDate`
- `dividRegistDate`
- `dividOperateDate`
- `dividPayDate`
- `dividStockMarketDate`
- `dividCashPsBeforeTax`
- `dividCashPsAfterTax`
- `dividStocksPs`
- `dividCashStock`
- `dividReserveToStockPs`

## 除权除息字段说明

- `dividPreNoticeDate`：预披露公告日
- `dividAgmPumDate`：股东大会公告日期
- `dividPlanAnnounceDate`：预案公告日
- `dividPlanDate`：分红实施公告日
- `dividRegistDate`：股权登记日
- `dividOperateDate`：除权除息日期
- `dividPayDate`：派息日
- `dividStockMarketDate`：红股上市交易日
- `dividCashPsBeforeTax`：每股股利税前
- `dividCashPsAfterTax`：每股股利税后
- `dividStocksPs`：每股红股
- `dividCashStock`：分红送转
- `dividReserveToStockPs`：每股转增资本

## 证券基本资料字段

- `code`
- `code_name`
- `ipoDate`
- `outDate`
- `type`
- `status`

## 证券基本资料字段说明

- `code`：证券代码
- `code_name`：证券名称
- `ipoDate`：上市日期
- `outDate`：退市日期
- `type`：证券类型，其中 `1` 股票、`2` 指数、`3` 其它、`4` 可转债、`5` ETF
- `status`：上市状态，其中 `1` 上市、`0` 退市

## 存款利率字段

- `pubDate`
- `demandDepositRate`
- `fixedDepositRate3Month`
- `fixedDepositRate6Month`
- `fixedDepositRate1Year`
- `fixedDepositRate2Year`
- `fixedDepositRate3Year`
- `fixedDepositRate5Year`
- `installmentFixedDepositRate1Year`
- `installmentFixedDepositRate3Year`
- `installmentFixedDepositRate5Year`

## 存款利率字段说明

- `pubDate`：发布日期
- `demandDepositRate`：活期存款(不定期)
- `fixedDepositRate3Month`：定期存款(三个月)
- `fixedDepositRate6Month`：定期存款(半年)
- `fixedDepositRate1Year`：定期存款整存整取(一年)
- `fixedDepositRate2Year`：定期存款整存整取(二年)
- `fixedDepositRate3Year`：定期存款整存整取(三年)
- `fixedDepositRate5Year`：定期存款整存整取(五年)
- `installmentFixedDepositRate1Year`：零存整取、整存零取、存本取息定期存款(一年)
- `installmentFixedDepositRate3Year`：零存整取、整存零取、存本取息定期存款(三年)
- `installmentFixedDepositRate5Year`：零存整取、整存零取、存本取息定期存款(五年)

## 贷款利率字段

- `pubDate`
- `loanRate6Month`
- `loanRate6MonthTo1Year`
- `loanRate1YearTo3Year`
- `loanRate3YearTo5Year`
- `loanRateAbove5Year`
- `mortgateRateBelow5Year`
- `mortgateRateAbove5Year`

## 贷款利率字段说明

- `pubDate`：发布日期
- `loanRate6Month`：6 个月贷款利率
- `loanRate6MonthTo1Year`：6 个月至 1 年贷款利率
- `loanRate1YearTo3Year`：1 年至 3 年贷款利率
- `loanRate3YearTo5Year`：3 年至 5 年贷款利率
- `loanRateAbove5Year`：5 年以上贷款利率
- `mortgateRateBelow5Year`：5 年以下住房公积金贷款利率
- `mortgateRateAbove5Year`：5 年以上住房公积金贷款利率

## 存款准备金率字段

- `pubDate`
- `effectiveDate`
- `bigInstitutionsRatioPre`
- `bigInstitutionsRatioAfter`
- `mediumInstitutionsRatioPre`
- `mediumInstitutionsRatioAfter`

## 存款准备金率字段说明

- `pubDate`：公告日期
- `effectiveDate`：生效日期
- `bigInstitutionsRatioPre`：人民币存款准备金率，大型存款类金融机构调整前
- `bigInstitutionsRatioAfter`：人民币存款准备金率，大型存款类金融机构调整后
- `mediumInstitutionsRatioPre`：人民币存款准备金率，中小型存款类金融机构调整前
- `mediumInstitutionsRatioAfter`：人民币存款准备金率，中小型存款类金融机构调整后

## 月度货币供应量字段

- `statYear`
- `statMonth`
- `m0Month`
- `m0YOY`
- `m0ChainRelative`
- `m1Month`
- `m1YOY`
- `m1ChainRelative`
- `m2Month`
- `m2YOY`
- `m2ChainRelative`

## 月度货币供应量字段说明

- `statYear`：统计年度
- `statMonth`：统计月份
- `m0Month`：货币供应量 M0（月）
- `m0YOY`：货币供应量 M0（同比）
- `m0ChainRelative`：货币供应量 M0（环比）
- `m1Month`：货币供应量 M1（月）
- `m1YOY`：货币供应量 M1（同比）
- `m1ChainRelative`：货币供应量 M1（环比）
- `m2Month`：货币供应量 M2（月）
- `m2YOY`：货币供应量 M2（同比）
- `m2ChainRelative`：货币供应量 M2（环比）

## 年度货币供应量字段

- `statYear`
- `m0Year`
- `m0YearYOY`
- `m1Year`
- `m1YearYOY`
- `m2Year`
- `m2YearYOY`

## 年度货币供应量字段说明

- `statYear`：统计年度
- `m0Year`：年货币供应量 M0（亿元）
- `m0YearYOY`：年货币供应量 M0（同比）
- `m1Year`：年货币供应量 M1（亿元）
- `m1YearYOY`：年货币供应量 M1（同比）
- `m2Year`：年货币供应量 M2（亿元）
- `m2YearYOY`：年货币供应量 M2（同比）

## 行业分类字段

- `updateDate`
- `code`
- `code_name`
- `industry`
- `industryClassification`

## 行业分类字段说明

- `updateDate`：更新日期
- `code`：证券代码
- `code_name`：证券名称
- `industry`：所属行业
- `industryClassification`：所属行业类别

## 指数成分股字段

- `updateDate`
- `code`
- `code_name`

## 指数成分股字段说明

- `updateDate`：更新日期
- `code`：证券代码
- `code_name`：证券名称

## 交易日字段

- `calendar_date`
- `is_trading_day`

## 交易日字段说明

- `calendar_date`：日历日期
- `is_trading_day`：是否交易日，`1` 表示交易日，`0` 表示非交易日

## `query_history_k_data_plus` 参数

- `code`：必填，证券代码，例如 `sh.600000`
- `fields`：必填，逗号分隔字段列表
- `start_date`：可选，开始日期，格式 `YYYY-MM-DD`
- `end_date`：可选，结束日期，格式 `YYYY-MM-DD`
- `frequency`：可选，`d`、`w`、`m`、`5`、`15`、`30`、`60`
- `adjustflag`：可选，`1` 后复权、`2` 前复权、`3` 不复权

## 历史行情指标字段

### 日线字段

- `date`
- `code`
- `open`
- `high`
- `low`
- `close`
- `preclose`
- `volume`
- `amount`
- `adjustflag`
- `turn`
- `tradestatus`
- `pctChg`
- `peTTM`
- `psTTM`
- `pcfNcfTTM`
- `pbMRQ`
- `isST`

### 周线 / 月线字段

- `date`
- `code`
- `open`
- `high`
- `low`
- `close`
- `volume`
- `amount`
- `adjustflag`
- `turn`
- `pctChg`

### 5 / 15 / 30 / 60 分钟线字段

- `date`
- `time`
- `code`
- `open`
- `high`
- `low`
- `close`
- `volume`
- `amount`
- `adjustflag`

## 指数 K 线参数

- `code`：必填，指数代码，例如 `sh.000001`、`sz.399001`
- `fields`：必填，逗号分隔字段列表
- `start_date`：可选，开始日期，格式 `YYYY-MM-DD`
- `end_date`：可选，结束日期，格式 `YYYY-MM-DD`
- `frequency`：可选，`d`、`w`、`m`

## 指数 K 线字段

- `date`
- `code`
- `open`
- `high`
- `low`
- `close`
- `preclose`
- `volume`
- `amount`
- `pctChg`
