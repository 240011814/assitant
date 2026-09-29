from __future__ import annotations

import logging
import time
from http import HTTPStatus
from typing import Any
from urllib.parse import parse_qs

import uvicorn
from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException

from baostock_api import (
    __version__,
    health_endpoint,
    query_deposit_rate_data_endpoint,
    query_lpr_endpoint,
    query_dupont_data_endpoint,
    query_adjust_factor_endpoint,
    query_balance_data_endpoint,
    query_cash_flow_data_endpoint,
    query_dividend_data_endpoint,
    query_all_stock_endpoint,
    query_forecast_report_endpoint,
    query_growth_data_endpoint,
    query_hs300_stocks_endpoint,
    query_loan_rate_data_endpoint,
    query_money_supply_data_month_endpoint,
    query_money_supply_data_year_endpoint,
    query_operation_data_endpoint,
    query_performance_express_report_endpoint,
    query_profit_data_endpoint,
    query_required_reserve_ratio_data_endpoint,
    query_daily_adjust_factor_endpoint,
    query_daily_history_k_astock_endpoint,
    query_daily_history_k_etf_endpoint,
    query_history_k_data_plus_endpoint,
    query_history_index_k_data_plus_endpoint,
    query_stock_basic_endpoint,
    query_stock_industry_endpoint,
    query_sz50_stocks_endpoint,
    query_trade_dates_endpoint,
    usage_endpoint,
    query_zz500_stocks_endpoint,
    query_lhb_detail_endpoint,
    query_fund_flow_endpoint,
    query_north_flow_endpoint,
    query_gdp_endpoint,
    query_cpi_endpoint,
    query_pmi_endpoint,
    query_ppi_endpoint,
)
from baostock_api.shared import (
    DAILY_LIMIT,
    HOST,
    PORT,
    BaostockQueryError,
    DailyLimitExceeded,
    force_disconnect,
    make_error_payload,
    usage_counter,
)

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [baostock-api] %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)

app = FastAPI(title="baostock-api", version=__version__)

QUERY_ENDPOINTS: dict[str, Any] = {
    "/query_all_stock": query_all_stock_endpoint,
    "/query_adjust_factor": query_adjust_factor_endpoint,
    "/query_daily_adjust_factor": query_daily_adjust_factor_endpoint,
    "/query_daily_history_k_astock": query_daily_history_k_astock_endpoint,
    "/query_daily_history_k_etf": query_daily_history_k_etf_endpoint,
    "/query_balance_data": query_balance_data_endpoint,
    "/query_cash_flow_data": query_cash_flow_data_endpoint,
    "/query_deposit_rate_data": query_deposit_rate_data_endpoint,
    "/query_dividend_data": query_dividend_data_endpoint,
    "/query_dupont_data": query_dupont_data_endpoint,
    "/query_forecast_report": query_forecast_report_endpoint,
    "/query_growth_data": query_growth_data_endpoint,
    "/query_hs300_stocks": query_hs300_stocks_endpoint,
    "/query_lpr": query_lpr_endpoint,
    "/query_loan_rate_data": query_loan_rate_data_endpoint,
    "/query_money_supply_data_month": query_money_supply_data_month_endpoint,
    "/query_money_supply_data_year": query_money_supply_data_year_endpoint,
    "/query_operation_data": query_operation_data_endpoint,
    "/query_performance_express_report": query_performance_express_report_endpoint,
    "/query_profit_data": query_profit_data_endpoint,
    "/query_required_reserve_ratio_data": query_required_reserve_ratio_data_endpoint,
    "/query_history_k_data_plus": query_history_k_data_plus_endpoint,
    "/query_history_index_k_data_plus": query_history_index_k_data_plus_endpoint,
    "/query_stock_basic": query_stock_basic_endpoint,
    "/query_stock_industry": query_stock_industry_endpoint,
    "/query_sz50_stocks": query_sz50_stocks_endpoint,
    "/query_trade_dates": query_trade_dates_endpoint,
    "/query_zz500_stocks": query_zz500_stocks_endpoint,
    "/query_lhb_detail": query_lhb_detail_endpoint,
    "/query_fund_flow": query_fund_flow_endpoint,
    "/query_north_flow": query_north_flow_endpoint,
    "/query_gdp": query_gdp_endpoint,
    "/query_cpi": query_cpi_endpoint,
    "/query_pmi": query_pmi_endpoint,
    "/query_ppi": query_ppi_endpoint,
}

# akshare 接口无串行限制和次数限额，单独列出，不走 usage_counter
AKSHARE_ENDPOINTS: set[str] = {
    "/query_gdp", "/query_cpi", "/query_pmi", "/query_ppi", "/query_lpr",
    "/query_lhb_detail", "/query_fund_flow", "/query_north_flow",
}


@app.get("/health")
def health() -> JSONResponse:
    return JSONResponse(status_code=HTTPStatus.OK, content=health_endpoint.handle())


@app.get("/usage")
def usage() -> JSONResponse:
    return JSONResponse(status_code=HTTPStatus.OK, content=usage_endpoint.handle())


