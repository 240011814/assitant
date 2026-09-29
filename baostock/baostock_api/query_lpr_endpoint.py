from __future__ import annotations

import logging

import akshare as ak
import pandas as pd


def parse_params(query: dict[str, list[str]]) -> dict[str, object]:
    # 该端点不使用查询参数; 返回扁平 dict 与其他端点签名保持一致(execute 忽略参数)
    return {}


def execute(params: dict[str, object]) -> dict[str, object]:
    df: pd.DataFrame = ak.macro_china_lpr()

    if df.empty:
        return {"fields": [], "total": 0, "items": []}

    fields = ["TRADE_DATE", "LPR1Y", "LPR5Y", "RATE_1", "RATE_2"]
    items: list[dict[str, str]] = []

    for _, row in df.iterrows():
        item: dict[str, str] = {}
        for col in fields:
            val = row.get(col)
            if pd.isna(val):
                item[col] = ""
            elif isinstance(val, pd.Timestamp):
                item[col] = val.strftime("%Y-%m-%d")
            else:
                item[col] = str(val)
        items.append(item)

    logging.info("query_lpr: fetched %d rows", len(items))
    return {"fields": fields, "total": len(items), "items": items}
