"""验证新版 baostock 每日更新接口 (query_daily_history_k_AStock / query_daily_adjust_factor)

用途: 部署前确认代理服务器上的 baostock 版本支持新接口, 并实测:
  1. 近期交易日返回的行数、市场分布 (sh/sz/bj)、是否包含指数
  2. 历史深度 (老日期是否可查, 决定按日回填策略)
  3. 按日复权因子是否可用

用法: python verify_daily_updates.py [date]   (默认最近一个交易日, 日历查询失败退回昨天)
"""

from __future__ import annotations

import sys
from datetime import date, timedelta

# 先导入应用公共模块: shared.py 在模块级打 socket 超时 / recv EOF / send_msg 补丁,
# 让裸 SDK 验证与代理服务行为一致, 否则验证时可能复现已修死的 recv EOF 死循环
import baostock_api.shared  # noqa: F401  应用 socket/超时补丁

import baostock as bs


def default_recent_date() -> str:
    """今天(含)之前最近的一个交易日; 交易日历查询失败时退回昨天的日历日期"""
    today = date.today()
    try:
        rs = bs.query_trade_dates(
            start_date=(today - timedelta(days=20)).isoformat(),
            end_date=today.isoformat(),
        )
        if rs.error_code == "0":
            last_trading_day = ""
            while rs.next():
                row = dict(zip(rs.fields, rs.get_row_data()))
                if row.get("is_trading_day") != "1":
                    continue
                calendar_date = str(row.get("calendar_date") or "")
                if calendar_date > last_trading_day:
                    last_trading_day = calendar_date
            if last_trading_day:
                return last_trading_day
        print(f"[warn] query_trade_dates unavailable (code={rs.error_code}), fallback to yesterday")
    except Exception as error:
        print(f"[warn] query_trade_dates failed, fallback to yesterday: {error}")
    return (today - timedelta(days=1)).isoformat()


def fetch(method_name: str, date_str: str | None) -> list[dict[str, str]]:
    method = getattr(bs, method_name)
    kwargs = {"date": date_str} if date_str else {}
    rs = method(**kwargs)
    if rs.error_code != "0":
        raise RuntimeError(f"{method_name} failed: {rs.error_code} {rs.error_msg}")
    rows: list[dict[str, str]] = []
    while rs.next():
        rows.append(dict(zip(rs.fields, rs.get_row_data())))
    if rs.error_code != "0":
        # 翻页中途失败: 不检查会把已取到的部分数据当成全量, 静默截断
        raise RuntimeError(f"{method_name} paging failed: {rs.error_code} {rs.error_msg}")
    return rows


def market_stats(codes: set[str]) -> str:
    return " ".join(
        f"{prefix}={sum(1 for c in codes if c.startswith(prefix))}"
        for prefix in ("sh.", "sz.", "bj.")
    )


def main() -> None:
    lg = bs.login()
    if lg.error_code != "0":
        raise SystemExit(f"login failed: {lg.error_msg}")

    try:
        recent = sys.argv[1] if len(sys.argv) > 1 else default_recent_date()

        rows = fetch("query_daily_history_k_AStock", recent)
        codes = {r["code"] for r in rows}
        print(f"[AStock {recent}] rows={len(rows)}")
        print(f"  fields={list(rows[0].keys()) if rows else '-'}")
        print(f"  markets: {market_stats(codes)}")
        print(f"  index: sh.000001={'sh.000001' in codes} sz.399001={'sz.399001' in codes}")
        if rows:
            print(f"  sample={rows[0]}")

        for old in ("2005-01-04", "1995-01-03"):
            try:
                old_rows = fetch("query_daily_history_k_AStock", old)
                old_codes = {r["code"] for r in old_rows}
                print(f"[AStock {old}] rows={len(old_rows)} {market_stats(old_codes)}")
            except Exception as e:
                print(f"[AStock {old}] FAILED: {e}")

        factor_rows = fetch("query_daily_adjust_factor", recent)
        factor_sample = factor_rows[0] if factor_rows else "-"
        print(f"[adjust_factor {recent}] rows={len(factor_rows)} sample={factor_sample}")
    finally:
        bs.logout()


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        # 部署门禁: 任何失败都以非零码退出
        print(f"VERIFY FAILED: {error}")
        sys.exit(1)