# baostock 服务端会不定期重置长连接 (Connection reset/Broken pipe), SDK 内部连接状态
# 又无法手动重建, 重置后第一次重新登录仍可能失败; 查询异常时 force_disconnect 后自动
# 重试整个请求, 把瞬时断连在代理内消化掉, 不抛 502 给调用方触发同步任务中断
MAX_QUERY_ATTEMPTS = 3

# 瞬时错误重试之间的退避(秒), 按尝试次数递增。服务端重置长连接后需要一点时间释放旧会话,
# 退避太短会导致连续几次重连都被拒(打印"服务器连接失败")进而连环 502, 故给到秒级
RETRY_BACKOFF_SECONDS = (1.0, 3.0)


def _execute_with_retry(endpoint: Any, query: dict[str, list[str]]) -> Any:
    for attempt in range(1, MAX_QUERY_ATTEMPTS + 1):
        try:
            return endpoint.execute(endpoint.parse_params(query))
        except ValueError:
            # 本地参数错误是确定性问题, 重试无意义
            raise
        except BaostockQueryError as error:
            if "未登录" not in str(error):
                # 其他确定性错误(参数错误/限额等), 连接本身正常, 重连重试也无效
                raise
            # session 过期导致的"用户未登录", 需要 force_disconnect + 重新登录
            err = error
        except Exception as error:
            # 其余视为瞬时错误(连接重置/登录失败等), 断开重连后重试
            err = error
        else:
            continue

        force_disconnect()
        if attempt >= MAX_QUERY_ATTEMPTS:
            raise err
        backoff = RETRY_BACKOFF_SECONDS[min(attempt - 1, len(RETRY_BACKOFF_SECONDS) - 1)]
        logging.warning(
            "query attempt %d/%d failed, force reconnect and retry in %.1fs: %s",
            attempt,
            MAX_QUERY_ATTEMPTS,
            backoff,
            err,
        )
        time.sleep(backoff)


def _handle_query(request: Request, endpoint: Any, path: str) -> JSONResponse:
    query = parse_qs(request.url.query)

    usage = usage_counter.get_stats()
    if path not in AKSHARE_ENDPOINTS:
        try:
            usage = usage_counter.consume()
        except DailyLimitExceeded:
            return JSONResponse(
                status_code=HTTPStatus.TOO_MANY_REQUESTS,
                content=make_error_payload(
                    f"daily limit exceeded: {DAILY_LIMIT}",
                    "daily_limit_exceeded",
                    usage_counter.get_stats(),
                ),
            )
        except OSError as error:
            # usage.json 持久化失败(磁盘满/权限等): 内存计数仍然生效, 只是本次放弃写盘。
            # 降级继续主查询, 不能让配额写盘问题把请求打成非 JSON 500
            logging.error("usage stats persist failed, continue with in-memory count: %s", error)

    started = time.monotonic()
    try:
        payload = _execute_with_retry(endpoint, query)
    except ValueError as error:
        return JSONResponse(
            status_code=HTTPStatus.BAD_REQUEST,
            content=make_error_payload(str(error), "invalid_request", usage),
        )
    except Exception as error:
        logging.error(
            "query failed after %d attempts: %s?%s: %s",
            MAX_QUERY_ATTEMPTS,
            request.url.path,
            request.url.query,
            error,
        )
        return JSONResponse(
            status_code=HTTPStatus.BAD_GATEWAY,
            content=make_error_payload(str(error), "baostock_query_failed", usage),
        )
    finally:
        elapsed = time.monotonic() - started
        if elapsed > 3:
            logging.info("SLOW %s?%s took %.1fs", request.url.path, request.url.query, elapsed)

    return JSONResponse(
        status_code=HTTPStatus.OK,
        content={
            "ok": True,
            "data": payload,
            "usage": usage.to_dict(),
        },
    )


def _make_query_handler(endpoint: Any, path: str):
    def handler(request: Request) -> JSONResponse:
        return _handle_query(request, endpoint, path)

    return handler


for _path, _endpoint in QUERY_ENDPOINTS.items():
    app.add_api_route(
        _path, _make_query_handler(_endpoint, _path), methods=["GET"], name=_path.lstrip("/")
    )


@app.exception_handler(StarletteHTTPException)
async def http_exception_handler(request: Request, exc: StarletteHTTPException) -> JSONResponse:
    code = "not_found" if exc.status_code == HTTPStatus.NOT_FOUND else "http_error"
    return JSONResponse(
        status_code=exc.status_code,
        content=make_error_payload(str(exc.detail), code, usage_counter.get_stats()),
    )

log_config = {
    "version": 1,
    "formatters": {
        "access": {
            "format": "%(levelname)s:    %(asctime)s| %(message)s"
        }
    },
    "handlers": {
        "access": {
            "class": "logging.StreamHandler",
            "formatter": "access",
        }
    },
    "loggers": {
        "uvicorn.access": {
            "handlers": ["access"],
            "level": "INFO",
            # 不向 root 传播: 否则 access 记录会同时被本 handler 和 root(basicConfig,
            # 带 [baostock-api] 前缀)处理, 日志打印两遍
            "propagate": False,
        }
    }
}


def main() -> None:
    logging.info("baostock api running on http://%s:%s", HOST, PORT)
    uvicorn.run(app, host=HOST, port=PORT, log_level="info", log_config=log_config)


if __name__ == "__main__":
    main()
